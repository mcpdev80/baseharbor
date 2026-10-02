package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	postgresTLSCAContainerPrefix = "/run/baseharbor/bindings/postgres"
	valkeyTLSCAContainerPrefix   = "/run/baseharbor/bindings/valkey"
	rabbitmqTLSCAContainerPrefix = "/run/baseharbor/bindings/rabbitmq"
	mongodbTLSCAContainerPrefix  = "/run/baseharbor/bindings/mongodb"
)

func postgresAccessService(instance string) string {
	return runtimeServiceName("postgres", instance)
}

func valkeyAccessService(instance string) string {
	return runtimeServiceName("valkey", instance) + "-access"
}

func rabbitmqAccessService(instance string) string {
	return runtimeServiceName("rabbitmq", instance) + "-access"
}

func mongodbAccessService(instance string) string {
	return runtimeServiceName("mongodb", instance) + "-access"
}

func postgresTLSCAKey(instance string) string {
	return postgresRuntimeKey(instance, "TLS_CA_FILE")
}

func valkeyTLSCAKey(instance string) string {
	return valkeyRuntimeKey(instance, "TLS_CA_FILE")
}

func rabbitmqTLSCAKey(instance string) string {
	return rabbitmqRuntimeKey(instance, "TLS_CA_FILE")
}

func mongodbTLSCAKey(instance string) string {
	return mongodbRuntimeKey(instance, "TLS_CA_FILE")
}

func postgresTLSCAContainerPath(instance string) string {
	return postgresTLSCAContainerPrefix + "/" + instance + "/ca.pem"
}

func valkeyTLSCAContainerPath(instance string) string {
	return valkeyTLSCAContainerPrefix + "/" + instance + "/ca.pem"
}

func rabbitmqTLSCAContainerPath(instance string) string {
	return rabbitmqTLSCAContainerPrefix + "/" + instance + "/ca.pem"
}

func mongodbTLSCAContainerPath(instance string) string {
	return mongodbTLSCAContainerPrefix + "/" + instance + "/ca.pem"
}

func backendAccessRoot(files RuntimeFiles, kind, instance string) string {
	return filepath.Join(files.Dir, "providers", kind, instance)
}

func EnsureBackendServiceAccess(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	if !UsesSharedPostgreSQL(m) {
		for _, instance := range SQLInstanceNames(m) {
			policy, err := serviceaccess.Resolve(m.Environment, "postgresql", serviceaccess.AuthenticationNative)
			if err != nil {
				return err
			}
			root := backendAccessRoot(files, "postgresql", instance)
			material, err := serviceaccess.EnsureTLSMaterial(
				ctx,
				issuer,
				policy,
				filepath.Join(root, "service-access", "pki"),
				runtimeServiceName("postgres", instance),
				"127.0.0.1",
			)
			if err != nil {
				return fmt.Errorf("prepare PostgreSQL native TLS for %s: %w", instance, err)
			}
			if err := projectPostgresServerMaterial(root, material); err != nil {
				return fmt.Errorf("project PostgreSQL native TLS for %s: %w", instance, err)
			}
			ca, err := projectBackendCA(files, "postgres", instance, material.CA)
			if err != nil {
				return err
			}
			values[postgresTLSCAKey(instance)] = ca
		}
	}
	if !UsesSharedValkey(m) {
		for _, instance := range ValkeyInstanceNames(m) {
			policy, err := serviceaccess.Resolve(m.Environment, "valkey", serviceaccess.AuthenticationNative)
			if err != nil {
				return err
			}
			root := backendAccessRoot(files, "valkey", instance)
			_, err = serviceaccess.EnsureTCPGateway(ctx, issuer, policy, root, valkeyGatewaySpec(m, instance))
			if err != nil {
				return fmt.Errorf("prepare Valkey TLS access for %s: %w", instance, err)
			}
			material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(root, "service-access", "pki"))
			if err != nil {
				return err
			}
			ca, err := projectBackendCA(files, "valkey", instance, material.CA)
			if err != nil {
				return err
			}
			values[valkeyTLSCAKey(instance)] = ca
		}
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		policy, err := serviceaccess.Resolve(m.Environment, "rabbitmq", serviceaccess.AuthenticationNative)
		if err != nil {
			return err
		}
		root := backendAccessRoot(files, "rabbitmq", instance)
		_, err = serviceaccess.EnsureTCPGateway(ctx, issuer, policy, root, serviceaccess.TCPGatewaySpec{
			ServiceName:      rabbitmqAccessService(instance),
			UpstreamHost:     runtimeServiceName("rabbitmq", instance),
			UpstreamPort:     5672,
			Upstreams:        rabbitmqGatewayUpstreams(m, instance),
			PublishedPortEnv: rabbitmqRuntimeKey(instance, "HOST_PORT"),
			ContainerPort:    5672,
		})
		if err != nil {
			return fmt.Errorf("prepare RabbitMQ TLS access for %s: %w", instance, err)
		}
		material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(root, "service-access", "pki"))
		if err != nil {
			return err
		}
		ca, err := projectBackendCA(files, "rabbitmq", instance, material.CA)
		if err != nil {
			return err
		}
		values[rabbitmqTLSCAKey(instance)] = ca
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		policy, err := serviceaccess.Resolve(m.Environment, "mongodb", serviceaccess.AuthenticationNative)
		if err != nil {
			return err
		}
		root := backendAccessRoot(files, "mongodb", instance)
		names := make([]string, 0, mongodbMemberCount(m, instance)+2)
		names = append(names, "localhost", "127.0.0.1")
		for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
			names = append(names, mongodbMemberServiceName(instance, ordinal))
		}
		material, err := serviceaccess.EnsureTLSMaterial(
			ctx,
			issuer,
			policy,
			filepath.Join(root, "service-access", "pki"),
			names...,
		)
		if err != nil {
			return fmt.Errorf("prepare MongoDB native TLS for %s: %w", instance, err)
		}
		if err := projectMongoDBServerMaterial(root, material); err != nil {
			return fmt.Errorf("project MongoDB native TLS for %s: %w", instance, err)
		}
		ca, err := projectBackendCA(files, "mongodb", instance, material.CA)
		if err != nil {
			return err
		}
		values[mongodbTLSCAKey(instance)] = ca
	}
	return writeRuntimeEnv(files.Env, m, values)
}

func projectPostgresServerMaterial(root string, material serviceaccess.TLSMaterial) error {
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		return err
	}
	for source, target := range map[string]string{
		material.ServerCertificate: filepath.Join(runtimeDir, "server-cert.pem"),
		material.ServerKey:         filepath.Join(runtimeDir, "server-key.pem"),
	} {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return fmt.Errorf("PostgreSQL TLS material %s is empty", filepath.Base(source))
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return err
		}
		// The directory is owner-only on the host. Inside the unprivileged
		// PostgreSQL container the mounted source must be readable so the
		// process can copy the key into tmpfs and tighten it to 0600.
		if err := os.Chmod(target, 0o644); err != nil {
			return err
		}
	}
	const hba = `local all all trust
hostssl all all 0.0.0.0/0 scram-sha-256
hostssl all all ::/0 scram-sha-256
hostnossl all all 0.0.0.0/0 reject
hostnossl all all ::/0 reject
`
	if err := os.WriteFile(filepath.Join(runtimeDir, "pg_hba.conf"), []byte(hba), 0o644); err != nil {
		return err
	}
	return nil
}

func projectMongoDBServerMaterial(root string, material serviceaccess.TLSMaterial) error {
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		return err
	}
	cert, err := os.ReadFile(material.ServerCertificate)
	if err != nil {
		return err
	}
	key, err := os.ReadFile(material.ServerKey)
	if err != nil {
		return err
	}
	ca, err := os.ReadFile(material.CA)
	if err != nil {
		return err
	}
	if len(cert) == 0 || len(key) == 0 || len(ca) == 0 {
		return fmt.Errorf("MongoDB TLS material is incomplete")
	}
	serverPEM := append(append([]byte(nil), cert...), key...)
	for path, data := range map[string][]byte{
		filepath.Join(runtimeDir, "server.pem"): serverPEM,
		filepath.Join(runtimeDir, "ca.pem"):     ca,
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func projectBackendCA(files RuntimeFiles, kind, instance, source string) (string, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%s TLS trust bundle is empty", kind)
	}
	dir := filepath.Join(files.Bindings, kind, instance)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "ca.pem")
	if err := writeOwnerOnlyFile(path, data); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func backendGatewayComposeFiles(kind, instance string) serviceaccess.TCPGatewayFiles {
	root := "./" + filepath.ToSlash(filepath.Join("providers", kind, instance, "service-access"))
	return serviceaccess.TCPGatewayFiles{
		Config:   root + "/haproxy.cfg",
		PEM:      root + "/runtime/server.pem",
		Material: serviceaccess.TLSMaterial{ServerName: ""},
	}
}

func valkeyGatewayCompose(m Manifest, instance string) string {
	return serviceaccess.TCPGatewayComposeService(
		backendGatewayComposeFiles("valkey", instance),
		valkeyGatewaySpec(m, instance),
	)
}

func rabbitmqGatewayCompose(m Manifest, instance string) string {
	return serviceaccess.TCPGatewayComposeService(
		backendGatewayComposeFiles("rabbitmq", instance),
		serviceaccess.TCPGatewaySpec{
			ServiceName:      rabbitmqAccessService(instance),
			UpstreamHost:     runtimeServiceName("rabbitmq", instance),
			UpstreamPort:     5672,
			Upstreams:        rabbitmqGatewayUpstreams(m, instance),
			PublishedPortEnv: rabbitmqRuntimeKey(instance, "HOST_PORT"),
			ContainerPort:    5672,
		},
	)
}

func mongodbGatewayCompose(instance string) string {
	return serviceaccess.TCPGatewayComposeService(
		backendGatewayComposeFiles("mongodb", instance),
		serviceaccess.TCPGatewaySpec{
			ServiceName:      mongodbAccessService(instance),
			UpstreamHost:     runtimeServiceName("mongodb", instance),
			UpstreamPort:     27017,
			PublishedPortEnv: mongodbRuntimeKey(instance, "HOST_PORT"),
			ContainerPort:    27017,
		},
	)
}

type BackendTLSLifecycleObservation struct {
	Kind      string                             `json:"kind"`
	Instance  string                             `json:"instance"`
	Lifecycle serviceaccess.LifecycleObservation `json:"lifecycle"`
}

func InspectBackendTLSLifecycle(files RuntimeFiles, m Manifest) ([]BackendTLSLifecycleObservation, error) {
	out := make([]BackendTLSLifecycleObservation, 0, len(SQLInstanceNames(m))+len(ValkeyInstanceNames(m))+len(RabbitMQInstanceNames(m))+len(DocumentDatabaseInstanceNames(m)))
	if !UsesSharedPostgreSQL(m) {
		for _, instance := range SQLInstanceNames(m) {
			lifecycle, err := serviceaccess.InspectLifecycle(filepath.Join(backendAccessRoot(files, "postgresql", instance), "service-access", "pki"))
			if err != nil {
				return nil, fmt.Errorf("inspect PostgreSQL TLS lifecycle for %s: %w", instance, err)
			}
			out = append(out, BackendTLSLifecycleObservation{Kind: "sql", Instance: instance, Lifecycle: lifecycle})
		}
	}
	if !UsesSharedValkey(m) {
		for _, instance := range ValkeyInstanceNames(m) {
			lifecycle, err := serviceaccess.InspectLifecycle(filepath.Join(backendAccessRoot(files, "valkey", instance), "service-access", "pki"))
			if err != nil {
				return nil, fmt.Errorf("inspect Valkey TLS lifecycle for %s: %w", instance, err)
			}
			out = append(out, BackendTLSLifecycleObservation{Kind: "cache", Instance: instance, Lifecycle: lifecycle})
		}
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		lifecycle, err := serviceaccess.InspectLifecycle(filepath.Join(backendAccessRoot(files, "rabbitmq", instance), "service-access", "pki"))
		if err != nil {
			return nil, fmt.Errorf("inspect RabbitMQ TLS lifecycle for %s: %w", instance, err)
		}
		out = append(out, BackendTLSLifecycleObservation{Kind: "messaging", Instance: instance, Lifecycle: lifecycle})
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		lifecycle, err := serviceaccess.InspectLifecycle(filepath.Join(backendAccessRoot(files, "mongodb", instance), "service-access", "pki"))
		if err != nil {
			return nil, fmt.Errorf("inspect MongoDB TLS lifecycle for %s: %w", instance, err)
		}
		out = append(out, BackendTLSLifecycleObservation{Kind: "document-database", Instance: instance, Lifecycle: lifecycle})
	}
	return out, nil
}

func InspectSharedBackendTLSLifecycleAt(dataDir, namespace string, m Manifest) ([]BackendTLSLifecycleObservation, error) {
	if !HasSharedBackends(m) {
		return nil, nil
	}
	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	var out []BackendTLSLifecycleObservation
	if UsesSharedPostgreSQL(m) {
		lifecycle, err := serviceaccess.InspectLifecycle(filepath.Join(shared.Dir, "postgresql", "service-access", "pki"))
		if err != nil {
			return nil, fmt.Errorf("inspect shared PostgreSQL TLS lifecycle: %w", err)
		}
		for _, instance := range SQLInstanceNames(m) {
			out = append(out, BackendTLSLifecycleObservation{Kind: "sql", Instance: instance, Lifecycle: lifecycle})
		}
	}
	if UsesSharedValkey(m) {
		for _, instance := range ValkeyInstanceNames(m) {
			root := filepath.Join(shared.Dir, "valkey", sharedBackendToken(m.Name), sharedBackendToken(instance))
			lifecycle, err := serviceaccess.InspectLifecycle(filepath.Join(root, "service-access", "pki"))
			if err != nil {
				return nil, fmt.Errorf("inspect shared Valkey TLS lifecycle for %s: %w", instance, err)
			}
			out = append(out, BackendTLSLifecycleObservation{Kind: "cache", Instance: instance, Lifecycle: lifecycle})
		}
	}
	return out, nil
}

func backendCertificates(path string) (string, error) {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("backend trust bundle is empty")
	}
	return value, nil
}
