package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func hasManagementUI(m Manifest) bool {
	services := m.Services
	return services.SQLManagementUI || services.CacheManagementUI || services.KeyValueManagementUI ||
		services.MessagingManagementUI || services.DocumentDatabaseManagementUI ||
		services.ObjectStorageManagementUI || services.SecretsManagementUI ||
		services.IdentityManagementUI || services.ObservabilityManagementUI || HasIdentity(m)
}

// MaterializeManagementUIEndpointBinding publishes non-secret canonical and
// direct-IP browser URLs to the application's read-only workload bindings.
func MaterializeManagementUIEndpointBinding(m Manifest, files RuntimeFiles, canonical, directIP map[string]string) error {
	if !hasManagementUI(m) {
		return nil
	}
	if files.Dir == "" {
		return errors.New("application runtime directory is required for management UI endpoint binding")
	}
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		return fmt.Errorf("encode canonical management UI endpoints: %w", err)
	}
	directIPJSON, err := json.Marshal(directIP)
	if err != nil {
		return fmt.Errorf("encode direct-IP management UI endpoints: %w", err)
	}
	return writeManagementUIBinding(filepath.Join(workloadServiceBindingProjectionDir(files), "management-ui"), map[string]string{
		"type":     "management-ui",
		"provider": "baseharbor",
		"urls":     string(canonicalJSON),
		"ip_urls":  string(directIPJSON),
	})
}

func writeManagementUIBinding(dir string, entries map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		return err
	}
	for name, value := range entries {
		if value == "" {
			return fmt.Errorf("management UI binding %s is empty", name)
		}
		target := filepath.Join(dir, name)
		tmp := target + ".tmp"
		if err := os.WriteFile(tmp, []byte(value+"\n"), 0o600); err != nil {
			return err
		}
		if err := os.Chmod(tmp, 0o444); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	return nil
}
