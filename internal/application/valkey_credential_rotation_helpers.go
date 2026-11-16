package application

import (
	"context"
	"errors"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func valkeyAddPassword(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, oldPassword, newPassword string) error {
	script := "IFS= read -r old_password\nIFS= read -r new_password\nexport VALKEYCLI_AUTH=\"$old_password\"\nprintf '>%s' \"$new_password\" | valkey-cli -h 127.0.0.1 -p 6379 -x ACL SETUSER default on >/dev/null\n"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(oldPassword+"\n"+newPassword+"\n"), service, "sh", "-ceu", script)
	return err
}

func valkeyRemovePassword(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, activePassword, retiredPassword string) error {
	script := "IFS= read -r active_password\nIFS= read -r retired_password\nexport VALKEYCLI_AUTH=\"$active_password\"\nprintf '<%s' \"$retired_password\" | valkey-cli -h 127.0.0.1 -p 6379 -x ACL SETUSER default >/dev/null\n"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(activePassword+"\n"+retiredPassword+"\n"), service, "sh", "-ceu", script)
	return err
}

func valkeyVerifyPassword(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, password string, wantAccepted bool) error {
	script := "IFS= read -r password\nexport VALKEYCLI_AUTH=\"$password\"\nout=\"$(valkey-cli -h 127.0.0.1 -p 6379 ping 2>&1 || true)\"\nprintf '%s' \"$out\"\n"
	out, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(password+"\n"), service, "sh", "-ceu", script)
	if err != nil {
		return err
	}
	accepted := strings.TrimSpace(out) == "PONG"
	if accepted != wantAccepted {
		if wantAccepted {
			return errors.New("credential was rejected")
		}
		return errors.New("retired credential is still accepted")
	}
	return nil
}
