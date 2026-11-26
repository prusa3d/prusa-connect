package routes_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/accessapi"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"

	"github.com/prusa3d/materials/internal/dbtest"
	"github.com/prusa3d/materials/internal/importer"
	"github.com/prusa3d/materials/internal/opt"
	"github.com/prusa3d/materials/internal/printers"
	"github.com/prusa3d/materials/internal/routes"
	"github.com/prusa3d/materials/internal/settings"
	"github.com/prusa3d/materials/internal/stock"
)

// The handlers driven as plain HTTP, with the principal the host would hand
// over on the request's context. routesapi.Serve does the same before it calls
// the handler, so what is proved here is what the host's call reaches.

type harness struct {
	handler http.Handler
	db      *sql.DB
	store   *stock.Store
	imp     *importer.Importer
	live    *settings.Live
	fleet   fleet
	access  *access
	seen    *seen
}

// fleet is the printers as the follower last read them, by id.
type fleet map[string]printers.Printer

func (f fleet) Printer(id string) (printers.Printer, bool) {
	p, ok := f[id]

	return p, ok
}

// Listed is true: the test's fleet is listed whole from the start.
func (fleet) Listed() bool { return true }

// Printers leaves the tools out, as the follower does.
func (f fleet) Printers() []printers.Printer {
	out := make([]printers.Printer, 0, len(f))
	for _, p := range f {
		p.Tools = connectorapi.ToolTree{}
		out = append(out, p)
	}

	return out
}

// access answers by the role each person holds on every printer, operate or
// observe, or fails with err. Operate holds observe, as the host's roles do.
// It counts the questions it was asked.
type access struct {
	mu    sync.Mutex
	roles map[string]string
	// hidden are the printers nobody may observe or operate.
	hidden map[string]bool
	err    error
	asked  int
}

func (a *access) Check(_ context.Context, p accessapi.CheckParams) (accessapi.Decision, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if (p.Action != "operate" && p.Action != "observe") || p.ObjectType != "device" {
		return accessapi.Decision{}, errors.New("the module asked about " + p.Action + " on " + p.ObjectType)
	}

	a.asked++
	role := a.roles[p.Subject]

	return accessapi.Decision{Allowed: !a.hidden[p.ObjectID] && (role == "operate" || role == p.Action)}, a.err
}

// grant gives who a role on every printer, or none for "".
func (a *access) grant(who, role string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.roles[who] = role
}

// seen is the host's listing for a person: the printers of the fleet the
// access fake lets them observe, a page at a time.
type seen struct {
	fleet  fleet
	access *access
	mu     sync.Mutex
	pages  int
}

func (s *seen) List(_ context.Context, p deviceapi.ListParams) (deviceapi.Page, error) {
	s.mu.Lock()
	s.pages++
	s.mu.Unlock()

	s.access.mu.Lock()
	role, hidden, err := s.access.roles[p.Principal], s.access.hidden, s.access.err
	s.access.mu.Unlock()

	if err != nil {
		return deviceapi.Page{}, err
	}

	all := []deviceapi.Device{}

	for _, id := range slices.Sorted(maps.Keys(s.fleet)) {
		if role != "" && !hidden[id] {
			all = append(all, deviceapi.Device{ID: id})
		}
	}

	from := min(p.Offset, len(all))

	return deviceapi.Page{Items: all[from:min(from+p.Limit, len(all))], Total: len(all), Offset: from}, nil
}

// printer is a printer in state with one tool, 0, that reports reported, or
// no material for nil.
func printer(id, state string, reported *string) printers.Printer {
	return printers.Printer{
		ID: id, Name: "Printer " + id, Connected: true, State: state,
		Tools: connectorapi.ToolTree{Tools: map[connectorapi.ToolID]*connectorapi.Tool{
			"0": {ID: "0", Material: reported},
		}},
	}
}

// newHarness serves a fresh SQLite database. fetch stands in for the
// download, and nil fails every import at once.
func newHarness(t *testing.T, fetch importer.Fetch) harness {
	t.Helper()

	if fetch == nil {
		fetch = func(context.Context) (io.ReaderAt, int64, error) { return nil, 0, errors.New("no network in tests") }
	}

	db, grant := dbtest.SQLite(t)
	store := stock.New(db, grant.Engine)
	imp := importer.New(store, time.Now)
	live := &settings.Live{}
	printers := fleet{"d1": printer("d1", "IDLE", nil)}
	asks := &access{roles: map[string]string{"user:alice": "operate"}}

	listing := &seen{fleet: printers, access: asks}

	return harness{
		handler: routes.New(store, imp, fetch, live, printers, listing, asks),
		seen:    listing,
		db:      db, store: store, imp: imp, live: live, fleet: printers, access: asks,
	}
}

// send makes one request as who, and answers the status and the raw body.
func (h harness) sendRaw(t *testing.T, who, method, path, body string) (int, []byte) {
	t.Helper()

	target, query, _ := strings.Cut(path, "?")

	request, err := routesapi.Request{
		Method: method, Path: target, Query: query, Principal: who, Body: []byte(body),
	}.HTTP(context.Background())
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}

	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)

	return recorder.Code, recorder.Body.Bytes()
}

// send makes one request as who, and answers the status and the JSON body,
// empty for a 204.
func (h harness) send(t *testing.T, who, method, path, body string) (int, map[string]any) {
	t.Helper()

	status, raw := h.sendRaw(t, who, method, path, body)
	answer := map[string]any{}

	if len(raw) == 0 {
		return status, answer
	}

	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("%s %s answered a body that is not JSON: %s", method, path, raw)
	}

	return status, answer
}

// seed loads a catalogue of one brand, one material and n packages.
func (h harness) seed(t *testing.T, packages int) {
	t.Helper()

	snap := opt.Snapshot{
		Meta:   opt.Meta{DataHash: "h1", LastModified: "2026-10-01"},
		Brands: []opt.Brand{{UUID: "b-1", Slug: "prusament", Name: "Prusament"}},
		Materials: []opt.Material{
			{
				UUID: "m-1", Slug: "prusament-pla", BrandSlug: "prusament", Name: "PLA", Class: "FFF",
				ColorRGBA: "#1a1a1aff",
			},
		},
	}

	for i := range packages {
		slug := strings.Repeat("p", 100) + strconv.Itoa(i)
		snap.Packages = append(snap.Packages, opt.Package{
			UUID: "pk-" + strconv.Itoa(i), Slug: slug, Class: "FFF", MaterialSlug: "prusament-pla",
		})
	}

	if _, _, err := h.store.ReplaceCatalogue(context.Background(), snap, time.Now()); err != nil {
		t.Fatalf("seed the catalogue: %v", err)
	}
}

func TestARequestNobodyMadeIsRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)

	status, answer := h.send(t, "", http.MethodGet, "/materials", "")
	if status != http.StatusForbidden || answer["code"] != routes.NoPrincipal.Name {
		t.Errorf("a request with no principal answered %d %v", status, answer)
	}
}

func TestEachRefusalCarriesItsOwnCode(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 1)

	if err := h.live.Set(settings.Values{MaxSpools: 1}); err != nil {
		t.Fatal(err)
	}

	spool := `{"material_uuid":"m-1","remaining_weight_g":500,"quantity":1}`

	for _, step := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{http.MethodGet, "/materials?favourites=mine", "", http.StatusBadRequest, routes.InvalidQuery.Name},
		{http.MethodGet, "/materials?order=size", "", http.StatusBadRequest, routes.InvalidQuery.Name},
		{http.MethodGet, "/materials?color=%23ff", "", http.StatusBadRequest, routes.InvalidQuery.Name},
		{http.MethodGet, "/materials?limit=-1", "", http.StatusBadRequest, routes.InvalidQuery.Name},
		{http.MethodGet, "/materials?desc=maybe", "", http.StatusBadRequest, routes.InvalidQuery.Name},
		{http.MethodPatch, "/spools/abc", `{}`, http.StatusBadRequest, routes.InvalidQuery.Name},
		{http.MethodGet, "/materials/nope", "", http.StatusNotFound, routes.NotFound.Name},
		{http.MethodPut, "/favourites/nope", "", http.StatusNotFound, routes.NotFound.Name},
		{http.MethodDelete, "/custom-materials/nope", "", http.StatusNotFound, routes.NotFound.Name},
		{http.MethodDelete, "/spools/9", "", http.StatusNotFound, routes.NotFound.Name},
		{http.MethodGet, "/nowhere", "", http.StatusNotFound, routes.NotFound.Name},
		{http.MethodPost, "/custom-materials", `{"name":"","class":"FFF"}`, http.StatusUnprocessableEntity, routes.MaterialInvalid.Name},
		{http.MethodPost, "/custom-materials", `{"name":"X","class":"FFF","colour":"red"}`, http.StatusUnprocessableEntity, routes.MaterialInvalid.Name},
		{http.MethodPost, "/spools", strings.Replace(spool, `"quantity"`, `"condition":"new","quantity"`, 1), http.StatusUnprocessableEntity, routes.SpoolInvalid.Name},
		{http.MethodPost, "/spools", strings.Replace(spool, `"quantity":1`, `"quantity":0`, 1), http.StatusUnprocessableEntity, routes.SpoolInvalid.Name},
		{http.MethodPost, "/spools", spool, http.StatusCreated, ""},
		{http.MethodPost, "/spools", spool, http.StatusConflict, routes.ShelfFull.Name},
		{http.MethodPatch, "/spools/1", `{"remaining_weight_g":-1}`, http.StatusUnprocessableEntity, routes.SpoolInvalid.Name},
		{http.MethodPost, "/spools/delete", `{"ids":[]}`, http.StatusUnprocessableEntity, routes.SpoolInvalid.Name},
		{http.MethodPost, "/spools/delete", `{"ids":[1,9]}`, http.StatusNotFound, routes.NotFound.Name},
		{http.MethodPost, "/spools/delete", `{"ids":[1]}`, http.StatusNoContent, ""},
		{http.MethodDelete, "/spools/1", "", http.StatusNotFound, routes.NotFound.Name},
		{http.MethodPost, "/custom-materials", `{"name":"Mine","class":"FFF"}`, http.StatusCreated, ""},
	} {
		status, answer := h.send(t, "user:alice", step.method, step.path, step.body)
		if status != step.status {
			t.Errorf("%s %s answered %d and not %d: %v", step.method, step.path, status, step.status, answer)

			continue
		}

		if step.code != "" && answer["code"] != step.code {
			t.Errorf("%s %s answered the code %v and not %s", step.method, step.path, answer["code"], step.code)
		}
	}

	// A custom material a live spool holds stays.
	_, page := h.send(t, "user:alice", http.MethodGet, "/materials?custom=true", "")
	custom := page["rows"].([]any)[0].(map[string]any)["uuid"].(string)

	if err := h.live.Set(settings.Values{MaxSpools: 10}); err != nil {
		t.Fatal(err)
	}

	h.send(t, "user:alice", http.MethodPost, "/spools", strings.Replace(spool, "m-1", custom, 1))

	status, answer := h.send(t, "user:alice", http.MethodDelete, "/custom-materials/"+custom, "")
	if status != http.StatusConflict || answer["code"] != routes.MaterialInUse.Name {
		t.Errorf("deleting a custom material in use answered %d %v", status, answer)
	}
}

// TestABusyDatabaseAsksForARetry holds SQLite's write lock from another
// connection, as an import does, and makes the store's write fail at once.
func TestABusyDatabaseAsksForARetry(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)

	// One connection, which waits for no lock, so the write fails now and not
	// after moduledb's five seconds.
	h.db.SetMaxOpenConns(1)

	if _, err := h.db.Exec(`PRAGMA busy_timeout = 0`); err != nil {
		t.Fatal(err)
	}

	other, err := sql.Open("sqlite", h.dbPath(t))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = other.Close() })

	conn, err := other.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	if _, err = conn.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _, _ = conn.ExecContext(context.Background(), `ROLLBACK`) })

	status, answer := h.send(t, "user:alice", http.MethodPost, "/custom-materials", `{"name":"Mine","class":"FFF"}`)
	if status != http.StatusServiceUnavailable || answer["code"] != routes.Busy.Name ||
		answer["retryable"] != true {
		t.Errorf("a write against a locked database answered %d %v", status, answer)
	}
}

// dbPath is the file the harness's database is in.
func (h harness) dbPath(t *testing.T) string {
	t.Helper()

	var seq int

	var name, file string
	if err := h.db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &file); err != nil {
		t.Fatal(err)
	}

	return file
}

func TestTheBiggestPageFitsTheModuleBound(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 200)

	for i := range 100 {
		name := strings.Repeat("n", 117) + strconv.Itoa(1000 + i)[1:]
		if _, err := h.store.CreateCustom(
			context.Background(),
			"user:alice",
			stock.MaterialFields{Name: name, Class: "FFF"},
		); err != nil {
			t.Fatal(err)
		}
	}

	status, body := h.sendRaw(t, "user:alice", http.MethodGet, "/materials?limit=100", "")
	if status != http.StatusOK || len(body) >= routesapi.MaxModuleBody {
		t.Errorf(
			"a page of 100 answered %d with %d bytes, and the bound is %d",
			status,
			len(body),
			routesapi.MaxModuleBody,
		)
	}

	var page struct {
		Rows []any `json:"rows"`
	}

	if err := json.Unmarshal(body, &page); err != nil || len(page.Rows) != 100 {
		t.Errorf("the page holds %d rows (%v), want 100", len(page.Rows), err)
	}

	status, body = h.sendRaw(t, "user:alice", http.MethodGet, "/materials/m-1", "")
	if status != http.StatusOK || len(body) >= routesapi.MaxModuleBody {
		t.Errorf("a material of 200 packages answered %d with %d bytes", status, len(body))
	}

	var detail struct {
		Packages []any `json:"packages"`
	}

	if err := json.Unmarshal(body, &detail); err != nil || len(detail.Packages) != 200 {
		t.Errorf("the material holds %d packages (%v), want 200", len(detail.Packages), err)
	}

	// The longest texts a person may write, of characters JSON escapes, on a
	// custom material and on 100 spools of it.
	quotes := strings.Repeat(`"<`, 100)

	custom, err := h.store.CreateCustom(context.Background(), "user:alice", stock.MaterialFields{
		Name: quotes, BrandName: quotes, Class: "FFF", Type: quotes, Abbreviation: quotes, ColorRGBA: "#ffffffff",
	})
	if err != nil {
		t.Fatal(err)
	}

	place, err := h.store.CreateLocation(context.Background(), quotes)
	if err != nil {
		t.Fatal(err)
	}

	draft, _ := json.Marshal(map[string]any{
		"material_uuid": custom.UUID, "remaining_weight_g": 1, "quantity": 100,
		"location_id": place.ID, "notes": strings.Repeat(`"<`, 1000),
	})

	status, body = h.sendRaw(t, "user:alice", http.MethodPost, "/spools", string(draft))

	var added struct {
		Added int     `json:"added"`
		IDs   []int64 `json:"ids"`
	}

	if err = json.Unmarshal(body, &added); status != http.StatusCreated || err != nil || added.Added != 100 ||
		len(added.IDs) != 100 {
		t.Errorf("adding 100 spools answered %d %s", status, body)
	}

	status, body = h.sendRaw(t, "user:alice", http.MethodGet, "/materials/"+custom.UUID+"/spools", "")
	if status != http.StatusOK || len(body) >= routesapi.MaxModuleBody {
		t.Errorf("a page of 100 spools answered %d with %d bytes", status, len(body))
	}

	var spools struct {
		Spools []any `json:"spools"`
		Total  int   `json:"total"`
	}

	if err = json.Unmarshal(body, &spools); err != nil || len(spools.Spools) != 100 || spools.Total != 100 {
		t.Errorf("the spools page holds %d of %d (%v), want 100", len(spools.Spools), spools.Total, err)
	}
}

func TestTheCallerComesFromTheHost(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)

	status, custom := h.send(
		t,
		"user:alice",
		http.MethodPost,
		"/custom-materials",
		`{"name":"Alice PLA","class":"FFF"}`,
	)
	if status != http.StatusCreated {
		t.Fatalf("creating a custom material answered %d %v", status, custom)
	}

	id := custom["uuid"].(string)

	for _, path := range []string{"/favourites/" + id, "/favourites/m-1"} {
		if starred, answer := h.send(t, "user:alice", http.MethodPut, path, ""); starred != http.StatusNoContent {
			t.Fatalf("PUT %s answered %d %v", path, starred, answer)
		}
	}

	// Alice sees both starred, Bob neither, and not her custom material.
	_, mine := h.send(t, "user:alice", http.MethodGet, "/materials?favourites=true", "")
	if mine["total"] != 2.0 {
		t.Errorf("alice's favourites are %v, want 2", mine)
	}

	_, his := h.send(t, "user:bob", http.MethodGet, "/materials", "")
	if his["total"] != 1.0 || his["rows"].([]any)[0].(map[string]any)["is_favourite"] != false {
		t.Errorf("bob's list is %v, want the one catalogue material, unstarred", his)
	}

	if status, _ = h.send(t, "user:bob", http.MethodGet, "/materials/"+id, ""); status != http.StatusNotFound {
		t.Errorf("bob reads alice's custom material with %d, want 404", status)
	}

	status, _ = h.send(t, "user:bob", http.MethodPatch, "/custom-materials/"+id, `{"name":"Mine now","class":"FFF"}`)
	if status != http.StatusNotFound {
		t.Errorf("bob changes alice's custom material with %d, want 404", status)
	}

	// Unstarring takes it out of her favourites.
	h.send(t, "user:alice", http.MethodDelete, "/favourites/m-1", "")

	if _, mine = h.send(t, "user:alice", http.MethodGet, "/materials?favourites=true", ""); mine["total"] != 1.0 {
		t.Errorf("alice's favourites after unstarring are %v, want 1", mine)
	}
}

// The stock belongs to the server and is shared by everyone who uses it: the
// host decides who may write, and whoever may write changes any spool.
func TestEveryoneWhoMayWriteKeepsTheSameSpools(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)

	status, added := h.send(t, "user:alice", http.MethodPost, "/spools",
		`{"material_uuid":"m-1","remaining_weight_g":500,"quantity":2}`)
	if status != http.StatusCreated {
		t.Fatalf("alice adds spools with %d %v", status, added)
	}

	ids := added["ids"].([]any)
	first, second := strconv.Itoa(int(ids[0].(float64))), strconv.Itoa(int(ids[1].(float64)))

	if _, page := h.send(t, "user:bob", http.MethodGet, "/materials/m-1/spools", ""); page["total"] != 2.0 {
		t.Errorf("bob sees %v, want alice's two spools", page)
	}

	status, _ = h.send(t, "user:bob", http.MethodPatch, "/spools/"+first, `{"remaining_weight_g":100}`)
	if status != http.StatusOK {
		t.Errorf("bob changes alice's spool with %d, want 200", status)
	}

	if status, _ = h.send(t, "user:bob", http.MethodDelete, "/spools/"+second, ""); status != http.StatusNoContent {
		t.Errorf("bob deletes alice's spool with %d, want 204", status)
	}
}

// A request that reached the routes a configure replaced asks for a retry,
// which reaches the new ones.
func TestADownloadOnAClosedImporterIsBusy(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.imp.Close()

	status, answer := h.send(t, "user:alice", http.MethodPost, "/catalogue/download", "")
	if status != http.StatusServiceUnavailable || answer["code"] != routes.Busy.Name || answer["retryable"] != true {
		t.Errorf("a download on a closed importer answered %d %v", status, answer)
	}
}

func TestDownloadAnswers202AndThen409(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	h := newHarness(t, func(context.Context) (io.ReaderAt, int64, error) {
		<-release

		return nil, 0, errors.New("the test let it go")
	})

	status, answer := h.send(t, "user:alice", http.MethodPost, "/catalogue/download", "")
	if status != http.StatusAccepted || answer["loaded"] != false ||
		answer["import"].(map[string]any)["phase"] != "running" {
		t.Fatalf("the first download answered %d %v", status, answer)
	}

	status, answer = h.send(t, "user:alice", http.MethodPost, "/catalogue/download", "")
	if status != http.StatusConflict || answer["code"] != routes.ImportRunning.Name {
		t.Errorf("a download during a download answered %d %v", status, answer)
	}

	close(release)

	for deadline := time.Now().Add(5 * time.Second); h.imp.Status().Phase == importer.Running; {
		if time.Now().After(deadline) {
			t.Fatal("the import never finished")
		}

		time.Sleep(10 * time.Millisecond)
	}

	_, answer = h.send(t, "user:alice", http.MethodGet, "/catalogue", "")
	if run := answer["import"].(map[string]any); run["phase"] != "failed" || run["reason"] != "download" {
		t.Errorf("the catalogue after a failed import reads %v", answer)
	}
}

// A store failure answers the caller a bare STORE_FAILED; the reason goes to
// standard error, which the host keeps as the module's log. Not parallel: it
// swaps the process's standard error.
func TestAStoreFailureIsLoggedWithItsRequest(t *testing.T) {
	h := newHarness(t, nil)

	if _, err := h.db.Exec(`DROP TABLE spools`); err != nil {
		t.Fatal(err)
	}

	log, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}

	held := os.Stderr
	os.Stderr = log //nolint:reassign // the log under test is the process's own standard error.

	status, answer := h.send(t, "user:alice", http.MethodGet, "/materials/m-1/spools", "")

	os.Stderr = held //nolint:reassign // put back before the log is read.

	if status != routes.StoreFailed.Status || answer["code"] != routes.StoreFailed.Name {
		t.Fatalf("a missing table answered %d %v", status, answer)
	}

	written, err := os.ReadFile(log.Name())
	if err != nil {
		t.Fatal(err)
	}

	if line := string(written); !strings.Contains(line, "GET /materials/m-1/spools") ||
		!strings.Contains(line, "no such table: spools") {
		t.Errorf("standard error reads %q, want the request and the reason", line)
	}
}

func TestAssignNeedsOperateOnThePrinter(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.access.grant("user:alice", "observe")

	status, answer := h.send(t, "user:alice", http.MethodPut, "/printers/d1/tools/0", `{"material_uuid":"m-1"}`)
	if status != http.StatusForbidden || answer["code"] != routes.NotOperator.Name {
		t.Errorf("an assign without operate answered %d %v", status, answer)
	}

	if status, answer = h.send(t, "user:alice", http.MethodDelete, "/printers/d1/tools/0", ""); status !=
		http.StatusForbidden || answer["code"] != routes.NotOperator.Name {
		t.Errorf("an unassign without operate answered %d %v", status, answer)
	}

	if stored, err := h.store.Assignments(context.Background(), "d1"); err != nil || len(stored) != 0 {
		t.Errorf("a refused assign stored %+v (%v)", stored, err)
	}

	_, listed := h.send(t, "user:alice", http.MethodGet, "/printers", "")
	if p := listed["printers"].([]any)[0].(map[string]any); p["id"] != "d1" || p["may_operate"] != false {
		t.Errorf("the printers read %v, want d1 that alice observes and may not operate", listed)
	}
}

// One person's answer is kept for that person only.
func TestOnePersonsOperateIsNotAnothers(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.access.grant("user:bob", "observe")

	if status, answer := h.send(
		t,
		"user:alice",
		http.MethodPut,
		"/printers/d1/tools/0",
		`{"material_uuid":"m-1"}`,
	); status != http.StatusOK {
		t.Fatalf("alice's assign answered %d %v", status, answer)
	}

	status, answer := h.send(t, "user:bob", http.MethodDelete, "/printers/d1/tools/0", "")
	if status != http.StatusForbidden || answer["code"] != routes.NotOperator.Name {
		t.Errorf("bob's unassign after alice's answered %d %v", status, answer)
	}

	if stored := h.mustAssignments(t, "d1"); len(stored) != 1 {
		t.Errorf("bob's refused unassign left %+v", stored)
	}
}

// A person who may not observe a printer is told nothing of it: it is not
// listed, and every route on it answers as for a printer nobody knows.
func TestAPrinterYouMayNotObserveIsNotThere(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)

	if _, listed := h.send(t, "user:carol", http.MethodGet, "/printers", ""); len(listed["printers"].([]any)) != 0 {
		t.Errorf("carol lists %v, want no printer", listed)
	}

	for _, step := range []struct{ method, path, body string }{
		{http.MethodGet, "/printers/d1/tools", ""},
		{http.MethodPut, "/printers/d1/tools/0", `{"material_uuid":"m-1"}`},
		{http.MethodDelete, "/printers/d1/tools/0", ""},
		{http.MethodPost, "/printers/d1/tools/0/spool", ""},
	} {
		status, answer := h.send(t, "user:carol", step.method, step.path, step.body)
		if status != http.StatusNotFound || answer["code"] != routes.NotFound.Name {
			t.Errorf("carol's %s %s answered %d %v, want the 404 of a printer nobody knows",
				step.method, step.path, status, answer)
		}
	}

	// A printer the follower does not hold is never asked about.
	asked := h.access.asked
	h.send(t, "user:alice", http.MethodGet, "/printers/d9/tools", "")

	if h.access.asked != asked {
		t.Errorf("a printer nobody knows was asked about")
	}
}

func TestAssignRefusedWhilePrinting(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.fleet["d1"] = printer("d1", "PRINTING", new("PLA"))

	status, answer := h.send(t, "user:alice", http.MethodPut, "/printers/d1/tools/0", `{"material_uuid":"m-1"}`)
	if status != http.StatusConflict || answer["code"] != routes.ToolRefused.Name ||
		answer["params"].(map[string]any)["reason"] != "printing" {
		t.Errorf("an assign while printing answered %d %v", status, answer)
	}

	_, tools := h.send(t, "user:alice", http.MethodGet, "/printers/d1/tools", "")
	if tool := tools["tools"].([]any)[0].(map[string]any); tools["state"] != "PRINTING" ||
		tool["assign"] != "printing" || tool["unassign"] != "printing" || tool["assignment"] != nil {
		t.Errorf("the tools while printing read %v", tools)
	}

	// The catalogue's PLA carries no type, so a printer reporting PLA refuses it.
	h.fleet["d1"] = printer("d1", "IDLE", new("PLA"))

	if _, answer = h.send(
		t,
		"user:alice",
		http.MethodPut,
		"/printers/d1/tools/0",
		`{"material_uuid":"m-1"}`,
	); answer["params"].(map[string]any)["reason"] != "reported-loaded" {
		t.Errorf("an assign the report does not match answered %v", answer)
	}

	// Idle on a printer that reports nothing, the same assign stands.
	h.fleet["d1"] = printer("d1", "IDLE", nil)

	if status, answer = h.send(
		t,
		"user:alice",
		http.MethodPut,
		"/printers/d1/tools/0",
		`{"material_uuid":"m-1"}`,
	); status != http.StatusOK || answer["by"] != "person" ||
		answer["material_name"] != "PLA" {
		t.Fatalf("an assign while idle answered %d %v", status, answer)
	}

	if status, _ = h.send(
		t,
		"user:alice",
		http.MethodPut,
		"/printers/d9/tools/0",
		`{"material_uuid":"m-1"}`,
	); status != http.StatusNotFound {
		t.Errorf("an assign on a printer nobody knows answered %d", status)
	}
}

func TestAssignAnswersRetryableWhenAccessFails(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.access.err = errors.New("the host is gone")

	status, answer := h.send(t, "user:alice", http.MethodPut, "/printers/d1/tools/0", `{"material_uuid":"m-1"}`)
	if status != http.StatusServiceUnavailable || answer["code"] != routes.AccessUnknown.Name ||
		answer["retryable"] != true {
		t.Errorf("an assign the host could not decide answered %d %v", status, answer)
	}

	if stored, err := h.store.Assignments(context.Background(), "d1"); err != nil || len(stored) != 0 {
		t.Errorf("an undecided assign stored %+v (%v)", stored, err)
	}

	// The list comes from the host's listing for the person: without it the
	// list is not known, and the caller is told to try again rather than
	// shown an empty one.
	status, listed := h.send(t, "user:alice", http.MethodGet, "/printers", "")
	if status != http.StatusServiceUnavailable || listed["code"] != routes.AccessUnknown.Name ||
		listed["retryable"] != true {
		t.Errorf("the list while the host fails answered %d %v", status, listed)
	}

	// A failed question is not kept: the next one asks again and passes.
	h.access.err = nil

	if status, answer = h.send(
		t,
		"user:alice",
		http.MethodPut,
		"/printers/d1/tools/0",
		`{"material_uuid":"m-1"}`,
	); status != http.StatusOK {
		t.Errorf("an assign after the host came back answered %d %v", status, answer)
	}

	// The answer is kept per person and printer.
	asked := h.access.asked
	h.send(t, "user:alice", http.MethodDelete, "/printers/d1/tools/0", "")

	if h.access.asked != asked {
		t.Errorf("a second write asked the host again: %d questions, want %d", h.access.asked, asked)
	}
}

func TestAnArchivedSpoolCannotGoOnATool(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.send(t, "user:alice", http.MethodPost, "/spools",
		`{"material_uuid":"m-1","remaining_weight_g":500,"quantity":1}`)

	status, answer := h.send(t, "user:alice", http.MethodPatch, "/spools/1",
		`{"archived":true,"archive_reason":"defective","archive_reason_note":"brittle"}`)
	if status != http.StatusOK || answer["archived_at"] == nil || answer["archive_reason"] != "defective" ||
		answer["archive_reason_note"] != "brittle" {
		t.Fatalf("archiving answered %d %v", status, answer)
	}

	if status, answer = h.send(
		t,
		"user:alice",
		http.MethodPut,
		"/printers/d1/tools/0",
		`{"spool_id":1}`,
	); status != http.StatusConflict ||
		answer["code"] != routes.SpoolArchived.Name {
		t.Errorf("assigning an archived spool answered %d %v", status, answer)
	}
}

func TestEditingAnAssignedSpoolIsRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.send(t, "user:alice", http.MethodPost, "/spools",
		`{"material_uuid":"m-1","remaining_weight_g":500,"quantity":1}`)

	status, answer := h.send(t, "user:alice", http.MethodPut, "/printers/d1/tools/0", `{"spool_id":1}`)
	if status != http.StatusOK || answer["spool_id"] != 1.0 {
		t.Fatalf("assigning the spool answered %d %v", status, answer)
	}

	_, page := h.send(t, "user:alice", http.MethodGet, "/materials/m-1/spools", "")
	if tool := page["spools"].([]any)[0].(map[string]any)["tool"]; tool == nil ||
		tool.(map[string]any)["device_id"] != "d1" || tool.(map[string]any)["tool_id"] != "0" {
		t.Errorf("the spool on d1 reads its tool as %v", tool)
	}

	h.fleet["d2"] = printer("d2", "IDLE", nil)
	if status, answer = h.send(
		t,
		"user:alice",
		http.MethodPut,
		"/printers/d2/tools/0",
		`{"spool_id":1}`,
	); status != http.StatusConflict ||
		answer["code"] != routes.SpoolOnAnotherTool.Name {
		t.Errorf("the spool put on a second tool answered %d %v", status, answer)
	}

	// A person may correct its weight on the tool.
	if status, answer = h.send(t, "user:alice", http.MethodPatch, "/spools/1", `{"remaining_weight_g":100}`); status !=
		http.StatusOK || answer["remaining_weight_g"] != 100.0 {
		t.Errorf("the weight of an assigned spool answered %d %v", status, answer)
	}

	for _, step := range []struct{ method, path, body string }{
		{http.MethodPatch, "/spools/1", `{"archived":true}`},
		{http.MethodDelete, "/spools/1", ""},
		{http.MethodPost, "/spools/delete", `{"ids":[1]}`},
	} {
		if status, answer = h.send(t, "user:alice", step.method, step.path, step.body); status != http.StatusConflict ||
			answer["code"] != routes.SpoolAssigned.Name {
			t.Errorf("%s %s of an assigned spool answered %d %v", step.method, step.path, status, answer)
		}
	}

	if status, _ = h.send(
		t,
		"user:alice",
		http.MethodDelete,
		"/printers/d1/tools/0",
		"",
	); status != http.StatusNoContent {
		t.Fatalf("unassigning answered %d", status)
	}

	if status, _ = h.send(t, "user:alice", http.MethodDelete, "/spools/1", ""); status != http.StatusNoContent {
		t.Errorf("deleting the unassigned spool answered %d", status)
	}
}

// tagged is an idle printer whose tool 0 read a spool's NFC tag.
func tagged(id, abbreviation string, r connectorapi.ReportedMaterial) printers.Printer {
	p := printer(id, "IDLE", &abbreviation)
	p.Tools.Tools["0"].Reported = &r

	return p
}

func TestAddReportedSpoolTwiceMakesOne(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.fleet["d1"] = tagged("d1", "PLA", connectorapi.ReportedMaterial{
		InstanceUUID: "tag-1", MaterialUUID: "m-1", Type: "PLA", ColorRGBA: "#ff0000ff", FullWeight: new(1000.0),
	})

	var first map[string]any

	for range 2 {
		status, answer := h.send(t, "user:alice", http.MethodPost, "/printers/d1/tools/0/spool", "")
		if status != http.StatusOK || answer["uuid"] != "tag-1" || answer["source"] != "printer" ||
			answer["material_name"] != "PLA" || answer["full_weight_g"] != 1000.0 ||
			answer["remaining_weight_g"] != 1000.0 || answer["color_rgba"] != "#ff0000ff" {
			t.Fatalf("adding the reported spool answered %d %v", status, answer)
		}

		if first != nil && answer["id"] != first["id"] {
			t.Errorf("the second add made spool %v beside %v", answer["id"], first["id"])
		}

		first = answer
	}

	if _, page := h.send(t, "user:alice", http.MethodGet, "/materials/m-1/spools", ""); page["total"] != 1.0 {
		t.Errorf("the material holds %v, want the one reported spool", page)
	}

	if stored := h.mustAssignments(t, "d1"); len(stored) != 1 || stored[0].SpoolID == nil ||
		float64(*stored[0].SpoolID) != first["id"] {
		t.Errorf("the tool holds %+v, want the reported spool", stored)
	}

	// A material the catalogue lacks is labelled by what the printer reports.
	h.fleet["d2"] = tagged("d2", "PETG", connectorapi.ReportedMaterial{InstanceUUID: "tag-2", MaterialUUID: "x-9"})

	status, answer := h.send(t, "user:alice", http.MethodPost, "/printers/d2/tools/0/spool", "")
	if status != http.StatusOK || answer["material_uuid"] != "x-9" || answer["material_name"] != "PETG" {
		t.Errorf("adding a spool of an unknown material answered %d %v", status, answer)
	}

	if stored := h.mustAssignments(t, "d2"); len(stored) != 1 || stored[0].Abbreviation != "PETG" {
		t.Errorf("the tool holds %+v, want the spool labelled PETG", stored)
	}

	// A tool whose spool carries no tag has nothing to add.
	h.fleet["d3"] = printer("d3", "IDLE", new("PLA"))
	if status, answer = h.send(t, "user:alice", http.MethodPost, "/printers/d3/tools/0/spool", ""); status !=
		http.StatusNotFound || answer["code"] != routes.NotReported.Name {
		t.Errorf("adding a spool with no tag answered %d %v", status, answer)
	}

	h.access.grant("user:bob", "observe")
	if status, _ = h.send(t, "user:bob", http.MethodPost, "/printers/d1/tools/0/spool", ""); status !=
		http.StatusForbidden {
		t.Errorf("adding a spool without operate answered %d", status)
	}
}

func TestMergeKeepsTheNFCIdentity(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.fleet["d1"] = tagged("d1", "PLA", connectorapi.ReportedMaterial{InstanceUUID: "tag-1", MaterialUUID: "m-1"})

	_, nfc := h.send(t, "user:alice", http.MethodPost, "/printers/d1/tools/0/spool", "")
	_, shelf := h.send(t, "user:alice", http.MethodPost, "/locations", `{"name":"Shelf"}`)
	_, added := h.send(t, "user:alice", http.MethodPost, "/spools", fmt.Sprintf(
		`{"material_uuid":"m-1","remaining_weight_g":500,"quantity":2,"location_id":%v,"notes":"dry"}`,
		shelf["id"],
	))
	ids := added["ids"].([]any)
	manual, spare := ids[0].(float64), ids[1].(float64)
	into := nfc["id"].(float64)

	h.fleet["d2"] = printer("d2", "IDLE", nil)
	h.send(t, "user:alice", http.MethodPut, "/printers/d2/tools/0", fmt.Sprintf(`{"spool_id":%v}`, spare))

	for _, step := range []struct {
		from, into float64
		code       string
	}{
		{into, manual, routes.MergeRefused.Name},
		{manual, manual, routes.MergeRefused.Name},
		{spare, into, routes.SpoolAssigned.Name},
	} {
		status, answer := h.send(t, "user:alice", http.MethodPost, fmt.Sprintf("/spools/%v/merge", step.from),
			fmt.Sprintf(`{"into":%v}`, step.into))
		if answer["code"] != step.code {
			t.Errorf("merging %v into %v answered %d %v, want %s", step.from, step.into, status, answer, step.code)
		}
	}

	status, merged := h.send(t, "user:alice", http.MethodPost, fmt.Sprintf("/spools/%v/merge", manual),
		fmt.Sprintf(`{"into":%v}`, into))
	if status != http.StatusOK || merged["id"] != into || merged["uuid"] != "tag-1" || merged["source"] != "printer" ||
		merged["location"] != "Shelf" || merged["notes"] != "dry" {
		t.Fatalf("the merge answered %d %v", status, merged)
	}

	if _, page := h.send(t, "user:alice", http.MethodGet, "/materials/m-1/spools", ""); page["total"] != 2.0 {
		t.Errorf("after the merge the material holds %v, want the NFC spool and the spare", page)
	}
}

// mustAssignments answers what is on each tool of one printer.
func (h harness) mustAssignments(t *testing.T, device string) []stock.Assignment {
	t.Helper()

	stored, err := h.store.Assignments(context.Background(), device)
	if err != nil {
		t.Fatal(err)
	}

	return stored
}

func TestUnassignKeepsWhatThePrinterReports(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.fleet["d1"] = tagged("d1", "PLA", connectorapi.ReportedMaterial{InstanceUUID: "tag-1", MaterialUUID: "m-1"})
	h.fleet["d2"] = printer("d2", "IDLE", nil)

	_, spool := h.send(t, "user:alice", http.MethodPost, "/printers/d1/tools/0/spool", "")
	h.send(t, "user:alice", http.MethodPut, "/printers/d2/tools/0", `{"material_uuid":"m-1"}`)

	for _, device := range []string{"d1", "d2"} {
		if status, answer := h.send(t, "user:alice", http.MethodDelete, "/printers/"+device+"/tools/0", ""); status !=
			http.StatusNoContent {
			t.Fatalf("unassigning %s answered %d %v", device, status, answer)
		}
	}

	// The reporting tool keeps the printer's material and its spool.
	if stored := h.mustAssignments(t, "d1"); len(stored) != 1 || stored[0].By != stock.ByPrinter ||
		stored[0].Abbreviation != "PLA" || stored[0].SpoolID == nil || float64(*stored[0].SpoolID) != spool["id"] {
		t.Errorf("the reporting tool holds %+v, want the printer's PLA and its spool", stored)
	}

	if _, page := h.send(
		t,
		"user:alice",
		http.MethodGet,
		"/materials/m-1/spools",
		"",
	); page["spools"].([]any)[0].(map[string]any)["tool"] == nil {
		t.Errorf("the NFC spool left its tool: %v", page)
	}

	if stored := h.mustAssignments(t, "d2"); len(stored) != 0 {
		t.Errorf("the tool that reports nothing holds %+v", stored)
	}
}

// mmu is an idle printer whose head 0 carries a feeder of five slots, as an
// MMU printer reports it. The head reports no material of its own.
func mmu(id string) printers.Printer {
	tools := map[connectorapi.ToolID]*connectorapi.Tool{
		"0": {
			ID: "0", Type: connectorapi.ToolTypeHead,
			Feeder: &connectorapi.Feeder{SlotCount: 5, SlotCountKnown: true},
		},
	}

	for slot := range 5 {
		id := connectorapi.ToolID("0." + strconv.Itoa(slot))
		tools[id] = &connectorapi.Tool{ID: id, Type: connectorapi.ToolTypeFeederSlot}
	}

	return printers.Printer{ID: id, Name: "Printer " + id, Connected: true, State: "IDLE",
		Tools: connectorapi.ToolTree{Tools: tools}}
}

// A head that carries a feeder holds no material: each of its slots does.
// Only the slots are listed and take a material.
func TestAnMMUOffersItsSlotsAndNotItsHead(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.fleet["m1"] = mmu("m1")

	_, tools := h.send(t, "user:alice", http.MethodGet, "/printers/m1/tools", "")

	var ids []string
	for _, tool := range tools["tools"].([]any) {
		ids = append(ids, tool.(map[string]any)["id"].(string))
	}

	if strings.Join(ids, " ") != "0.0 0.1 0.2 0.3 0.4" {
		t.Errorf("the MMU printer lists tools %v, want its five slots", ids)
	}

	status, answer := h.send(t, "user:alice", http.MethodPut, "/printers/m1/tools/0", `{"material_uuid":"m-1"}`)
	if status != http.StatusNotFound || answer["code"] != routes.NotFound.Name {
		t.Errorf("an assign on the MMU head answered %d %v, want the 404 of a tool that is not there", status, answer)
	}

	if status, answer = h.send(
		t, "user:alice", http.MethodPut, "/printers/m1/tools/0.2", `{"material_uuid":"m-1"}`,
	); status != http.StatusOK {
		t.Errorf("an assign on slot 3 answered %d %v", status, answer)
	}

	if stored := h.mustAssignments(t, "m1"); len(stored) != 1 || stored[0].ToolID != "0.2" {
		t.Errorf("the MMU printer holds %+v, want slot 3 only", stored)
	}
}

// A spool can stay on a tool that is gone: its printer left the host, or the
// printer no longer has the tool. Its row says so, and a person clears it.
// A printer still in the fleet still takes operate, and one that left takes
// only write access to Materials, which the host checked.
func TestATiedSpoolOnAGoneToolIsCleared(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.access.grant("user:bob", "observe")
	// Carol holds no role on any printer.
	h.access.grant("user:carol", "")

	_, added := h.send(t, "user:alice", http.MethodPost, "/spools",
		`{"material_uuid":"m-1","remaining_weight_g":500,"quantity":2}`)
	ids := added["ids"].([]any)

	for i, place := range []stock.ToolRef{{DeviceID: "left", ToolID: "0"}, {DeviceID: "d1", ToolID: "5"}} {
		id := int64(ids[i].(float64))
		if _, err := h.store.Assign(context.Background(), "alice", stock.Assignment{
			DeviceID: place.DeviceID, ToolID: place.ToolID, SpoolID: &id, By: stock.ByPerson,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// The tool d1 lost is gone. A printer the host no longer lists is not
	// named: the follower clears a removed one, and any other may only be
	// hidden from the module.
	_, page := h.send(t, "user:alice", http.MethodGet, "/materials/m-1/spools", "")
	tools := map[string]any{}
	for _, s := range page["spools"].([]any) {
		tool := s.(map[string]any)["tool"].(map[string]any)
		tools[fmt.Sprint(tool["device_id"], "/", tool["tool_id"])] = tool["gone"]
	}

	if want := map[string]any{"/0": false, "d1/5": true}; !maps.Equal(tools, want) {
		t.Errorf("the spools' tools read %v, want %v", tools, want)
	}

	// A printer the host no longer lists answers as a printer that never was,
	// so the answer does not tell which ids once held an assignment.
	_, never := h.send(t, "user:carol", http.MethodDelete, "/printers/nobody/tools/0", "")
	_, gone := h.send(t, "user:carol", http.MethodDelete, "/printers/left/tools/0", "")
	if never["code"] != gone["code"] || fmt.Sprint(never["params"]) != strings.ReplaceAll(fmt.Sprint(gone["params"]),
		"left", "nobody") {
		t.Errorf("a printer that left answered %v, a printer that never was %v", gone, never)
	}

	for _, step := range []struct {
		who, path string
		status    int
	}{
		{"user:alice", "/printers/left/tools/0", http.StatusNotFound},
		{"user:carol", "/printers/d1/tools/5", http.StatusNotFound},
		{"user:bob", "/printers/d1/tools/5", http.StatusForbidden},
		{"user:alice", "/printers/d1/tools/7", http.StatusNotFound},
		{"user:alice", "/printers/d1/tools/5", http.StatusNoContent},
	} {
		if status, answer := h.send(t, step.who, http.MethodDelete, step.path, ""); status != step.status {
			t.Errorf("%s's DELETE %s answered %d %v, want %d", step.who, step.path, status, answer, step.status)
		}
	}

	if left := h.mustAssignments(t, "left"); len(left) != 1 {
		t.Errorf("the printer that left lost %+v", left)
	}

	if d1 := h.mustAssignments(t, "d1"); len(d1) != 0 {
		t.Errorf("the gone tool still holds %+v", d1)
	}

	// Only an unassign reaches a gone tool.
	status, _ := h.send(t, "user:alice", http.MethodPut, "/printers/d1/tools/5", `{"material_uuid":"m-1"}`)
	if status != http.StatusNotFound {
		t.Errorf("an assign on a tool the printer lacks answered %d", status)
	}
}

// A spool on a printer the person may not observe says it is on a printer,
// and not which one.
func TestASpoolOnAPrinterYouMayNotObserveHidesThePrinter(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.send(t, "user:alice", http.MethodPost, "/spools",
		`{"material_uuid":"m-1","remaining_weight_g":500,"quantity":1}`)
	h.send(t, "user:alice", http.MethodPut, "/printers/d1/tools/0", `{"spool_id":1}`)

	for who, device := range map[string]string{"user:alice": "d1", "user:carol": ""} {
		_, page := h.send(t, who, http.MethodGet, "/materials/m-1/spools", "")

		tool, _ := page["spools"].([]any)[0].(map[string]any)["tool"].(map[string]any)
		if tool == nil || tool["device_id"] != device || tool["tool_id"] != "0" || tool["gone"] != false {
			t.Errorf("%s reads the spool's tool as %v, want device %q", who, tool, device)
		}
	}
}

// A tool that reports what it used of the spool adds the spool with the rest.
func TestAddReportedSpoolTakesOffWhatTheToolUsed(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)

	for tag, used := range map[string]float64{"tag-1": 250, "tag-2": 1200} {
		p := tagged("p-"+tag, "PLA", connectorapi.ReportedMaterial{InstanceUUID: tag, FullWeight: new(1000.0)})
		p.Tools.Tools["0"].ConsumedWeight = new(used)
		h.fleet[p.ID] = p
	}

	for device, want := range map[string]float64{"p-tag-1": 750, "p-tag-2": 0} {
		status, answer := h.send(t, "user:alice", http.MethodPost, "/printers/"+device+"/tools/0/spool", "")
		if status != http.StatusOK || answer["full_weight_g"] != 1000.0 || answer["remaining_weight_g"] != want {
			t.Errorf("adding the spool on %s answered %d %v, want %v g left", device, status, answer, want)
		}
	}
}

// A tag with no actual weight, or one of 0 g, is weighed by its package's
// nominal weight, and an actual weight wins over it. The tools show the same.
func TestAddReportedSpoolTakesTheNominalWeight(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)

	for tag, actual := range map[string]*float64{"tag-1": nil, "tag-2": new(800.0), "tag-3": new(0.0)} {
		p := tagged("p-"+tag, "PLA", connectorapi.ReportedMaterial{
			InstanceUUID: tag, FullWeight: actual, NominalFullWeight: new(1000.0),
		})
		p.Tools.Tools["0"].ConsumedWeight = new(595.0)
		h.fleet[p.ID] = p
	}

	for device, full := range map[string]float64{"p-tag-1": 1000, "p-tag-2": 800, "p-tag-3": 1000} {
		_, view := h.send(t, "user:alice", http.MethodGet, "/printers/"+device+"/tools", "")
		if got := view["tools"].([]any)[0].(map[string]any)["reported"].(map[string]any)["full_weight_g"]; got != full {
			t.Errorf("the tools of %s report a full weight of %v, want %v", device, got, full)
		}

		status, answer := h.send(t, "user:alice", http.MethodPost, "/printers/"+device+"/tools/0/spool", "")
		if status != http.StatusOK || answer["full_weight_g"] != full || answer["remaining_weight_g"] != full-595 {
			t.Errorf("adding the spool on %s answered %d %v, want %v g full", device, status, answer, full)
		}
	}
}

func TestEverySpoolInStockIsOneRead(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)

	_, custom := h.send(t, "user:alice", http.MethodPost, "/custom-materials",
		`{"name":"Galaxy","class":"FFF","type":"PLA","abbreviation":"PLA"}`)
	h.send(t, "user:alice", http.MethodPost, "/spools",
		`{"material_uuid":"m-1","remaining_weight_g":500,"quantity":2}`)
	h.send(t, "user:alice", http.MethodPost, "/spools",
		`{"material_uuid":"`+custom["uuid"].(string)+`","remaining_weight_g":300,"quantity":1}`)
	h.send(t, "user:alice", http.MethodDelete, "/spools/1", "")

	count := func(path string) (int, map[string]any) {
		status, page := h.send(t, "sink:farm", http.MethodGet, path, "")
		if status != http.StatusOK {
			t.Fatalf("%s answered %d %v", path, status, page)
		}

		return len(page["spools"].([]any)), page
	}

	if n, page := count("/spools"); n != 2 || page["total"] != 2.0 {
		t.Errorf("every live spool: %d of %v", n, page["total"])
	}

	if n, _ := count("/spools?material=m-1"); n != 1 {
		t.Errorf("the spools of m-1: %d, want the one not deleted", n)
	}

	if n, page := count("/spools?limit=1"); n != 1 || page["next"] != 1.0 {
		t.Errorf("a page of one: %d, next %v", n, page["next"])
	}
}

func TestEveryToolIsOneRead(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)
	h.access.grant("sink:farm", "observe")

	h.fleet["d2"] = printers.Printer{
		ID: "d2", Name: "Printer d2", Connected: true, State: "IDLE",
		Tools: connectorapi.ToolTree{Tools: map[connectorapi.ToolID]*connectorapi.Tool{
			"0":   {ID: "0", Feeder: &connectorapi.Feeder{SlotCount: 2, SlotCountKnown: true}},
			"0.0": {ID: "0.0", Material: new("PLA")},
			"0.1": {ID: "0.1", Material: new("PETG")},
		}},
	}

	added, err := h.store.AddSpools(context.Background(), "user:alice", stock.SpoolDraft{
		MaterialUUID: "m-1", RemainingWeightG: 640, FullWeightG: new(1000.0), Condition: "new",
	}, 1, 10)
	if err != nil {
		t.Fatal(err)
	}

	spool := added[0]

	if status, put := h.send(t, "user:alice", http.MethodPut, "/printers/d1/tools/0",
		fmt.Sprintf(`{"spool_id":%d}`, spool.ID)); status != http.StatusOK {
		t.Fatalf("assign answered %d %v", status, put)
	}

	status, page := h.send(t, "sink:farm", http.MethodGet, "/printers/tools", "")
	if status != http.StatusOK {
		t.Fatalf("answered %d %v", status, page)
	}

	got := map[string]map[string]any{}
	for _, row := range page["tools"].([]any) {
		r := row.(map[string]any)
		got[r["device_id"].(string)+"/"+r["tool_id"].(string)] = r
	}

	if len(got) != 3 || got["d2/0"] != nil || got["d2/0.0"] == nil || got["d2/0.1"] == nil {
		t.Fatalf("the tools are %v, want d1/0 and the two slots of d2", slices.Collect(maps.Keys(got)))
	}

	on := got["d1/0"]
	if on["spool_id"] != float64(spool.ID) || on["spool_uuid"] != spool.UUID || on["material_uuid"] != "m-1" ||
		on["brand_name"] != "Prusament" || on["color_family"] != "black" || on["remaining_weight_g"] != 640.0 || on["full_weight_g"] != 1000.0 || on["by"] != "person" {
		t.Errorf("the tool with the spool reads %v", on)
	}

	// A tool with no material assigned has no colour, so no family either.
	if family, ok := got["d2/0.0"]["color_family"]; ok {
		t.Errorf("the tool with nothing assigned reads the colour family %v", family)
	}

	if page["total"] != 2.0 {
		t.Errorf("total %v, want the two printers", page["total"])
	}

	if _, first := h.send(t, "sink:farm", http.MethodGet, "/printers/tools?limit=1", ""); first["next"] != 1.0 ||
		len(first["tools"].([]any)) != 1 {
		t.Errorf("a page of one printer: %v", first)
	}

	if _, none := h.send(t, "sink:nobody", http.MethodGet, "/printers/tools", ""); len(none["tools"].([]any)) != 0 {
		t.Errorf("a module that may observe no printer reads %v", none["tools"])
	}
}

// unlisted is a fleet the follower has not listed yet: it holds no printer.
type unlisted struct{}

func (unlisted) Printer(string) (printers.Printer, bool) { return printers.Printer{}, false }
func (unlisted) Listed() bool                            { return false }
func (unlisted) Printers() []printers.Printer            { return nil }

// Before the printers are listed, a spool's printer is named only to a caller
// the host says may observe it.
func TestASpoolsPrinterIsHiddenBeforeThePrintersAreListed(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	h.seed(t, 0)

	added, err := h.store.AddSpools(context.Background(), "user:alice", stock.SpoolDraft{
		MaterialUUID: "m-1", RemainingWeightG: 500, Condition: "new",
	}, 1, 10)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = h.store.Assign(context.Background(), "user:alice", stock.Assignment{
		DeviceID: "d1", ToolID: "0", SpoolID: &added[0].ID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	h.handler = routes.New(h.store, h.imp, nil, h.live, unlisted{}, &seen{access: h.access}, h.access)

	tool := func(who string) map[string]any {
		_, page := h.send(t, who, http.MethodGet, "/spools", "")

		return page["spools"].([]any)[0].(map[string]any)["tool"].(map[string]any)
	}

	if got := tool("sink:nobody"); got["device_id"] != nil && got["device_id"] != "" {
		t.Errorf("a caller who may observe no printer reads %v", got)
	}

	if got := tool("user:alice"); got["device_id"] != "d1" {
		t.Errorf("a caller who may observe the printer reads %v", got)
	}
}

func TestASlotReadsItsHeadsNozzleAndFeeder(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	pla, yes := "PLA", true
	h.fleet["mmu"] = printers.Printer{
		ID: "mmu", Name: "MK4S", Connected: true, State: "IDLE",
		Tools: connectorapi.ToolTree{Tools: map[connectorapi.ToolID]*connectorapi.Tool{
			"0": {
				ID:   "0",
				Type: connectorapi.ToolTypeHead,
				Features: map[string]json.RawMessage{
					"nozzle_diameter":  json.RawMessage(`0.4`),
					"nozzle_high_flow": json.RawMessage(`true`),
				},
				Feeder: &connectorapi.Feeder{SlotCount: 2, SlotCountKnown: true, Model: "MMU3S"},
			},
			"0.0": {ID: "0.0", Type: connectorapi.ToolTypeFeederSlot, Material: &pla},
			"0.1": {ID: "0.1", Type: connectorapi.ToolTypeFeederSlot, Active: &yes},
		}},
	}

	status, answer := h.send(t, "user:alice", http.MethodGet, "/printers/mmu/tools", "")
	if status != http.StatusOK {
		t.Fatalf("%d %v", status, answer)
	}

	var got []string
	for _, raw := range answer["tools"].([]any) {
		tool := raw.(map[string]any)
		got = append(
			got,
			fmt.Sprintf("%v %v %v %v %v", tool["id"], tool["kind"], tool["active"], tool["feeder"], tool["nozzle"]),
		)
	}

	want := []string{
		"0.0 slot false MMU3S map[diameter_mm:0.4 high_flow:true]",
		"0.1 slot true MMU3S map[diameter_mm:0.4 high_flow:true]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the tools read %q, want %q", got, want)
	}
}

func TestPrintersAreSearchedAndPagedByName(t *testing.T) {
	t.Parallel()

	for hidden, cases := range map[string]map[string]string{
		"": {
			"":                  "[alpha bravo Charlie Delta Echo] <nil>",
			"?limit=2":          "[alpha bravo] 2",
			"?limit=2&offset=2": "[Charlie Delta] 4",
			"?limit=2&offset=4": "[Echo] <nil>",
			"?q=xl":             "[Echo] <nil>",
			"?q=%20HAR%20":      "[Charlie] <nil>",
			"?id=c&id=a&id=zz":  "[Charlie Delta] <nil>",
			"?id=c,a":           "[Charlie Delta] <nil>",
			"?id=c&q=xl":        "[] <nil>",
		},
		// A printer the caller may not see leaves no gap in a page, and no
		// next that leads to an empty one.
		"b e": {
			"?limit=2":          "[bravo Charlie] 2",
			"?limit=2&offset=2": "[Delta] <nil>",
			"?q=xl":             "[] <nil>",
			"?id=b&id=c":        "[Charlie] <nil>",
		},
	} {
		h := newHarness(t, nil)
		delete(h.fleet, "d1")

		h.access.hidden = map[string]bool{}
		for id := range strings.FieldsSeq(hidden) {
			h.access.hidden[id] = true
		}

		for id, name := range map[string]string{"a": "Delta", "b": "alpha", "c": "Charlie", "d": "bravo", "e": "Echo"} {
			p := printer(id, "IDLE", nil)
			p.Name, p.Model = name, "MK4S"
			if id == "e" {
				p.Model = "XL"
			}

			h.fleet[id] = p
		}

		for query, want := range cases {
			status, answer := h.send(t, "user:alice", http.MethodGet, "/printers"+query, "")
			if status != http.StatusOK {
				t.Fatalf("%s answered %d %v", query, status, answer)
			}

			got := []string{}
			for _, p := range answer["printers"].([]any) {
				got = append(got, p.(map[string]any)["name"].(string))
			}

			if fmt.Sprint(got, " ", answer["next"]) != want {
				t.Errorf("hiding %q, %q answered %v %v, want %s", hidden, query, got, answer["next"], want)
			}
		}
	}

	h := newHarness(t, nil)
	if status, _ := h.send(t, "user:alice", http.MethodGet, "/printers?limit=-1", ""); status != http.StatusBadRequest {
		t.Errorf("a negative limit answered %d", status)
	}
}

func TestAFleetOfHundredsIsListedInAFewCalls(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	for n := range 1001 {
		id := fmt.Sprintf("p%04d", n)
		h.fleet[id] = printer(id, "IDLE", nil)
	}

	status, answer := h.send(t, "user:alice", http.MethodGet, "/printers/tools?limit=10", "")
	if status != http.StatusOK || answer["total"] != 1002.0 {
		t.Fatalf("%d %v", status, answer["total"])
	}

	// The host lists 200 a page, and is asked about no printer one by one.
	if h.seen.pages > 6 || h.access.asked != 0 {
		t.Errorf("the tools of 1002 printers took %d pages and %d questions", h.seen.pages, h.access.asked)
	}

	if status, _ = h.send(t, "user:alice", http.MethodGet, "/printers?limit=10", ""); status != http.StatusOK {
		t.Fatal(status)
	}

	// Only the page's rows are asked whether the caller may operate them.
	if h.access.asked != 10 {
		t.Errorf("a page of 10 printers asked %d questions", h.access.asked)
	}

	if status, _ = h.send(t, "user:alice", http.MethodGet, "/printers?id=p0500&id=p0501", ""); status != http.StatusOK {
		t.Fatal(status)
	}

	// Printers asked for by id are the only ones asked about.
	if h.access.asked != 12 {
		t.Errorf("2 printers asked for by id took %d questions", h.access.asked-10)
	}
}
