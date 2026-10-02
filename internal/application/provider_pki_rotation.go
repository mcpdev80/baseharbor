package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type ManagedProviderPKIKind string

const (
	ManagedProviderPKIValkey   ManagedProviderPKIKind = "valkey"
	ManagedProviderPKIRabbitMQ ManagedProviderPKIKind = "rabbitmq"
	ManagedProviderPKIMongoDB  ManagedProviderPKIKind = "mongodb"
)

func RotateManagedProviderPKI(ctx context.Context, runtime bhruntime.RuntimeProvider, issuer serviceaccess.Issuer, m Manifest, files RuntimeFiles, kind ManagedProviderPKIKind, instance string) error {
	if runtime == nil {
		return errors.New("managed provider PKI rotation requires a runtime provider")
	}
	if issuer == nil {
		return errors.New("managed provider PKI rotation requires an issuer")
	}

	if err := EnsureBackendServiceAccess(ctx, issuer, files, m); err != nil {
		return fmt.Errorf("prepare provider PKI overlap: %w", err)
	}
	if err := EnsureApplicationManagementUIs(ctx, issuer, files, m); err != nil {
		return fmt.Errorf("prepare management UI PKI overlap: %w", err)
	}
	if _, err := EnsureRuntimeContract(m, files); err != nil {
		return err
	}
	if err := reloadManagedProviderPKI(ctx, runtime, m, files, kind, instance); err != nil {
		return fmt.Errorf("reconcile provider PKI: %w", err)
	}
	if err := reloadManagedProviderManagementUI(ctx, runtime, m, files, kind, instance); err != nil {
		return fmt.Errorf("reconcile management UI PKI: %w", err)
	}
	if err := verifyManagedProviderPKIRotation(ctx, runtime, m, files, kind); err != nil {
		return fmt.Errorf("verify provider PKI rotation: %w", err)
	}
	if err := retireManagedProviderPKIOverlap(ctx, issuer, m, files, kind, instance); err != nil {
		return err
	}
	if err := EnsureBackendServiceAccess(ctx, issuer, files, m); err != nil {
		return fmt.Errorf("project retired provider trust: %w", err)
	}
	if err := EnsureApplicationManagementUIs(ctx, issuer, files, m); err != nil {
		return fmt.Errorf("project retired management UI trust: %w", err)
	}
	if _, err := EnsureRuntimeContract(m, files); err != nil {
		return err
	}
	if kind == ManagedProviderPKIMongoDB {
		if err := reloadMongoDBCertificates(ctx, runtime, m, files, instance); err != nil {
			return fmt.Errorf("reload MongoDB new-only trust: %w", err)
		}
	}
	return verifyManagedProviderPKIRotation(ctx, runtime, m, files, kind)
}

func reloadManagedProviderPKI(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, kind ManagedProviderPKIKind, instance string) error {
	switch kind {
	case ManagedProviderPKIValkey:
		_, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, valkeyAccessService(instance), "sh", "-ec", "kill -USR2 1")
		return err
	case ManagedProviderPKIRabbitMQ:
		_, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, rabbitmqAccessService(instance), "sh", "-ec", "kill -USR2 1")
		return err
	case ManagedProviderPKIMongoDB:
		return reloadMongoDBCertificates(ctx, runtime, m, files, instance)
	default:
		return fmt.Errorf("unsupported managed provider PKI kind %q", kind)
	}
}

func reloadMongoDBCertificates(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, instance string) error {
	for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
		service := mongodbMemberServiceName(instance, ordinal)
		script := "mongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username \"$MONGO_INITDB_ROOT_USERNAME\" --password \"$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --eval 'const r=db.adminCommand({rotateCertificates:1}); if (!r.ok) throw new Error(JSON.stringify(r));'"
		if _, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, service, "sh", "-ec", script); err != nil {
			return fmt.Errorf("rotate MongoDB certificates on %s: %w", service, err)
		}
	}
	return nil
}

func reloadManagedProviderManagementUI(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, kind ManagedProviderPKIKind, instance string) error {
	service := ""
	switch kind {
	case ManagedProviderPKIValkey:
		if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
			service = "cache-ui-access"
		}
	case ManagedProviderPKIRabbitMQ:
		if m.Services.MessagingManagementUI {
			service = rabbitmqUIServiceName(instance)
		}
	case ManagedProviderPKIMongoDB:
		if m.Services.DocumentDatabaseManagementUI {
			service = mongodbUIAccessServiceName(instance)
		}
	}
	if service == "" {
		return nil
	}
	_, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, service, "/run/baseharbor/caddy", "reload", "--force", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile")
	return err
}

func verifyManagedProviderPKIRotation(ctx context.Context, runtime bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles, kind ManagedProviderPKIKind) error {
	switch kind {
	case ManagedProviderPKIValkey:
		if err := VerifyValkeyRuntime(ctx, runtime, m, files); err != nil {
			return err
		}
		if err := VerifyValkeyHACluster(ctx, runtimeCredentialExecutor{runtime: runtime, files: files}, m, files); err != nil {
			return err
		}
	case ManagedProviderPKIRabbitMQ:
		if err := VerifyRabbitMQRuntime(ctx, m, files); err != nil {
			return err
		}
		if err := VerifyRabbitMQHACluster(ctx, runtimeCredentialExecutor{runtime: runtime, files: files}, m, files); err != nil {
			return err
		}
	case ManagedProviderPKIMongoDB:
		if err := VerifyMongoDBRuntime(ctx, m, files); err != nil {
			return err
		}
		if err := VerifyMongoDBHACluster(ctx, runtimeCredentialExecutor{runtime: runtime, files: files}, m, files); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported managed provider PKI kind %q", kind)
	}
	return VerifyApplicationManagementUIs(ctx, m, files)
}

func retireManagedProviderPKIOverlap(ctx context.Context, issuer serviceaccess.Issuer, m Manifest, files RuntimeFiles, kind ManagedProviderPKIKind, instance string) error {
	provider := string(kind)
	policy, err := serviceaccess.Resolve(m.Environment, provider, serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	if err := serviceaccess.RetireTLSOverlap(ctx, issuer, policy, filepath.Join(backendAccessRoot(files, provider, instance), "service-access", "pki")); err != nil {
		return fmt.Errorf("retire %s provider CA overlap: %w", provider, err)
	}

	var uiProvider, uiDir string
	switch kind {
	case ManagedProviderPKIValkey:
		if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
			uiProvider = "redis-commander"
			uiDir = filepath.Join(files.Dir, "providers", "management-ui", "cache", "pki")
		}
	case ManagedProviderPKIRabbitMQ:
		if m.Services.MessagingManagementUI {
			uiProvider = "rabbitmq-management"
			uiDir = filepath.Join(files.Dir, "providers", "management-ui", "rabbitmq", instance, "pki")
		}
	case ManagedProviderPKIMongoDB:
		if m.Services.DocumentDatabaseManagementUI {
			uiProvider = "mongodb-management"
			uiDir = filepath.Join(files.Dir, "providers", "management-ui", "mongodb", instance, "pki")
		}
	}
	if uiDir == "" {
		return nil
	}
	uiPolicy, err := serviceaccess.Resolve(m.Environment, uiProvider, serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	if err := serviceaccess.RetireTLSOverlap(ctx, issuer, uiPolicy, uiDir); err != nil {
		return fmt.Errorf("retire %s management UI CA overlap: %w", provider, err)
	}
	return nil
}
