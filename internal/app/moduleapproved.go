package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
)

// recordsRequest is the request line of a manifest that asks for more room for
// its records, in the words requests and approved both use.
func recordsRequest(mib int) string {
	return fmt.Sprintf("records %d MiB", mib)
}

// grantedRecords is the room an install grants the module's records: what the
// operator named, or what the manifest asked for when they named nothing. A
// room named without approve is refused, because nothing is granted then and
// the operator would read their number as given.
func grantedRecords(request moduleRequest, asked int) (int, error) {
	switch {
	case request.recordsMiB == 0:
		return asked, nil
	case !request.approve:
		return 0, fmt.Errorf("%w: records_mib is granted only with approve", ErrUsage)
	case request.recordsMiB < 0 || request.recordsMiB > install.MaxRecordsMiB:
		return 0, fmt.Errorf("%w: records_mib is %d, expected 1 to %d", ErrUsage,
			request.recordsMiB, install.MaxRecordsMiB)
	}

	return request.recordsMiB, nil
}

// recordsGranted is what `module add -approve` prints for the room it gave,
// against the default the namespace would otherwise hold.
func recordsGranted(mib int) string {
	return fmt.Sprintf("%d MiB, where the default is %d MiB (records_mib)",
		mib, stateapi.DefaultNamespaceBytes/stateapi.NamespaceMiB)
}

// approvedOf splits what an approved install gave from what it did not. A
// permission is given only when a grant for it was written into the
// authorization graph. One the organization cannot confer, and one this install
// had no graph to write into, is recorded in the ceiling with no grant behind
// it, so the module cannot use it and it is withheld.
//
// check_absent_people, list_people, the routes, the events and the browser
// origins are given by the declaration itself, which an approved install
// writes whatever happens to
// the grants.
func approvedOf(declared manifest.Manifest, namespace string, written []authz.Action) ([]string, []string) {
	var given, withheld []string

	for _, action := range declared.Permissions {
		if !slices.Contains(written, authz.Action(strings.TrimSpace(action))) {
			withheld = append(withheld, "permission "+action)

			continue
		}

		given = append(given, "permission "+action)
	}

	for _, action := range declared.Checks {
		given = append(given, checkRequest(action))
	}

	if declared.CheckAbsentPeople {
		given = append(given, "permission "+checkAbsentPeople)
	}

	if declared.ListPeople {
		given = append(given, "permission "+listPeople)
	}

	for _, namespace := range declared.Calls {
		given = append(given, callRequest(namespace))
	}

	for _, route := range declared.Routes.Public {
		given = append(given, publicRequest(route.String()))
	}

	for _, route := range install.StagedRoutesOf(declared.Routes.Staged) {
		given = append(given, stagedRequest(route))
	}

	for _, header := range declared.Routes.Headers {
		given = append(given, headerRequest(install.RequestHeader(header)))
	}

	for _, origin := range declared.AllowedOrigins {
		given = append(given, originRequest(origin))
	}

	if declared.RecordsMiB > 0 {
		given = append(given, recordsRequest(declared.RecordsMiB))
	}

	for endpoint, name := range declared.Events {
		given = append(given, "event "+endpoint+" as "+namespace+"."+name)
	}

	for _, listen := range declared.Needs.Listen {
		given = append(given, listen.Request())
	}

	if declared.Needs.Data {
		given = append(given, dataRequest)
	}

	if declared.Needs.Database {
		given = append(given, databaseRequest)
	}

	slices.Sort(given)

	return given, withheld
}
