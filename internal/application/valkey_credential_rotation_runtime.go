package application

import (
	"context"
	"fmt"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func valkeyReconcileCredentialConsumers(ctx context.Context, runtime bhruntime.RuntimeProvider, issuer serviceaccess.Issuer, m Manifest, files RuntimeFiles, instance, authPassword, desiredPassword string) error {
	if err := valkeyReconcileReplicationCredential(ctx, runtime, m, files, instance, authPassword, desiredPassword); err != nil {
		return err
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	values[valkeyRuntimeKey(instance, "PASSWORD")] = desiredPassword
	if err := writeRuntimeEnv(files.Env, m, values); err != nil {
		return err
	}
	if _, err := EnsureRuntimeContract(m, files); err != nil {
		return err
	}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		if err := EnsureApplicationManagementUIs(ctx, issuer, files, m); err != nil {
			return err
		}
	}
	return recreateValkeyCredentialConsumers(ctx, runtime, m, files, instance)
}

func valkeyRollbackCredentialConsumers(ctx context.Context, runtime bhruntime.RuntimeProvider, issuer serviceaccess.Issuer, m Manifest, files RuntimeFiles, instance, oldPassword, newPassword string) error {
	if err := valkeyReconcileReplicationCredential(ctx, runtime, m, files, instance, newPassword, oldPassword); err != nil {
		return err
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	values[valkeyRuntimeKey(instance, "PASSWORD")] = oldPassword
	if err := writeRuntimeEnv(files.Env, m, values); err != nil {
		return err
	}
	if _, err := EnsureRuntimeContract(m, files); err != nil {
		return err
	}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		if err := EnsureApplicationManagementUIs(ctx, issuer, files, m); err != nil {
			return err
		}
	}
	if err := recreateValkeyCredentialConsumers(ctx, runtime, m, files, instance); err != nil {
		return err
	}
	for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
		service := valkeyMemberServiceName(instance, ordinal)
		_ = valkeyRemovePassword(ctx, runtime, files, service, oldPassword, newPassword)
	}
	return nil
}

func valkeyReconcileReplicationCredential(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, instance, authPassword, desiredPassword string) error {
	if valkeyMemberCount(m, instance) <= 1 {
		return nil
	}
	memberScript := "IFS= read -r auth_password\nIFS= read -r desired_password\nexport VALKEYCLI_AUTH=\"$auth_password\"\nprintf '%s' \"$desired_password\" | valkey-cli -h 127.0.0.1 -p 6379 -x CONFIG SET masterauth >/dev/null\n"
	for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
		service := valkeyMemberServiceName(instance, ordinal)
		if _, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(authPassword+"\n"+desiredPassword+"\n"), service, "sh", "-ceu", memberScript); err != nil {
			return fmt.Errorf("update Valkey replication credential on %s: %w", service, err)
		}
	}
	sentinelScript := "IFS= read -r desired_password\nprintf '%s' \"$desired_password\" | valkey-cli -p 26379 -x SENTINEL SET baseharbor auth-pass >/dev/null\n"
	for ordinal := 0; ordinal < 3; ordinal++ {
		service := valkeySentinelServiceName(instance, ordinal)
		if _, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(desiredPassword+"\n"), service, "sh", "-ceu", sentinelScript); err != nil {
			return fmt.Errorf("update Valkey Sentinel credential on %s: %w", service, err)
		}
	}
	return nil
}

func recreateValkeyCredentialConsumers(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, instance string) error {
	environment, err := RuntimeEnvironment(files)
	if err != nil {
		return err
	}
	services := []string{valkeyAccessService(instance)}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		services = append(services, "cache-ui")
	}
	return runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, files.Dir, environment, services, files.Compose)
}
