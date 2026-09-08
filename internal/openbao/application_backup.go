package openbao

import (
	"context"
	"errors"
	"fmt"
	"sort"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// ApplicationSecretBackup is durable application secret state. Keys are the
// stable application-level names, including dyn-* references. Runtime tokens,
// AppRole credentials, broker projections and mTLS identities are deliberately
// outside this structure and must be regenerated during restore.
type ApplicationSecretBackup struct {
	Values map[string][]byte `json:"values"`
}

func ExportApplicationSecrets(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string) (ApplicationSecretBackup, error) {
	if err := InspectApplicationScope(ctx, executor, files, identity, credentialsPath); err != nil {
		return ApplicationSecretBackup{}, err
	}
	credentials, err := loadApplicationCredentials(credentialsPath)
	if err != nil {
		return ApplicationSecretBackup{}, err
	}
	token, err := loginApplication(ctx, executor, files, credentials)
	if err != nil {
		return ApplicationSecretBackup{}, err
	}
	keys, err := listApplicationSecretKeysWithToken(ctx, executor, files, token, identity)
	if err != nil {
		return ApplicationSecretBackup{}, err
	}
	values := make(map[string][]byte, len(keys))
	for _, key := range keys {
		value, err := readApplicationSecretValue(ctx, executor, files, token, applicationSecretKeyPath(identity, key))
		if err != nil {
			return ApplicationSecretBackup{}, fmt.Errorf("export application secret %s: %w", key, err)
		}
		values[key] = append([]byte(nil), value...)
	}
	return ApplicationSecretBackup{Values: values}, nil
}

func RestoreApplicationSecrets(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string, backup ApplicationSecretBackup) error {
	if backup.Values == nil {
		return errors.New("application secret backup is missing")
	}
	if err := EnsureApplicationSecretNamespace(ctx, executor, files, identity, credentialsPath); err != nil {
		return err
	}

	existing, err := ListApplicationSecretKeys(ctx, executor, files, identity, credentialsPath)
	if err != nil {
		return err
	}
	wanted := make(map[string]struct{}, len(backup.Values))
	keys := make([]string, 0, len(backup.Values))
	for key, value := range backup.Values {
		if err := validateApplicationSecretKey(key); err != nil {
			return fmt.Errorf("backup contains invalid application secret key: %w", err)
		}
		if len(value) == 0 || len(value) > maxApplicationSecretBytes {
			return fmt.Errorf("backup contains invalid value for application secret %s", key)
		}
		wanted[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Restore is authoritative for the application namespace. Remove stale
	// durable documents first so a restored application cannot accidentally see
	// secrets that were created after the backup point.
	for _, key := range existing {
		if _, ok := wanted[key]; ok {
			continue
		}
		if err := DeleteApplicationSecret(ctx, executor, files, identity, credentialsPath, key); err != nil {
			return fmt.Errorf("remove stale application secret %s during restore: %w", key, err)
		}
	}
	for _, key := range keys {
		if err := SetApplicationSecret(ctx, executor, files, identity, credentialsPath, key, backup.Values[key]); err != nil {
			return fmt.Errorf("restore application secret %s: %w", key, err)
		}
	}

	verified, err := ExportApplicationSecrets(ctx, executor, files, identity, credentialsPath)
	if err != nil {
		return fmt.Errorf("verify restored application secrets: %w", err)
	}
	if len(verified.Values) != len(backup.Values) {
		return errors.New("verify restored application secrets: key count differs")
	}
	for key, expected := range backup.Values {
		actual, ok := verified.Values[key]
		if !ok || string(actual) != string(expected) {
			return fmt.Errorf("verify restored application secret %s failed", key)
		}
	}
	return nil
}
