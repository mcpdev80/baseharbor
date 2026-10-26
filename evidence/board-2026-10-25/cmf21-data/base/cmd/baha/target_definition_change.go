package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func changeTargetDefinition(ctx context.Context, name, recovery string, out io.Writer) (targetMutationResult, error) {
	result := targetMutationResult{ContractVersion: machine.ContractVersion, Name: name}
	plan, err := buildTargetDefinitionChangePlan(ctx, name, recovery)
	if err != nil {
		return result, err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return result, err
	}
	if out != nil {
		fmt.Fprintln(out, "Achtung: review this Target access change, retained data and actual recovery location.")
		if err := writeJSON(out, plan); err != nil {
			return result, err
		}
	}
	in := applicationInput(ctx, nil)
	if consent == nil && out != nil && !noInput(ctx) && in != nil && readerIsTerminal(in) {
		value, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, application.ProviderImpactPlan{PlanID: plan.Scope.PlanID, ConfirmationsRequired: plan.Scope.ConfirmationsRequired})
		if err != nil {
			return result, err
		}
		consent = &value
	}
	ack := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID}
	if consent != nil {
		ack = *consent
	}
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, ack); err != nil {
		return result, targetDefinitionChangeError(plan, err)
	}
	unlock, err := lockExistingTargetDefinitionLifecycle(name)
	if err != nil {
		return result, err
	}
	defer unlock()
	current, err := buildTargetDefinitionChangePlan(ctx, name, recovery)
	if err != nil {
		return result, err
	}
	if current.Scope.PlanID != plan.Scope.PlanID {
		return result, targetDefinitionChangeError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	result.RecoveryFile = plan.RecoveryFile
	if plan.Operation == "detach" {
		delete(plan.cfg.Targets, name)
		if plan.cfg.DefaultTarget == name {
			plan.cfg.DefaultTarget = ""
		}
		result.RecoveryFile, err = plan.cfg.SaveExpectedPreservingTarget(plan.expected, name, func() error {
			path, err := targetSelectionPath()
			if err != nil {
				return err
			}
			current, err := runtimeMutationOwnerBytes(path, true)
			if err != nil {
				return err
			}
			if (current == nil) != (plan.selection == nil) || !bytes.Equal(current, plan.selection) {
				return deployment.ErrConfigurationChanged
			}
			if plan.SelectionBlocked {
				return writeTargetSelectionExpected(path, plan.selection, []byte(detachedTargetSelectionPrefix+name+"\n"))
			}
			return nil
		})
	} else {
		plan.cfg, err = plan.recovery.RestoreInto(plan.cfg)
		if err == nil {
			err = plan.cfg.SaveExpected(plan.expected)
		}
	}
	if err != nil {
		return result, targetDefinitionChangePartial(result, err)
	}
	actual, _, err := deployment.LoadConfigForMutation()
	if err != nil {
		return result, targetDefinitionChangePartial(result, err)
	}
	if plan.Operation == "detach" {
		_, exists := actual.Targets[name]
		retained, readErr := deployment.LoadTargetDefinitionRecovery(result.RecoveryFile, name)
		if exists || readErr != nil || !reflect.DeepEqual(retained, plan.recovery) {
			return result, targetDefinitionChangePartial(result, errors.New("Target detachment/recovery verification failed"))
		}
		result.Deleted = true
	} else {
		if !reflect.DeepEqual(actual.Targets[name], plan.recovery.Definition) || actual.Access[plan.recovery.Definition.Access.Reference] != plan.recovery.Access {
			return result, targetDefinitionChangePartial(result, errors.New("Target restoration verification failed"))
		}
		result.Created = true
	}
	selectionPath, selectionErr := targetSelectionPath()
	selection, selectionReadErr := runtimeMutationOwnerBytes(selectionPath, true)
	expectedSelection := plan.selection
	if plan.Operation == "detach" && plan.SelectionBlocked {
		expectedSelection = []byte(detachedTargetSelectionPrefix + name + "\n")
	}
	verified := selectionErr == nil && selectionReadErr == nil && (selection == nil) == (expectedSelection == nil) && bytes.Equal(selection, expectedSelection)
	paths := []string{}
	for path := range plan.protected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		revision := plan.protected[path]
		data, err := runtimeMutationOwnerBytes(path, false)
		preserved := err == nil && runtimeMutationRevision(data) == revision
		zeroBytes(data)
		verified = verified && preserved
		result.RetainedMaterials = append(result.RetainedMaterials, application.MaterialOutcome{MaterialID: "target-definition/path/" + path, Disposition: "retained", Location: &application.ResourceLocation{Owner: "target/" + name, Target: name, Kind: "path", Reference: path}, Verified: preserved, Evidence: "Read-back of the acknowledged protected owner bytes; native data contents and function continuity are not inferred."})
	}
	ownerDisposition := "removed"
	if plan.Operation == "restore" {
		ownerDisposition = "retained"
	}
	scopeResult, scopeErr := application.RecordProviderImpactScopeResult(plan.Scope, application.ProviderScopeOwnerOutcome{Owner: plan.Scope.Owner, Disposition: ownerDisposition, Verified: true, Evidence: "Read-back verifies the selected configuration entry and protected original-definition recovery; unrelated configuration is preserved by exact-byte CAS."}, nil)
	result.ProviderChangeResult = &scopeResult
	if !verified || scopeErr != nil {
		return result, targetDefinitionChangePartial(result, scopeErr)
	}
	return result, nil
}

func targetDefinitionChangePartial(result targetMutationResult, err error) error {
	return &machine.Error{Code: machine.ErrorConflict, CauseCode: "target_definition_change_partial", Message: "Target definition change is partial; inspect the actual entry, protected recovery and selection before retrying or restoring.", Details: result, Cause: err}
}

func lockExistingTargetDefinitionLifecycle(name string) (func(), error) {
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return func() {}, nil
	}
	if err != nil || !info.IsDir() || !bhruntime.ProtectedStateOwner(root, info) {
		return nil, runtimeMutationOwnerError()
	}
	if _, err := runtimeMutationOwnerBytes(filepath.Join(root, ".installation.lock"), true); err != nil {
		return nil, err
	}
	return coreinstallation.AcquireLifecycleLock(root)
}
