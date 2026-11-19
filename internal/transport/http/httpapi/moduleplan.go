package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Requests is what an offer says its release asks for, in the shape an
// install's expect carries.
func (o ModuleOffer) Requests() ModuleRequests {
	return ModuleRequests{
		Permissions:       o.Permissions,
		Checks:            o.Checks,
		Events:            o.Events,
		CheckAbsentPeople: o.CheckAbsentPeople,
		ListPeople:        o.ListPeople,
		Calls:             o.Calls,
		PublicRoutes:      o.PublicRoutes,
		StagedRoutes:      o.StagedRoutes,
		RequestHeaders:    o.RequestHeaders,
		RecordsMiB:        o.RecordsMiB,
		Listen:            o.Listen,
		Data:              o.Data,
		Database:          o.Database,
		AllowedOrigins:    o.AllowedOrigins,
	}
}

// Approved is the plan as an install sends it back once a person approved
// it: each module with its digest and the requests the plan showed for it.
// approveListen grants the ports of every module that lists any.
func (p ModulePlan) Approved(approveListen bool) []ModuleReleaseRef {
	out := make([]ModuleReleaseRef, 0, len(p.Dependencies))

	for _, planned := range p.Dependencies {
		requests := planned.Requests()
		out = append(out, ModuleReleaseRef{
			Module: planned.Module, Version: planned.Version, Sha256: planned.Sha256,
			Expect: &requests, ApproveListen: approveListen && len(planned.Listen) > 0,
		})
	}

	return out
}

type modulePlanInput struct {
	Body ModuleReleaseRef
}

type modulePlanOutput struct {
	Body ModulePlan
}

func registerModulePlan(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "plan-module-install",
		Method:      http.MethodPost,
		Path:        modulePlanPath,
		Summary:     "The modules an install would add",
		Description: "Resolves one release against the modules installed here and the index, and answers the " +
			"modules it requires that this server does not have, with the release chosen for each and " +
			"every request each one makes. For each missing module the resolver takes the newest release " +
			"in the range of every module that names it, the new one and every installed one, and the " +
			"release the lock names when it is in range. Nothing is written. Send the answer back as " +
			"`dependencies` on install-module, which adds exactly these with the same approve.\n\n" +
			"A release no set of modules fits answers MODULE_DEPENDENCY_CONFLICT, which names who needs " +
			"what. A module no index offers answers MODULE_DEPENDENCY_NOT_OFFERED. An installed module " +
			"outside a range answers MODULE_DEPENDENCY_CONFLICT and names it, because an install never " +
			"updates another module. It takes configure_resource, as install-module does.",
		Tags:          []string{"resources"},
		DefaultStatus: http.StatusOK,
		Errors:        moduleInstallErrors,
	}, func(ctx context.Context, in *modulePlanInput) (*modulePlanOutput, error) {
		if err := requireModuleAdmin(ctx, cfg); err != nil {
			return nil, err
		}

		plan, err := cfg.ModuleInstall.Plan(ctx, principalFrom(ctx), in.Body)
		if err != nil {
			return nil, err
		}

		if plan.Dependencies == nil {
			plan.Dependencies = []ModulePlanned{}
		}

		return &modulePlanOutput{Body: plan}, nil
	})
}
