package openbao

import (
	"context"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// CheckApplicationProvisioning verifies the non-mutating prerequisites that can
// be established without granting the manager token extra system introspection
// privileges. The subsequent convergence step remains authoritative for the
// actual policy/AppRole mutation and fails closed if those writes are denied.
func CheckApplicationProvisioning(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity) error {
	if err := validateApplicationIdentity(identity); err != nil {
		return err
	}
	state, err := Inspect(ctx, executor, files)
	if err != nil {
		return err
	}
	if !state.Initialized {
		return ErrNotInitialized
	}
	if state.Sealed {
		return ErrSealed
	}
	credentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	if _, err := loginManager(ctx, executor, files, credentials); err != nil {
		return err
	}
	return nil
}
