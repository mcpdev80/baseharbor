package runtimebroker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

const (
	DefaultImage = "ghcr.io/mcpdev80/baseharbor-runtime:latest"
	ServiceName  = "broker"
	RuntimeURL   = "https://baseharbor-secrets:8443"
)

type Files struct {
	Compose string
	Image   string
}

func ProjectName(m application.Manifest) string {
	return "baseharbor-broker-" + m.Name + "-" + m.Environment
}

func Ensure(m application.Manifest, appFiles application.RuntimeFiles, mtls openbao.RuntimeMTLSFiles) (Files, error) {
	if !m.Services.Secrets {
		return Files{}, errors.New("runtime secret broker requires managed secrets")
	}
	tokenPath, err := application.EnsureRuntimeIdentity(m, appFiles)
	if err != nil {
		return Files{}, err
	}
	image, err := ensureImage(appFiles.Dir)
	if err != nil {
		return Files{}, err
	}
	composePath := filepath.Join(appFiles.Dir, "broker.compose.yaml")
	content := composeYAML(m, appFiles, mtls, tokenPath, image)
	if err := os.WriteFile(composePath, []byte(content), 0o600); err != nil {
		return Files{}, fmt.Errorf("write runtime broker compose file: %w", err)
	}
	if err := os.Chmod(composePath, 0o600); err != nil {
		return Files{}, fmt.Errorf("protect runtime broker compose file: %w", err)
	}
	return Files{Compose: composePath, Image: image}, nil
}

func Existing(appFiles application.RuntimeFiles) (Files, error) {
	composePath := filepath.Join(appFiles.Dir, "broker.compose.yaml")
	imagePath := filepath.Join(appFiles.Dir, "broker-image")
	for _, path := range []string{composePath, imagePath} {
		if _, err := os.Stat(path); err != nil {
			return Files{}, err
		}
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return Files{}, err
	}
	return Files{Compose: composePath, Image: strings.TrimSpace(string(data))}, nil
}

func ensureImage(dir string) (string, error) {
	path := filepath.Join(dir, "broker-image")
	if data, err := os.ReadFile(path); err == nil {
		image := strings.TrimSpace(string(data))
		if image == "" || strings.ContainsAny(image, "\r\n\x00") {
			return "", errors.New("runtime broker image state is invalid")
		}
		return image, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read runtime broker image state: %w", err)
	}
	image := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"))
	if image == "" {
		image = DefaultImage
	}
	if strings.ContainsAny(image, "\r\n\x00") {
		return "", errors.New("BASEHARBOR_RUNTIME_IMAGE is invalid")
	}
	if err := os.WriteFile(path, []byte(image+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write runtime broker image state: %w", err)
	}
	return image, nil
}

func composeYAML(m application.Manifest, appFiles application.RuntimeFiles, mtls openbao.RuntimeMTLSFiles, tokenPath, image string) string {
	backendNetwork := application.ApplicationBackendNetworkName(m)
	credPath := openbao.ApplicationCredentialsPath(appFiles.Dir)
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  broker:\n")
	fmt.Fprintf(&b, "    image: %s\n", strconv.Quote(image))
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    command: [\"serve\"]\n")
	b.WriteString("    environment:\n")
	b.WriteString("      BASEHARBOR_API_LISTEN_ADDR: \"0.0.0.0:8443\"\n")
	b.WriteString("      BASEHARBOR_API_TLS_CERT_FILE: \"/run/baseharbor/identity/broker-cert.pem\"\n")
	b.WriteString("      BASEHARBOR_API_TLS_KEY_FILE: \"/run/secrets/broker-key\"\n")
	b.WriteString("      BASEHARBOR_API_TLS_CLIENT_CA_FILE: \"/run/baseharbor/identity/ca.pem\"\n")
	fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_APP_NAME: %s\n", strconv.Quote(m.Name))
	fmt.Fprintf(&b, "      BASEHARBOR_RUNTIME_ENVIRONMENT: %s\n", strconv.Quote(m.Environment))
	b.WriteString("      BASEHARBOR_RUNTIME_OPENBAO_URL: \"http://openbao:8200\"\n")
	b.WriteString("      BASEHARBOR_RUNTIME_OPENBAO_CREDENTIALS_FILE: \"/run/secrets/openbao-credentials\"\n")
	b.WriteString("      BASEHARBOR_RUNTIME_TOKEN_FILE: \"/run/secrets/runtime-token\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - \"/tmp:rw,noexec,nosuid,nodev,size=16m\"\n")
	b.WriteString("    cap_drop:\n")
	b.WriteString("      - ALL\n")
	b.WriteString("    security_opt:\n")
	b.WriteString("      - \"no-new-privileges:true\"\n")
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(mtls.CA+":/run/baseharbor/identity/ca.pem:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(mtls.BrokerCert+":/run/baseharbor/identity/broker-cert.pem:ro"))
	b.WriteString("    secrets:\n")
	b.WriteString("      - broker-key\n")
	b.WriteString("      - openbao-credentials\n")
	b.WriteString("      - runtime-token\n")
	b.WriteString("    networks:\n")
	b.WriteString("      backend:\n")
	b.WriteString("        aliases:\n")
	b.WriteString("          - baseharbor-secrets\n")
	b.WriteString("      secrets: {}\n")
	b.WriteString("\nsecrets:\n")
	b.WriteString("  broker-key:\n")
	fmt.Fprintf(&b, "    file: %s\n", strconv.Quote(mtls.BrokerKey))
	b.WriteString("  openbao-credentials:\n")
	fmt.Fprintf(&b, "    file: %s\n", strconv.Quote(credPath))
	b.WriteString("  runtime-token:\n")
	fmt.Fprintf(&b, "    file: %s\n", strconv.Quote(tokenPath))
	b.WriteString("\nnetworks:\n")
	b.WriteString("  backend:\n")
	b.WriteString("    external: true\n")
	fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(backendNetwork))
	b.WriteString("  secrets:\n")
	b.WriteString("    external: true\n")
	b.WriteString("    name: baseharbor-secrets\n")
	return b.String()
}
