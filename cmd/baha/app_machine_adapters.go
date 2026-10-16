package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// appMachineError exposes stable machine codes without leaking internal errors.
type appMachineError struct{ Code string }

func (e *appMachineError) Error() string { return "application machine: " + e.Code }

type appMachineRead func(context.Context, string) (GuidedReadContext, error)

// appMachineView is a read-only projection; authorization must already have
// happened in the existing GuidedReadContext reader.
type appMachineView struct {
	Target           string                           `json:"target"`
	ApplicationID    string                           `json:"application_id"`
	Requirements     []capability.Requirement         `json:"requirements"`
	Preferences      []application.ProviderPreference `json:"provider_preferences"`
	Footprint        application.Footprint            `json:"footprint"`
	SnapshotRevision string                           `json:"snapshot_revision"`
	PlanRevision     string                           `json:"plan_revision"`
}

func appMachineInspect(ctx context.Context, target string, read appMachineRead) (appMachineView, error) {
	if target == "" || read == nil {
		return appMachineView{}, &appMachineError{Code: "unsupported"}
	}
	view, err := read(ctx, target)
	if err != nil {
		return appMachineView{}, appMachineReadError(err)
	}
	if view.Target != target || view.Footprint.Target != target ||
		view.Footprint.Version != application.FootprintVersion ||
		view.ApplicationID == "" || view.PlanRevision == "" || view.SnapshotRevision == "" {
		return appMachineView{}, &appMachineError{Code: "conflict"}
	}
	// The Footprint owns requirements also stored outside the portable
	// Capabilities list (for example managed secrets).
	requirements := append([]capability.Requirement(nil), view.Capabilities...)
	for _, resolved := range view.Footprint.Requirements {
		found := false
		for _, requirement := range requirements {
			if requirement == resolved.Requirement {
				found = true
				break
			}
		}
		if !found {
			requirements = append(requirements, resolved.Requirement)
		}
	}
	// Marshal-copy the domain values, never expose the wider read context.
	var projection appMachineView
	raw, err := json.Marshal(appMachineView{
		Target: target, ApplicationID: view.ApplicationID,
		Requirements: requirements, Preferences: view.Preferences,
		Footprint: view.Footprint, SnapshotRevision: view.SnapshotRevision,
		PlanRevision: view.PlanRevision,
	})
	if err != nil || json.Unmarshal(raw, &projection) != nil {
		return appMachineView{}, &appMachineError{Code: "unsupported"}
	}
	return projection, nil
}

// appMachineHydrate uses the accepted YAML/source reconciliation authority.
func appMachineHydrate(ctx context.Context, target string, source []byte, read appEditLoadReader) (appEditLoaded, error) {
	loaded, err := appEditLoad(ctx, target, source, read)
	if err != nil {
		var e *appEditLoadError
		if errors.As(err, &e) {
			switch e.Kind {
			case "missing", "wrong_target", "ambiguous", "incompatible":
				return appEditLoaded{}, &appMachineError{Code: "conflict"}
			}
		}
		return appEditLoaded{}, &appMachineError{Code: "denied"}
	}
	return loaded, nil
}

func appMachineReadError(err error) error {
	var guided *GuidedReadError
	if errors.As(err, &guided) {
		switch guided.Kind {
		case "denied":
			return &appMachineError{Code: "denied"}
		case "wrong_target", "ambiguous", "incompatible", "missing":
			return &appMachineError{Code: "conflict"}
		case "unsupported":
			return &appMachineError{Code: "unsupported"}
		}
	}
	return &appMachineError{Code: "denied"}
}

// The registered caller must provide these existing, authorized read owners.
type appMachinePlanReader func(context.Context, string) (application.ProviderImpactPlan, error)
type appMachineAuthorize func(context.Context, string, string) error

// appMachineApply is an injected, already authorized application-owned
// execution path. This adapter does not implement runtime/provider mutations.
type appMachineApply func(context.Context, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error)

type appMachineChangeRequest struct {
	Target           string
	SnapshotRevision string
	PlanRevision     string
	Consent          application.ProviderChangeConsent
}

// appMachineChange binds one fresh authorized read to the exact impact plan.
// No execution occurs when any revision or approval is stale.
func appMachineChange(ctx context.Context, request appMachineChangeRequest, read appMachineRead,
	planRead appMachinePlanReader, authorize appMachineAuthorize, apply appMachineApply) (application.ProviderChangeResult, error) {
	if read == nil || planRead == nil || authorize == nil || request.Target == "" {
		return application.ProviderChangeResult{}, &appMachineError{Code: "unsupported"}
	}
	view, err := read(ctx, request.Target)
	if err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if view.Target != request.Target || view.Footprint.Target != request.Target ||
		view.ApplicationID == "" || request.SnapshotRevision == "" ||
		request.PlanRevision == "" || request.SnapshotRevision != view.SnapshotRevision ||
		request.PlanRevision != view.PlanRevision {
		return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
	}
	if err := authorize(ctx, request.Target, view.ApplicationID); err != nil {
		return application.ProviderChangeResult{}, &appMachineError{Code: "denied"}
	}
	plan, err := planRead(ctx, request.Target)
	if err != nil {
		return application.ProviderChangeResult{}, appMachineReadError(err)
	}
	if plan.Target != request.Target || plan.PlanID != view.PlanRevision ||
		plan.Version != application.ProviderImpactVersion {
		return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
	}
	if err := application.ValidateProviderChangeConsent(plan, request.Consent); err != nil {
		var change *application.ProviderChangeError
		if errors.As(err, &change) {
			if change.Code == "stale-impact-consent" || change.Code == "invalid-impact-plan" {
				return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
			}
			if change.Code == "impact-confirmation-required" || change.Code == "data-deletion-consent-required" {
				return application.ProviderChangeResult{}, &appMachineError{Code: "cancel"}
			}
		}
		return application.ProviderChangeResult{}, &appMachineError{Code: "denied"}
	}
	if apply == nil {
		return application.ProviderChangeResult{}, &appMachineError{Code: "unsupported"}
	}
	outcomes, executionErr := apply(ctx, plan, request.Consent)
	result, err := application.RecordProviderChangeResult(plan, outcomes)
	if err != nil {
		return application.ProviderChangeResult{}, &appMachineError{Code: "partial"}
	}
	if executionErr != nil {
		return result, &appMachineError{Code: "partial"}
	}
	return result, nil
}

// appMachineFootprintJSON exposes exactly the canonical domain footprint bytes.
func appMachineFootprintJSON(view GuidedReadContext, target string) ([]byte, error) {
	var out bytes.Buffer
	if err := appFootprintShowContext(&out, view, appFootprintShowOptions{Target: target, JSON: true}); err != nil {
		return nil, &appMachineError{Code: "conflict"}
	}
	return bytes.Clone(bytes.TrimSpace(out.Bytes())), nil
}
