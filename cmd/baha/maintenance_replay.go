package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// replayApplicationMaintenance answers one committed maintenance revision of the
// selected deployment through the production revision lookup, local and remote
// alike. The lookup reads the durable window and its history beneath the
// deployment root; after a destroy tore that root down it reads the committed
// tombstone instead. The route mutates nothing and never resolves a deployment
// from a client path, so the same revision always answers the same durable
// record and a lost response resolves to the operation it was.
func replayApplicationMaintenance(ctx context.Context, resolved resolvedApplication,
	options applicationMaintenanceOptions, out io.Writer) error {
	if !maintenanceSubmissionAllowed(ctx) {
		return maintenanceSubmissionCancelled()
	}
	record, found, err := maintenanceRevisionReplay(resolved, options.Revision)
	if err != nil {
		return err
	}
	if !found {
		return machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("The selected deployment has no durable maintenance record at revision %d.", options.Revision),
			"Inspect the durable window with baha app maintenance <NAME> --operation <backup|restore|update|destroy>; nothing was replayed.", false)
	}
	if options.Operation != "" && options.Operation != record.Operation {
		return machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("Revision %d records %s, not %s.", record.Revision, record.Operation, options.Operation),
			"Replay the revision with the operation it durably records.", false)
	}
	payload := maintenanceReplayView{
		ContractVersion: record.ContractVersion,
		ApplicationID:   record.ApplicationID,
		DeploymentID:    record.DeploymentID,
		Target:          record.Target,
		Environment:     record.Environment,
		Operation:       record.Operation,
		Revision:        record.Revision,
		Digest:          record.Digest,
		ArchiveBinding:  record.ArchiveBinding,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode maintenance replay: %w", err)
	}
	_, err = fmt.Fprintf(out, "%s\n", encoded)
	return err
}

// maintenanceRevisionReplay is the seam onto the production revision lookup: it
// resolves the deployment's state root the same way every other durable
// maintenance route does and asks the authority which operation it committed at
// one revision. The commit path itself runs the same lookup, so a replay of a
// committed revision never re-commits or re-stamps anything.
//
// It deliberately does NOT reload the deployment authority first: a successful
// destroy removed the deployment record, and a replay must answer the destroyed
// revision precisely because the record is gone. The durable binding itself is
// validated fail-closed by the lookup it calls.
var maintenanceRevisionReplay = func(resolved resolvedApplication, revision int) (application.MaintenanceAuthorityRecord, bool, error) {
	return application.LoadMaintenanceRevision(resolved.stateRoot(), revision)
}

// maintenanceReplayView is the machine-safe answer of one replayed revision:
// identity, operation, revision and bindings — never a server path and never
// secret material.
type maintenanceReplayView struct {
	ContractVersion string `json:"contract_version"`
	ApplicationID   string `json:"application_id"`
	DeploymentID    string `json:"deployment_id"`
	Target          string `json:"target"`
	Environment     string `json:"environment"`
	Operation       string `json:"operation"`
	Revision        int    `json:"revision"`
	Digest          string `json:"digest"`
	ArchiveBinding  string `json:"archive_binding,omitempty"`
}
