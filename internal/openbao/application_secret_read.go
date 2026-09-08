package openbao

import (
	"context"
	"errors"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// GetApplicationSecret returns one application-scoped secret value. Callers
// must already be authorized for the owning application; OpenBao credentials
// further confine the read to that application's namespace.
func GetApplicationSecret(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath, key string) ([]byte, error) {
	if err := validateApplicationSecretKey(key); err != nil {
		return nil, err
	}
	if err := InspectApplicationScope(ctx, executor, files, identity, credentialsPath); err != nil {
		return nil, err
	}
	credentials, err := loadApplicationCredentials(credentialsPath)
	if err != nil {
		return nil, err
	}
	token, err := loginApplication(ctx, executor, files, credentials)
	if err != nil {
		return nil, err
	}
	keys, err := listApplicationSecretKeysWithToken(ctx, executor, files, token, identity)
	if err != nil {
		return nil, err
	}
	if !containsApplicationSecretKey(keys, key) {
		return nil, ErrApplicationSecretNotFound
	}
	value, err := readApplicationSecretValue(ctx, executor, files, token, applicationSecretKeyPath(identity, key))
	if err != nil {
		return nil, errors.New("read application secret from OpenBao failed")
	}
	if len(value) == 0 {
		return nil, errors.New("application secret value is empty")
	}
	return value, nil
}
