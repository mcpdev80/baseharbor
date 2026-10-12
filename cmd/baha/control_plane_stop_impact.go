package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type controlPlaneStopPlan struct {
	Scope     application.ProviderImpactScope      `json:"scope"`
	Target    string                               `json:"target"`
	Project   string                               `json:"project"`
	Services  []string                             `json:"services"`
	Resources []bhruntime.ProjectResource          `json:"resources"`
	Functions []application.FunctionConsequence    `json:"functions"`
	Materials []application.MaterialConsequence    `json:"materials"`
	Effects   []application.OperationalConsequence `json:"effects"`
	inventory bhruntime.ControlPlaneRuntimeInventory
	protected map[string]string
}

// Observe the exact Core module owner, not every service that happens to share
// its native project. Existing provider and deployment owners describe all
// potentially dependent consumers; uncertainty requires deliberate consent.
func buildControlPlaneStopPlan(ctx context.Context) (controlPlaneStopPlan, error) {
	p := controlPlaneStopPlan{Functions: []application.FunctionConsequence{}, Materials: []application.MaterialConsequence{}}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return p, err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return p, err
	}
	p.inventory, err = bhruntime.ExistingControlPlaneRuntimeInventory(root, targetRuntimeProjectName(target))
	if err != nil {
		return p, runtimeMutationOwnerError()
	}
	p.Target, p.Project = target.Name, p.inventory.Files.Project
	p.Services = append(append([]string{}, p.inventory.SQLServices...), p.inventory.SecretsServices...)
	p.Resources = p.inventory.Resources
	dataRoot, err := targetDataRoot(target)
	if err != nil {
		return p, err
	}
	registryPath := filepath.Join(dataRoot, "provider-registry.json")
	registry, err := (capability.RegistryStore{Path: registryPath}).Load()
	if err != nil {
		return p, invalidProviderConsumerInventory(err)
	}
	configPath, err := deployment.ConfigPath()
	if err != nil {
		return p, err
	}
	paths := map[string]bool{configPath: false, registryPath: true, filepath.Join(root, "installation.json"): true}
	records, err := deployment.ListDeployments(target.Name)
	if err != nil {
		return p, invalidProviderConsumerInventory(err)
	}
	for _, record := range records {
		location, err := deployment.DeploymentRoot(record.Identity)
		if err != nil {
			return p, err
		}
		paths[filepath.Join(location, "deployment.json")] = false
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "deployment/" + record.Identity.DeploymentID, Function: "core-dependent-availability", Via: p.Services, Outcome: "unknown", Behavior: "Only the selected PostgreSQL/OpenBao Core services stop. Workloads and other provider containers remain. Shared SQL access, secret retrieval, credential leases, PKI issuance/renewal and dependent authentication/access may fail; exact consumer reconnection, cached validity and in-flight effects require actual verification. Authentication is never bypassed.", Evidence: "existing Target deployment registration; unverified dependencies remain unknown"})
	}
	for _, binding := range registry.Bindings {
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "application/" + binding.ApplicationID + "/" + binding.Environment, Function: string(binding.Resource.Kind), Via: []string{binding.ProviderInstanceID}, Required: true, Outcome: "unknown", Behavior: "This existing binding is retained. Direct or transitive dependency on the selected Core services, current token/certificate/lease validity and future renewal are unverified; continued access is not claimed.", Evidence: "authoritative existing provider binding"})
	}
	p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "management/" + target.Name, Function: "management-console-and-remote-access", Via: p.Services, Outcome: "unknown", Behavior: "SQL/OpenBao-dependent management, Console and remote access may become unavailable. Existing TLS/OIDC/mTLS policy and recovery remain configured; no unauthenticated fallback or password change is made.", Evidence: "selected Core owner; independent consumer behavior is unverified"})
	materials, _, err := observeOwnedProviderFiles("target/"+target.Name, target.Name, "core-runtime", root)
	if err != nil {
		return p, runtimeMutationOwnerError()
	}
	paths[p.inventory.Files.Compose], paths[p.inventory.Files.Env] = false, false
	for _, material := range materials {
		if material.Verified && material.Location != nil && material.Location.Kind == "path" {
			paths[material.Location.Reference] = false
		}
		disposition := application.MaterialDisposition{MaterialID: material.ID, Disposition: "unknown"}
		if material.Verified {
			disposition.Disposition, disposition.Destination, disposition.Supported = "retained", material.Location, true
		}
		p.Materials = append(p.Materials, application.MaterialConsequence{Before: material, After: disposition, Meaning: "No file removal is selected. Protected ownership is revalidated where known; unverified material remains unknown. Retention is recovery ownership, not active secret delivery, future certificate renewal or usable application data proof."})
	}
	p.protected = map[string]string{}
	orderedPaths := []string{}
	for path := range paths {
		orderedPaths = append(orderedPaths, path)
	}
	sort.Strings(orderedPaths)
	for _, path := range orderedPaths {
		data, err := runtimeMutationOwnerBytes(path, paths[path])
		if err != nil {
			return p, err
		}
		p.protected[path] = runtimeMutationRevision(data)
		zeroBytes(data)
		if data != nil {
			p.Materials = append(p.Materials, runtimeRetainedOwnerMaterial(target.Name, filepath.Base(path), path))
		}
	}
	p.Effects = []application.OperationalConsequence{{Aspect: "selected-core-stop", Before: "Canonical owned Core module services at the displayed resources.", After: "Stop these exact service processes; preserve containers, networks, volumes, bindings, connectivity policy, other providers and workloads. No explicit data deletion. In-flight transactions, leases, sessions, transient data and subsequent recovery remain unverified.", Evidence: "canonical Core renderer and selected native service operation"}}
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "control-plane-runtime", Reference: p.inventory.Files.Compose, Subject: p.Project, Revision: runtimeMutationRevision(struct {
		Plan   controlPlaneStopPlan
		Owners map[string]string
	}{p, p.protected})}, nil, false)
	return p, err
}

func authorizeControlPlaneStop(ctx context.Context, out io.Writer) (controlPlaneStopPlan, error) {
	p, err := buildControlPlaneStopPlan(ctx)
	if err != nil {
		return p, err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return p, err
	}
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintln(out, "Achtung: review this selected Core stop, affected consumers and retained recovery locations.")
	if err := writeJSON(out, p); err != nil {
		return p, err
	}
	in := applicationInput(ctx, nil)
	if consent == nil && !noInput(ctx) && in != nil && readerIsTerminal(in) {
		value, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, application.ProviderImpactPlan{PlanID: p.Scope.PlanID, ConfirmationsRequired: p.Scope.ConfirmationsRequired})
		if err != nil {
			return p, err
		}
		consent = &value
	}
	ack := application.ProviderChangeConsent{PlanID: p.Scope.PlanID}
	if consent != nil {
		ack = *consent
	}
	if err := application.ValidateProviderImpactScopeConsent(p.Scope, ack); err != nil {
		return p, controlPlaneStopPlanError(p, err)
	}
	current, err := buildControlPlaneStopPlan(ctx)
	if err != nil {
		return p, err
	}
	if current.Scope.PlanID != p.Scope.PlanID {
		return current, controlPlaneStopPlanError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	return p, nil
}

func controlPlaneStopPlanError(plan controlPlaneStopPlan, err error) error {
	classified := classifyProviderChangeConsent(err)
	var failure *machine.Error
	if errors.As(classified, &failure) {
		failure.Details = plan
	}
	return classified
}
