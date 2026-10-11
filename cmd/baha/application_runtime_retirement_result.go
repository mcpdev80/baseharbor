package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"io"
)

func applicationDestroyResult(resolved resolvedApplication, fullReset bool) (applicationlifecycle.Result, error) {
	result := applicationlifecycle.NewResult("destroy", resolved.Manifest.Name, resolved.Manifest.Environment, "destroyed", true).WithTarget(resolved.Target.Name)
	if fullReset || isRemoteApplication(resolved) {
		if fullReset {
			deleted := true
			result.DataDeleted = &deleted
		}
		return result, nil
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		return result, err
	}
	if record.Observed.State != "runtime-retired" || record.Observed.VerifiedAt.IsZero() {
		return result, fmt.Errorf("retirement lacks verified retained deployment owner")
	}
	var receipt application.ProviderChangeResult
	if decodeRetirementReceipt(record.Applied.GeneratedState["provider_change_result"], &receipt) != nil {
		return result, fmt.Errorf("retirement consequence receipt is invalid")
	}
	if decodeRetirementReceipt(record.Applied.GeneratedState["retired_materials"], &result.RetainedMaterials) != nil {
		return result, fmt.Errorf("retirement material inventory is invalid")
	}
	if application.ValidateProviderChangeReceiptMetadata(receipt, result.RetainedMaterials) != nil {
		return result, fmt.Errorf("retirement receipt metadata is invalid")
	}
	deleted := false
	result.DataDeleted = &deleted
	result.State = "runtime-retired"
	result.OwnerPreserved = true
	result.ProviderChangeResult = &receipt
	result.Detail = "Owned runtime retired; persistent data and recovery owner retained. Consequence receipt remains partial where function, access or recovery usability is unverified."
	return result, nil
}
func decodeRetirementReceipt(raw string, value any) error {
	if len(raw) == 0 || len(raw) > 1<<20 {
		return fmt.Errorf("invalid retirement receipt size")
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid retirement receipt structure")
	}
	return nil
}
