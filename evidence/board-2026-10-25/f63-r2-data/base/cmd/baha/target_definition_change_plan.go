package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

type targetDefinitionChangePlan struct {
	Name             string                               `json:"name"`
	Operation        string                               `json:"operation"`
	Scope            application.ProviderImpactScope      `json:"scope"`
	Functions        []application.FunctionConsequence    `json:"functions"`
	Materials        []application.MaterialConsequence    `json:"materials"`
	Effects          []application.OperationalConsequence `json:"effects"`
	RecoveryFile     string                               `json:"recovery_file"`
	SelectionBlocked bool                                 `json:"selection_blocked"`
	cfg              deployment.Config
	expected         []byte
	selection        []byte
	protected        map[string]string
	recovery         deployment.TargetDefinitionRecovery
}

// Configuration detachment changes access to existing owners, not their native
// processes or data. Unknown live consumer behavior is explicit and requires
// both scope acknowledgements; protected recovery is an actual selected action.
func buildTargetDefinitionChangePlan(ctx context.Context, name, recoveryFile string) (targetDefinitionChangePlan, error) {
	p := targetDefinitionChangePlan{Name: name, Operation: "detach", Functions: []application.FunctionConsequence{}, Materials: []application.MaterialConsequence{}, protected: map[string]string{}}
	if err := deployment.ValidateTargetName(name); err != nil {
		return p, err
	}
	if recoveryFile != "" {
		var err error
		recoveryFile, err = filepath.Abs(recoveryFile)
		if err != nil {
			return p, err
		}
	}
	operation := "target.delete"
	if recoveryFile != "" {
		operation = "target.create"
	}
	// Restoration cannot resolve a configuration entry that is deliberately
	// absent. Authorize the exact requested Target directly through the existing
	// machine permission authority, then authorize each retained consumer below.
	if err := authorizeMCPOperation(ctx, operation, name, "dev", "", ""); err != nil {
		return p, err
	}
	cfg, raw, err := deployment.LoadConfigForMutation()
	if err != nil {
		return p, err
	}
	p.cfg, p.expected = cfg, raw
	if recoveryFile == "" {
		p.recovery, err = deployment.TargetRecoveryFromConfig(raw, name)
		if err != nil {
			return p, err
		}
		p.RecoveryFile, err = p.recovery.OwnerPath()
	} else {
		p.Operation, p.RecoveryFile = "restore", recoveryFile
		p.recovery, err = deployment.LoadTargetDefinitionRecovery(recoveryFile, name)
		if err == nil {
			_, err = p.recovery.RestoreInto(cfg)
			// RestoreInto updates maps; capture fresh original config for apply.
			if err == nil {
				p.cfg, p.expected, err = deployment.LoadConfigForMutation()
			}
		}
	}
	if err != nil {
		return p, err
	}
	if tenant, scoped := tenancy.FromContext(ctx); scoped && (p.recovery.Definition.TenantID == "" || p.recovery.Definition.TenantID != tenant.TenantID) {
		return p, &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "target_definition_tenant_owner_unverifiable", Message: "The original Target definition has no verified ownership for the requesting tenant."}
	}
	selectionPath, err := targetSelectionPath()
	if err != nil {
		return p, err
	}
	p.selection, err = runtimeMutationOwnerBytes(selectionPath, true)
	if err != nil {
		return p, err
	}
	active := strings.TrimSpace(string(p.selection))
	if detached, ok := targetSelectionDetached(p.selection); ok {
		p.SelectionBlocked = detached == name
	} else {
		if active != "" && deployment.ValidateTargetName(active) != nil {
			return p, runtimeMutationOwnerError()
		}
		p.SelectionBlocked = active == name || active == "" && (cfg.DefaultTarget == "" || cfg.DefaultTarget == name)
	}
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		return p, err
	}
	if _, statErr := os.Lstat(root); statErr == nil {
		materials, _, walkErr := observeOwnedProviderFiles("target/"+name, name, "target-definition", root)
		if walkErr != nil {
			return p, runtimeMutationOwnerError()
		}
		for _, m := range materials {
			if m.Location != nil && (filepath.Base(m.Location.Reference) == ".installation.lock" || filepath.Base(m.Location.Reference) == ".lifecycle.lock") {
				continue
			}
			disposition := application.MaterialDisposition{MaterialID: m.ID, Disposition: "unknown"}
			if m.Verified {
				data, err := runtimeMutationOwnerBytes(m.Location.Reference, false)
				if err != nil {
					return p, err
				}
				p.protected[m.Location.Reference] = runtimeMutationRevision(data)
				zeroBytes(data)
				disposition.Disposition, disposition.Destination, disposition.Supported = "retained", m.Location, true
			}
			p.Materials = append(p.Materials, application.MaterialConsequence{Before: m, After: disposition, Meaning: "Remains at the original Target owner. No runtime or data removal is selected. Configuration restoration is not verified service access, secret delivery, certificate renewal or authentication continuity."})
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return p, runtimeMutationOwnerError()
	}
	records, err := deployment.ListDeployments(name)
	if err != nil {
		return p, invalidProviderConsumerInventory(err)
	}
	for _, record := range records {
		if err := authorizeMCPOperation(ctx, operation, name, record.Identity.Environment, record.Identity.ApplicationID, ""); err != nil {
			return p, err
		}
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "deployment/" + record.Identity.DeploymentID, Function: "target-control-and-access", Required: true, Outcome: "unknown", Behavior: "The original workload, canonical intent and provider bindings remain at their existing owners. Removing/restoring the Target definition changes BaseHarbor's configured access route; native workload availability, reconnection and authentication are not inferred.", Evidence: "existing Target deployment owner"})
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}).Load()
	if err != nil {
		return p, invalidProviderConsumerInventory(err)
	}
	for _, binding := range registry.Bindings {
		if err := authorizeMCPOperation(ctx, operation, name, binding.Environment, binding.ApplicationID, ""); err != nil {
			return p, err
		}
		p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "application/" + binding.ApplicationID + "/" + binding.Environment, Function: string(binding.Resource.Kind), Via: []string{binding.ProviderInstanceID}, Required: true, Outcome: "unknown", Behavior: "The authoritative binding remains unchanged at this Target. Provider processes are not stopped, revoked, replaced or handed off. Current delivery, lease/token validity, renewal and consumer reconnection remain unverified.", Evidence: "existing provider binding"})
	}
	p.Functions = append(p.Functions, application.FunctionConsequence{Consumer: "management/" + name, Function: "configured-target-access", Outcome: "unknown", Behavior: "Detachment removes the configuration access route while preserving runtime, enrollment, TLS/OIDC/mTLS and operator recovery owners. Restoration reuses the original references only; live access and authentication require verification. No alternate access or unauthenticated fallback is selected.", Evidence: "existing protected Target definition and referenced access owner"})
	p.Effects = []application.OperationalConsequence{{Aspect: "target-definition-" + p.Operation, Before: "Existing Target configuration, provider/deployment owners and runtime resources.", After: "Only the selected configuration entry is detached or restored. Unrelated Targets and shared access definitions remain. Persist protected original-definition recovery before detachment; preserve all data and runtime resources. Block automatic Target selection when its chosen entry is detached. No data deletion, credential rotation, native teardown or automatic provider handoff.", Evidence: "exact configuration snapshot and existing state owners"}}
	configPath, err := deployment.ConfigPath()
	if err != nil {
		return p, err
	}
	if p.Operation == "restore" {
		data, err := runtimeMutationOwnerBytes(recoveryFile, false)
		if err != nil {
			return p, err
		}
		p.protected[recoveryFile] = runtimeMutationRevision(data)
		zeroBytes(data)
	}
	p.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "target-definition", Reference: configPath, Subject: name, Revision: runtimeMutationRevision(struct {
		Plan      targetDefinitionChangePlan
		Config    []byte
		Selection []byte
		Protected map[string]string
	}{p, p.expected, p.selection, p.protected})}, nil, false)
	return p, err
}

func targetDefinitionChangeError(plan targetDefinitionChangePlan, err error) error {
	err = classifyProviderChangeConsent(err)
	var failure *machine.Error
	if errors.As(err, &failure) {
		failure.Details = plan
	}
	return err
}
