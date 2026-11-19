package app

import (
	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// statesRequests reports whether the row states any request of the release.
func (row moduleIndexRow) statesRequests() bool {
	return row.Permissions != nil || row.Checks != nil || row.Events != nil || row.CheckAbsentPeople != nil ||
		row.ListPeople != nil || row.Calls != nil || row.PublicRoutes != nil ||
		row.StagedRoutes != nil || row.RequestHeaders != nil || row.RecordsMiB != nil || row.MemoryMiB != nil ||
		row.Listen != nil || row.Data != nil || row.Database != nil || row.AllowedOrigins != nil
}

// calls is the modules the row says the release calls, and nil for none.
func (row moduleIndexRow) calls() []string {
	if row.Calls == nil {
		return nil
	}

	return *row.Calls
}

// requestsOf fills an offer's requests from what the index row states, or
// from the manifest of the release directory the row points at. A row that
// states none and points at an archive or an address keeps them unknown,
// because reading them would unpack or download a release on every listing.
//
// Only a release directory's manifest can say the release runs no process.
// Any other row is taken to run one, so its budget is still counted.
func requestsOf(offer *httpapi.ModuleOffer, row moduleIndexRow) {
	declared, read := localManifest(row.Source)
	offer.Process = !read || declared.ContractMajor > 0

	if read {
		offer.License = declared.License
		offer.Egress, offer.EgressKnown = egressOf(declared.Egress), true
	}

	if row.statesRequests() {
		row.fill(offer)

		return
	}

	if read {
		withAsks(offer, asksOf(declared))
	}
}

func withAsks(offer *httpapi.ModuleOffer, asked install.ModuleAsks) {
	offer.RequestsKnown = true
	offer.Permissions = asked.Permissions
	offer.Checks = asked.Checks
	offer.Events = asked.Events
	offer.CheckAbsentPeople = asked.CheckAbsentPeople
	offer.ListPeople = asked.ListPeople
	offer.Calls = asked.Calls
	offer.PublicRoutes = asked.PublicRoutes
	offer.StagedRoutes = resource.StagedRoutesOf(asked.StagedRoutes)
	offer.RequestHeaders = resource.RequestHeadersOf(asked.RequestHeaders)
	offer.RecordsMiB = asked.RecordsMiB
	offer.MemoryMiB = asked.MemoryMiB
	offer.Listen = resource.RequestsOf(asked).Listen
	offer.Data = asked.Data
	offer.Database = asked.Database
	offer.AllowedOrigins = asked.AllowedOrigins
}

// fill copies what the index row states into an offer. A request the row does
// not state stays as it is.
func (row moduleIndexRow) fill(offer *httpapi.ModuleOffer) {
	offer.RequestsKnown = true

	if row.Permissions != nil {
		offer.Permissions = *row.Permissions
	}

	if row.Checks != nil {
		offer.Checks = *row.Checks
	}

	if row.Events != nil {
		offer.Events = *row.Events
	}

	if row.CheckAbsentPeople != nil {
		offer.CheckAbsentPeople = *row.CheckAbsentPeople
	}

	if row.ListPeople != nil {
		offer.ListPeople = *row.ListPeople
	}

	if row.RecordsMiB != nil {
		offer.RecordsMiB = *row.RecordsMiB
	}

	if row.MemoryMiB != nil {
		offer.MemoryMiB = *row.MemoryMiB
	}

	if row.Listen != nil {
		offer.Listen = resource.RequestsOf(install.ModuleAsks{Listen: *row.Listen}).Listen
	}

	if row.Data != nil {
		offer.Data = *row.Data
	}

	if row.Database != nil {
		offer.Database = *row.Database
	}

	if row.AllowedOrigins != nil {
		offer.AllowedOrigins = *row.AllowedOrigins
	}

	offer.Calls = row.calls()

	if row.PublicRoutes != nil {
		offer.PublicRoutes = *row.PublicRoutes
	}

	if row.StagedRoutes != nil {
		offer.StagedRoutes = resource.StagedRoutesOf(*row.StagedRoutes)
	}

	if row.RequestHeaders != nil {
		offer.RequestHeaders = resource.RequestHeadersOf(shownRequestHeaders(*row.RequestHeaders))
	}
}
