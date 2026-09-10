package openbao

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	projectName     = "baseharbor"
	serviceName     = "openbao"
	adminFileName   = "openbao-admin.env"
	recoveryVersion = 1
)

var (
	ErrNotInitialized       = errors.New("OpenBao is not initialized")
	ErrAlreadyInitialized   = errors.New("OpenBao is already initialized")
	ErrSealed               = errors.New("OpenBao is sealed")
	ErrManagerNotConfigured = errors.New("OpenBao manager credentials are not configured")
	ErrInvalidRecoveryFile  = errors.New("invalid OpenBao recovery file")
)

// Executor is the narrow runtime boundary required for OpenBao lifecycle work.
// Sensitive input must be supplied through stdin rather than command arguments.
type Executor interface {
	ExecProject(ctx context.Context, project, composeFile, envFile, service string, args ...string) (string, error)
	ExecProjectInput(ctx context.Context, project, composeFile, envFile string, input []byte, service string, args ...string) (string, error)
}

type State struct {
	Initialized bool   `json:"initialized"`
	Sealed      bool   `json:"sealed"`
	Version     string `json:"version"`
}

type recoveryBundle struct {
	Version      int      `json:"version"`
	KeyShares    int      `json:"key_shares"`
	KeyThreshold int      `json:"key_threshold"`
	UnsealKeys   []string `json:"unseal_keys"`
}

type AdminCredentials struct {
	RoleID   string
	SecretID string
}

func Inspect(ctx context.Context, executor Executor, files bhruntime.Files) (State, error) {
	const script = `bao status -format=json 2>/dev/null
code=$?
if [ "$code" -eq 0 ] || [ "$code" -eq 2 ]; then
  exit 0
fi
exit "$code"`
	out, err := executor.ExecProject(ctx, projectName, files.Compose, files.Env, serviceName, "sh", "-c", script)
	if err != nil {
		return State{}, fmt.Errorf("inspect OpenBao status: %w", err)
	}
	var state State
	if err := json.Unmarshal([]byte(out), &state); err != nil {
		return State{}, errors.New("inspect OpenBao status: invalid status response")
	}
	return state, nil
}

func AdminCredentialsPath(files bhruntime.Files) string {
	return filepath.Join(filepath.Dir(files.Env), adminFileName)
}

// Bootstrap initializes the single-node Shamir seal workflow, writes only the
// unseal material to an operator-selected recovery file, configures a KV v2
// mount plus a least-privilege manager AppRole, and revokes the initial root
// token after the manager identity has been verified.
func Bootstrap(ctx context.Context, executor Executor, files bhruntime.Files, recoveryPath string) error {
	state, err := Inspect(ctx, executor, files)
	if err != nil {
		return err
	}
	if state.Initialized {
		return ErrAlreadyInitialized
	}

	adminPath := AdminCredentialsPath(files)
	if _, err := os.Stat(adminPath); err == nil {
		return fmt.Errorf("OpenBao manager credential state already exists at %s", adminPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect OpenBao manager credentials: %w", err)
	}

	recoveryPath, recoveryFile, err := reserveRecoveryFile(files, recoveryPath)
	if err != nil {
		return err
	}
	keepRecoveryFile := false
	defer func() {
		_ = recoveryFile.Close()
		if !keepRecoveryFile {
			_ = os.Remove(recoveryPath)
		}
	}()

	out, err := executor.ExecProject(ctx, projectName, files.Compose, files.Env, serviceName,
		"bao", "operator", "init", "-key-shares=1", "-key-threshold=1", "-format=json")
	if err != nil {
		return fmt.Errorf("initialize OpenBao: %w", err)
	}
	var initReply struct {
		UnsealKeys []string `json:"unseal_keys_b64"`
		RootToken  string   `json:"root_token"`
	}
	if err := json.Unmarshal([]byte(out), &initReply); err != nil || len(initReply.UnsealKeys) != 1 || strings.TrimSpace(initReply.RootToken) == "" {
		return errors.New("initialize OpenBao: invalid initialization response")
	}

	bundle := recoveryBundle{
		Version:      recoveryVersion,
		KeyShares:    1,
		KeyThreshold: 1,
		UnsealKeys:   []string{initReply.UnsealKeys[0]},
	}
	if err := json.NewEncoder(recoveryFile).Encode(bundle); err != nil {
		return fmt.Errorf("write OpenBao recovery file: %w", err)
	}
	if err := recoveryFile.Sync(); err != nil {
		return fmt.Errorf("sync OpenBao recovery file: %w", err)
	}
	if err := recoveryFile.Close(); err != nil {
		return fmt.Errorf("close OpenBao recovery file: %w", err)
	}
	keepRecoveryFile = true

	if err := unsealWithKey(ctx, executor, files, initReply.UnsealKeys[0]); err != nil {
		return err
	}

	rootToken := initReply.RootToken
	if err := configureManager(ctx, executor, files, rootToken); err != nil {
		return err
	}
	credentials, err := issueManagerCredentials(ctx, executor, files, rootToken)
	if err != nil {
		return err
	}
	if err := writeAdminCredentials(adminPath, credentials); err != nil {
		return err
	}

	managerToken, err := loginManager(ctx, executor, files, credentials)
	if err != nil {
		return fmt.Errorf("verify OpenBao manager authentication: %w", err)
	}
	if err := verifyManagerKV(ctx, executor, files, managerToken); err != nil {
		return fmt.Errorf("verify OpenBao manager KV access: %w", err)
	}

	if _, err := execWithToken(ctx, executor, files, rootToken, `exec bao token revoke -self`); err != nil {
		return fmt.Errorf("revoke initial OpenBao root token: %w", err)
	}
	rootToken = ""
	initReply.RootToken = ""

	if err := CheckManager(ctx, executor, files); err != nil {
		return fmt.Errorf("verify OpenBao manager after root revocation: %w", err)
	}
	return nil
}

func Unseal(ctx context.Context, executor Executor, files bhruntime.Files, recoveryPath string) error {
	state, err := Inspect(ctx, executor, files)
	if err != nil {
		return err
	}
	if !state.Initialized {
		return ErrNotInitialized
	}
	if !state.Sealed {
		return nil
	}
	bundle, err := loadRecoveryFile(recoveryPath)
	if err != nil {
		return err
	}
	return unsealWithKey(ctx, executor, files, bundle.UnsealKeys[0])
}

func CheckManager(ctx context.Context, executor Executor, files bhruntime.Files) error {
	credentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	_, err = loginManager(ctx, executor, files, credentials)
	return err
}

func LoadAdminCredentials(files bhruntime.Files) (AdminCredentials, error) {
	path := AdminCredentialsPath(files)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AdminCredentials{}, ErrManagerNotConfigured
		}
		return AdminCredentials{}, fmt.Errorf("inspect OpenBao manager credentials: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return AdminCredentials{}, fmt.Errorf("OpenBao manager credential file is accessible by group or others (%o)", info.Mode().Perm())
	}
	f, err := os.Open(path)
	if err != nil {
		return AdminCredentials{}, fmt.Errorf("open OpenBao manager credentials: %w", err)
	}
	defer f.Close()
	values := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return AdminCredentials{}, ErrManagerNotConfigured
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := s.Err(); err != nil {
		return AdminCredentials{}, fmt.Errorf("read OpenBao manager credentials: %w", err)
	}
	credentials := AdminCredentials{RoleID: values["OPENBAO_ROLE_ID"], SecretID: values["OPENBAO_SECRET_ID"]}
	if credentials.RoleID == "" || credentials.SecretID == "" {
		return AdminCredentials{}, ErrManagerNotConfigured
	}
	return credentials, nil
}

func reserveRecoveryFile(files bhruntime.Files, requested string) (string, *os.File, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", nil, errors.New("OpenBao recovery file path is required")
	}
	abs, err := filepath.Abs(requested)
	if err != nil {
		return "", nil, fmt.Errorf("resolve OpenBao recovery file path: %w", err)
	}
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", nil, fmt.Errorf("create OpenBao recovery directory: %w", err)
	}

	stateRoot, err := recoveryProtectedStateRoot(files)
	if err != nil {
		return "", nil, err
	}
	stateRoot, err = filepath.EvalSymlinks(stateRoot)
	if err != nil {
		return "", nil, fmt.Errorf("resolve BaseHarbor state symlinks: %w", err)
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", nil, fmt.Errorf("resolve OpenBao recovery directory symlinks: %w", err)
	}
	realTarget := filepath.Join(realParent, filepath.Base(abs))
	rel, err := filepath.Rel(stateRoot, realTarget)
	if err != nil {
		return "", nil, fmt.Errorf("compare OpenBao recovery file path: %w", err)
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return "", nil, errors.New("OpenBao recovery file must be stored outside BaseHarbor state")
	}

	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("reserve OpenBao recovery file: %w", err)
	}
	return abs, f, nil
}

func recoveryProtectedStateRoot(files bhruntime.Files) (string, error) {
	runtimeDir, err := filepath.Abs(filepath.Dir(files.Compose))
	if err != nil {
		return "", fmt.Errorf("resolve BaseHarbor runtime state path: %w", err)
	}
	base := filepath.Base(runtimeDir)
	parent := filepath.Dir(runtimeDir)
	if base == "runtime" {
		parentBase := filepath.Base(parent)
		if parentBase == ".baseharbor" || parentBase == "baseharbor" {
			return parent, nil
		}
	}
	return runtimeDir, nil
}

func loadRecoveryFile(path string) (recoveryBundle, error) {
	info, err := os.Stat(path)
	if err != nil {
		return recoveryBundle{}, fmt.Errorf("inspect OpenBao recovery file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return recoveryBundle{}, fmt.Errorf("OpenBao recovery file is accessible by group or others (%o)", info.Mode().Perm())
	}
	f, err := os.Open(path)
	if err != nil {
		return recoveryBundle{}, fmt.Errorf("open OpenBao recovery file: %w", err)
	}
	defer f.Close()
	var bundle recoveryBundle
	if err := json.NewDecoder(f).Decode(&bundle); err != nil {
		return recoveryBundle{}, ErrInvalidRecoveryFile
	}
	if bundle.Version != recoveryVersion || bundle.KeyShares != 1 || bundle.KeyThreshold != 1 || len(bundle.UnsealKeys) != 1 || strings.TrimSpace(bundle.UnsealKeys[0]) == "" {
		return recoveryBundle{}, ErrInvalidRecoveryFile
	}
	return bundle, nil
}

func unsealWithKey(ctx context.Context, executor Executor, files bhruntime.Files, key string) error {
	payload, err := json.Marshal(map[string]string{"key": key})
	if err != nil {
		return errors.New("encode OpenBao unseal request")
	}
	if _, err := executor.ExecProjectInput(ctx, projectName, files.Compose, files.Env, payload, serviceName,
		"bao", "write", "-format=json", "sys/unseal", "-"); err != nil {
		return fmt.Errorf("unseal OpenBao: %w", err)
	}
	state, err := Inspect(ctx, executor, files)
	if err != nil {
		return err
	}
	if state.Sealed {
		return ErrSealed
	}
	return nil
}

func configureManager(ctx context.Context, executor Executor, files bhruntime.Files, rootToken string) error {
	commands := []string{
		`exec bao secrets enable -path=baseharbor -version=2 kv`,
		`exec bao auth enable approle`,
	}
	for _, command := range commands {
		if _, err := execWithToken(ctx, executor, files, rootToken, command); err != nil {
			return fmt.Errorf("configure OpenBao trust plane: %w", err)
		}
	}

	const policyScript = `tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cat >"$tmp"
bao policy write baseharbor-manager "$tmp" >/dev/null`
	if _, err := execWithTokenPayload(ctx, executor, files, rootToken, policyScript, managerPolicy); err != nil {
		return fmt.Errorf("configure OpenBao manager policy: %w", err)
	}

	const roleCommand = `exec bao write auth/approle/role/baseharbor-manager token_policies=baseharbor-manager token_no_default_policy=true secret_id_ttl=0 secret_id_num_uses=0 token_ttl=15m token_max_ttl=1h`
	if _, err := execWithToken(ctx, executor, files, rootToken, roleCommand); err != nil {
		return fmt.Errorf("configure OpenBao manager AppRole: %w", err)
	}
	return nil
}

func issueManagerCredentials(ctx context.Context, executor Executor, files bhruntime.Files, rootToken string) (AdminCredentials, error) {
	out, err := execWithToken(ctx, executor, files, rootToken, `exec bao read -format=json auth/approle/role/baseharbor-manager/role-id`)
	if err != nil {
		return AdminCredentials{}, fmt.Errorf("read OpenBao manager RoleID: %w", err)
	}
	var roleReply struct {
		Data struct {
			RoleID string `json:"role_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &roleReply); err != nil || strings.TrimSpace(roleReply.Data.RoleID) == "" {
		return AdminCredentials{}, errors.New("read OpenBao manager RoleID: invalid response")
	}

	out, err = execWithToken(ctx, executor, files, rootToken, `exec bao write -format=json -f auth/approle/role/baseharbor-manager/secret-id`)
	if err != nil {
		return AdminCredentials{}, fmt.Errorf("create OpenBao manager SecretID: %w", err)
	}
	var secretReply struct {
		Data struct {
			SecretID string `json:"secret_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &secretReply); err != nil || strings.TrimSpace(secretReply.Data.SecretID) == "" {
		return AdminCredentials{}, errors.New("create OpenBao manager SecretID: invalid response")
	}
	return AdminCredentials{RoleID: roleReply.Data.RoleID, SecretID: secretReply.Data.SecretID}, nil
}

func writeAdminCredentials(path string, credentials AdminCredentials) (err error) {
	content := fmt.Sprintf("OPENBAO_ROLE_ID=%s\nOPENBAO_SECRET_ID=%s\n", credentials.RoleID, credentials.SecretID)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("write OpenBao manager credentials: %w", err)
	}
	keep := false
	defer func() {
		_ = f.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write OpenBao manager credentials: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync OpenBao manager credentials: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close OpenBao manager credentials: %w", err)
	}
	keep = true
	return nil
}

func loginManager(ctx context.Context, executor Executor, files bhruntime.Files, credentials AdminCredentials) (string, error) {
	payload, err := json.Marshal(map[string]string{"role_id": credentials.RoleID, "secret_id": credentials.SecretID})
	if err != nil {
		return "", errors.New("encode OpenBao AppRole login request")
	}
	out, err := executor.ExecProjectInput(ctx, projectName, files.Compose, files.Env, payload, serviceName,
		"bao", "write", "-format=json", "auth/approle/login", "-")
	if err != nil {
		return "", errors.New("OpenBao AppRole login failed")
	}
	var reply struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil || strings.TrimSpace(reply.Auth.ClientToken) == "" {
		return "", errors.New("OpenBao AppRole login returned an invalid response")
	}
	return reply.Auth.ClientToken, nil
}

func verifyManagerKV(ctx context.Context, executor Executor, files bhruntime.Files, token string) error {
	if _, err := execWithToken(ctx, executor, files, token, `exec bao kv put -mount=baseharbor apps/_baseharbor/bootstrap-probe value=ok`); err != nil {
		return errors.New("OpenBao manager cannot write application secrets")
	}
	out, err := execWithToken(ctx, executor, files, token, `exec bao kv get -field=value -mount=baseharbor apps/_baseharbor/bootstrap-probe`)
	if err != nil || strings.TrimSpace(out) != "ok" {
		return errors.New("OpenBao manager cannot read application secrets")
	}
	if _, err := execWithToken(ctx, executor, files, token, `exec bao kv metadata delete -mount=baseharbor apps/_baseharbor/bootstrap-probe`); err != nil {
		return errors.New("OpenBao manager cannot delete application secret metadata")
	}
	return nil
}

func execWithToken(ctx context.Context, executor Executor, files bhruntime.Files, token, command string) (string, error) {
	return execWithTokenPayload(ctx, executor, files, token, command, "")
}

func execWithTokenPayload(ctx context.Context, executor Executor, files bhruntime.Files, token, command, payload string) (string, error) {
	const prefix = `IFS= read -r BAO_TOKEN
export BAO_TOKEN
`
	input := []byte(token + "\n" + payload)
	return executor.ExecProjectInput(ctx, projectName, files.Compose, files.Env, input, serviceName, "sh", "-ceu", prefix+command)
}

const managerPolicy = `path "baseharbor/data/apps/*" {
  capabilities = ["create", "update", "read", "delete"]
}

path "baseharbor/metadata/apps/*" {
  capabilities = ["read", "list", "delete"]
}

path "baseharbor/delete/apps/*" {
  capabilities = ["update"]
}

path "baseharbor/undelete/apps/*" {
  capabilities = ["update"]
}

path "baseharbor/destroy/apps/*" {
  capabilities = ["update"]
}

path "sys/policies/acl/baseharbor-app-*" {
  capabilities = ["create", "update", "read", "delete"]
}

path "auth/approle/role/baseharbor-app-*" {
  capabilities = ["create", "update", "read", "delete"]
}
`
