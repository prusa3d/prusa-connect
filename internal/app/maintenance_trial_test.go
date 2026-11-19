package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/upgrade"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

type trialModules map[ref.Ref]httpapi.ResourceStatus

func (m trialModules) Stop(ref.Ref) bool { return false }

func (m trialModules) Restart(ref.Ref) bool { return false }

func (m trialModules) Status(instance ref.Ref) (httpapi.ResourceStatus, bool) {
	status, found := m[instance]

	return status, found
}

func (m trialModules) Database(ref.Ref) (wirerpc.DatabaseState, bool) {
	return wirerpc.DatabaseState{}, false
}

func TestTrialRequiresBundledAndPreviouslyServingExtensions(t *testing.T) {
	spools, err := ref.New(ref.KindSink, "spools")
	require.NoError(t, err)
	queue, err := ref.New(ref.KindSink, "queue")
	require.NoError(t, err)
	offline, err := ref.New(ref.KindSink, "offline")
	require.NoError(t, err)

	holder := maintenance{modules: trialModules{
		spools: {Serving: true},
		queue:  {Serving: false},
	}}
	journal := upgrade.Journal{Extensions: []upgrade.StagedExtension{{
		BundleExtension: upgrade.BundleExtension{Kind: "sink", Name: "offline"},
	}}}
	impacts := []upgrade.ModuleImpact{
		{Module: spools.String(), Outcome: "ok"},
		{Module: queue.String(), Outcome: "ok"},
		{Module: "sink:incompatible", Outcome: "disabled"},
	}

	require.Equal(t, []string{offline.String(), spools.String()},
		holder.trialExtensions(journal, impacts))
}
