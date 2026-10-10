package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func loginManager(ctx context.Context, executor Executor, files bhruntime.Files, credentials AdminCredentials) (string, error) {
	payload, err := json.Marshal(map[string]string{"role_id": credentials.RoleID, "secret_id": credentials.SecretID})
	if err != nil {
		return "", errors.New("encode OpenBao AppRole login request")
	}
	out, err := executor.ExecProjectInput(ctx, projectNameForFiles(files), files.Compose, files.Env, payload, serviceName,
		"bao", "write", "-format=json", "auth/approle/login", "-")
	if err != nil {
		return "", fmt.Errorf("OpenBao AppRole login failed: %w", err)
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
	retry := func(command string, validate func(string) bool) error {
		var lastErr error
		for attempt := 0; attempt < 20; attempt++ {
			out, err := execWithToken(ctx, executor, files, token, command)
			if err == nil && (validate == nil || validate(out)) {
				return nil
			}
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
		if lastErr != nil {
			return lastErr
		}
		return errors.New("OpenBao manager verification did not converge")
	}

	if err := retry(`exec bao kv put -mount=baseharbor apps/_baseharbor/bootstrap-probe value=ok`, nil); err != nil {
		return errors.New("OpenBao manager cannot write application secrets")
	}
	if err := retry(`exec bao kv get -field=value -mount=baseharbor apps/_baseharbor/bootstrap-probe`, func(out string) bool {
		return strings.TrimSpace(out) == "ok"
	}); err != nil {
		return errors.New("OpenBao manager cannot read application secrets")
	}
	if err := retry(`exec bao delete baseharbor/metadata/apps/_baseharbor/bootstrap-probe`, nil); err != nil {
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
	inputText := token + "\n"
	if payload != "" {
		inputText += payload
		if !strings.HasSuffix(inputText, "\n") {
			inputText += "\n"
		}
	}
	input := []byte(inputText)
	return executor.ExecProjectInput(ctx, projectNameForFiles(files), files.Compose, files.Env, input, serviceName, "sh", "-ceu", prefix+command)
}

const managerPolicy = `path "sys/capabilities-self" {
  capabilities = ["update"]
}

path "baseharbor/data/managed/*" {
  capabilities = ["create", "update", "read", "delete"]
}

path "baseharbor/metadata/managed/*" {
  capabilities = ["read", "list", "delete"]
}

path "baseharbor/data/apps/*" {
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

path "sys/policy/baseharbor-app-*" {
  capabilities = ["create", "update", "read", "delete"]
}

path "sys/leader" {
  capabilities = ["read"]
}

path "sys/auth" {
  capabilities = ["read"]
}

path "sys/auth/baseharbor-dev" {
  capabilities = ["create", "update", "read", "delete", "sudo"]
}

path "auth/baseharbor-dev/users/*" {
  capabilities = ["create", "update", "read", "delete"]
}

path "auth/approle/role/baseharbor-app-*" {
  capabilities = ["create", "update", "read", "delete"]
}

path "auth/approle/role/baseharbor-manager/role-id" {
  capabilities = ["read"]
}

path "auth/approle/role/baseharbor-manager/secret-id" {
  capabilities = ["create", "update"]
}

path "auth/approle/role/baseharbor-manager/secret-id/destroy" {
  capabilities = ["create", "update"]
}

path "baseharbor-pki/issue/baseharbor-services" {
  capabilities = ["create", "update"]
}

path "baseharbor-pki/sign/baseharbor-core" {
  capabilities = ["create", "update"]
}
path "baseharbor-pki/sign/baseharbor-nodes" {
  capabilities = ["create", "update"]
}

path "baseharbor-pki/cert/ca" {
  capabilities = ["read"]
}

path "baseharbor-pki/cert/*" {
  capabilities = ["read"]
}

path "baseharbor-pki/revoke" {
  capabilities = ["create", "update"]
}

path "baseharbor-pki/root/rotate/internal" {
  capabilities = ["create", "update"]
}

path "baseharbor-pki/config/issuers" {
  capabilities = ["read", "update"]
}
`
