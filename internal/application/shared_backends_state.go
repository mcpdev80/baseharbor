package application

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadSharedBackendState(path, environment string) (sharedBackendState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return sharedBackendState{Version: sharedBackendStateVersion, Environment: environment, Applications: map[string]sharedBackendAppState{}}, nil
	}
	if err != nil {
		return sharedBackendState{}, err
	}
	var state sharedBackendState
	if err := json.Unmarshal(data, &state); err != nil {
		return sharedBackendState{}, err
	}
	if state.Version != sharedBackendStateVersion {
		return sharedBackendState{}, fmt.Errorf("unsupported shared backend state version %d", state.Version)
	}
	if state.Applications == nil {
		state.Applications = map[string]sharedBackendAppState{}
	}
	return state, nil
}

func writeSharedBackendState(path string, state sharedBackendState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeOwnerOnlyFile(path, data)
}

func sharedBackendApplicationKey(m Manifest) string {
	return sharedBackendToken(m.Name) + "/" + sharedBackendToken(m.Environment)
}

func sharedPostgresService(environment string) string {
	return "shared-postgres-" + sharedBackendToken(environment)
}

func sharedPostgresAlias() string { return "postgres-access" }

func sharedPostgresMemberService(environment string, ordinal int) string {
	return fmt.Sprintf("%s-member-%d", sharedPostgresService(environment), ordinal)
}

func sharedPostgresEtcdService(environment string, ordinal int) string {
	return fmt.Sprintf("%s-etcd-%d", sharedPostgresService(environment), ordinal)
}

func sharedValkeyService(m Manifest, instance string) string {
	return sharedValkeyServiceFor(m.Name, m.Environment, instance)
}

func sharedValkeyServiceFor(application, environment, instance string) string {
	return "shared-valkey-" + sharedBackendToken(application+"-"+environment+"-"+instance)
}

func sharedValkeyAccessService(m Manifest, instance string) string {
	return sharedValkeyAccessServiceFor(m.Name, m.Environment, instance)
}

func sharedValkeyAccessServiceFor(application, environment, instance string) string {
	return sharedValkeyServiceFor(application, environment, instance) + "-access"
}

func sharedValkeyAccessAlias(m Manifest, instance string) string {
	return sharedValkeyAccessService(m, instance)
}

func sharedValkeyPortEnv(m Manifest, instance string) string {
	return sharedValkeyPortEnvFor(m.Name, m.Environment, instance)
}

func sharedValkeyPortEnvFor(application, environment, instance string) string {
	return "SHARED_VALKEY_" + envInstanceToken(sharedBackendToken(application+"-"+environment+"-"+instance)) + "_HOST_PORT"
}

func sharedValkeyPasswordEnvFor(application, environment, instance string) string {
	return "SHARED_VALKEY_" + envInstanceToken(sharedBackendToken(application+"-"+environment+"-"+instance)) + "_PASSWORD"
}

func postgresContainerHostKey(instance string) string {
	return postgresRuntimeKey(instance, "CONTAINER_HOST")
}
func valkeyContainerHostKey(instance string) string {
	return valkeyRuntimeKey(instance, "CONTAINER_HOST")
}

func sharedBackendToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func sharedPostgresDatabaseName(m Manifest, instance string) string {
	base := "baha_" + sharedBackendToken(m.Name) + "_" + sharedBackendToken(m.Environment)
	if instance != defaultServiceInstance {
		base += "_" + sharedBackendToken(instance)
	}
	return postgresIdentifierWithHash(base, m.Name+"|"+m.Environment+"|"+instance+"|db")
}

func sharedPostgresRoleName(m Manifest, instance string) string {
	base := "baha_" + sharedBackendToken(m.Name) + "_" + sharedBackendToken(m.Environment)
	if instance != defaultServiceInstance {
		base += "_" + sharedBackendToken(instance)
	}
	return postgresIdentifierWithHash(base, m.Name+"|"+m.Environment+"|"+instance+"|role")
}

func postgresIdentifierWithHash(base, identity string) string {
	sum := sha256.Sum256([]byte(identity))
	suffix := fmt.Sprintf("_%x", sum[:4])
	base = strings.ReplaceAll(base, "-", "_")
	maxBase := 63 - len(suffix)
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return strings.Trim(base, "_") + suffix
}

func ensureSharedValkeyCredential(root, owner, instance string) (string, error) {
	token := sharedBackendToken(owner)
	if token == "" {
		token = "application"
	}
	name := token
	if strings.TrimSpace(instance) != "" {
		name += "-" + sharedBackendToken(instance)
	}
	ref := filepath.ToSlash(filepath.Join("credentials", "valkey", name+".password"))
	path := filepath.Join(root, filepath.FromSlash(ref))
	if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
		return ref, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	password, err := sharedBackendSecret(32)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := writeOwnerOnlyFile(path, []byte(password+"\n")); err != nil {
		return "", err
	}
	return ref, nil
}

func ensureSharedPostgresCredential(root, owner, instance string) (string, error) {
	token := sharedBackendToken(owner)
	if token == "" {
		token = "provider"
	}
	name := token
	if strings.TrimSpace(instance) != "" {
		name += "-" + sharedBackendToken(instance)
	}
	ref := filepath.ToSlash(filepath.Join("credentials", "postgres", name+".password"))
	path := filepath.Join(root, filepath.FromSlash(ref))
	if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
		return ref, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	password, err := sharedBackendSecret(32)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := writeOwnerOnlyFile(path, []byte(password+"\n")); err != nil {
		return "", err
	}
	return ref, nil
}

func readSharedBackendCredential(root, reference string) (string, error) {
	reference = filepath.Clean(filepath.FromSlash(strings.TrimSpace(reference)))
	if reference == "." || filepath.IsAbs(reference) || strings.HasPrefix(reference, ".."+string(filepath.Separator)) {
		return "", errors.New("shared backend credential reference is invalid")
	}
	data, err := os.ReadFile(filepath.Join(root, reference))
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", errors.New("shared backend credential is empty")
	}
	return value, nil
}

func removeSharedBackendCredential(root, reference string) error {
	reference = filepath.Clean(filepath.FromSlash(strings.TrimSpace(reference)))
	if reference == "." || filepath.IsAbs(reference) || strings.HasPrefix(reference, ".."+string(filepath.Separator)) {
		return errors.New("shared backend credential reference is invalid")
	}
	if err := os.Remove(filepath.Join(root, reference)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func sharedBackendSecret(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func quotePostgresIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quotePostgresLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func sharedBackendPostgresUIRequested(state sharedBackendState) bool {
	for _, app := range state.Applications {
		if app.SQLManagementUI {
			return true
		}
	}
	return false
}

func sharedBackendCacheUIRequested(state sharedBackendState) bool {
	for _, app := range state.Applications {
		if app.CacheManagementUI {
			return true
		}
	}
	return false
}
