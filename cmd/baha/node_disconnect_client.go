package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

func requestNodeDisconnect(ctx context.Context, record nodeRecord, path string, consent *application.ProviderChangeConsent) (nodeDisconnectResult, *nodeDisconnectPlan, error) {
	var result nodeDisconnectResult
	if validateCoreURL(record.CoreURL) != nil {
		return result, nil, nodeDisconnectOwnerError()
	}
	session, err := operatorauth.LoadSession(record.CoreTarget, record.Environment)
	if err != nil || !session.ValidAt(time.Now()) {
		return result, nil, machine.NewError(machine.ErrorAuthenticationFailed, "Node lifecycle requires a valid Core operator session.", "Log in to the registered Core Target/environment.", false)
	}
	client, err := nodeHTTPSClient(record.CAFile)
	if err != nil {
		return result, nil, machine.NewError(machine.ErrorProviderUnavailable, "Registered Core trust is unavailable.", "Recover the existing protected trust owner.", false)
	}
	payload, err := json.Marshal(struct {
		Target      string                             `json:"target_id"`
		Node        string                             `json:"node_id"`
		Environment string                             `json:"environment"`
		Consent     *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
	}{record.TargetID, record.NodeID, record.Environment, consent})
	if err != nil {
		return result, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(record.CoreURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return result, nil, nodeDisconnectOwnerError()
	}
	req.Header.Set("Authorization", "Bearer "+session.IDToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return result, nil, machine.NewError(machine.ErrorProviderUnavailable, "Core node impact request failed.", "Verify authenticated Core reachability and trust before retrying.", true)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || machine.ValidateJSONObject(data, 1<<20) != nil {
		return result, nil, errors.New("invalid Core node impact response")
	}
	if response.StatusCode != http.StatusOK {
		var failure machine.ErrorResult
		if json.Unmarshal(data, &failure) != nil || failure.ContractVersion != machine.ContractVersion || failure.Error == nil {
			return result, nil, errors.New("Core node impact request was not admitted")
		}
		var plan nodeDisconnectPlan
		details, _ := json.Marshal(failure.Error.Details)
		var partial struct {
			Result nodeDisconnectResult `json:"result"`
		}
		if json.Unmarshal(details, &partial) == nil && partial.Result.ContractVersion == machine.ContractVersion && partial.Result.TargetID == record.TargetID && partial.Result.NodeID == record.NodeID {
			result = partial.Result
		}
		if json.Unmarshal(details, &plan) == nil && plan.TargetID == record.TargetID && plan.NodeID == record.NodeID && plan.Scope.Version == application.ProviderImpactScopeVersion && plan.Scope.PlanID != "" {
			return result, &plan, failure.Error
		}
		return result, nil, failure.Error
	}
	if json.Unmarshal(data, &result) != nil || result.ContractVersion != machine.ContractVersion || result.TargetID != record.TargetID || result.NodeID != record.NodeID || !result.Revoked || result.Deleted || result.DataDeleted || !result.OwnerPreserved || result.State != "disconnected" {
		return result, nil, errors.New("Core node disconnect did not provide a verified retained-owner result")
	}
	return result, nil, nil
}

func localNodeDisconnectPlan(record nodeRecord, core nodeDisconnectPlan, before []byte) (nodeDisconnectPlan, error) {
	scope := targetenrollment.Scope{TenantID: record.TenantID, TargetID: record.TargetID, NodeID: record.NodeID, Runtime: record.Runtime}
	local, err := planNodeDisconnect(scope, targetenrollment.NodeStatus{})
	if err != nil {
		return core, err
	}
	if core.Scope.Owner.Subject != scope.Identity() {
		return core, nodeDisconnectOwnerError()
	}
	plan := core
	plan.Functions = append(append([]application.FunctionConsequence{}, core.Functions...), local.Functions...)
	plan.Materials = append([]application.MaterialConsequence{}, core.Materials...)
	for _, material := range local.Materials {
		material.Before.ID = "local/" + material.Before.ID
		material.After.MaterialID = material.Before.ID
		plan.Materials = append(plan.Materials, material)
	}
	path, err := nodeRecordPath(record.TargetID)
	if err != nil {
		return plan, err
	}
	plan.Materials = append(plan.Materials, nodeDisconnectRetainedMaterial(record.TargetID, "local-node-registration", path))
	plan.storageRevision = nodeDisconnectRevision(struct {
		Registration string
		Storage      string
	}{nodeDisconnectRevision(before), local.storageRevision})
	coreScope := core.Scope
	plan.CoreScope = &coreScope
	revision := nodeDisconnectRevision(struct {
		Core         application.ProviderImpactScope
		Local        application.ProviderImpactScope
		Registration string
		Functions    []application.FunctionConsequence
		Materials    []application.MaterialConsequence
	}{core.Scope, local.Scope, nodeDisconnectRevision(before), plan.Functions, plan.Materials})
	plan.Scope, err = application.ComposeProviderImpactScope(application.ProviderScopeOwner{Owner: "connector-and-local-recovery", Reference: path, Subject: scope.Identity(), Revision: revision}, nil, false)
	return plan, err
}

func runNodeDisconnectWithImpact(ctx context.Context, target string, format cliOutputFormat, out io.Writer) error {
	path, err := nodeRecordPath(target)
	if err != nil {
		return err
	}
	if err := inspectNodeDisconnectOwner(path, false); err != nil {
		return err
	}
	before, err := os.ReadFile(path)
	if err != nil {
		return nodeDisconnectOwnerError()
	}
	record, err := loadNodeRecord(target)
	if err != nil {
		return err
	}
	if err := authorizeCurrentMCPContext(ctx, "node.disconnect", target, record.Environment, ""); err != nil {
		return err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return err
	}
	_, core, planErr := requestNodeDisconnect(ctx, record, targetenrollment.DisconnectPlanPath, nil)
	var failure *machine.Error
	if !errors.As(planErr, &failure) || failure.Code != machine.ErrorApprovalRequired || core == nil {
		if planErr != nil {
			return planErr
		}
		return errors.New("Core did not provide a read-only node disconnect plan")
	}
	plan, err := localNodeDisconnectPlan(record, *core, before)
	if err != nil {
		return err
	}
	if format != outputJSON {
		fmt.Fprintln(out, "Achtung: review node access loss and all known Core/local recovery owners.")
		if err := writeJSON(out, plan); err != nil {
			return err
		}
	}
	if consent == nil && format != outputJSON && !noInput(ctx) && readerIsTerminal(nodeInput) {
		presentation := application.ProviderImpactPlan{PlanID: plan.Scope.PlanID, ConfirmationsRequired: plan.Scope.ConfirmationsRequired, Destructive: plan.Scope.Destructive}
		value, err := readProviderImpactAcknowledgements(bufio.NewReader(nodeInput), out, presentation)
		if err != nil {
			return err
		}
		consent = &value
	}
	if consent == nil {
		return nodeDisconnectPlanError(plan, application.ValidateProviderImpactScopeConsent(plan.Scope, application.ProviderChangeConsent{PlanID: plan.Scope.PlanID}))
	}
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, *consent); err != nil {
		return nodeDisconnectPlanError(plan, err)
	}
	if err := inspectNodeDisconnectOwner(path, false); err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, before) {
		return nodeDisconnectOwnerError()
	}
	revalidated, err := localNodeDisconnectPlan(record, *core, current)
	if err != nil {
		return err
	}
	if revalidated.Scope.PlanID != plan.Scope.PlanID {
		return nodeDisconnectPlanError(revalidated, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	// The aggregate acknowledgement explicitly includes this immutable Core plan.
	// Direct HTTP callers independently acknowledge the same Core authority.
	coreConsent := *consent
	coreConsent.PlanID = core.Scope.PlanID
	result, _, operationErr := requestNodeDisconnect(ctx, record, targetenrollment.DisconnectPath, &coreConsent)
	if operationErr != nil && result.ContractVersion != machine.ContractVersion {
		return operationErr
	}
	current, err = os.ReadFile(path)
	after, retentionErr := localNodeDisconnectPlan(record, *core, current)
	localRetained := err == nil && retentionErr == nil && bytes.Equal(current, before) && after.storageRevision == plan.storageRevision
	coreResult := result.Scope
	result.CoreScope = &coreResult
	result.OwnerPreserved = result.OwnerPreserved && localRetained
	owner := application.ProviderScopeOwnerOutcome{Owner: plan.Scope.Owner, Disposition: "unknown", Evidence: "Local recovery-owner verification is incomplete."}
	if localRetained {
		owner.Disposition, owner.Verified, owner.Evidence = "retained", true, "Local registration, Target bindings and deployment recovery owners remain unchanged."
	}
	result.Scope, err = application.RecordProviderImpactScopeResult(plan.Scope, owner, nil)
	for _, material := range plan.Materials[len(core.Materials):] {
		result.RetainedMaterials = append(result.RetainedMaterials, application.MaterialOutcome{MaterialID: material.Before.ID, Disposition: owner.Disposition, Location: material.Before.Location, Recovery: material.Before.Recovery, Verified: localRetained, Evidence: owner.Evidence})
	}
	if operationErr != nil || err != nil || !result.OwnerPreserved {
		result.State = "partial"
		return &machine.Error{Code: machine.ErrorConflict, Message: "Node disconnect is partial; review actual admission, certificate revocation and retained owners.", Details: result}
	}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "Node %s disconnected. Target and recovery owners remain; native data continuity is unverified.\n", record.NodeID)
	return writeJSON(out, result)
}
