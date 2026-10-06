package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type RuntimeBackendProbeExecutor struct {
	runtime RuntimeProjectExecutor
	remote  ProjectServiceExecutor
	files   RuntimeFiles
}

// RuntimeProjectExecutor keeps the local probe adapter independent of the
// complete runtime provider interface.
type RuntimeProjectExecutor interface {
	ExecProject(context.Context, string, string, string, string, ...string) (string, error)
}

// ProjectServiceExecutor resolves and verifies ownership at the selected node.
// The Core's existing probes keep their TLS verification and credential refs.
type ProjectServiceExecutor interface {
	ExecService(context.Context, string, string, ...string) (string, error)
}

func NewRuntimeBackendProbeExecutor(runtime RuntimeProjectExecutor, files RuntimeFiles) RuntimeBackendProbeExecutor {
	return RuntimeBackendProbeExecutor{runtime: runtime, files: files}
}

func NewRemoteBackendProbeExecutor(remote ProjectServiceExecutor, project string) RuntimeBackendProbeExecutor {
	return RuntimeBackendProbeExecutor{remote: remote, files: RuntimeFiles{Project: project}}
}

func (e RuntimeBackendProbeExecutor) execute(ctx context.Context, service string, argv ...string) (string, error) {
	if e.remote != nil {
		return e.remote.ExecService(ctx, e.files.Project, service, argv...)
	}
	if e.runtime == nil {
		return "", errors.New("backend probe runtime is unavailable")
	}
	return e.runtime.ExecProject(ctx, e.files.Project, e.files.Compose, e.files.Env, service, argv...)
}

func (e RuntimeBackendProbeExecutor) ProbeBackend(ctx context.Context, probe BackendProbe) (string, error) {
	instance := strings.TrimSpace(probe.Instance)
	if instance == "" {
		instance = defaultServiceInstance
	}
	switch probe.Kind {
	case BackendProbeSQLSelectOne:
		service := runtimeServiceName("postgres", instance)
		database := strings.TrimSpace(probe.Database)
		if database == "" {
			return "", errors.New("postgres verification database is required")
		}
		command := fmt.Sprintf("PGPASSWORD=\"$POSTGRES_PASSWORD\" psql \"host=%s port=5432 user=baseharbor dbname=%s sslmode=verify-ca sslrootcert=/run/baseharbor/tls/ca.pem\" -tAc 'SELECT 1'", postgresAccessService(instance), database)
		return e.execute(ctx, service, "sh", "-ec", command)
	case BackendProbeCachePing:
		service := runtimeServiceName("valkey", instance)
		command := fmt.Sprintf(`VALKEYCLI_AUTH="$VALKEY_PASSWORD" valkey-cli --tls --cacert /run/baseharbor/tls/ca.pem -h %s -p 6379 ping`, valkeyAccessService(instance))
		return e.execute(ctx, service, "sh", "-ec", command)
	case BackendProbeDurableKeyValueRW:
		service := runtimeServiceName("valkey", instance)
		command := fmt.Sprintf(`VALKEYCLI_AUTH="$VALKEY_PASSWORD" valkey-cli --tls --cacert /run/baseharbor/tls/ca.pem -h %s -p 6379 set __baseharbor_verify__ durable >/dev/null && VALKEYCLI_AUTH="$VALKEY_PASSWORD" valkey-cli --tls --cacert /run/baseharbor/tls/ca.pem -h %s -p 6379 get __baseharbor_verify__ && VALKEYCLI_AUTH="$VALKEY_PASSWORD" valkey-cli --tls --cacert /run/baseharbor/tls/ca.pem -h %s -p 6379 del __baseharbor_verify__ >/dev/null`, valkeyAccessService(instance), valkeyAccessService(instance), valkeyAccessService(instance))
		return e.execute(ctx, service, "sh", "-ec", command)
	default:
		return "", fmt.Errorf("unsupported backend probe %q", probe.Kind)
	}
}
