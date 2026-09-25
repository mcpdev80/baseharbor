package runtimeexecutor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

const (
	ProjectName        = "baseharbor-runtime-executor"
	ServiceName        = "executor"
	ControlNetworkName = "baseharbor-runtime-control"
	ExecutorURL        = "https://baseharbor-runtime-executor:9443"
	DefaultImage       = "ghcr.io/mcpdev80/baseharbor-runtime:edge"
)

type Files struct {
	Dir            string
	Compose        string
	Env            string
	Image          string
	Project        string
	ControlNetwork string
}

type ObservabilityBinding struct {
	Endpoint   string
	CA         string
	ClientCert string
	ClientKey  string
}

type Runtime interface {
	DestroyProject(context.Context, string, string, string) error
}

func EnsureFiles(dataDir string, identity openbao.RuntimeExecutorMTLSFiles, adminCredentialsPath, s3Endpoint, s3TrustPath string, observability ...ObservabilityBinding) (Files, error) {
	return EnsureFilesAt(dataDir, "", identity, adminCredentialsPath, s3Endpoint, s3TrustPath, observability...)
}

func EnsureFilesAt(dataDir, namespace string, identity openbao.RuntimeExecutorMTLSFiles, adminCredentialsPath, s3Endpoint, s3TrustPath string, observability ...ObservabilityBinding) (Files, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return Files{}, errors.New("BaseHarbor data directory is required for runtime executor")
	}
	dir := filepath.Join(dataDir, "runtime-executor")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Files{}, fmt.Errorf("create runtime executor state directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return Files{}, fmt.Errorf("protect runtime executor state directory: %w", err)
	}
	if strings.TrimSpace(s3Endpoint) == "" || strings.TrimSpace(s3TrustPath) == "" {
		return Files{}, errors.New("runtime executor S3 HTTPS endpoint and trust bundle are required")
	}
	required := map[string]string{
		"runtime CA":           identity.CA,
		"executor certificate": identity.Cert,
		"executor private key": identity.Key,
		"S3 admin credentials": adminCredentialsPath,
		"S3 trust bundle":      s3TrustPath,
	}
	var observer ObservabilityBinding
	if len(observability) > 0 {
		observer = observability[0]
		observer.Endpoint = strings.TrimSpace(observer.Endpoint)
		if observer.Endpoint != "" {
			required["observability CA"] = observer.CA
			required["observability client certificate"] = observer.ClientCert
			required["observability client key"] = observer.ClientKey
		}
	}
	for label, path := range required {
		info, err := os.Stat(path)
		if err != nil {
			return Files{}, fmt.Errorf("inspect %s: %w", label, err)
		}
		if !info.Mode().IsRegular() {
			return Files{}, fmt.Errorf("%s must be a regular file", label)
		}
	}

	adminProjection, err := projectContainerReadableSecret(dir, adminCredentialsPath, "s3-admin.env", "S3 admin credentials")
	if err != nil {
		return Files{}, err
	}
	executorKeyProjection, err := projectContainerReadableSecret(dir, identity.Key, "executor-key.pem", "runtime executor private key")
	if err != nil {
		return Files{}, err
	}
	identity.Key = executorKeyProjection
	if observer.Endpoint != "" {
		observerCA, err := projectContainerReadablePublicFile(dir, observer.CA, "observability-ca.pem", "runtime observability CA")
		if err != nil {
			return Files{}, err
		}
		observerCert, err := projectContainerReadablePublicFile(dir, observer.ClientCert, "observer-client-cert.pem", "runtime observability client certificate")
		if err != nil {
			return Files{}, err
		}
		observerKey, err := projectContainerReadableSecret(dir, observer.ClientKey, "observer-client-key.pem", "runtime observability client key")
		if err != nil {
			return Files{}, err
		}
		observer.CA = observerCA
		observer.ClientCert = observerCert
		observer.ClientKey = observerKey
	}
	s3TrustProjection, err := projectContainerReadablePublicFile(dir, s3TrustPath, "s3-ca.pem", "S3 trust bundle")
	if err != nil {
		return Files{}, err
	}

	image := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"))
	if image == "" {
		image = DefaultImage
	}
	if strings.ContainsAny(image, "\r\n\x00") {
		return Files{}, errors.New("BASEHARBOR_RUNTIME_IMAGE is invalid")
	}

	composePath := filepath.Join(dir, "compose.yaml")
	envPath := filepath.Join(dir, "runtime.env")
	if err := os.WriteFile(envPath, nil, 0o600); err != nil {
		return Files{}, fmt.Errorf("write runtime executor environment: %w", err)
	}
	if err := os.Chmod(envPath, 0o600); err != nil {
		return Files{}, fmt.Errorf("protect runtime executor environment: %w", err)
	}
	project := scopedName(ProjectName, namespace)
	controlNetwork := scopedName(ControlNetworkName, namespace)
	objectStorageNetwork := scopedName("baseharbor-object-storage", namespace)
	telemetryNetwork := scopedName("baseharbor-telemetry", namespace)
	content := composeYAMLForNetworks(image, identity, adminProjection, s3Endpoint, s3TrustProjection, controlNetwork, objectStorageNetwork, telemetryNetwork, observer)
	if err := os.WriteFile(composePath, []byte(content), 0o600); err != nil {
		return Files{}, fmt.Errorf("write runtime executor compose file: %w", err)
	}
	if err := os.Chmod(composePath, 0o600); err != nil {
		return Files{}, fmt.Errorf("protect runtime executor compose file: %w", err)
	}
	return Files{Dir: dir, Compose: composePath, Env: envPath, Image: image, Project: project, ControlNetwork: controlNetwork}, nil
}

func scopedName(base, namespace string) string {
	namespace = strings.TrimSpace(strings.ReplaceAll(namespace, ".", "-"))
	if namespace == "" {
		return base
	}
	return base + "-" + namespace
}

func projectContainerReadableSecret(dir, source, targetName, label string) (string, error) {
	return projectContainerReadableFile(dir, source, targetName, label, true)
}

func projectContainerReadablePublicFile(dir, source, targetName, label string) (string, error) {
	return projectContainerReadableFile(dir, source, targetName, label, false)
}

func projectContainerReadableFile(dir, source, targetName, label string, requireOwnerOnly bool) (string, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s must be a regular file", label)
	}
	if requireOwnerOnly && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%s is accessible by group or others (%o)", label, info.Mode().Perm())
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%s is empty", label)
	}
	path := filepath.Join(dir, targetName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("write %s projection: %w", label, err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("prepare %s projection: %w", label, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("install %s projection: %w", label, err)
	}
	return path, nil
}

func ExistingFiles(dataDir string) (Files, error) {
	return ExistingFilesAt(dataDir, "")
}

func ExistingFilesAt(dataDir, namespace string) (Files, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return Files{}, errors.New("BaseHarbor data directory is required for runtime executor")
	}
	dir := filepath.Join(dataDir, "runtime-executor")
	files := Files{
		Dir:            dir,
		Compose:        filepath.Join(dir, "compose.yaml"),
		Env:            filepath.Join(dir, "runtime.env"),
		Project:        scopedName(ProjectName, namespace),
		ControlNetwork: scopedName(ControlNetworkName, namespace),
	}
	for _, path := range []string{files.Compose, files.Env} {
		if _, err := os.Stat(path); err != nil {
			return Files{}, err
		}
	}
	return files, nil
}

func DestroyShared(ctx context.Context, runtime Runtime, dataDir string) error {
	return DestroySharedAt(ctx, runtime, dataDir, "")
}

func DestroySharedAt(ctx context.Context, runtime Runtime, dataDir, namespace string) error {
	files, err := ExistingFilesAt(dataDir, namespace)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if runtime == nil {
		return errors.New("runtime executor lifecycle runtime is required")
	}
	if err := runtime.DestroyProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func composeYAML(image string, identity openbao.RuntimeExecutorMTLSFiles, adminCredentialsPath, s3Endpoint, s3TrustPath string, observability ...ObservabilityBinding) string {
	return composeYAMLForNetworks(image, identity, adminCredentialsPath, s3Endpoint, s3TrustPath, ControlNetworkName, "baseharbor-object-storage", "baseharbor-telemetry", observability...)
}

func composeYAMLForNetworks(image string, identity openbao.RuntimeExecutorMTLSFiles, adminCredentialsPath, s3Endpoint, s3TrustPath, controlNetwork, objectStorageNetwork, telemetryNetwork string, observability ...ObservabilityBinding) string {
	var observer ObservabilityBinding
	if len(observability) > 0 {
		observer = observability[0]
	}
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  executor:\n")
	fmt.Fprintf(&b, "    image: %s\n", strconv.Quote(image))
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"65532:65532\"\n")
	b.WriteString("    command: [\"serve\"]\n")
	b.WriteString("    environment:\n")
	b.WriteString("      BASEHARBOR_RUNTIME_EXECUTOR_MODE: \"true\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_LISTEN_ADDR: \"0.0.0.0:9443\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_TLS_CERT_FILE: \"/run/baseharbor/identity/executor-cert.pem\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_TLS_KEY_FILE: \"/run/secrets/executor-key\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_TLS_CLIENT_CA_FILE: \"/run/baseharbor/identity/ca.pem\"\n")
	fmt.Fprintf(&b, "      BASEHARBOR_EXECUTOR_S3_ENDPOINT: %s\n", strconv.Quote(s3Endpoint))
	b.WriteString("      BASEHARBOR_EXECUTOR_S3_CA_FILE: \"/run/baseharbor/provider/s3-ca.pem\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_S3_ADMIN_CREDENTIALS_FILE: \"/run/secrets/s3-admin\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_STATE_DIR: \"/var/lib/baseharbor/runtime-resources\"\n")
	if observer.Endpoint != "" {
		fmt.Fprintf(&b, "      OTEL_EXPORTER_OTLP_ENDPOINT: %s\n", strconv.Quote(observer.Endpoint))
		b.WriteString("      OTEL_EXPORTER_OTLP_PROTOCOL: \"http/protobuf\"\n")
		b.WriteString("      OTEL_SERVICE_NAME: \"runtime-executor\"\n")
		b.WriteString("      OTEL_RESOURCE_ATTRIBUTES: \"service.namespace=baseharbor,baseharbor.component=runtime-executor\"\n")
		b.WriteString("      OTEL_EXPORTER_OTLP_CERTIFICATE: \"/run/baseharbor/observability/ca.pem\"\n")
		b.WriteString("      OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE: \"/run/baseharbor/observability/client-cert.pem\"\n")
		b.WriteString("      OTEL_EXPORTER_OTLP_CLIENT_KEY: \"/run/secrets/observer-client-key\"\n")
	}
	b.WriteString("    read_only: true\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - \"/tmp:rw,noexec,nosuid,nodev,size=16m\"\n")
	b.WriteString("    cap_drop:\n")
	b.WriteString("      - ALL\n")
	b.WriteString("    security_opt:\n")
	b.WriteString("      - \"no-new-privileges:true\"\n")
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(identity.CA+":/run/baseharbor/identity/ca.pem:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(identity.Cert+":/run/baseharbor/identity/executor-cert.pem:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(s3TrustPath+":/run/baseharbor/provider/s3-ca.pem:ro"))
	if observer.Endpoint != "" {
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(observer.CA+":/run/baseharbor/observability/ca.pem:ro"))
		fmt.Fprintf(&b, "      - %s\n", strconv.Quote(observer.ClientCert+":/run/baseharbor/observability/client-cert.pem:ro"))
	}
	b.WriteString("      - runtime-resource-state:/var/lib/baseharbor/runtime-resources\n")
	b.WriteString("    secrets:\n")
	b.WriteString("      - executor-key\n")
	b.WriteString("      - s3-admin\n")
	if observer.Endpoint != "" {
		b.WriteString("      - observer-client-key\n")
	}
	b.WriteString("    networks:\n")
	b.WriteString("      runtime-control:\n")
	b.WriteString("        aliases:\n")
	b.WriteString("          - baseharbor-runtime-executor\n")
	b.WriteString("      object-storage: {}\n")
	if observer.Endpoint != "" {
		b.WriteString("      telemetry: {}\n")
	}
	b.WriteString("\nsecrets:\n")
	b.WriteString("  executor-key:\n")
	fmt.Fprintf(&b, "    file: %s\n", strconv.Quote(identity.Key))
	b.WriteString("  s3-admin:\n")
	fmt.Fprintf(&b, "    file: %s\n", strconv.Quote(adminCredentialsPath))
	if observer.Endpoint != "" {
		b.WriteString("  observer-client-key:\n")
		fmt.Fprintf(&b, "    file: %s\n", strconv.Quote(observer.ClientKey))
	}
	b.WriteString("\nvolumes:\n")
	b.WriteString("  runtime-resource-state:\n")
	b.WriteString("\nnetworks:\n")
	b.WriteString("  runtime-control:\n")
	fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(controlNetwork))
	b.WriteString("    internal: true\n")
	b.WriteString("  object-storage:\n")
	b.WriteString("    external: true\n")
	fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(objectStorageNetwork))
	if observer.Endpoint != "" {
		b.WriteString("  telemetry:\n")
		b.WriteString("    external: true\n")
		fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(telemetryNetwork))
	}
	return b.String()
}
