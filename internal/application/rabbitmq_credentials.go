package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type rabbitMQCredentialRuntime interface {
	Run(context.Context, string, ...string) (string, error)
	RunSensitive(context.Context, string, []byte, ...string) (string, error)
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

if rabbitmqctl list_users -q | awk '{print $1}' | grep -Fxq "$app_user"; then
  rabbitmqctl change_password "$app_user" "$app_password" >/dev/null
else
  rabbitmqctl add_user "$app_user" "$app_password" >/dev/null
fi
rabbitmqctl set_permissions -p / "$app_user" '.*' '.*' '.*' >/dev/null

if [ -n "$admin_user" ]; then
  if rabbitmqctl list_users -q | awk '{print $1}' | grep -Fxq "$admin_user"; then
    rabbitmqctl change_password "$admin_user" "$admin_password" >/dev/null
  else
    rabbitmqctl add_user "$admin_user" "$admin_password" >/dev/null
  fi
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
		if err := waitRabbitMQCredentialAuthority(ctx, runtime, m, instance, service); err != nil {
			return fmt.Errorf("wait for RabbitMQ credential authority %s: %w", instance, err)
		}
		if err := reconcileRabbitMQCredentialSet(ctx, runtime, service, input, script); err != nil {
			return fmt.Errorf("reconcile RabbitMQ credentials for %s: %w", instance, err)
		}
	}
	return nil
}

func waitRabbitMQCredentialAuthority(ctx context.Context, runtime rabbitMQCredentialRuntime, m Manifest, instance, service string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	consecutive := 0
	var lastErr error
	for {
		status, err := runtime.Run(ctx, service, "rabbitmqctl", "cluster_status")
		if err == nil {
			missing := ""
			if rabbitmqMemberCount(m) > 1 {
				for ordinal := 0; ordinal < rabbitmqMemberCount(m); ordinal++ {
					node := "rabbit@" + rabbitmqMemberServiceName(instance, ordinal)
					if !strings.Contains(status, node) {
						missing = node
						break
					}
				}
			}
			if missing == "" {
				consecutive++
				if consecutive >= 2 {
					return nil
				}
				lastErr = nil
			} else {
				consecutive = 0
				lastErr = fmt.Errorf("cluster status is missing member %s", missing)
			}
		} else {
			consecutive = 0
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-deadline.C:
			return fmt.Errorf("RabbitMQ credential authority %s did not become stable: %w", service, lastErr)
		case <-ticker.C:
		}
	}
}

func reconcileRabbitMQCredentialSet(ctx context.Context, runtime rabbitMQCredentialRuntime, service string, input []byte, script string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		if _, err := runtime.Run(ctx, service, "rabbitmq-diagnostics", "-q", "ping"); err == nil {
			if _, err := runtime.RunSensitive(ctx, service, input, "sh", "-ceu", script); err == nil {
				return nil
			} else {
				lastErr = err
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-deadline.C:
			return fmt.Errorf("RabbitMQ credential mutation did not reach a stable running node: %w", lastErr)
		case <-ticker.C:
		}
	}
}
