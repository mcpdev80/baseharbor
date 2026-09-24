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
)

func postgresAccessService(instance string) string {
	return runtimeServiceName("postgres", instance)
}

func valkeyAccessService(instance string) string {
	return runtimeServiceName("valkey", instance) + "-access"
}

func postgresTLSCAKey(instance string) string {
	return postgresRuntimeKey(instance, "TLS_CA_FILE")
}

func valkeyTLSCAKey(instance string) string {
	return valkeyRuntimeKey(instance, "TLS_CA_FILE")
}

func postgresTLSCAContainerPath(instance string) string {
	return postgresTLSCAContainerPrefix + "/" + instance + "/ca.pem"
}

func valkeyTLSCAContainerPath(instance string) string {
	return valkeyTLSCAContainerPrefix + "/" + instance + "/ca.pem"
}

func backendAccessRoot(files RuntimeFiles, kind, instance string) string {
	return filepath.Join(files.Dir, "providers", kind, instance)
}

func EnsureBackendServiceAccess(ctx context.Context, issuer serviceaccess.Issuer, files RuntimeFiles, m Manifest) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
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
	for _, instance := range CacheInstanceNames(m) {
		policy, err := serviceaccess.Resolve(m.Environment, "valkey", serviceaccess.AuthenticationNative)
		if err != nil {
			return err
		}
		root := backendAccessRoot(files, "valkey", instance)
		_, err = serviceaccess.EnsureTCPGateway(ctx, issuer, policy, root, serviceaccess.TCPGatewaySpec{
			ServiceName:      valkeyAccessService(instance),
			UpstreamHost:     runtimeServiceName("valkey", instance),
			UpstreamPort:     6379,
			PublishedPortEnv: valkeyRuntimeKey(instance, "HOST_PORT"),
			ContainerPort:    6379,
		})
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

func valkeyGatewayCompose(instance string) string {
	return serviceaccess.TCPGatewayComposeService(
		backendGatewayComposeFiles("valkey", instance),
		serviceaccess.TCPGatewaySpec{
			ServiceName:      valkeyAccessService(instance),
			UpstreamHost:     runtimeServiceName("valkey", instance),
			UpstreamPort:     6379,
			PublishedPortEnv: valkeyRuntimeKey(instance, "HOST_PORT"),
			ContainerPort:    6379,
		},
	)
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
