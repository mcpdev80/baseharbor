package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

type GuidedReadContext struct {
	ApplicationID    string                           `json:"application_id"`
	Name             string                           `json:"name"`
	Environment      string                           `json:"environment"`
	Target           string                           `json:"target"`
	Capabilities     []capability.Requirement         `json:"capabilities"`
	Preferences      []application.ProviderPreference `json:"provider_preferences"`
	Footprint        application.Footprint            `json:"footprint"`
	Changes          []application.ProviderChange     `json:"provider_changes"`
	Complete         bool                             `json:"complete"`
	SnapshotRevision string                           `json:"snapshot_revision"`
	PlanRevision     string                           `json:"plan_revision"`
}

type GuidedReadError struct {
	Kind  string
	Cause error
}

func (e *GuidedReadError) Error() string { return fmt.Sprintf("guided read %s: %v", e.Kind, e.Cause) }
func (e *GuidedReadError) Unwrap() error { return e.Cause }

func guidedRevision(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func readGuidedApplicationContext(ctx context.Context, resolved resolvedApplication) (GuidedReadContext, error) {
	if resolved.Target.Name == "" || resolved.Manifest.Name == "" {
		return GuidedReadContext{}, &GuidedReadError{Kind: "missing", Cause: errors.New("Target and application intent are required")}
	}
	editing := resolved.DeploymentRecord != nil || resolved.DeploymentIdentity.DeploymentID != "" || resolved.DeploymentIdentity.ApplicationID != ""
	if editing && resolved.DeploymentIdentity.Target != resolved.Target.Name {
		return GuidedReadContext{}, &GuidedReadError{Kind: "wrong_target", Cause: errors.New("deployment Target differs")}
	}
	if editing && (resolved.Manifest.ApplicationID == "" || resolved.DeploymentIdentity.ApplicationID != resolved.Manifest.ApplicationID) {
		return GuidedReadContext{}, &GuidedReadError{Kind: "unverifiable", Cause: errors.New("system-owned application identity differs")}
	}
	contract, err := application.PortableContractFromManifest(resolved.Manifest)
	if err != nil {
		return GuidedReadContext{}, &GuidedReadError{Kind: "incompatible", Cause: err}
	}
	snapshot, preferences, err := applicationLifecycleSnapshot(resolved, observeApplicationProviderFacts(ctx, resolved))
	if err != nil {
		return GuidedReadContext{}, &GuidedReadError{Kind: "unverifiable", Cause: err}
	}
	footprint, err := application.ResolveFootprint(resolved.Manifest, snapshot, preferences)
	if err != nil {
		return GuidedReadContext{}, &GuidedReadError{Kind: "incompatible", Cause: err}
	}
	plan, err := resolveApplicationProviderPlanFromSnapshot(resolved, snapshot, preferences)
	if err != nil {
		return GuidedReadContext{}, &GuidedReadError{Kind: "unverifiable", Cause: err}
	}
	snapshotRev, err := guidedRevision(struct {
		Manifest    application.Manifest
		Snapshot    application.ResolutionSnapshot
		Preferences []application.ProviderPreference
	}{resolved.Manifest, snapshot, preferences})
	if err != nil {
		return GuidedReadContext{}, &GuidedReadError{Kind: "unverifiable", Cause: err}
	}
	planRev, err := guidedRevision(plan)
	if plan.Impact != nil && plan.Impact.PlanID != "" {
		planRev = plan.Impact.Scope.PlanID
	}
	if err != nil {
		return GuidedReadContext{}, &GuidedReadError{Kind: "unverifiable", Cause: err}
	}
	return GuidedReadContext{
		ApplicationID: resolved.Manifest.ApplicationID,
		Name:          resolved.Manifest.Name, Environment: resolved.Manifest.Environment,
		Target:       resolved.Target.Name,
		Capabilities: append([]capability.Requirement{}, contract.Capabilities...),
		Preferences:  append([]application.ProviderPreference{}, preferences...),
		Footprint:    footprint, Changes: append([]application.ProviderChange{}, plan.Changes...),
		Complete:         footprint.Complete && plan.Complete,
		SnapshotRevision: snapshotRev, PlanRevision: planRev,
	}, nil
}
