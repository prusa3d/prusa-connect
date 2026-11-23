package app

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

const slicerOrigin = "https://slicer.example"

// memoryOrigins is the machine-owned file as the origins read and write it.
type memoryOrigins struct {
	stored []string
	fail   error
}

func (m *memoryOrigins) AllowedOrigins() ([]string, error) { return m.stored, nil }

func (m *memoryOrigins) RecordAllowedOrigins(origins []string) error {
	if m.fail != nil {
		return m.fail
	}

	m.stored = origins

	return nil
}

// asRead is the precondition of a write that quotes the version the list has
// now, as a person who just read it sends.
func asRead(t *testing.T, origins *browserOrigins) httpapi.Precondition {
	t.Helper()

	listed, err := origins.AllowedOrigins(t.Context())
	require.NoError(t, err)

	return httpapi.Precondition{IfMatch: []string{listed.Version}}
}

func originsOver(t *testing.T, store *memoryOrigins, file ...string) *browserOrigins {
	t.Helper()

	var core config.Core
	core.API.AllowedOrigins = file

	return newBrowserOrigins(t.Context(), core, store, discard())
}

// The answer is the three lists together, each origin with where it comes
// from, and the CORS answer allows every one of them.
func TestAnOriginIsAllowedFromTheFileTheSettingsAndAModule(t *testing.T) {
	t.Parallel()

	origins := originsOver(t, &memoryOrigins{stored: []string{"https://stored.example"}}, "https://file.example")
	origins.SyncModuleOrigins(t.Context(), []resource.ModuleOrigin{
		{Origin: slicerOrigin, Namespace: "com.github.acme.slicer", Title: "Web slicer"},
	})

	listed, err := origins.AllowedOrigins(t.Context())
	require.NoError(t, err)
	require.Equal(t, []httpapi.AllowedOrigin{
		{Origin: "https://file.example", Source: httpapi.OriginFromFile},
		{Origin: "https://stored.example", Source: httpapi.OriginFromSettings},
		{
			Origin: slicerOrigin, Source: httpapi.OriginFromExtension,
			Extension: "com.github.acme.slicer", Title: "Web slicer",
		},
	}, listed.Items)

	for _, origin := range []string{"https://file.example", "https://stored.example", slicerOrigin} {
		require.True(t, origins.Allows(origin), origin)
	}

	require.False(t, origins.Allows("https://elsewhere.example"))
}

// A module that is switched off or removed leaves the list the store hands
// over, and its origin stops being answered with that call.
func TestAModuleOriginLeavesWithItsModule(t *testing.T) {
	t.Parallel()

	origins := originsOver(t, &memoryOrigins{})
	require.False(t, origins.Any())

	origins.SyncModuleOrigins(t.Context(), []resource.ModuleOrigin{{Origin: slicerOrigin, Namespace: "ns.slicer"}})
	require.True(t, origins.Allows(slicerOrigin))

	origins.SyncModuleOrigins(t.Context(), nil)
	require.False(t, origins.Allows(slicerOrigin))
	require.False(t, origins.Any())
}

// A stored list replaces the one before, applies at once, and changes neither
// the file's origins nor a module's.
func TestStoringTheSettingsListAppliesAtOnce(t *testing.T) {
	t.Parallel()

	store := &memoryOrigins{stored: []string{"https://old.example"}}
	origins := originsOver(t, store, "https://file.example")

	before := asRead(t, origins)

	listed, err := origins.SetAllowedOrigins(t.Context(),
		[]string{" https://new.example ", "http://localhost:5173"}, before)
	require.NoError(t, err)
	require.Equal(t, []string{"http://localhost:5173", "https://new.example"}, store.stored)
	require.Len(t, listed.Items, 3)
	require.NotEqual(t, before.IfMatch[0], listed.Version, "the write left the version as it was")

	require.False(t, origins.Allows("https://old.example"))
	require.True(t, origins.Allows("https://new.example"))
	require.True(t, origins.Allows("https://file.example"))
}

// A value a browser never sends, a wildcard and a repeat are refused before
// anything is stored.
func TestAStoredOriginIsHeldToWhatABrowserSends(t *testing.T) {
	t.Parallel()

	for _, list := range [][]string{
		{"*"},
		{"https://*.slicer.example"},
		{"https://slicer.example:0443"},
		{"https://slicer.example/"},
		{"https://Slicer.example"},
		{"https://slicer.example:443"},
		{"slicer.example"},
		{slicerOrigin, slicerOrigin},
	} {
		store := &memoryOrigins{}
		origins := originsOver(t, store)

		_, err := origins.SetAllowedOrigins(t.Context(), list, asRead(t, origins))

		var raised *fault.Fault

		require.ErrorAs(t, err, &raised, "%v", list)
		require.Nil(t, store.stored, "%v was stored", list)
		require.False(t, origins.Any())
	}
}

// A file that refuses the write leaves the answer as it was.
func TestAListTheFileRefusedChangesNothing(t *testing.T) {
	t.Parallel()

	origins := originsOver(t, &memoryOrigins{fail: errors.New("read-only")})

	_, err := origins.SetAllowedOrigins(t.Context(), []string{slicerOrigin}, asRead(t, origins))
	require.Error(t, err)
	require.False(t, origins.Allows(slicerOrigin))
}

// A write that quotes a version the stored list no longer has is refused and
// stores nothing, so it does not undo a change made since it was read.
func TestAWriteFromAnOldReadChangesNothing(t *testing.T) {
	t.Parallel()

	store := &memoryOrigins{stored: []string{"https://old.example"}}
	origins := originsOver(t, store)
	stale := asRead(t, origins)

	_, err := origins.SetAllowedOrigins(t.Context(), nil, stale)
	require.NoError(t, err)

	_, err = origins.SetAllowedOrigins(t.Context(), []string{"https://old.example", "https://new.example"}, stale)
	require.True(t, fault.HasCode(err, httpfault.CodeVersionConflict()), "got %v", err)
	require.Empty(t, store.stored)
	require.False(t, origins.Allows("https://old.example"))
}

// A repeat in the operator's file or in the machine-owned file is listed
// once, so a page draws one row for it and a write of the list read back is
// not refused as a repeat.
func TestARepeatedOriginIsListedOnce(t *testing.T) {
	t.Parallel()

	store := &memoryOrigins{stored: []string{slicerOrigin, slicerOrigin}}
	origins := originsOver(t, store, "https://file.example", "https://file.example")

	listed, err := origins.AllowedOrigins(t.Context())
	require.NoError(t, err)
	require.Equal(t, []httpapi.AllowedOrigin{
		{Origin: "https://file.example", Source: httpapi.OriginFromFile},
		{Origin: slicerOrigin, Source: httpapi.OriginFromSettings},
	}, listed.Items)

	_, err = origins.SetAllowedOrigins(t.Context(), []string{slicerOrigin, "https://new.example"}, asRead(t, origins))
	require.NoError(t, err)
}
