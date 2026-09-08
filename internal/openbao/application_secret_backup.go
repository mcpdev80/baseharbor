package openbao

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const maxApplicationBackupSecrets = 4096

type ApplicationSecretBackup struct {
	Identity ApplicationIdentity
	Secrets  []ApplicationSecretBackupEntry
}

type ApplicationSecretBackupEntry struct {
	Key   string
	Value []byte
}

func ExportApplicationSecrets(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string) (ApplicationSecretBackup, error) {
	if err := validateApplicationIdentity(identity); err != nil {
		return ApplicationSecretBackup{}, err
	}
	keys, err := ListApplicationSecretKeys(ctx, executor, files, identity, credentialsPath)
	if err != nil {
		return ApplicationSecretBackup{}, err
	}
	if len(keys) > maxApplicationBackupSecrets {
		return ApplicationSecretBackup{}, fmt.Errorf("application secret backup contains too many keys: %d", len(keys))
	}
	backup := ApplicationSecretBackup{Identity: identity, Secrets: make([]ApplicationSecretBackupEntry, 0, len(keys))}
	for _, key := range keys {
		value, err := GetApplicationSecret(ctx, executor, files, identity, credentialsPath, key)
		if err != nil {
			return ApplicationSecretBackup{}, fmt.Errorf("export application secret %s: %w", key, err)
		}
		backup.Secrets = append(backup.Secrets, ApplicationSecretBackupEntry{Key: key, Value: append([]byte(nil), value...)})
	}
	return backup, nil
}

func RestoreApplicationSecrets(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string, backup ApplicationSecretBackup) error {
	if err := validateApplicationIdentity(identity); err != nil {
		return err
	}
	if backup.Identity != identity {
		return errors.New("OpenBao backup identity does not match restore target")
	}
	if err := validateApplicationSecretBackup(backup); err != nil {
		return err
	}
	existing, err := ListApplicationSecretKeys(ctx, executor, files, identity, credentialsPath)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		return errors.New("OpenBao restore target already contains application secrets")
	}

	created := make([]string, 0, len(backup.Secrets))
	rollback := func() {
		for i := len(created) - 1; i >= 0; i-- {
			_ = DeleteApplicationSecret(ctx, executor, files, identity, credentialsPath, created[i])
		}
	}
	for _, entry := range backup.Secrets {
		if err := SetApplicationSecret(ctx, executor, files, identity, credentialsPath, entry.Key, entry.Value); err != nil {
			rollback()
			return fmt.Errorf("restore application secret %s: %w", entry.Key, err)
		}
		created = append(created, entry.Key)
	}
	for _, entry := range backup.Secrets {
		value, err := GetApplicationSecret(ctx, executor, files, identity, credentialsPath, entry.Key)
		if err != nil || !bytes.Equal(value, entry.Value) {
			rollback()
			return fmt.Errorf("verify restored application secret %s failed", entry.Key)
		}
	}
	return nil
}

func validateApplicationSecretBackup(backup ApplicationSecretBackup) error {
	if err := validateApplicationIdentity(backup.Identity); err != nil {
		return err
	}
	if len(backup.Secrets) > maxApplicationBackupSecrets {
		return fmt.Errorf("application secret backup contains too many keys: %d", len(backup.Secrets))
	}
	seen := make(map[string]struct{}, len(backup.Secrets))
	for _, entry := range backup.Secrets {
		if err := validateApplicationSecretKey(entry.Key); err != nil {
			return err
		}
		if _, duplicate := seen[entry.Key]; duplicate {
			return fmt.Errorf("duplicate application secret backup key %q", entry.Key)
		}
		seen[entry.Key] = struct{}{}
		if len(entry.Value) == 0 {
			return fmt.Errorf("application secret backup key %q has an empty value", entry.Key)
		}
		if len(entry.Value) > maxApplicationSecretBytes {
			return fmt.Errorf("application secret backup key %q exceeds maximum size", entry.Key)
		}
	}
	return nil
}
