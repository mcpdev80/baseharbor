package openbao

import (
	"context"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// DestroyVerifiedApplicationScope is the destructive application-scope entrypoint.
// It validates the exact BaseHarbor-managed policy and AppRole before any
// application secret metadata or identity object is removed.
func DestroyVerifiedApplicationScope(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity) error {
	rolePresent, policyPresent, err := inspectApplicationScopeOwnership(ctx, executor, files, identity, true)
	if err != nil {
		return err
	}
	return destroyApplicationScope(ctx, executor, files, identity, rolePresent, policyPresent)
}
