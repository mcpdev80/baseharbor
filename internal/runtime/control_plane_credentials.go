package runtime

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ControlPlaneCredentials struct {
	PostgresUser             string `json:"postgres_user"`
	PostgresPassword         string `json:"postgres_password"`
	PostgresInternalUser     string `json:"postgres_internal_user"`
	PostgresInternalPassword string `json:"postgres_internal_password"`
	PostgresReplicationUser  string `json:"postgres_replication_user"`
	PostgresReplicationPass  string `json:"postgres_replication_password"`
	OpenBaoDBUser            string `json:"openbao_db_user"`
	OpenBaoDBPassword        string `json:"openbao_db_password"`
}

type ControlPlaneCredentialRotationPhase string

const (
	ControlPlaneRotationPrepared  ControlPlaneCredentialRotationPhase = "PREPARED"
	ControlPlaneRotationProjected ControlPlaneCredentialRotationPhase = "PROJECTED"
	ControlPlaneRotationVerified  ControlPlaneCredentialRotationPhase = "VERIFIED"
	ControlPlaneRotationRetired   ControlPlaneCredentialRotationPhase = "RETIRED"
)

type ControlPlaneCredentialRotationState struct {
	Version  int                                 `json:"version"`
	Phase    ControlPlaneCredentialRotationPhase `json:"phase"`
	Previous ControlPlaneCredentials             `json:"previous"`
	Next     ControlPlaneCredentials             `json:"next"`
}

const controlPlaneCredentialRotationStateName = "control-plane-credential-rotation.json"

func LoadControlPlaneCredentials(files Files) (ControlPlaneCredentials, error) {
	values, err := loadRuntimeEnvironment(files.Env)
	if err != nil {
		return ControlPlaneCredentials{}, err
	}
	credentials := ControlPlaneCredentials{
		PostgresUser:             values["BASEHARBOR_POSTGRES_USER"],
		PostgresPassword:         values["BASEHARBOR_POSTGRES_PASSWORD"],
		PostgresInternalUser:     values["BASEHARBOR_POSTGRES_INTERNAL_USER"],
		PostgresInternalPassword: values["BASEHARBOR_POSTGRES_INTERNAL_PASSWORD"],
		PostgresReplicationUser:  values["BASEHARBOR_POSTGRES_REPLICATION_USER"],
		PostgresReplicationPass:  values["BASEHARBOR_POSTGRES_REPLICATION_PASSWORD"],
		OpenBaoDBUser:            values["BASEHARBOR_OPENBAO_DB_USER"],
		OpenBaoDBPassword:        values["BASEHARBOR_OPENBAO_DB_PASSWORD"],
	}
	if credentials.PostgresInternalUser == "" {
		credentials.PostgresInternalUser = "postgres"
	}
	if credentials.PostgresReplicationUser == "" {
		credentials.PostgresReplicationUser = "baseharbor_replication"
	}
	if credentials.OpenBaoDBUser == "" {
		credentials.OpenBaoDBUser = "openbao"
	}
	if credentials.PostgresUser == "" || credentials.PostgresPassword == "" ||
		credentials.PostgresInternalUser == "" || credentials.PostgresInternalPassword == "" ||
		credentials.PostgresReplicationUser == "" || credentials.PostgresReplicationPass == "" ||
		credentials.OpenBaoDBUser == "" || credentials.OpenBaoDBPassword == "" {
		return ControlPlaneCredentials{}, errors.New("control-plane credential state is incomplete")
	}
	return credentials, nil
}

func ReplaceControlPlaneCredentials(files Files, next ControlPlaneCredentials) error {
	if next.PostgresUser == "" || next.PostgresPassword == "" ||
		next.PostgresInternalUser == "" || next.PostgresInternalPassword == "" ||
		next.PostgresReplicationUser == "" || next.PostgresReplicationPass == "" ||
		next.OpenBaoDBUser == "" || next.OpenBaoDBPassword == "" {
		return errors.New("replacement control-plane credentials are incomplete")
	}
	updates := map[string]string{
		"BASEHARBOR_POSTGRES_USER":                 next.PostgresUser,
		"BASEHARBOR_POSTGRES_PASSWORD":             next.PostgresPassword,
		"BASEHARBOR_POSTGRES_INTERNAL_USER":        next.PostgresInternalUser,
		"BASEHARBOR_POSTGRES_INTERNAL_PASSWORD":    next.PostgresInternalPassword,
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

func LoadControlPlaneCredentialRotation(files Files) (ControlPlaneCredentialRotationState, bool, error) {
	path := filepath.Join(filepath.Dir(files.Env), controlPlaneCredentialRotationStateName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ControlPlaneCredentialRotationState{}, false, nil
	}
	if err != nil {
		return ControlPlaneCredentialRotationState{}, false, err
	}
	var state ControlPlaneCredentialRotationState
	if err := json.Unmarshal(data, &state); err != nil {
		return ControlPlaneCredentialRotationState{}, false, fmt.Errorf("decode control-plane credential rotation state: %w", err)
	}
	if state.Version != 1 {
		return ControlPlaneCredentialRotationState{}, false, fmt.Errorf("unsupported control-plane credential rotation state version %d", state.Version)
	}
	switch state.Phase {
	case ControlPlaneRotationPrepared, ControlPlaneRotationProjected, ControlPlaneRotationVerified, ControlPlaneRotationRetired:
	default:
		return ControlPlaneCredentialRotationState{}, false, fmt.Errorf("invalid control-plane credential rotation phase %q", state.Phase)
	}
	if err := validateControlPlaneCredentials(state.Previous); err != nil {
		return ControlPlaneCredentialRotationState{}, false, fmt.Errorf("invalid previous control-plane credentials: %w", err)
	}
	if err := validateControlPlaneCredentials(state.Next); err != nil {
		return ControlPlaneCredentialRotationState{}, false, fmt.Errorf("invalid replacement control-plane credentials: %w", err)
	}
	return state, true, nil
}

func SaveControlPlaneCredentialRotation(files Files, state ControlPlaneCredentialRotationState) error {
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Version != 1 {
		return fmt.Errorf("unsupported control-plane credential rotation state version %d", state.Version)
	}
	switch state.Phase {
	case ControlPlaneRotationPrepared, ControlPlaneRotationProjected, ControlPlaneRotationVerified, ControlPlaneRotationRetired:
	default:
		return fmt.Errorf("invalid control-plane credential rotation phase %q", state.Phase)
	}
	if err := validateControlPlaneCredentials(state.Previous); err != nil {
		return err
	}
	if err := validateControlPlaneCredentials(state.Next); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, byte(10))
	path := filepath.Join(filepath.Dir(files.Env), controlPlaneCredentialRotationStateName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
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

func ClearControlPlaneCredentialRotation(files Files) error {
	path := filepath.Join(filepath.Dir(files.Env), controlPlaneCredentialRotationStateName)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func validateControlPlaneCredentials(credentials ControlPlaneCredentials) error {
	if credentials.PostgresUser == "" || credentials.PostgresPassword == "" ||
		credentials.PostgresInternalUser == "" || credentials.PostgresInternalPassword == "" ||
		credentials.PostgresReplicationUser == "" || credentials.PostgresReplicationPass == "" ||
		credentials.OpenBaoDBUser == "" || credentials.OpenBaoDBPassword == "" {
		return errors.New("control-plane credential state is incomplete")
	}
	return nil
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
