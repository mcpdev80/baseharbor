package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type rabbitMQCredentialRuntime interface {
	ExecProjectInput(context.Context, string, string, string, []byte, string, ...string) (string, error)
}

func ReconcileRabbitMQCredentials(ctx context.Context, runtime rabbitMQCredentialRuntime, m Manifest, files RuntimeFiles) error {
	if len(RabbitMQInstanceNames(m)) == 0 {
		return nil
	}
	if runtime == nil {
		return errors.New("RabbitMQ credential reconciliation requires a runtime provider")
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	const script = `IFS= read -r app_user
IFS= read -r app_password
IFS= read -r admin_user
IFS= read -r admin_password

rabbitmqctl add_user "$app_user" "$app_password" >/dev/null 2>&1 ||
  rabbitmqctl change_password "$app_user" "$app_password" >/dev/null
rabbitmqctl set_permissions -p / "$app_user" '.*' '.*' '.*' >/dev/null

if [ -n "$admin_user" ]; then
  rabbitmqctl add_user "$admin_user" "$admin_password" >/dev/null 2>&1 ||
    rabbitmqctl change_password "$admin_user" "$admin_password" >/dev/null
  rabbitmqctl set_user_tags "$admin_user" administrator >/dev/null
  rabbitmqctl set_permissions -p / "$admin_user" '.*' '.*' '.*' >/dev/null
fi
`
	for _, instance := range RabbitMQInstanceNames(m) {
		appUser, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "USER"))
		if err != nil {
			return err
		}
		appPassword, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "PASSWORD"))
		if err != nil {
			return err
		}
		adminUser := ""
		adminPassword := ""
		if m.Services.MessagingManagementUI {
			adminUser, err = requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "ADMIN_USER"))
			if err != nil {
				return err
			}
			adminPassword, err = requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "ADMIN_PASSWORD"))
			if err != nil {
				return err
			}
		}
		for name, value := range map[string]string{
			"application username": appUser,
			"application password": appPassword,
			"management username":  adminUser,
			"management password":  adminPassword,
		} {
			if strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("RabbitMQ %s must be a single line", name)
			}
		}
		input := []byte(appUser + "\n" + appPassword + "\n" + adminUser + "\n" + adminPassword + "\n")
		service := rabbitmqMemberServiceName(instance, 0)
		if _, err := runtime.ExecProjectInput(
			ctx, files.Project, files.Compose, files.Env, input, service,
			"sh", "-ceu", script,
		); err != nil {
			return fmt.Errorf("reconcile RabbitMQ credentials for %s: %w", instance, err)
		}
	}
	return nil
}
