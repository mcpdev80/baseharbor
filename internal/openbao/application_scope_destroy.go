package openbao

import (
	"context"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// DestroyVerifiedApplicationScope is the destructive application-scope entrypoint.
// It validates the exact BaseHarbor-managed policy and AppRole before any
// application secret metadata or identity object is removed.
func DestroyVerifiedApplicationScope(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity) error {
	if err := CheckApplicationScopeOwnership(ctx, executor, files, identity); err != nil {
		return err
	}
	return DestroyApplicationScope(ctx, executor, files, identity)
}
