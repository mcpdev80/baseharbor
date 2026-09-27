package openbao

import (
	"context"
	"errors"
	"fmt"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func EnsureDevelopmentUserpass(ctx context.Context, executor Executor, files bhruntime.Files, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return errors.New("OpenBao development credentials are incomplete")
	}
	for _, r := range username {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return fmt.Errorf("OpenBao development username %q is invalid", username)
		}
	}

	credentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	token, err := loginManager(ctx, executor, files, credentials)
	if err != nil {
		return fmt.Errorf("authenticate OpenBao manager for developer access: %w", err)
	}

	const enable = `if ! bao auth list -format=json | grep -q '"baseharbor-dev/"'; then
  bao auth enable -path=baseharbor-dev userpass >/dev/null
fi`
	if _, err := execWithToken(ctx, executor, files, token, enable); err != nil {
		return fmt.Errorf("enable OpenBao developer userpass auth: %w", err)
	}

	script := `IFS= read -r PASSWORD
test -n "$PASSWORD"
exec bao write auth/baseharbor-dev/users/` + username + ` password="$PASSWORD" policies=baseharbor-manager token_no_default_policy=true`
	if _, err := execWithTokenPayload(ctx, executor, files, token, script, password); err != nil {
		return fmt.Errorf("reconcile OpenBao development user: %w", err)
	}
	return nil
}
