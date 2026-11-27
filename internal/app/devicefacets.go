package app

// What a listing tells a client about how it may be narrowed and ordered: the
// values present in the caller's own fleet with the count each would list, and
// the sort fields on offer. It is computed in the walk the listing already
// makes, so describing the fleet costs no statement the page did not.

import (
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// The order a toolbar draws the filters in.
const (
	filterSearch   = "search"
	filterState    = "state"
	filterModel    = "model"
	filterSeverity = "severity"
	filterTag      = "tag"
)

// listingSorts is every field the fleet may be ordered by, with the direction
// a person reading it expects first. A last-seen order reads newest first.
//
//nolint:gochecknoglobals // a read-only table.
var listingSorts = []httpapi.DeviceSort{
	{Field: sortByUrgency, DefaultOrder: orderAscending},
	{Field: sortByName, DefaultOrder: orderAscending},
	{Field: sortByModel, DefaultOrder: orderAscending},
	{Field: sortByState, DefaultOrder: orderAscending},
	{Field: sortBySeen, DefaultOrder: orderDescending},
	{Field: sortByID, DefaultOrder: orderAscending},
}

// tally counts one dimension's values by a folded key and keeps the first
// spelling a device carried, so "MK4S" and "mk4s" are one chip spelled as a
// machine spells it. A value only the request chose is spelled as the request
// spelled it, until a device carrying it turns up.
type tally struct {
	spelling map[string]string
	chosen   map[string]string
	counts   map[string]int
}

func newTally() *tally {
	return &tally{spelling: map[string]string{}, chosen: map[string]string{}, counts: map[string]int{}}
}

// add lists value, spelled as a device carries it, and counts one printer
// under it when counted is set.
func (t *tally) add(fold func(string) string, value string, counted bool) {
	if value == "" {
		return
	}

	key := fold(value)
	if _, held := t.spelling[key]; !held {
		t.spelling[key] = value
	}

	if counted {
		t.counts[key]++
	} else {
		t.counts[key] += 0
	}
}

// keep lists a chosen value even when no printer carries it.
func (t *tally) keep(fold func(string) string, value string) {
	key := fold(value)
	if _, held := t.chosen[key]; !held {
		t.chosen[key] = value
	}

	t.counts[key] += 0
}

func (t *tally) spelled(key string) string {
	if spelling, held := t.spelling[key]; held {
		return spelling
	}

	return t.chosen[key]
}

// values is the dimension's values in the order a person reads them.
func (t *tally) values() []httpapi.DeviceFilterValue {
	out := make([]httpapi.DeviceFilterValue, 0, len(t.counts))
	for key, count := range t.counts {
		out = append(out, httpapi.DeviceFilterValue{Value: t.spelled(key), Count: count})
	}

	slices.SortFunc(out, func(a, b httpapi.DeviceFilterValue) int {
		if foldedLess(a.Value, b.Value) {
			return -1
		}

		if foldedLess(b.Value, a.Value) {
			return 1
		}

		return 0
	})

	return out
}

func tagFold(tag string) string { return strings.ToLower(withoutJoiners(tag)) }

// facets is the tallies of one listing request.
type facets struct {
	state, model, tag, severity *tally
}

// newFacets seeds the tallies with what the request chose, spelled as the
// request spelled it.
func newFacets(filter deviceFilter, query httpapi.DeviceQuery) facets {
	out := facets{state: newTally(), model: newTally(), tag: newTally(), severity: newTally()}

	for _, value := range trimmed(query.States) {
		out.state.keep(strings.ToLower, value)
	}

	for _, value := range trimmed(query.Models) {
		out.model.keep(strings.ToLower, value)
	}

	for _, value := range filter.tags {
		out.tag.keep(tagFold, value)
	}

	for _, value := range []string{severityError, severityWarning, severityInfo} {
		out.severity.keep(strings.ToLower, value)
	}

	return out
}

// count adds one device to every dimension whose other clauses it passes.
func (f facets) count(match clauseMatch, facts deviceFacts, tags []string, issues []httpapi.DeviceIssue) {
	f.state.add(strings.ToLower, facts.listedState(), match.search && match.model && match.tags && match.severity)
	f.model.add(strings.ToLower, facts.model, match.search && match.state && match.tags && match.severity)

	// The tag filter is an AND, so a tag's count is the printers carrying
	// every chosen tag and this one: the clause itself stays applied.
	seen := map[string]bool{}

	for _, tag := range tags {
		if key := tagFold(tag); !seen[key] {
			seen[key] = true
			f.tag.add(tagFold, tag, match.search && match.state && match.model && match.tags && match.severity)
		}
	}

	seen = map[string]bool{}

	for _, issue := range issues {
		if !seen[issue.Severity] {
			seen[issue.Severity] = true
			f.severity.add(strings.ToLower, issue.Severity, match.search && match.state && match.model && match.tags)
		}
	}
}

// describe is the filters the page advertises, in toolbar order.
func (f facets) describe() []httpapi.DeviceFilter {
	severity := f.severity.values()
	slices.SortFunc(severity, func(a, b httpapi.DeviceFilterValue) int {
		return severityPlace(a.Value) - severityPlace(b.Value)
	})

	return []httpapi.DeviceFilter{
		{Param: filterSearch, Kind: "text", MaxLength: maxSearchLength},
		{Param: filterState, Kind: "multi", Values: f.state.values()},
		{Param: filterModel, Kind: "multi", Values: f.model.values()},
		{Param: filterSeverity, Kind: "multi", Values: severity},
		{Param: filterTag, Kind: "multi", Values: f.tag.values()},
	}
}

// severityPlace is worst first, the order the strip draws its chips in.
func severityPlace(severity string) int {
	return slices.Index([]string{severityError, severityWarning, severityInfo}, severity)
}

// trimmed is the chosen values with the blank ones dropped, in the caller's
// spelling.
func trimmed(values []string) []string {
	out := make([]string, 0, len(values))

	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}

	return out
}
