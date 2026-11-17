package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/prusa3d/prusa-connect/internal/printer/echodevice"
	"github.com/prusa3d/prusa-connect/internal/printer/simulator"
	"github.com/prusa3d/prusa-connect/tools/camerasim"
)

const (
	demoCancelShare = 0.05
	demoFailShare   = 0.07
	demoStopVerb    = "core.command.stop_print"

	// Print lengths in frames, and idle gaps, at the demo's own pace and the
	// fast pace a test asks for.
	demoGapSpread    = 4 * time.Minute
	demoFramesBase   = 180
	demoFramesSpread = 721
	demoFastBase     = 10
	demoFastSpread   = 11

	// A cut print is stopped or failed this far into its frames.
	demoCutFrom = 0.2
	demoCutSpan = 0.6

	// demoEndHeld is the shortest wait between a print's end and the next
	// start. The core reads a print's end off its record flush, so a printer
	// that starts again before a flush has run leaves that end unrecorded.
	// Two intervals, because a flush that runs long under load skips a tick.
	demoEndHeld = 2 * recordFlushInterval
)

// The waits the demo makes on its own core. They are budgets rather than
// unbounded waits: a core that will not come up is something to say out loud,
// not a reason to sit at a blank screen.
const (
	demoStartBudget  = 60 * time.Second
	demoSettleBudget = 90 * time.Second
	demoPoll         = 250 * time.Millisecond
	demoRequest      = 15 * time.Second
	// demoRetryPause is the wait between two tries of one call that the core
	// did not answer; long enough for a loaded core to breathe, short enough
	// that the banner is not late.
	demoRetryPause = time.Second
)

// The print the demo starts, so the fleet is reporting a job rather than
// holding still. The path is the storage-prefixed spelling the file listing
// returns, which is the one a person types into the same endpoint.
const (
	demoStartVerb = "core.command.start_print"
	demoPrintPath = "usb/tangle.bgcode"
	// demoJobFrames is how many telemetry frames the print takes. At the
	// simulator's 1 Hz printing cadence that is five minutes, so the progress a
	// reader watches moves a percent every three seconds rather than holding at
	// zero, which is what a job with no length reports.
	demoJobFrames = 300
	// demoMMUModel is the model the demo fits a multi-material unit to.
	demoMMUModel = "MK4S"
)

// demoDevice is the part of one listed printer this run reads. The listing
// carries far more, and reading all of it here would make the demo the thing
// that has to be kept in step with the API.
type demoDevice struct {
	ID        string `json:"id"`
	Serial    string `json:"serial"`
	Connected bool   `json:"connected"`
	// Capabilities tells the Prusas from the echo machine, which takes no G-code.
	Capabilities []string `json:"capabilities"`
}

// demoListing is one page of the fleet.
type demoListing struct {
	Items []demoDevice `json:"items"`
	Total int          `json:"total"`
}

// demoFleet is the printers this run drives.
type demoFleet struct {
	printers []*simulator.Printer
	// other is the one machine in the fleet that is not a Prusa. It is not a
	// simulator.Printer, because the simulator only speaks the xBuddy dialect
	// and only for a model the shipped profile file covers. See the demoEcho
	// constants in demo.go for which dialect it does speak.
	other  *echodevice.Device
	client *http.Client
	// login is the demo account, written before the core started and printed
	// once the fleet is up.
	login demoLogin
	// token is what the demo's own requests carry once it has signed in. It
	// is minted over the API like anybody else's, so the reads the demo makes
	// are the reads a script makes.
	token string
	// operatorFile is the file the demo laid its settings over, empty when
	// there was none to read.
	operatorFile string
	// offered is how many modules this run named in core.modules_local, and zero
	// for a checkout where nothing has been built.
	offered int
	// cameras are the simulated devices this run started, one per printer,
	// held so they can be shut down with the fleet. They are the far end of
	// the wire the declared camera instances read from, and nothing in the
	// core owns them.
	cameras []*camerasim.Server
}

// newDemoFleet builds the printers without starting any of them, so a model
// nobody covers is refused before the core takes a port.
func newDemoFleet(opts DemoOptions) (*demoFleet, error) {
	printers := make([]*simulator.Printer, 0, opts.Printers)

	for i := range opts.Printers {
		name := opts.Models[i%len(opts.Models)]

		printer, err := simulator.New(simulator.Config{
			Model:     name,
			Serial:    fmt.Sprintf(demoSerial, i+1),
			BaseURL:   opts.baseURL(),
			JobFrames: demoJobFramesOf(opts),
			// An idle fleet is planned by a module that keeps what each
			// printer holds, which a fixed report would contradict.
			NoMaterial: opts.Idle,
			// The demo's MK4S carries an MMU3, so the fleet shows a printer
			// with filament slots beside the XL with its toolheads.
			MMU: strings.EqualFold(name, demoMMUModel),
			// The demo's XL has its enclosure on, so the fleet shows a
			// chamber reading on a machine that cannot heat it.
			Enclosure: strings.EqualFold(name, demoEnclosureModel),
			// A file that takes seconds to reach a printer is one a person
			// can watch on its way.
			TransferRate: opts.TransferRate,
			// A folder that takes seconds to list is what a full USB stick
			// costs a real printer, and what the Files tab has to survive.
			ListingDelay: opts.ListingDelay,
		})
		if err != nil {
			return nil, fmt.Errorf("build a %s for the demo fleet: %w", name, err)
		}

		printers = append(printers, printer)
	}

	other, err := echodevice.New(echodevice.Config{
		BaseURL:   opts.baseURL(),
		Connector: demoEchoConnector,
		Vendor:    demoEchoVendor,
		Model:     demoEchoModel,
		Serial:    demoEchoSerial,
		Firmware:  demoEchoFirmware,
	})
	if err != nil {
		return nil, fmt.Errorf("build the %s machine for the demo fleet: %w", demoEchoModel, err)
	}

	return &demoFleet{
		printers: printers, other: other, client: &http.Client{Timeout: demoRequest},
	}, nil
}

// run waits for the core, dials the fleet at it, says where to look, and then
// holds the printers on the wire until the person stops the demo.
func (f *demoFleet) run(ctx context.Context, opts DemoOptions, dir string) error {
	if err := f.await(ctx, opts.baseURL()+"/api/v1/health", demoStartBudget,
		func(demoListing) bool { return true }); err != nil {
		return fmt.Errorf("the core did not answer on %s: %w", opts.baseURL(), err)
	}

	// The sign-in comes before the fleet dials. Every read below takes a
	// credential, and a sign-in that fails is worth knowing before three
	// printers are on the wire.
	token, signInErr := f.signIn(ctx, opts, f.login)
	if signInErr != nil {
		return signInErr
	}

	f.token = token

	group, fleetCtx := errgroup.WithContext(ctx)
	defer func() { _ = group.Wait() }()

	for _, printer := range f.printers {
		group.Go(func() error {
			if err := printer.RegisterAndRun(fleetCtx); err != nil && ctx.Err() == nil {
				return fmt.Errorf("the simulated %s stopped: %w", printer.Serial(), err)
			}

			return nil
		})
	}

	group.Go(func() error {
		if err := f.other.Run(fleetCtx); err != nil && ctx.Err() == nil {
			return fmt.Errorf("the simulated %s stopped: %w", f.other.Serial(), err)
		}

		return nil
	})

	// The Prusa machines and the one that is not, because the fleet is not up
	// until every machine a person is about to be shown has arrived.
	expected := opts.Printers + 1

	if err := f.await(ctx, opts.baseURL()+"/api/v1/devices", demoSettleBudget,
		func(page demoListing) bool { return connectedIn(page) >= expected }); err != nil {
		return fmt.Errorf("only some of the %d printers reached the core: %w", expected, err)
	}

	page := f.fleetPage(ctx, opts)

	// The listing is ordered by the identifier the server minted, which reads
	// as no order at all. A person matches a row against the serial on a label,
	// so the fleet is named back to them in that order.
	slices.SortFunc(page.Items, func(a, b demoDevice) int { return strings.Compare(a.Serial, b.Serial) })

	// The cameras come before the print. A print start that the core answers
	// late is repeated until the settle budget ends, and a page that waits
	// for its cameras behind that wait sees an empty listing for the whole
	// of it.
	cameras := f.demoCameras(ctx, opts, page)
	defer f.stopDemoCameras()

	bySerial := map[string]string{}
	for _, device := range page.Items {
		bySerial[device.Serial] = device.ID
	}

	printing := "no printer was listed, so nothing was asked to print"
	if opts.Idle {
		printing = "the printers wait for a module to print on them"
	}
	printed := ""

	for _, printer := range f.printers {
		id, ok := bySerial[printer.Serial()]
		if !ok || opts.Idle || f.holdsDialog(printer) {
			continue
		}

		if printed == "" {
			printed, printing = id, "is printing, and every printer without a dialog keeps printing"
		}

		go f.printLoop(ctx, opts, printer, id, id == printed)
	}

	owned := f.claimPrinters(ctx, opts, page)

	f.raiseDialog(opts)

	go f.swapStick(ctx, opts)

	f.banner(opts, dir, page, printed, printing, cameras, owned)

	// After the banner, because the backfill waits for history to serve and
	// whoever reads the banner for the password should not wait with it.
	f.say(opts, f.backfill(ctx, opts, dir, page))

	return f.flap(ctx, opts)
}

// await polls one URL until it answers 200 and the answer satisfies the
// condition, or the budget runs out. It is the demo's whole readiness model:
// the core is up when it answers, and the fleet has arrived when the listing a
// person would read says so.
func (f *demoFleet) await(ctx context.Context, url string, budget time.Duration,
	holds func(demoListing) bool,
) error {
	deadline := time.Now().Add(budget)

	// What the last attempt saw, so a budget that runs out reports the answer
	// rather than the timeout. Empty until the first attempt has been made.
	var last string

	for {
		var page demoListing

		status, err := f.call(ctx, http.MethodGet, url, "", &page)

		switch {
		case err != nil:
			last = err.Error()
		case status != http.StatusOK:
			last = "status " + http.StatusText(status)
		case holds(page):
			return nil
		default:
			last = fmt.Sprintf("%d of %d printers connected", connectedIn(page), len(page.Items))
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%s in %s, and the last answer was %s", url, budget, last)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(demoPoll):
		}
	}
}

// call makes one request and decodes the answer when the caller wants it. Both
// verbs the demo uses go through here: reading the fleet, and offering a
// printer a command.
func (f *demoFleet) call(ctx context.Context, method, url, body string, into any) (int, error) {
	return f.request(ctx, method, url, body, nil, into)
}

// write is call for a request that carries headers of its own and wants no
// answer decoded. The resource API fences every write with a precondition
// header, so a write is a request shape and not just a verb.
func (f *demoFleet) write(ctx context.Context, method, url, body string, header http.Header) (int, error) {
	return f.request(ctx, method, url, body, header, nil)
}

// request is the one HTTP call the demo makes, in all its shapes.
func (f *demoFleet) request(
	ctx context.Context, method, url, body string, header http.Header, into any,
) (int, error) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, fmt.Errorf("build %s %s: %w", method, url, err)
	}

	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	if f.token != "" {
		request.Header.Set("Authorization", "Bearer "+f.token)
	}

	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	response, err := f.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, url, err)
	}

	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, fmt.Errorf("read the answer to %s %s: %w", method, url, err)
	}

	// A mint answers 201 and a read answers 200, and both carry a body the
	// caller asked for. Anything outside that range is a refusal, and its
	// body is a problem document the caller did not ask to decode.
	if into == nil || response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response.StatusCode, nil
	}

	if err = json.Unmarshal(payload, into); err != nil {
		return response.StatusCode, fmt.Errorf("read the answer to %s %s: %w", method, url, err)
	}

	return response.StatusCode, nil
}

// fleetPage is the listing as a person reading /api/v1/devices sees it.
func (f *demoFleet) fleetPage(ctx context.Context, opts DemoOptions) demoListing {
	var page demoListing

	_, _ = f.call(ctx, http.MethodGet, opts.baseURL()+"/api/v1/devices", "", &page)

	return page
}

// startPrint offers one printer the verb that starts a print, and reports what
// the server said. A demo where nothing is printing shows a fleet holding
// still, and holding still is what a filled table looks like.
func (f *demoFleet) startPrint(ctx context.Context, opts DemoOptions, id string) string {
	body := fmt.Sprintf(`{"verb":%q,"args":{"path":%q}}`, demoStartVerb, demoPrintPath)

	status, err := f.retry(ctx, func() (int, error) {
		return f.call(ctx, http.MethodPost, opts.baseURL()+"/api/v1/devices/"+id+"/commands", body, nil)
	})
	if err != nil {
		return "the print could not be started: " + err.Error()
	}

	if status != http.StatusOK {
		return fmt.Sprintf("the print was refused with status %d", status)
	}

	return "is printing " + demoPrintPath
}

// dialogFleet is the smallest fleet with a printer that neither prints nor
// flaps.
const dialogFleet = 3

// holdsDialog reports whether printer is the one raiseDialog asks a question
// on. It never prints, because a print would replace the waiting state.
func (f *demoFleet) holdsDialog(printer *simulator.Printer) bool {
	return len(f.printers) >= dialogFleet && printer == f.printers[1]
}

// raiseDialog puts a question on the display of the second printer, which is
// neither the first to print nor the one that flaps (the last), so the
// printer page's Status tab has a dialog to answer. A fleet of fewer than
// three has no such printer.
func (f *demoFleet) raiseDialog(opts DemoOptions) {
	if len(f.printers) < dialogFleet || opts.Idle {
		return
	}

	printer := f.printers[1]

	printer.RaiseDialog(simulator.Dialog{
		ID:      1,
		Code:    "10101",
		Title:   "Filament runout",
		Text:    "Load filament and continue.",
		Buttons: []string{"Continue", "Cancel"},
	})
	f.say(opts, printer.Serial()+" shows a dialog on its display: Filament runout (Continue, Cancel).")
}

// demoStickSwap is how long the dialog printer keeps its USB stick in, and then
// out, so the printer page has a printer with no drive and one that just got
// one. The simulated printer says so only when asked, which is what the
// page's own refresh is for.
const demoStickSwap = 45 * time.Second

// swapStick pulls the dialog printer's stick out and puts it back in, in turn,
// until ctx ends. It is the printer that never prints, so nothing it is doing
// loses its file. A fleet of fewer than three has no such printer.
func (f *demoFleet) swapStick(ctx context.Context, opts DemoOptions) {
	// An idle fleet is a module's to print on, and a printer with no drive
	// takes no print.
	if len(f.printers) < dialogFleet || opts.Idle {
		return
	}

	printer := f.printers[1]
	stick := simulator.Storage{Mountpoint: "/usb", Type: "USB", FreeSpace: demoStickFree}

	for in := true; demoSleep(ctx, demoStickSwap); in = !in {
		if in {
			printer.RemoveStorage(stick.Mountpoint)
			f.say(opts, printer.Serial()+" has its USB stick pulled out.")

			continue
		}

		printer.InsertStorage(stick)
		f.say(opts, printer.Serial()+" has its USB stick put back.")
	}
}

// demoStickFree is the free space the dialog printer's stick reports.
const demoStickFree = 8 << 30

// demoBetween is the idle wait before a printer's next print.
func demoBetween(opts DemoOptions, rng *rand.Rand) time.Duration {
	if opts.PrintGap > 0 {
		return max(opts.PrintGap, demoEndHeld)
	}

	return time.Minute + time.Duration(rng.Int64N(int64(demoGapSpread)))
}

// demoJobFramesOf is how many frames a print anybody else starts takes: the
// demo's five minutes, or the fast pace's shortest print when a test asked
// for it, so a module that plans the prints sees them end in seconds too.
func demoJobFramesOf(opts DemoOptions) int {
	if opts.PrintGap > 0 {
		return demoFastBase
	}

	return demoJobFrames
}

// demoFrames is a print's length in frames, one frame a second: 3 to 15
// minutes, or 10 to 20 frames when a test asked for a fast pace.
func demoFrames(opts DemoOptions, rng *rand.Rand) int {
	if opts.PrintGap > 0 {
		return demoFastBase + rng.IntN(demoFastSpread)
	}

	return demoFramesBase + rng.IntN(demoFramesSpread)
}

func (o DemoOptions) failShare() float64 {
	switch {
	case o.FailShare < 0:
		return 0
	case o.FailShare == 0:
		return demoFailShare
	default:
		return o.FailShare
	}
}

// printLoop keeps one printer busy until ctx ends. The first print starts at
// once so the fleet is printing when the banner is read.
func (f *demoFleet) printLoop(
	ctx context.Context,
	opts DemoOptions,
	printer *simulator.Printer,
	id string,
	first bool,
) {
	//nolint:gosec // the demo's pacing needs no unpredictability
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(len(id))))

	for {
		if !first && !demoSleep(ctx, demoBetween(opts, rng)) {
			return
		}

		first = false

		frames := demoFrames(opts, rng)
		printer.SetJobFrames(frames)
		f.startPrint(ctx, opts, id)

		fate := rng.Float64()
		if fate < demoCancelShare+opts.failShare() {
			cut := time.Duration(float64(frames)*(demoCutFrom+demoCutSpan*rng.Float64())) * time.Second
			if !demoSleep(ctx, cut) {
				return
			}

			switch {
			case fate < opts.failShare():
				printer.FailJob()
			case fate < opts.failShare()+demoCancelShare/2:
				printer.AbortBeforeStart()
			default:
				f.stopPrint(ctx, opts, id)
			}
		}

		for printer.Printing() {
			if !demoSleep(ctx, time.Second) {
				return
			}
		}
	}
}

// demoSleep waits d or until ctx ends, and reports whether it was the wait.
func demoSleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (f *demoFleet) stopPrint(ctx context.Context, opts DemoOptions, id string) {
	body := fmt.Sprintf(`{"verb":%q,"args":{}}`, demoStopVerb)
	_, _ = f.call(ctx, http.MethodPost, opts.baseURL()+"/api/v1/devices/"+id+"/commands", body, nil)
}

// flap takes one printer off the wire and puts it back, for as long as the demo
// runs. It is the fault a fleet actually has: a machine switched off, a cable
// pulled, a switch rebooted. Zero leaves the whole fleet connected.
func (f *demoFleet) flap(ctx context.Context, opts DemoOptions) error {
	if opts.Flap <= 0 {
		<-ctx.Done()

		return nil
	}

	away := f.printers[len(f.printers)-1]

	stretch := demoStretch(opts.Flap)

	ticker := time.NewTicker(opts.Flap)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		away.GoOffline()
		f.say(opts, away.Serial()+" went away. The listing reports it disconnected.")

		select {
		case <-ctx.Done():
			away.GoOnline()

			return nil
		case <-time.After(stretch):
		}

		away.GoOnline()
		f.say(opts, away.Serial()+" came back.")
	}
}

// banner is what the person reads. It says the addresses out loud, because a
// page named without its server running is a page nobody can open.
func (f *demoFleet) banner(
	opts DemoOptions, dir string, page demoListing, printed, printing string,
	cameras []string, owned string,
) {
	base := opts.baseURL()

	lines := []string{
		"",
		fmt.Sprintf("%d simulated printers are connected to the core on %s.", len(page.Items), opts.APIAddr),
		"",
	}

	for _, item := range page.Items {
		note := "is idle"
		if item.ID == printed {
			note = printing
		}

		lines = append(lines, fmt.Sprintf("  %-10s %s  %s", item.Serial, item.ID, note))
	}

	if opts.Flap > 0 {
		lines = append(lines, "",
			fmt.Sprintf("  %s goes away for %s every %s, and comes back.",
				f.printers[len(f.printers)-1].Serial(), demoStretch(opts.Flap), opts.Flap))
	}

	if len(cameras) > 0 {
		lines = append(lines, "", "Each printer has a simulated camera. The live view of each:", "")
		lines = append(lines, cameras...)
	}

	lines = append(lines,
		"",
		"Sign in as the demo account. Its password is new on every run:",
		"",
		"  username  "+f.login.username,
		"  password  "+f.login.password,
		"  "+owned,
		"",
		"Open these while this command keeps running:",
		"",
		"  "+base+"/docs               the API reference",
		"  "+base+"/dev                the fleet, live, once this browser holds a session",
		"  "+base+"/api/v1/devices     the same fleet as JSON, with a token",
		"",
		"A browser gets its session from the console of the /docs page:",
		"",
		"  "+f.browserLogin(),
		"",
		"Read it from another terminal. Log in, mint a token, and send it:",
		"",
		"  "+f.curlLogin(base),
		"  "+f.curlMint(base),
		`  curl -H "Authorization: Bearer $TOKEN" `+base+"/api/v1/devices",
		"",
		f.configured(),
		f.offering(),
		"It writes the result and its database to",
		"  "+dir,
		keptOrRemoved(opts),
		"",
		"The server's own log is quiet here. Add -log-level info to read it.",
		"",
		"Ctrl+C stops the printers and the server.",
		"")

	f.say(opts, lines...)
}

// jsonHeader is the header every request below sends, as curl spells it.
const jsonHeader = "-H 'content-type: application/json'"

// browserLogin is the one line a browser runs on a page this server serves,
// which sets the session cookie on this origin. The developer view refuses a
// browser with no session, and this line gives the browser one.
func (f *demoFleet) browserLogin() string {
	return `fetch("/api/v1/auth/login", {method: "POST", headers: {"content-type": "application/json"}, ` +
		"body: '" + f.login.body() + "'})"
}

// curlLogin is the login as a terminal makes it, with the cookie kept.
func (f *demoFleet) curlLogin(base string) string {
	return "curl -s -c cookies.txt -X POST " + base + "/api/v1/auth/login " + jsonHeader +
		" -d '" + f.login.body() + "'"
}

// curlMint mints a token over the session that login kept.
func (f *demoFleet) curlMint(base string) string {
	return "curl -s -b cookies.txt -X POST " + base + "/api/v1/auth/credentials " + jsonHeader +
		` -d '{"name":"terminal","scopes":["administer"]}'`
}

// configured says which file this run started from, because a module declared
// there and not starting is the first thing a person would otherwise have to
// guess about.
func (f *demoFleet) configured() string {
	if f.operatorFile == "" {
		return "There is no configuration file, so the demo runs on its defaults."
	}

	return "The demo read " + f.operatorFile + " and laid its own settings over it."
}

// offering says what the extensions page has to install. A demo run from a
// checkout where no module was built offers nothing, and the line says which command
// changes that rather than leaving an empty page unexplained.
func (f *demoFleet) offering() string {
	if f.offered == 0 {
		return "No module is built, so the extensions page offers none. " +
			"Run `make demo-modules` and start the demo again."
	}

	return fmt.Sprintf(
		"%d modules are on the extensions page. They install off this machine, with no network.",
		f.offered)
}

// say writes to wherever the caller asked for the demo's own output. It is not
// the core's logger: the logger's level is the operator's to set, and these
// lines are the point of the command.
func (f *demoFleet) say(opts DemoOptions, lines ...string) {
	_, _ = io.WriteString(opts.Out, strings.Join(lines, "\n")+"\n")
}

// keptOrRemoved says what happens to the working directory, because the answer
// is different for a directory the caller named and one this run made.
func keptOrRemoved(opts DemoOptions) string {
	if opts.Dir != "" {
		return "and keeps that directory, because -dir named it."
	}

	return "and removes that directory when it stops."
}

// demoAwayShare is the largest share of one period the printer may be away for,
// written as a divisor. Half.
const demoAwayShare = 2

// demoStretch is how long the flapping printer stays away, held under half the
// period. A stretch longer than the period leaves the printer off the wire from
// the first tick onwards, which reads as a broken demo and not as a printer
// coming back.
func demoStretch(period time.Duration) time.Duration {
	return min(demoAway, period/demoAwayShare)
}

// connectedIn counts the printers the server holds a session for.
func connectedIn(page demoListing) int {
	var total int

	for _, item := range page.Items {
		if item.Connected {
			total++
		}
	}

	return total
}

// retry repeats one call on the demo's own core until it answers or the settle
// budget ends. Under load the core can take longer than one request budget to
// answer, and a call made once and given up on leaves the demo without its
// camera or its print, and a test waiting for a listing that never fills. A
// refusal with a status below 500 is an answer and is not repeated.
func (f *demoFleet) retry(ctx context.Context, do func() (int, error)) (int, error) {
	deadline := time.Now().Add(demoSettleBudget)

	for {
		status, err := do()

		transient := err != nil || status >= http.StatusInternalServerError
		if !transient || time.Now().After(deadline) {
			return status, err
		}

		select {
		case <-ctx.Done():
			return status, err
		case <-time.After(demoRetryPause):
		}
	}
}
