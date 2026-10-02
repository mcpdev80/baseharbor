package runtime

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ControlPlaneCredentials struct {
	PostgresUser            string
	PostgresPassword        string
	PostgresReplicationUser string
	PostgresReplicationPass string
	OpenBaoDBUser           string
	OpenBaoDBPassword       string
}

func LoadControlPlaneCredentials(files Files) (ControlPlaneCredentials, error) {
	values, err := loadRuntimeEnvironment(files.Env)
	if err != nil {
		return ControlPlaneCredentials{}, err
	}
	credentials := ControlPlaneCredentials{
		PostgresUser:            values["BASEHARBOR_POSTGRES_USER"],
		PostgresPassword:        values["BASEHARBOR_POSTGRES_PASSWORD"],
		PostgresReplicationUser: values["BASEHARBOR_POSTGRES_REPLICATION_USER"],
		PostgresReplicationPass: values["BASEHARBOR_POSTGRES_REPLICATION_PASSWORD"],
		OpenBaoDBUser:           values["BASEHARBOR_OPENBAO_DB_USER"],
		OpenBaoDBPassword:       values["BASEHARBOR_OPENBAO_DB_PASSWORD"],
	}
	if credentials.PostgresReplicationUser == "" {
		credentials.PostgresReplicationUser = "baseharbor_replication"
	}
	if credentials.OpenBaoDBUser == "" {
		credentials.OpenBaoDBUser = "openbao"
	}
	if credentials.PostgresUser == "" || credentials.PostgresPassword == "" ||
		credentials.PostgresReplicationUser == "" || credentials.PostgresReplicationPass == "" ||
		credentials.OpenBaoDBUser == "" || credentials.OpenBaoDBPassword == "" {
		return ControlPlaneCredentials{}, errors.New("control-plane credential state is incomplete")
	}
	return credentials, nil
}

func ReplaceControlPlaneCredentials(files Files, next ControlPlaneCredentials) error {
	if next.PostgresUser == "" || next.PostgresPassword == "" ||
		next.PostgresReplicationUser == "" || next.PostgresReplicationPass == "" ||
		next.OpenBaoDBUser == "" || next.OpenBaoDBPassword == "" {
		return errors.New("replacement control-plane credentials are incomplete")
	}
	updates := map[string]string{
		"BASEHARBOR_POSTGRES_USER":                 next.PostgresUser,
		"BASEHARBOR_POSTGRES_PASSWORD":             next.PostgresPassword,
		"BASEHARBOR_POSTGRES_REPLICATION_USER":     next.PostgresReplicationUser,
		"BASEHARBOR_POSTGRES_REPLICATION_PASSWORD": next.PostgresReplicationPass,
		"BASEHARBOR_OPENBAO_DB_USER":               next.OpenBaoDBUser,
		"BASEHARBOR_OPENBAO_DB_PASSWORD":           next.OpenBaoDBPassword,
	}
	if err := rewriteRuntimeEnvironment(files.Env, updates); err != nil {
		return err
	}
	if err := writeOpenBaoRuntimeConfig(filepath.Dir(files.Compose), next.OpenBaoDBUser, next.OpenBaoDBPassword); err != nil {
		return fmt.Errorf("write replacement OpenBao storage configuration: %w", err)
	}
	return nil
}

func RuntimeEnvironment(files Files) (map[string]string, error) {
	return loadRuntimeEnvironment(files.Env)
}

func loadRuntimeEnvironment(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid runtime environment line %q", line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func rewriteRuntimeEnvironment(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for i, line := range lines {
		key, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value, replace := updates[key]
		if !replace {
			continue
		}
		lines[i] = key + "=" + value
		seen[key] = true
	}
	for key, value := range updates {
		if !seen[key] {
			lines = append(lines, key+"="+value)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
