package application

import (
	"context"
	"errors"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func rabbitMQCreateApplicationUser(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, username, password string) error {
	script := "IFS= read -r username\nIFS= read -r password\nrabbitmqctl add_user \"$username\" \"$password\" >/dev/null\nrabbitmqctl set_permissions -p / \"$username\" '.*' '.*' '.*' >/dev/null\n"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(username+"\n"+password+"\n"), service, "sh", "-ceu", script)
	return err
}

func rabbitMQChangePassword(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, username, password string) error {
	script := "IFS= read -r username\nIFS= read -r password\nrabbitmqctl change_password \"$username\" \"$password\" >/dev/null\n"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(username+"\n"+password+"\n"), service, "sh", "-ceu", script)
	return err
}

func rabbitMQDeleteUser(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, username string) error {
	script := "IFS= read -r username\nrabbitmqctl delete_user \"$username\" >/dev/null\n"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(username+"\n"), service, "sh", "-ceu", script)
	return err
}

func rabbitMQVerifyCredential(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, username, password string, wantAccepted bool) error {
	script := "IFS= read -r username\nIFS= read -r password\nif rabbitmqctl authenticate_user \"$username\" \"$password\" >/dev/null 2>&1; then printf accepted; else printf rejected; fi\n"
	out, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(username+"\n"+password+"\n"), service, "sh", "-ceu", script)
	if err != nil {
		return err
	}
	accepted := strings.TrimSpace(out) == "accepted"
	if accepted != wantAccepted {
		if wantAccepted {
			return errors.New("RabbitMQ credential was rejected")
		}
		return errors.New("retired RabbitMQ credential is still accepted")
	}
	return nil
}
