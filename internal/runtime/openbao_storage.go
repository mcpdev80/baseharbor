package runtime

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	openBaoStorageUserKey            = "BASEHARBOR_OPENBAO_DB_USER"
	openBaoStorageCredentialKey      = "BASEHARBOR_OPENBAO_DB_PASSWORD"
	postgresReplicationUserKey       = "BASEHARBOR_POSTGRES_REPLICATION_USER"
	postgresReplicationCredentialKey = "BASEHARBOR_POSTGRES_REPLICATION_PASSWORD"
)

func ensureOpenBaoStorageUser(envPath string) (string, error) {
	return ensureRuntimeFixedValue(envPath, openBaoStorageUserKey, "openbao")
}

func ensurePostgresReplicationUser(envPath string) (string, error) {
	return ensureRuntimeFixedValue(envPath, postgresReplicationUserKey, "baseharbor_replication")
}

func ensureOpenBaoStorageCredential(envPath string) (string, error) {
	return ensureRuntimeCredential(envPath, openBaoStorageCredentialKey)
}

func ensurePostgresReplicationCredential(envPath string) (string, error) {
	return ensureRuntimeCredential(envPath, postgresReplicationCredentialKey)
}

func ensureRuntimeCredential(envPath, credentialKey string) (string, error) {
	f, err := os.Open(envPath)
	if err != nil {
		return "", err
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	_ = f.Close()
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if value := values[credentialKey]; value != "" {
		return value, nil
	}
	value, err := randomSecret(32)
	if err != nil {
		return "", err
	}
	out, err := os.OpenFile(envPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := fmt.Fprintf(out, "%s=%s\n", credentialKey, value); err != nil {
		return "", err
	}
	return value, nil
}

func ensureRuntimeFixedValue(envPath, key, defaultValue string) (string, error) {
	f, err := os.Open(envPath)
	if err != nil {
		return "", err
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	_ = f.Close()
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if value := values[key]; value != "" {
		return value, nil
	}
	out, err := os.OpenFile(envPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := fmt.Fprintf(out, "%s=%s\\n", key, defaultValue); err != nil {
		return "", err
	}
	return defaultValue, nil
}
