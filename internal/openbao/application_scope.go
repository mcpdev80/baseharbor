package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var ErrApplicationScopeUnavailable = errors.New("OpenBao application secret scope is unavailable")

type ApplicationIdentity struct {
	Name        string
	Environment string
}

type ApplicationCredentials struct {
	RoleID   string
	SecretID string
}

func ApplicationCredentialsPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, "openbao.env")
}

func EnsureApplicationScope(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string) error {
	if err := validateApplicationIdentity(identity); err != nil {
		return err
	}
	state, err := Inspect(ctx, executor, files)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrApplicationScopeUnavailable, err)
	}
	if !state.Initialized {
		return ErrNotInitialized
	}
	if state.Sealed {
		return ErrSealed
	}
	managerCredentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	managerToken, err := loginManager(ctx, executor, files, managerCredentials)
	if err != nil {
		return fmt.Errorf("authenticate OpenBao manager for application scope: %w", err)
	}

	policyName := applicationPolicyName(identity)
	roleName := applicationRoleName(identity)
	policy := applicationPolicy(identity)
	policyScript := fmt.Sprintf(`tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cat >"$tmp"
bao policy write %s "$tmp" >/dev/null`, policyName)
	if _, err := execWithTokenPayload(ctx, executor, files, managerToken, policyScript, policy); err != nil {
		return errors.New("OpenBao manager cannot provision application policies; bootstrap or reconcile the trust plane with the current BaseHarbor version")
	}

	roleCommand := fmt.Sprintf(`exec bao write auth/approle/role/%s token_policies=%s token_no_default_policy=true secret_id_ttl=0 secret_id_num_uses=0 token_ttl=15m token_max_ttl=1h`, roleName, policyName)
	if _, err := execWithToken(ctx, executor, files, managerToken, roleCommand); err != nil {
		return errors.New("OpenBao manager cannot provision application AppRoles; bootstrap or reconcile the trust plane with the current BaseHarbor version")
	}

	roleID, err := readApplicationRoleID(ctx, executor, files, managerToken, roleName)
	if err != nil {
		return err
	}
	credentials, err := loadApplicationCredentials(credentialsPath)
	createdCredentials := false
	if errors.Is(err, os.ErrNotExist) {
		credentials, err = issueApplicationCredentials(ctx, executor, files, managerToken, roleName, roleID)
		if err != nil {
			return err
		}
		if err := writeApplicationCredentials(credentialsPath, credentials); err != nil {
			return err
		}
		createdCredentials = true
	} else if err != nil {
		return err
	} else if credentials.RoleID != roleID {
		return errors.New("OpenBao application RoleID changed unexpectedly; refusing implicit credential rotation")
	}

	if err := verifyApplicationScope(ctx, executor, files, identity, credentials); err != nil {
		if createdCredentials {
			_ = os.Remove(credentialsPath)
		}
		return err
	}
	return nil
}

func CheckApplicationScope(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string) error {
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
	return verifyApplicationScope(ctx, executor, files, identity, credentials)
}

func DestroyApplicationScope(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity) error {
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
	managerCredentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	managerToken, err := loginManager(ctx, executor, files, managerCredentials)
	if err != nil {
		return err
	}

	for _, secretPath := range []string{applicationSecretPath(identity), applicationProbePath(identity)} {
		if _, err := execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`bao kv metadata delete -mount=baseharbor %s >/dev/null 2>&1 || true`, secretPath)); err != nil {
			return fmt.Errorf("delete OpenBao application secret metadata: %w", err)
		}
	}
	if _, err := execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`exec bao delete auth/approle/role/%s`, applicationRoleName(identity))); err != nil {
		return fmt.Errorf("delete OpenBao application AppRole: %w", err)
	}
	if _, err := execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`exec bao policy delete %s`, applicationPolicyName(identity))); err != nil {
		return fmt.Errorf("delete OpenBao application policy: %w", err)
	}
	return nil
}

func applicationPolicy(identity ApplicationIdentity) string {
	secretPath := applicationSecretPath(identity)
	probePath := applicationProbePath(identity)
	return applicationPathPolicy(secretPath) + "\n" + applicationPathPolicy(probePath)
}

func applicationPathPolicy(path string) string {
	return fmt.Sprintf(`path "baseharbor/data/%s" {
  capabilities = ["create", "update", "read", "delete"]
}

path "baseharbor/metadata/%s" {
  capabilities = ["read", "delete"]
}

path "baseharbor/delete/%s" {
  capabilities = ["update"]
}

path "baseharbor/undelete/%s" {
  capabilities = ["update"]
}

path "baseharbor/destroy/%s" {
  capabilities = ["update"]
}
`, path, path, path, path, path)
}

func applicationPolicyName(identity ApplicationIdentity) string {
	return "baseharbor-app-" + identity.Name + "-" + identity.Environment
}

func applicationRoleName(identity ApplicationIdentity) string {
	return applicationPolicyName(identity)
}

func applicationSecretPath(identity ApplicationIdentity) string {
	return "apps/" + identity.Name + "/" + identity.Environment
}

func applicationProbePath(identity ApplicationIdentity) string {
	return "apps/_baseharbor-probes/" + identity.Name + "/" + identity.Environment
}

func validateApplicationIdentity(identity ApplicationIdentity) error {
	for label, value := range map[string]string{"application name": identity.Name, "environment": identity.Environment} {
		if value == "" {
			return fmt.Errorf("%s is required", label)
		}
		for i, r := range value {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') || (r == '-' && (i == 0 || i == len(value)-1)) {
				return fmt.Errorf("invalid %s %q", label, value)
			}
		}
	}
	return nil
}

func readApplicationRoleID(ctx context.Context, executor Executor, files bhruntime.Files, managerToken, roleName string) (string, error) {
	out, err := execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`exec bao read -format=json auth/approle/role/%s/role-id`, roleName))
	if err != nil {
		return "", fmt.Errorf("read OpenBao application RoleID: %w", err)
	}
	var reply struct {
		Data struct {
			RoleID string `json:"role_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil || strings.TrimSpace(reply.Data.RoleID) == "" {
		return "", errors.New("read OpenBao application RoleID: invalid response")
	}
	return reply.Data.RoleID, nil
}

func issueApplicationCredentials(ctx context.Context, executor Executor, files bhruntime.Files, managerToken, roleName, roleID string) (ApplicationCredentials, error) {
	out, err := execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`exec bao write -format=json -f auth/approle/role/%s/secret-id`, roleName))
	if err != nil {
		return ApplicationCredentials{}, fmt.Errorf("create OpenBao application SecretID: %w", err)
	}
	var reply struct {
		Data struct {
			SecretID string `json:"secret_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil || strings.TrimSpace(reply.Data.SecretID) == "" {
		return ApplicationCredentials{}, errors.New("create OpenBao application SecretID: invalid response")
	}
	return ApplicationCredentials{RoleID: roleID, SecretID: reply.Data.SecretID}, nil
}

func loginApplication(ctx context.Context, executor Executor, files bhruntime.Files, credentials ApplicationCredentials) (string, error) {
	payload, err := json.Marshal(map[string]string{"role_id": credentials.RoleID, "secret_id": credentials.SecretID})
	if err != nil {
		return "", errors.New("encode OpenBao application AppRole login request")
	}
	out, err := executor.ExecProjectInput(ctx, projectName, files.Compose, files.Env, payload, serviceName, "bao", "write", "-format=json", "auth/approle/login", "-")
	if err != nil {
		return "", errors.New("OpenBao application AppRole login failed")
	}
	var reply struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil || strings.TrimSpace(reply.Auth.ClientToken) == "" {
		return "", errors.New("OpenBao application AppRole login returned an invalid response")
	}
	return reply.Auth.ClientToken, nil
}

func verifyApplicationScope(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentials ApplicationCredentials) error {
	token, err := loginApplication(ctx, executor, files, credentials)
	if err != nil {
		return err
	}
	path := applicationProbePath(identity)
	if _, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv put -mount=baseharbor %s __baseharbor_probe=ok`, path)); err != nil {
		return errors.New("OpenBao application identity cannot write its verification scope")
	}
	out, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv get -field=__baseharbor_probe -mount=baseharbor %s`, path))
	if err != nil || strings.TrimSpace(out) != "ok" {
		return errors.New("OpenBao application identity cannot read its verification scope")
	}
	if _, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv metadata delete -mount=baseharbor %s`, path)); err != nil {
		return errors.New("OpenBao application identity cannot clean its verification scope")
	}
	return nil
}

func loadApplicationCredentials(path string) (ApplicationCredentials, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ApplicationCredentials{}, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return ApplicationCredentials{}, fmt.Errorf("OpenBao application credential file is accessible by group or others (%o)", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ApplicationCredentials{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return ApplicationCredentials{}, errors.New("invalid OpenBao application credential file")
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	credentials := ApplicationCredentials{RoleID: values["OPENBAO_ROLE_ID"], SecretID: values["OPENBAO_SECRET_ID"]}
	if credentials.RoleID == "" || credentials.SecretID == "" {
		return ApplicationCredentials{}, errors.New("invalid OpenBao application credential file")
	}
	return credentials, nil
}

func writeApplicationCredentials(path string, credentials ApplicationCredentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create OpenBao application credential directory: %w", err)
	}
	content := fmt.Sprintf("OPENBAO_ROLE_ID=%s\nOPENBAO_SECRET_ID=%s\n", credentials.RoleID, credentials.SecretID)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("write OpenBao application credentials: %w", err)
	}
	keep := false
	defer func() {
		_ = f.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write OpenBao application credentials: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync OpenBao application credentials: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close OpenBao application credentials: %w", err)
	}
	keep = true
	return nil
}
