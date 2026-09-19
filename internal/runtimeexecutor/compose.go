package runtimeexecutor

import (
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
	Dir     string
	Compose string
	Image   string
}

func EnsureFiles(dataDir string, identity openbao.RuntimeExecutorMTLSFiles, adminCredentialsPath string) (Files, error) {
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
	for label, path := range map[string]string{
		"runtime CA": identity.CA,
		"executor certificate": identity.Cert,
		"executor private key": identity.Key,
		"S3 admin credentials": adminCredentialsPath,
	} {
		info, err := os.Stat(path)
		if err != nil {
			return Files{}, fmt.Errorf("inspect %s: %w", label, err)
		}
		if !info.Mode().IsRegular() {
			return Files{}, fmt.Errorf("%s must be a regular file", label)
		}
	}

	image := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"))
	if image == "" {
		image = DefaultImage
	}
	if strings.ContainsAny(image, "\r\n\x00") {
		return Files{}, errors.New("BASEHARBOR_RUNTIME_IMAGE is invalid")
	}

	composePath := filepath.Join(dir, "compose.yaml")
	content := composeYAML(image, identity, adminCredentialsPath)
	if err := os.WriteFile(composePath, []byte(content), 0o600); err != nil {
		return Files{}, fmt.Errorf("write runtime executor compose file: %w", err)
	}
	if err := os.Chmod(composePath, 0o600); err != nil {
		return Files{}, fmt.Errorf("protect runtime executor compose file: %w", err)
	}
	return Files{Dir: dir, Compose: composePath, Image: image}, nil
}

func composeYAML(image string, identity openbao.RuntimeExecutorMTLSFiles, adminCredentialsPath string) string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  executor:\n")
	fmt.Fprintf(&b, "    image: %s\n", strconv.Quote(image))
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    command: [\"serve\"]\n")
	b.WriteString("    environment:\n")
	b.WriteString("      BASEHARBOR_RUNTIME_EXECUTOR_MODE: \"true\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_LISTEN_ADDR: \"0.0.0.0:9443\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_TLS_CERT_FILE: \"/run/baseharbor/identity/executor-cert.pem\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_TLS_KEY_FILE: \"/run/secrets/executor-key\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_TLS_CLIENT_CA_FILE: \"/run/baseharbor/identity/ca.pem\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_S3_ENDPOINT: \"http://seaweedfs:8333\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_S3_ADMIN_CREDENTIALS_FILE: \"/run/secrets/s3-admin\"\n")
	b.WriteString("      BASEHARBOR_EXECUTOR_STATE_DIR: \"/var/lib/baseharbor/runtime-resources\"\n")
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
	b.WriteString("      - runtime-resource-state:/var/lib/baseharbor/runtime-resources\n")
	b.WriteString("    secrets:\n")
	b.WriteString("      - executor-key\n")
	b.WriteString("      - s3-admin\n")
	b.WriteString("    networks:\n")
	b.WriteString("      runtime-control:\n")
	b.WriteString("        aliases:\n")
	b.WriteString("          - baseharbor-runtime-executor\n")
	b.WriteString("      object-storage: {}\n")
	b.WriteString("\nsecrets:\n")
	b.WriteString("  executor-key:\n")
	fmt.Fprintf(&b, "    file: %s\n", strconv.Quote(identity.Key))
	b.WriteString("  s3-admin:\n")
	fmt.Fprintf(&b, "    file: %s\n", strconv.Quote(adminCredentialsPath))
	b.WriteString("\nvolumes:\n")
	b.WriteString("  runtime-resource-state:\n")
	b.WriteString("\nnetworks:\n")
	b.WriteString("  runtime-control:\n")
	b.WriteString("    name: baseharbor-runtime-control\n")
	b.WriteString("    internal: true\n")
	b.WriteString("  object-storage:\n")
	b.WriteString("    external: true\n")
	b.WriteString("    name: baseharbor-object-storage\n")
	return b.String()
}
