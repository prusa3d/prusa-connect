//go:build !diagnostics

package app

import (
	"context"
	"log/slog"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/platform/observability"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// diagnosticsBuilt says this build leaves the support bundle out, so the
// first-run wizard does not offer its keys.
const diagnosticsBuilt = false

// diagnosticsRing keeps nothing in a build that leaves the bundle out.
func diagnosticsRing(config.Core) *observability.LogRing { return nil }

// ringSink keeps no copy in a build that leaves the bundle out, so the ring
// handler is not linked.
func ringSink(sink slog.Handler, _ *observability.LogRing) slog.Handler { return sink }

// keepCrashes records no crash in a build that leaves the bundle out, and the
// start says so in one line, whatever core.diagnostics.enabled says.
func keepCrashes(ctx context.Context, _ config.Core, logger *slog.Logger) string {
	logger.InfoContext(ctx, "diagnostics are off", "reason", "this build leaves them out")

	return ""
}

// servedDiagnostics serves no bundle in a build that leaves it out.
func servedDiagnostics(diagnosticsBundles) httpapi.Diagnostics { return nil }
