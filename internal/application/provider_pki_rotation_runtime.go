package application

import (
	"context"
	"fmt"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

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
