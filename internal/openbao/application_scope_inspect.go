package openbao

import (
	"context"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func InspectApplicationScope(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string) error {
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
	credentials, err := loadApplicationCredentials(credentialsPath)
	if err != nil {
		return err
	}
	if _, err := loginApplication(ctx, executor, files, credentials); err != nil {
		return err
	}
	return CheckApplicationScopeOwnership(ctx, executor, files, identity)
}
