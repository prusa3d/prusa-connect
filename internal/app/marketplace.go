package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// The module catalogue a marketplace serves. The index merges its rows after
// the local ones. An install with no address takes the release and its digest
// from the catalogue, so the bytes this server installs are the bytes the
// marketplace reviewed.

// The marketplace's paths, and the page a person browses its extensions on.
// The catalogue path names the version of the marketplace API this server
// reads, so moving to another version is this one constant.
const (
	marketplaceExtensionsPath = "/api/connect/v1/extensions"
	marketplaceBrowsePath     = "/connect"
)

// How long a read of the catalogue is kept before the next request refreshes
// it, and how much of one answer this server reads. The catalogue is a list of
// rows; anything claiming to be larger is refused rather than held in memory.
const (
	marketplaceTTL      = 5 * time.Minute
	maxMarketplaceBytes = 4 << 20
	// marketplaceRetry is how long a marketplace that never answered is left
	// alone before a request tries it again. Without it every visit to the
	// settings screen would wait the whole timeout for a marketplace that is
	// down.
	marketplaceRetry = 30 * time.Second
)

// The origin a row's offer carries.
const (
	originLocal       = "local"
	originMarketplace = "marketplace"
)

// errMarketplaceAnswer is a marketplace that answered with something other
// than its catalogue.
var errMarketplaceAnswer = errors.New("the marketplace answered with something other than its catalogue")

// marketplaceAuthor is who publishes a module on the marketplace.
type marketplaceAuthor struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Trust string `json:"trust"`
}

// The channels a release is published on. A release that names none is
// stable, so a marketplace that has no channels publishes stable releases.
const (
	channelStable = "stable"
	channelBeta   = "beta"
)

// marketplaceRelease is one published version of a module. The request keys
// are what the release's manifest asks for, as the marketplace reviewed it.
// An approved install is refused when the downloaded manifest asks for more.
type marketplaceRelease struct {
	Version           string                  `json:"version"`
	PublishedAt       string                  `json:"published_at"`
	Channel           string                  `json:"channel"`
	Trust             string                  `json:"trust"`
	Core              string                  `json:"core"`
	Kind              string                  `json:"kind"`
	Platforms         []string                `json:"platforms"`
	Permissions       []string                `json:"permissions"`
	Checks            []string                `json:"checks"`
	RecordsMiB        int                     `json:"records_mib"`
	MemoryMiB         int                     `json:"memory_mib"`
	Events            map[string]string       `json:"events"`
	CheckAbsentPeople bool                    `json:"check_absent_people"`
	ListPeople        bool                    `json:"list_people"`
	Calls             []string                `json:"calls"`
	PublicRoutes      []string                `json:"public_routes"`
	StagedRoutes      []install.StagedRoute   `json:"staged_routes"`
	RequestHeaders    []install.RequestHeader `json:"request_headers"`
	Listen            []install.ListenAsk     `json:"listen"`
	Data              bool                    `json:"data"`
	Database          bool                    `json:"database"`
	HasUI             bool                    `json:"has_ui"`
	Size              int64                   `json:"size"`
	Sha256            string                  `json:"sha256"`
	DownloadURL       string                  `json:"download_url"`
	Changelog         string                  `json:"changelog"`
	AllowedOrigins    []string                `json:"allowed_origins"`
	// The release manifest's require, suggest and conflict, in Composer's
	// shape, as the marketplace read them.
	Require  map[string]string `json:"require"`
	Suggest  map[string]string `json:"suggest"`
	Conflict map[string]string `json:"conflict"`
	// FileTypes and ViewTypes are what the release's manifest claims, for
	// the plan's conflict check.
	FileTypes []string `json:"file_types"`
	ViewTypes []string `json:"view_types"`
	// License and Egress are what a person reads before installing. A release
	// published before manifests declared them carries neither, and Egress is
	// then nil rather than empty.
	License string               `json:"license"`
	Egress  *[]marketplaceEgress `json:"egress"`
}

// maxEgressTarget bounds a host or an option name the marketplace states. A
// DNS name is at most 253 characters.
const maxEgressTarget = 253

// shownText reports whether text the marketplace states may be shown to a
// person: no longer than limit characters and with no control or invisible
// character, the rules a manifest is held to. What breaks them is not shown,
// so a catalogue cannot hide or reorder what the install dialog reads.
func shownText(text string, limit int) bool {
	return utf8.RuneCountInString(text) <= limit && manifest.PlainText(text)
}

// marketplaceEgress is one connection outside Prusa Connect, as the
// marketplace states a release's egress.
type marketplaceEgress struct {
	Host   string `json:"host"`
	Option string `json:"option"`
	Ports  []int  `json:"ports"`
	From   string `json:"from"`
	Why    string `json:"why"`
	Sends  string `json:"sends"`
}

// egress is the release's egress as the API states it, and false when the
// marketplace says nothing about it. An entry that names neither or both of
// host and option is dropped and the whole list is unknown, so a page never
// tells a person a release connects nowhere when it could not read it.
func (r marketplaceRelease) egress() ([]httpapi.ModuleEgress, bool) {
	if r.Egress == nil {
		return nil, false
	}

	out := make([]httpapi.ModuleEgress, 0, len(*r.Egress))

	for _, one := range *r.Egress {
		host, option := strings.TrimSpace(one.Host), strings.TrimSpace(one.Option)
		why, sends := strings.TrimSpace(one.Why), strings.TrimSpace(one.Sends)

		if (host == "") == (option == "") || why == "" || sends == "" ||
			!shownText(host, maxEgressTarget) || !shownText(option, maxEgressTarget) ||
			!shownText(why, manifest.MaxEgressWhy) || !shownText(sends, manifest.MaxEgressSends) {
			return nil, false
		}

		ports := one.Ports
		if len(ports) == 0 {
			ports = []int{443}
		}

		for _, port := range ports {
			if port < 1 || port > manifest.MaxPort {
				return nil, false
			}
		}

		from := one.From
		if from != string(manifest.EgressFromUI) {
			from = string(manifest.EgressFromProcess)
		}

		out = append(out, httpapi.ModuleEgress{
			Host: host, Option: option, Ports: ports, From: from,
			Why: why, Sends: sends,
		})
	}

	return out, true
}

// channel is the release's channel, stable when it names none.
func (r marketplaceRelease) channel() string {
	if r.Channel == channelBeta {
		return channelBeta
	}

	return channelStable
}

// requestHeaders is each request header the marketplace lists for the release
// that a manifest may ask for, with a reason that may be shown. One that breaks
// either rule is dropped, so an approved install of a manifest that asks for it
// is refused instead of approved from a line that could hide what it asks.
func (r marketplaceRelease) requestHeaders() []install.RequestHeader {
	return shownRequestHeaders(r.RequestHeaders)
}

// shownRequestHeaders is each header of a list an index states that a manifest
// may ask for, with a reason that may be shown, under its canonical name so it
// compares with the manifest's. A local index row is held to it as well.
func shownRequestHeaders(headers []install.RequestHeader) []install.RequestHeader {
	var out []install.RequestHeader

	for _, header := range headers {
		if manifest.CheckRequestHeader(header.Name) != nil || header.Reason == "" ||
			!shownText(header.Reason, manifest.MaxHeaderReason) {
			continue
		}

		out = append(out, install.RequestHeader{
			Name: http.CanonicalHeaderKey(header.Name), Reason: header.Reason, Required: header.Required,
		})
	}

	return out
}

// asks is what the marketplace says the release's manifest asks for.
func (r marketplaceRelease) asks() install.ModuleAsks {
	return install.ModuleAsks{
		Permissions:       slices.Clone(r.Permissions),
		Checks:            slices.Clone(r.Checks),
		Events:            maps.Clone(r.Events),
		CheckAbsentPeople: r.CheckAbsentPeople,
		ListPeople:        r.ListPeople,
		Calls:             slices.Clone(r.Calls),
		PublicRoutes:      slices.Clone(r.PublicRoutes),
		StagedRoutes:      slices.Clone(r.StagedRoutes),
		RequestHeaders:    r.requestHeaders(),
		RecordsMiB:        r.RecordsMiB,
		MemoryMiB:         r.MemoryMiB,
		Listen:            slices.Clone(r.Listen),
		Data:              r.Data,
		Database:          r.Database,
		AllowedOrigins:    slices.Clone(r.AllowedOrigins),
	}
}

// marketplaceModule is one row of the catalogue.
type marketplaceModule struct {
	ID       string               `json:"id"`
	Module   string               `json:"module"`
	Name     string               `json:"name"`
	Title    string               `json:"title"`
	Summary  string               `json:"summary"`
	Author   marketplaceAuthor    `json:"author"`
	Trust    string               `json:"trust"`
	PageURL  string               `json:"page_url"`
	Latest   *marketplaceRelease  `json:"latest"`
	Versions []marketplaceRelease `json:"versions"`
}

// marketplaceCatalogue reads one marketplace and keeps what it read.
//
// The first request waits for the first read. After that a request is
// answered from the copy, and a copy older than the TTL starts one refresh in
// the background, so a slow marketplace never makes the settings screen slow.
// A refresh that fails keeps the copy it had and records why.
type marketplaceCatalogue struct {
	base     string
	timeout  time.Duration
	client   *http.Client
	log      *slog.Logger
	now      func() time.Time
	core     string
	platform string
	portal   portalAuthorization

	mu         sync.Mutex
	rows       []marketplaceModule
	readAt     time.Time
	read       bool
	problem    string
	refreshing bool
	// failedAt is when the last read failed, and zero after one succeeded.
	failedAt time.Time
}

// marketplaceCore is the version the catalogue is asked for: this build's
// whole version, its prerelease included, or empty for a build whose version
// is no semantic version, such as one stamped with a commit. The marketplace
// holds a release's core range to it by the rule the install uses, so a
// tester on 0.0.1-alpha.6 is not offered a release that needs
// >=0.0.1-alpha.7. The build metadata is left out, because it takes no part
// in the comparison. The marketplace refuses a core it cannot read, so a build
// with no semantic version asks for the whole catalogue instead, the way it
// already skips the range check on install.
func marketplaceCore(running string) string {
	held, err := manifest.ParseVersion(running)
	if err != nil {
		return ""
	}

	held.Build = ""

	return held.String()
}

// newMarketplaceCatalogue is nil for a deployment that reads no marketplace.
func newMarketplaceCatalogue(
	m config.Marketplace, log *slog.Logger, authorization ...portalAuthorization,
) *marketplaceCatalogue {
	if !m.Configured() {
		return nil
	}

	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	catalogue := &marketplaceCatalogue{
		base:     m.Base(),
		timeout:  m.Timeout,
		client:   &http.Client{Timeout: m.Timeout, CheckRedirect: checkRemoteRedirect},
		log:      log,
		now:      time.Now,
		core:     marketplaceCore(coreVersion()),
		platform: runtime.GOOS + "_" + runtime.GOARCH,
	}
	if len(authorization) > 0 {
		catalogue.portal = authorization[0]
		catalogue.client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
			if err := checkRemoteRedirect(next, via); err != nil {
				return err
			}
			// Only a request that started signed is signed again: another
			// marketplace must not spend this server's pairing by redirecting.
			if len(via) == 0 || via[0].Header.Get("Authorization") == "" {
				next.Header.Del("Authorization")
				next.Header.Del("DPoP")

				return nil
			}

			return catalogue.portal.authorize(next)
		}
	}

	return catalogue
}

// status is how the last read went, for the index's marketplace object.
func (c *marketplaceCatalogue) status() *httpapi.ModuleMarketplace {
	c.mu.Lock()
	defer c.mu.Unlock()

	// A copy kept after a failed refresh is still listed, and it is stale:
	// the marketplace did not answer the last time it was asked.
	out := &httpapi.ModuleMarketplace{
		URL:       c.base,
		BrowseURL: c.base + marketplaceBrowsePath,
		Reachable: c.read && c.failedAt.IsZero(),
		Stale:     c.read && !c.failedAt.IsZero(),
		Problem:   c.problem,
	}

	if c.read {
		out.ReadAt = c.readAt.UTC().Format(time.RFC3339)
	}

	return out
}

// modules answers the rows of the catalogue whose newest release runs on this
// server, reading it first when nothing was ever read and starting a refresh
// when the copy is older than the TTL.
//
// Only one request waits for a first read. Another one that arrives meanwhile,
// or within marketplaceRetry of a first read that failed, answers no rows
// rather than waiting the timeout again.
func (c *marketplaceCatalogue) modules(ctx context.Context) []marketplaceModule {
	c.mu.Lock()
	now := c.now()
	read, stale := c.read, now.Sub(c.readAt) >= marketplaceTTL

	if read && stale && !c.refreshing {
		c.refreshing = true

		// The request that noticed is not the one that waits for it, so the
		// refresh outlives it and is bounded by its own timeout instead.
		go c.refresh(context.WithoutCancel(ctx))
	}

	waits := !read && !c.refreshing && (c.failedAt.IsZero() || now.Sub(c.failedAt) >= marketplaceRetry)
	if waits {
		c.refreshing = true
	}

	rows := c.rows
	c.mu.Unlock()

	if waits {
		c.refresh(ctx)

		c.mu.Lock()
		rows = c.rows
		c.mu.Unlock()
	}

	return c.runningHere(rows)
}

// runningHere is the rows whose newest release this server can run. The
// marketplace is asked for this platform and this core only; this holds a
// marketplace that answered with more than it was asked for to the same rule.
// A marketplace that compares a prerelease core by its three numbers alone is
// one such marketplace.
func (c *marketplaceCatalogue) runningHere(rows []marketplaceModule) []marketplaceModule {
	out := make([]marketplaceModule, 0, len(rows))

	for _, row := range rows {
		if row.Latest != nil && c.runsHere(*row.Latest) && c.fitsCore(*row.Latest) {
			out = append(out, row)
		}
	}

	return out
}

// refresh reads the catalogue once and keeps it, or keeps the old copy and the
// reason the read failed.
func (c *marketplaceCatalogue) refresh(ctx context.Context) {
	rows, err := c.list(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.refreshing = false

	if err != nil {
		c.problem = err.Error()
		c.failedAt = c.now()
		c.log.WarnContext(ctx, "the marketplace catalogue could not be read",
			"marketplace", c.base, "error", err.Error())

		return
	}

	c.rows, c.readAt, c.read, c.problem, c.failedAt = rows, c.now(), true, "", time.Time{}
}

// release is the one version of one module the marketplace offers this
// server, read fresh, because an install must not be decided on a copy a few
// minutes old.
func (c *marketplaceCatalogue) release(ctx context.Context, module, version string) (marketplaceRelease, error) {
	namespace, err := extension.NamespaceOf(module)
	if err != nil {
		return marketplaceRelease{}, httpfault.RequestInvalid("body.module", "the module's path", err.Error())
	}

	var document struct {
		Extension marketplaceModule `json:"extension"`
	}

	path := marketplaceExtensionsPath + "/" + url.PathEscape(string(namespace)) + "?" +
		url.Values{"platform": {c.platform}}.Encode()

	if err = c.get(ctx, path, &document); err != nil {
		if errors.Is(err, errMarketplaceNotFound) {
			return marketplaceRelease{}, httpfault.ModuleNotOffered(module, version)
		}

		return marketplaceRelease{}, httpfault.MarketplaceUnreachable(c.base, err.Error())
	}

	if document.Extension.Module != module {
		return marketplaceRelease{}, httpfault.ModuleNotOffered(module, version)
	}

	for _, offered := range document.Extension.Versions {
		if offered.Version == version && c.runsHere(offered) {
			return offered, nil
		}
	}

	return marketplaceRelease{}, httpfault.ModuleNotOffered(module, version)
}

// runsHere reports a release built for this server's platform. A release
// with no platform is a bundle and nothing else, which runs on every one. The
// core range is checked again by the install once the manifest is read.
func (c *marketplaceCatalogue) runsHere(release marketplaceRelease) bool {
	return len(release.Platforms) == 0 || slices.Contains(release.Platforms, c.platform)
}

// fitsCore reports whether this core is in the release's core range, by the
// rule the install holds the manifest to (manifest.Range.HoldsCore), so the
// index never offers a release the install then refuses. A release that names
// no range fits, and so does every release on a build with no semantic
// version, which skips the check on install too. A range this server cannot
// read does not fit, because the install refuses it.
func (c *marketplaceCatalogue) fitsCore(release marketplaceRelease) bool {
	if c.core == "" || strings.TrimSpace(release.Core) == "" {
		return true
	}

	in, readable := install.CoreHolds(release.Core, c.core)

	return !readable || in
}

// errMarketplaceNotFound is a marketplace that has no such module.
var errMarketplaceNotFound = errors.New("the marketplace has no such module")

// get reads one JSON document from the marketplace, bounded in time and size.
func (c *marketplaceCatalogue) get(ctx context.Context, path string, into any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, err := c.fetch(ctx, c.base+path, maxMarketplaceBytes, into)

	return err
}

// fetch reads one answer of the marketplace at an absolute address into
// `into`, held to `budget` bytes, and answers how many it read. The caller
// owns the deadline, so a walk over several pages spends one budget.
func (c *marketplaceCatalogue) fetch(ctx context.Context, address string, budget int, into any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, fmt.Errorf("address the marketplace: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	if err = c.portal.authorize(request); err != nil {
		return 0, fmt.Errorf("authorize the marketplace request: %w", err)
	}

	response, err := c.portal.send(c.client, request)
	if err != nil {
		if unreachable := reachFailure(err); unreachable != nil {
			return 0, unreachable
		}

		return 0, fmt.Errorf("reach the marketplace: %w", err)
	}

	defer func() { _ = response.Body.Close() }()

	switch {
	case response.StatusCode == http.StatusNotFound:
		return 0, errMarketplaceNotFound
	case response.StatusCode != http.StatusOK:
		return 0, fmt.Errorf("%w: it answered %d", errMarketplaceAnswer, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, int64(budget)+1))
	if err != nil {
		return 0, fmt.Errorf("read the marketplace's answer: %w", err)
	}

	if len(body) > budget {
		return 0, fmt.Errorf("%w: it is larger than %d bytes", errMarketplaceAnswer, budget)
	}

	if err = json.Unmarshal(body, into); err != nil {
		return 0, fmt.Errorf("%w: %w", errMarketplaceAnswer, err)
	}

	return len(body), nil
}

// offerOf is one marketplace row as the index lists it, and false for a row
// this server cannot install: no module path a namespace derives from, no
// release offered here, or a release with no address or no digest. An install
// with no from takes both from the marketplace and refuses a release without
// them, so a row offering one would be a button that always fails.
//
// The page link is kept only when it is https, or on the configured
// marketplace's own loopback origin, because a page opens it in a browser.
func (c *marketplaceCatalogue) offerOf(row marketplaceModule) (httpapi.ModuleOffer, bool) {
	if strings.TrimSpace(row.Module) == "" || row.Latest == nil ||
		row.Latest.DownloadURL == "" || row.Latest.Sha256 == "" {
		return httpapi.ModuleOffer{}, false
	}

	if _, err := extension.NamespaceOf(row.Module); err != nil {
		return httpapi.ModuleOffer{}, false
	}

	offer := httpapi.ModuleOffer{
		Module:      row.Module,
		Name:        row.Name,
		Title:       row.Title,
		Description: row.Summary,
		Version:     row.Latest.Version,
		Source:      row.Latest.DownloadURL,
		Author:      row.Author.Name,
		Kind:        row.Latest.Kind,
		Origin:      originMarketplace,
		Sha256:      row.Latest.Sha256,
		Trust:       row.Trust,
		PageURL:     c.pageURL(row.PageURL),
		Permissions: row.Latest.Permissions,
		Platforms:   row.Latest.Platforms,
		Channel:     row.Latest.channel(),
		Events:      row.Latest.Events,
		RecordsMiB:  row.Latest.RecordsMiB,
		MemoryMiB:   row.Latest.MemoryMiB,
		Process:     len(row.Latest.Platforms) > 0, // a release with no platform ships no binary (runsHere)
		// The marketplace states every request of the release, so a page can
		// show exactly what an install with approve grants.
		Checks:            row.Latest.Checks,
		CheckAbsentPeople: row.Latest.CheckAbsentPeople,
		ListPeople:        row.Latest.ListPeople,
		Calls:             row.Latest.Calls,
		AllowedOrigins:    row.Latest.AllowedOrigins,
		PublicRoutes:      row.Latest.PublicRoutes,
		StagedRoutes:      resource.StagedRoutesOf(row.Latest.StagedRoutes),
		RequestHeaders:    resource.RequestHeadersOf(row.Latest.requestHeaders()),
		Listen:            resource.RequestsOf(row.Latest.asks()).Listen,
		Data:              row.Latest.Data,
		Database:          row.Latest.Database,
		RequestsKnown:     true,
	}

	withComposer(&offer, row.Latest.Require, row.Latest.Suggest, row.Latest.Conflict)
	if row.Latest.Core != "" && offer.Requires == nil {
		offer.Requires = append(offer.Requires, httpapi.ResourceRequirement{Core: row.Latest.Core})
	}
	offer.FileTypes, offer.ViewTypes = row.Latest.FileTypes, row.Latest.ViewTypes
	if license := strings.TrimSpace(row.Latest.License); shownText(license, manifest.MaxLicense) {
		offer.License = license
	}
	offer.Egress, offer.EgressKnown = row.Latest.egress()

	return offer, true
}

// pageURL is a page address the index may hand a browser, and empty for one
// it may not.
func (c *marketplaceCatalogue) pageURL(raw string) string {
	page, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || page.User != nil || page.Hostname() == "" {
		return ""
	}

	if page.Scheme == "https" {
		return page.String()
	}

	base, err := url.Parse(c.base)
	if err == nil && page.Scheme == "http" && base.Scheme == "http" &&
		config.IsLoopbackHost(base.Hostname()) && strings.EqualFold(page.Host, base.Host) {
		return page.String()
	}

	return ""
}

// mergeOffers is the local rows, then each marketplace row whose module no
// local row names. The operator's own file wins: it is what they put there.
func (c *marketplaceCatalogue) mergeOffers(
	local []httpapi.ModuleOffer, remote []marketplaceModule,
) []httpapi.ModuleOffer {
	out := make([]httpapi.ModuleOffer, 0, len(local)+len(remote))
	named := make(map[string]bool, len(local)+len(remote))

	for _, offer := range local {
		offer.Origin = originLocal
		out = append(out, offer)
		named[offer.Module] = true
	}

	for _, row := range remote {
		offer, installable := c.offerOf(row)
		if !installable || named[offer.Module] {
			continue
		}

		out = append(out, offer)
		named[offer.Module] = true
	}

	return out
}

// betaOverStable reports a marketplace row offering a beta as the update of a
// module installed from the stable channel. The marketplace offers the newest
// stable release on a row. A row like that is left out, because the operator
// never chose the beta channel for this module.
func betaOverStable(offer httpapi.ModuleOffer, declared resource.Declaration) bool {
	if offer.Origin != originMarketplace || offer.Channel != channelBeta {
		return false
	}

	return declared.Spec.Module == nil || declared.Spec.Module.Channel != channelBeta
}
