package main

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// maintenanceLifecycleOwner is the single seam where a maintenance submission
// reaches an existing lifecycle owner. The client never implements backup,
// restore, update or destroy itself: it resolves the authority and hands the
// operation to the owner that already enforces consent. It also owns the durable
// maintenance window, because only the owner knows which operation actually
// completed. Tests replace it to prove a denied, cancelled or unsupported
// submission mutates nothing.
var maintenanceLifecycleOwner = func(ctx context.Context, operation string, resolved resolvedApplication, args []string, out, errOut io.Writer) error {
	switch operation {
	case "backup":
		return executeApplicationBackupWithMetadataLifecycle(ctx, resolved.Store, args, out, errOut)
	case "restore":
		return executeApplicationRestoreLifecycle(ctx, resolved.Store, args, out, errOut)
	case "update":
		return executeApplicationUpdateLifecycle(ctx, resolved.Store, args, out, errOut)
	case "destroy":
		return executeApplicationDestroyLifecycle(ctx, resolved.Store, args, out, errOut)
	default:
		return applicationMaintenanceUnknownOperation(operation)
	}
}

// maintenanceWindowOwner persists the durable window for one verified operation.
// It is a separate seam so a caller can prove an unsupported submission never
// reaches it, and so the binding an operator sees is the binding recorded.
var maintenanceWindowOwner = func(stateRoot string, record application.MaintenanceAuthorityRecord) (application.MaintenanceAuthorityRecord, bool, error) {
	return application.CommitMaintenanceAuthority(stateRoot, record)
}

// maintenanceContextDone reports whether the caller's submission was cancelled
// before it reached a lifecycle owner.
func maintenanceContextDone(ctx context.Context) bool {
	return ctx.Err() != nil
}

// isMissingDeploymentAuthority reports the two states where no installation owns
// the deployment yet: an absent record and a Target with no authoritative
// installation owner. Both must fail closed as an ownership problem rather than
// being routed around with a client-local path.
func isMissingDeploymentAuthority(err error) bool {
	var state *deployment.DeploymentRecordStateError
	if errors.As(err, &state) {
		return state.Kind == "incomplete" || state.Kind == "missing"
	}
	return errors.Is(err, deployment.ErrDeploymentOwnerMissing) || errors.Is(err, os.ErrNotExist)
}

// maintenanceRejectedBeforeTransport reports the typed refusal a remote
// submission gets when no qualified owner exists. It is separate from a
// transport failure so a caller can tell "not supported here" from "unreachable".
func maintenanceRejectedBeforeTransport(err error) bool {
	var failure *machine.Error
	return errors.As(err, &failure) && failure.Code == machine.ErrorCapabilityMissing
}
