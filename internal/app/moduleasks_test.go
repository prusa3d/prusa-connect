package app

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// recommendingManifest is the two-halves module at a version, recommending a
// memory budget for its process.
func recommendingManifest(version string, memoryMiB int) string {
	document := strings.Replace(bothHalvesManifest(), "version: 1.0.0", "version: "+version, 1)

	return strings.Replace(document, "permissions:\n",
		"needs:\n  memory_mib: "+strconv.Itoa(memoryMiB)+"\npermissions:\n", 1)
}

// installedSpools is the declaration the installer's files now hold for the
// module.
func installedSpools(t *testing.T, installer *moduleInstaller, configPath string) resource.Declaration {
	t.Helper()

	doc, err := resource.Load(resource.Paths{File: configPath, Overlay: installer.core.Overlay})
	require.NoError(t, err)

	instance, err := ref.New(ref.KindSink, "spools")
	require.NoError(t, err)

	declared, found := doc.Find(instance)
	require.True(t, found)

	return declared
}

func TestARaisedMemoryRecommendationIsAChangedRequest(t *testing.T) {
	t.Parallel()

	installer, _ := theInstaller(t, true)

	_, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: aRelease(t, recommendingManifest("1.0.0", 128), true),
		Approve: true,
	})
	require.NoError(t, err)

	_, err = installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.1.0", From: aRelease(t, recommendingManifest("1.1.0", 256), true),
		Approve: true,
		Expect:  &httpapi.ModuleRequests{Permissions: []string{"observe"}, MemoryMiB: 128},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MODULE_REQUESTS_CHANGED")
	require.Contains(t, err.Error(), "memory 256 MiB")
	require.NotContains(t, err.Error(), "permission observe")
}

// The operator set no budget, so the module's follows its author down.
func TestALoweredRecommendationLowersABudgetWithoutOverride(t *testing.T) {
	t.Parallel()

	installer, configPath := theInstaller(t, true)

	_, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: aRelease(t, recommendingManifest("1.0.0", 256), true),
		Approve: true,
	})
	require.NoError(t, err)

	_, err = installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.1.0", From: aRelease(t, recommendingManifest("1.1.0", 96), true),
		Approve: true,
	})
	require.NoError(t, err)

	budget, isService := resource.BudgetOf(installedSpools(t, installer, configPath).Spec)
	require.True(t, isService)
	require.Equal(t, resource.Budget{Bytes: 96 << 20, Source: resource.BudgetAuthor}, budget)
}

// The operator chose warn and a switch-off of their own, so an update must
// not put the module back on stop or on the server's switch-off.
func TestAnUpdateKeepsOnOverMemoryAndTheSwitchOff(t *testing.T) {
	t.Parallel()

	installer, configPath := theInstaller(t, true)

	_, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: aRelease(t, recommendingManifest("1.0.0", 256), true),
		Approve: true,
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(installer.core.Overlay)
	require.NoError(t, err)

	var overlay struct {
		Version   int              `json:"version"`
		Instances []map[string]any `json:"instances"`
	}
	require.NoError(t, json.Unmarshal(raw, &overlay))

	for _, instance := range overlay.Instances {
		if service, ok := instance["service"].(map[string]any); ok {
			service["on_over_memory"] = install.OnOverMemoryWarn
			service["switch_off"] = map[string]any{"failures": 0, "window": "6h"}
		}
	}

	raw, err = json.Marshal(overlay)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(installer.core.Overlay, raw, 0o600))
	require.Equal(t, install.OnOverMemoryWarn, installedSpools(t, installer, configPath).Spec.OnOverMemory())

	_, err = installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.1.0", From: aRelease(t, recommendingManifest("1.1.0", 256), true),
		Approve: true,
	})
	require.NoError(t, err)

	raw, err = os.ReadFile(installer.core.Overlay)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"window": "6h"`, "the overlay writes the window in its shortest form")

	updated := installedSpools(t, installer, configPath).Spec
	require.Equal(t, install.OnOverMemoryWarn, updated.OnOverMemory())

	switchOff, isService := resource.SwitchOffOf(updated, install.SwitchOff{})
	require.True(t, isService)
	require.Equal(
		t,
		resource.SwitchOffPolicy{Failures: 0, Window: 6 * time.Hour, Source: resource.BudgetOperator},
		switchOff,
	)
}

// fillByName sets each field of dst named like a ModuleAsks field, through a
// pointer when dst's field is one.
func fillByName(t *testing.T, dst any, src install.ModuleAsks) {
	t.Helper()

	target := reflect.ValueOf(dst).Elem()
	source := reflect.ValueOf(src)

	for i := range source.NumField() {
		name := source.Type().Field(i).Name
		field := target.FieldByName(name)
		require.True(t, field.IsValid(), "%T has no field %s", dst, name)

		if field.Kind() == reflect.Pointer {
			ptr := reflect.New(field.Type().Elem())
			ptr.Elem().Set(source.Field(i))
			field.Set(ptr)

			continue
		}

		field.Set(source.Field(i))
	}
}

func TestEveryAskSurvivesEveryHop(t *testing.T) {
	t.Parallel()

	all := install.ModuleAsks{
		Permissions:       []string{"observe"},
		Checks:            []string{"operate"},
		Events:            map[string]string{"printer.events": "events"},
		CheckAbsentPeople: true,
		ListPeople:        true,
		Calls:             []string{"user:spools"},
		PublicRoutes:      []string{"POST /hooks/announce/"},
		StagedRoutes:      []install.StagedRoute{{Method: "PUT", Path: "/files/{name}", MaxMiB: 1024}},
		RequestHeaders: []install.RequestHeader{
			{Name: "Print-After-Upload", Reason: "prints the file", Required: true},
		},
		RecordsMiB:     16,
		MemoryMiB:      128,
		Listen:         []install.ListenAsk{{Name: "web", Network: "tcp", Port: 8080, Why: "serves the page"}},
		Data:           true,
		Database:       true,
		AllowedOrigins: []string{"https://slicer.example"},
	}

	allType := reflect.TypeOf(all)
	for i := range allType.NumField() {
		require.False(t, reflect.ValueOf(all).Field(i).IsZero(), "fixture leaves %s zero", allType.Field(i).Name)
	}

	var release marketplaceRelease
	fillByName(t, &release, all)
	require.Equal(t, all, release.asks())

	var row moduleIndexRow
	fillByName(t, &row, all)

	var got, want httpapi.ModuleOffer
	row.fill(&got)
	withAsks(&want, all)
	require.Equal(t, want, got)

	for field := range allType.Fields() {
		require.Falsef(t, reflect.ValueOf(got).FieldByName(field.Name).IsZero(), "offer lost %s", field.Name)
	}

	require.Equal(t, all, resource.AsksOf(resource.RequestsOf(all)))

	for i := range allType.NumField() {
		t.Run(allType.Field(i).Name, func(t *testing.T) {
			t.Parallel()

			one := install.ModuleAsks{}
			reflect.ValueOf(&one).Elem().Field(i).Set(reflect.ValueOf(all).Field(i))

			var oneRow moduleIndexRow
			fillByName(t, &oneRow, one)
			require.True(t, oneRow.statesRequests())
			require.NotEmpty(t, beyondAsks(one, install.ModuleAsks{}, "ns"))
		})
	}
}
