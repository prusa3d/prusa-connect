package app

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
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
	}, listed)

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

	listed, err := origins.SetAllowedOrigins(t.Context(), []string{" https://new.example ", "http://localhost:5173"})
	require.NoError(t, err)
	require.Equal(t, []string{"http://localhost:5173", "https://new.example"}, store.stored)
	require.Len(t, listed, 3)

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

		_, err := origins.SetAllowedOrigins(t.Context(), list)

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

	_, err := origins.SetAllowedOrigins(t.Context(), []string{slicerOrigin})
	require.Error(t, err)
	require.False(t, origins.Allows(slicerOrigin))
}
