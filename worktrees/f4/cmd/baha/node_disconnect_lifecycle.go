package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

type nodeDisconnectAuthority interface {
	Status(context.Context, targetenrollment.Scope) (targetenrollment.NodeStatus, error)
	Disconnect(context.Context, targetenrollment.Scope) error
}
type nodeDisconnectResult struct {
	ContractVersion    string                                 `json:"contract_version"`
	TargetID           string                                 `json:"target_id"`
	NodeID             string                                 `json:"node_id"`
	State              string                                 `json:"state"`
	Revoked            bool                                   `json:"revoked"`
	CertificateRevoked bool                                   `json:"certificate_revoked"`
	Deleted            bool                                   `json:"deleted"`
	OwnerPreserved     bool                                   `json:"owner_preserved"`
	DataDeleted        bool                                   `json:"data_deleted"`
	CoreScope          *application.ProviderImpactScopeResult `json:"core_scope,omitempty"`
	Scope              application.ProviderImpactScopeResult  `json:"scope"`
	RetainedMaterials  []application.MaterialOutcome          `json:"retained_materials"`
}

func disconnectNodeWithImpact(ctx context.Context, authority nodeDisconnectAuthority, scope targetenrollment.Scope, raw json.RawMessage) (nodeDisconnectResult, error) {
	result := nodeDisconnectResult{ContractVersion: machine.ContractVersion, TargetID: scope.TargetID, NodeID: scope.NodeID, State: "partial", RetainedMaterials: []application.MaterialOutcome{}}
	status, err := authority.Status(ctx, scope)
	if err != nil {
		return result, machine.NewError(machine.ErrorProviderUnavailable, "Current node enrollment could not be verified.", "Inspect the existing Core enrollment authority before retrying.", true)
	}
	plan, err := planNodeDisconnect(scope, status)
	if err != nil {
		return result, err
	}
	consent := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID}
	if len(raw) != 0 && string(raw) != "null" {
		if machine.ValidateJSONObject(raw, 8192) != nil {
			return result, machine.NewError(machine.ErrorValidationFailed, "Invalid node impact acknowledgement.", "Supply one bounded unambiguous acknowledgement object.", false)
		}
		consent = application.ProviderChangeConsent{}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&consent) != nil || decoder.Decode(new(any)) != io.EOF {
			return result, machine.NewError(machine.ErrorValidationFailed, "Invalid node impact acknowledgement.", "Supply the concrete plan ID and explicit acknowledgement stages.", false)
		}
	}
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, consent); err != nil {
		return result, nodeDisconnectPlanError(plan, err)
	}
	// Re-read enrollment, binding and recovery owners after consent, before revocation.
	currentStatus, err := authority.Status(ctx, scope)
	if err != nil {
		return result, nodeDisconnectPlanError(plan, err)
	}
	current, err := planNodeDisconnect(scope, currentStatus)
	if err != nil {
		return result, nodeDisconnectPlanError(plan, err)
	}
	if current.Scope.PlanID != plan.Scope.PlanID {
		return result, nodeDisconnectPlanError(current, &application.ProviderChangeError{Code: "stale-impact-consent"})
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	mutationErr := authority.Disconnect(ctx, scope)
	actual, observationErr := authority.Status(ctx, scope)
	result.Revoked = observationErr == nil && actual.Revoked
	result.CertificateRevoked = mutationErr == nil && result.Revoked
	retained, retentionErr := planNodeDisconnect(scope, actual)
	result.OwnerPreserved = retentionErr == nil && retained.storageRevision == plan.storageRevision
	owner := application.ProviderScopeOwnerOutcome{Owner: plan.Scope.Owner, Disposition: "unknown", Evidence: "Post-mutation owner verification is incomplete."}
	if result.OwnerPreserved {
		owner.Disposition, owner.Verified, owner.Evidence = "retained", true, "Protected Target, provider bindings and deployment recovery owners are unchanged."
	}
	result.Scope, err = application.RecordProviderImpactScopeResult(plan.Scope, owner, nil)
	for _, material := range plan.Materials {
		result.RetainedMaterials = append(result.RetainedMaterials, application.MaterialOutcome{MaterialID: material.Before.ID, Disposition: owner.Disposition, Location: material.Before.Location, Recovery: material.Before.Recovery, Verified: result.OwnerPreserved, Evidence: owner.Evidence})
	}
	if mutationErr != nil || observationErr != nil || retentionErr != nil || err != nil || !result.Revoked || !result.OwnerPreserved {
		return result, &machine.Error{Code: machine.ErrorConflict, CauseCode: "node_disconnect_partial", Message: "Node disconnect is incomplete; review actual admission, certificate revocation and retained owners before retrying.", Details: struct {
			Plan   nodeDisconnectPlan   `json:"plan"`
			Result nodeDisconnectResult `json:"result"`
		}{plan, result}}
	}
	result.State = "disconnected"
	return result, nil
}
func nodeDisconnectPlanError(plan nodeDisconnectPlan, err error) error {
	classified := classifyProviderChangeConsent(err)
	var failure *machine.Error
	if !errors.As(classified, &failure) {
		failure = machine.NewError(machine.ErrorConflict, "Node disconnect ownership or enrollment changed.", "Inspect the current concrete plan and retry with renewed acknowledgement.", false)
	}
	failure.Details = plan
	return failure
}
