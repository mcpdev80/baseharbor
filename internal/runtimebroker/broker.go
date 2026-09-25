package runtimebroker

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

const (
	DefaultImage = "ghcr.io/mcpdev80/baseharbor-runtime:edge"
	ServiceName  = "broker"
	RuntimeURL   = "https://baseharbor-runtime:8443"
)

type Files struct {
	Compose string
	Image   string
	DocsURL string
}

func IsMutableDevelopmentImage(image string) bool {
	return strings.TrimSpace(image) == DefaultImage
}

func ProjectName(m application.Manifest) string {
	return "baseharbor-broker-" + m.Name + "-" + m.Environment
}

func ProjectNameForRuntime(m application.Manifest, files application.RuntimeFiles) string {
	namespace := strings.TrimSpace(strings.ReplaceAll(files.Namespace, ".", "-"))
	if namespace == "" {
		return ProjectName(m)
	}
	return "baseharbor-broker-" + namespace + "-" + m.Name + "-" + m.Environment
}

func ObservabilityNetworkName(m application.Manifest) string {
	return ProjectName(m) + "-observability"
}

func ObservabilityNetworkNameForRuntime(m application.Manifest, files application.RuntimeFiles) string {
	return ProjectNameForRuntime(m, files) + "-observability"
}

func Ensure(m application.Manifest, appFiles application.RuntimeFiles, mtls openbao.RuntimeMTLSFiles) (Files, error) {
	if !application.RequiresRuntimeBroker(m) {
		return Files{}, errors.New("application runtime broker requires at least one managed runtime capability")
	}
	canonicalToken, err := application.EnsureRuntimeIdentity(m, appFiles)
	if err != nil {
		return Files{}, err
	}
	credentialProjection := ""
	if m.Services.Secrets {
		credentialProjection, err = projectOwnerOnlyFile(appFiles, openbao.ApplicationCredentialsPath(appFiles.Dir), "openbao.env", "OpenBao application credentials")
		if err != nil {
			return Files{}, err
		}
	}
	tokenProjection, err := projectOwnerOnlyFile(appFiles, canonicalToken, "runtime-token", "application runtime identity token")
	if err != nil {
		return Files{}, err
	}
	image, err := ensureImage(appFiles.Dir)
	if err != nil {
		return Files{}, err
	}
	composePath, err := filepath.Abs(filepath.Join(appFiles.Dir, "broker.compose.yaml"))
	if err != nil {
		return Files{}, fmt.Errorf("resolve runtime broker compose path: %w", err)
	}
	docsPort, err := ensureDocsPort(m, appFiles)
	if err != nil {
		return Files{}, err
	}
	permissionsPath, err := ensureRuntimePermissionsFile(m, appFiles)
	if err != nil {
		return Files{}, err
	}
	serviceTokensPath, err := ensureRuntimeServiceTokensFile(m, appFiles)
	if err != nil {
		return Files{}, err
	}
	brokerKeyProjection, err := projectOwnerOnlyFile(appFiles, mtls.BrokerKey, "broker-key.pem", "runtime broker private key")
	if err != nil {
		return Files{}, err
	}
	clientKeyProjection, err := projectOwnerOnlyFile(appFiles, mtls.ClientKey, "probe-client-key.pem", "runtime probe client private key")
	if err != nil {
		return Files{}, err
	}
	mtls.BrokerKey = brokerKeyProjection
	mtls.ClientKey = clientKeyProjection
	otlpBinding, hasOTLPBinding, err := application.ExistingRuntimeOTLPBinding(m, appFiles)
	if err != nil {
		return Files{}, fmt.Errorf("resolve runtime broker OTLP binding: %w", err)
	}
	var otlp *application.RuntimeOTLPBinding
	if hasOTLPBinding {
		otlp = &otlpBinding
	}
	content, err := composeYAMLForRuntime(m, appFiles, mtls, tokenProjection, credentialProjection, permissionsPath, serviceTokensPath, image, docsPort, otlp)
	if err != nil {
		return Files{}, err
	}
	if err := os.WriteFile(composePath, []byte(content), 0o600); err != nil {
		return Files{}, fmt.Errorf("write runtime broker compose file: %w", err)
	}
	if err := os.Chmod(composePath, 0o600); err != nil {
		return Files{}, fmt.Errorf("protect runtime broker compose file: %w", err)
	}
	return Files{Compose: composePath, Image: image, DocsURL: docsURL(docsPort)}, nil
}

func Existing(appFiles application.RuntimeFiles) (Files, error) {
	composePath, err := filepath.Abs(filepath.Join(appFiles.Dir, "broker.compose.yaml"))
	if err != nil {
		return Files{}, fmt.Errorf("resolve runtime broker compose path: %w", err)
	}
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
	docsPort := ""
	if portData, err := os.ReadFile(filepath.Join(appFiles.Dir, "broker-docs-port")); err == nil {
		docsPort = strings.TrimSpace(string(portData))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Files{}, err
	}
	return Files{Compose: composePath, Image: strings.TrimSpace(string(data)), DocsURL: docsURL(docsPort)}, nil
}

func projectOwnerOnlyFile(appFiles application.RuntimeFiles, source, targetName, label string) (string, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return "", fmt.Errorf("inspect canonical %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("canonical %s must be a regular file", label)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("canonical %s is accessible by group or others (%o)", label, info.Mode().Perm())
	}
	value, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read canonical %s: %w", label, err)
	}
	if len(value) == 0 {
		return "", fmt.Errorf("canonical %s is empty", label)
	}

	dir := filepath.Join(appFiles.Bindings, "runtime-broker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create runtime broker binding directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("protect runtime broker binding directory: %w", err)
	}
	path := filepath.Join(dir, targetName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, value, 0o600); err != nil {
		return "", fmt.Errorf("write %s runtime projection: %w", label, err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("prepare %s runtime projection: %w", label, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("install %s runtime projection: %w", label, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s runtime projection: %w", label, err)
	}
	return absolute, nil
}

func ensureImage(dir string) (string, error) {
	path := filepath.Join(dir, "broker-image")
	requested := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"))
	if strings.ContainsAny(requested, "\r\n\x00") {
		return "", errors.New("BASEHARBOR_RUNTIME_IMAGE is invalid")
	}

	if data, err := os.ReadFile(path); err == nil {
		existing := strings.TrimSpace(string(data))
		if existing == "" || strings.ContainsAny(existing, "\r\n\x00") {
			return "", errors.New("runtime broker image state is invalid")
		}
		if requested == "" || requested == existing {
			return existing, nil
		}
		if err := os.WriteFile(path, []byte(requested+"\n"), 0o600); err != nil {
			return "", fmt.Errorf("update runtime broker image state: %w", err)
		}
		return requested, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read runtime broker image state: %w", err)
	}

	image := requested
	if image == "" {
		image = DefaultImage
	}
	if err := os.WriteFile(path, []byte(image+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write runtime broker image state: %w", err)
	}
	return image, nil
}

func ensureDocsPort(m application.Manifest, appFiles application.RuntimeFiles) (string, error) {
	enabled, err := runtimeDocsEnabled(m.Environment)
	if err != nil {
		return "", err
	}
	path := filepath.Join(appFiles.Dir, "broker-docs-port")
	if !enabled {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("remove disabled runtime docs port state: %w", err)
		}
		return "", nil
	}
	if data, err := os.ReadFile(path); err == nil {
		port := strings.TrimSpace(string(data))
		if _, err := strconv.Atoi(port); err != nil || port == "" {
			return "", errors.New("runtime docs port state is invalid")
		}
		return port, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read runtime docs port state: %w", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("allocate runtime docs port: %w", err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	_ = listener.Close()
	if err := os.WriteFile(path, []byte(port+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write runtime docs port state: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("protect runtime docs port state: %w", err)
	}
	return port, nil
}

func runtimeDocsEnabled(environment string) (bool, error) {
	if raw := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_DOCS_ENABLED")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return false, errors.New("BASEHARBOR_RUNTIME_DOCS_ENABLED must be a boolean")
		}
		return value, nil
	}
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "dev", "development":
		return true, nil
	default:
		return false, nil
	}
}

func docsURL(port string) string {
	if strings.TrimSpace(port) == "" {
		return ""
	}
	return "https://127.0.0.1:" + strings.TrimSpace(port) + "/"
}

func ensureRuntimeServiceTokensFile(m application.Manifest, appFiles application.RuntimeFiles) (string, error) {
	dir := filepath.Join(appFiles.Bindings, "runtime-broker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create runtime broker binding directory: %w", err)
	}
	identities, err := application.EnsureRuntimeServiceIdentities(m, appFiles)
	if err != nil {
		return "", err
	}
	values := map[string]string{}
	for service, path := range identities {
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("inspect runtime service identity for %s: %w", service, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("runtime service identity for %s is not protected", service)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read runtime service identity for %s: %w", service, err)
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", fmt.Errorf("runtime service identity for %s is empty", service)
		}
		values[service] = token
	}
	path := filepath.Join(dir, "service-tokens.json")
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode runtime service identities: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write runtime service identities: %w", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return "", fmt.Errorf("prepare runtime service identity projection: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve runtime service identities: %w", err)
	}
	return absolute, nil
}

func ensureRuntimePermissionsFile(m application.Manifest, appFiles application.RuntimeFiles) (string, error) {
	dir := filepath.Join(appFiles.Bindings, "runtime-broker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create runtime broker binding directory: %w", err)
	}
	path := filepath.Join(dir, "permissions.json")
	data, err := json.MarshalIndent(m.Runtime.Permissions, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode runtime broker permissions: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write runtime broker permissions: %w", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return "", fmt.Errorf("prepare runtime broker permissions projection: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve runtime broker permissions: %w", err)
	}
	return absolute, nil
}

func composeYAML(m application.Manifest, mtls openbao.RuntimeMTLSFiles, tokenPath, credPath, permissionsPath, serviceTokensPath, image, docsPort string, otlp *application.RuntimeOTLPBinding) (string, error) {
	return composeYAMLForRuntime(m, application.RuntimeFiles{}, mtls, tokenPath, credPath, permissionsPath, serviceTokensPath, image, docsPort, otlp)
}

func composeYAMLForRuntime(m application.Manifest, appFiles application.RuntimeFiles, mtls openbao.RuntimeMTLSFiles, tokenPath, credPath, permissionsPath, serviceTokensPath, image, docsPort string, otlp *application.RuntimeOTLPBinding) (string, error) {
	config, err := prepareRuntimeBrokerComposeConfig(m, appFiles, mtls, tokenPath, credPath, permissionsPath, serviceTokensPath, image, docsPort, otlp)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	config.writeService(&b)
	config.writeSecretsAndVolumes(&b)
	config.writeNetworks(&b)
	return b.String(), nil
}
