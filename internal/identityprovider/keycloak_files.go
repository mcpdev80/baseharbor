package identityprovider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	KeycloakImage      = "quay.io/keycloak/keycloak:26.7.4"
	KeycloakService    = "keycloak"
	keycloakPublicHost = "identity.localhost"
	keycloakHTTPSPort  = 8443
)

// KeycloakRuntime is the legacy local-runtime realization used by the current
// Docker/Podman reference implementation.
type KeycloakRuntime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

// KeycloakLifecycle is the provider-realization boundary consumed by identity
// semantics. It deliberately avoids Compose/project lifecycle vocabulary.
type KeycloakLifecycle interface {
	Validate(context.Context, KeycloakFiles) error
	Apply(context.Context, KeycloakFiles) error
	Destroy(context.Context, KeycloakFiles) error
}

type keycloakRuntimeLifecycle struct {
	runtime KeycloakRuntime
}

func NewKeycloakLifecycle(runtime KeycloakRuntime) KeycloakLifecycle {
	return keycloakRuntimeLifecycle{runtime: runtime}
}

func (l keycloakRuntimeLifecycle) Validate(ctx context.Context, files KeycloakFiles) error {
	return l.runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env)
}

func (l keycloakRuntimeLifecycle) Apply(ctx context.Context, files KeycloakFiles) error {
	return l.runtime.UpProject(ctx, files.Project, files.Compose, files.Env)
}

func (l keycloakRuntimeLifecycle) Destroy(ctx context.Context, files KeycloakFiles) error {
	return l.runtime.DestroyProject(ctx, files.Project, files.Compose, files.Env)
}

type KeycloakFiles struct {
	Dir                string
	Compose            string
	Env                string
	Project            string
	ConsumerNetwork    string
	InternalNetwork    string
	PublicPort         int
	AdminPort          int
	PublicURL          string
	CanonicalPublicURL string
	AdminURL           string
	PublicAccess       serviceaccess.HTTPGatewayFiles
	AdminAccess        serviceaccess.HTTPGatewayFiles
}

func EnsureKeycloakFilesAt(ctx context.Context, app application.Manifest, issuer serviceaccess.Issuer, dataDir, namespace string) (KeycloakFiles, error) {
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		return KeycloakFiles{}, err
	}
	if placement.Scope == capability.ScopeExternal {
		return KeycloakFiles{}, errors.New("external identity must use the external OIDC provider")
	}
	consumer, err := application.IdentityProviderNetworkName(app, namespace)
	if err != nil {
		return KeycloakFiles{}, err
	}
	dir, project, err := keycloakStateIdentity(app, placement, dataDir, namespace)
	if err != nil {
		return KeycloakFiles{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return KeycloakFiles{}, fmt.Errorf("create Keycloak provider state: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return KeycloakFiles{}, err
	}
	files := KeycloakFiles{
		Dir:             dir,
		Compose:         filepath.Join(dir, "compose.yaml"),
		Env:             filepath.Join(dir, "runtime.env"),
		Project:         project,
		ConsumerNetwork: consumer,
		InternalNetwork: consumer + "-internal",
	}
	values, err := readProtectedEnv(files.Env)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return KeycloakFiles{}, err
	}
	if values == nil {
		values = map[string]string{}
	}
	if values["BASEHARBOR_KEYCLOAK_PUBLIC_PORT"] == "" {
		port, err := allocateIdentityPort(nil)
		if err != nil {
			return KeycloakFiles{}, err
		}
		values["BASEHARBOR_KEYCLOAK_PUBLIC_PORT"] = strconv.Itoa(port)
	}
	publicPort, err := parseIdentityPort(values["BASEHARBOR_KEYCLOAK_PUBLIC_PORT"])
	if err != nil {
		return KeycloakFiles{}, err
	}
	// Native Keycloak HTTPS serves both BaseHarbor's OIDC and administrative
	// control paths on one loopback listener. Keep the legacy environment key
	// synchronized for state compatibility without allocating a second port.
	values["BASEHARBOR_KEYCLOAK_ADMIN_PORT"] = strconv.Itoa(publicPort)
	for _, key := range []string{"BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD", "BASEHARBOR_KEYCLOAK_DB_PASSWORD"} {
		if strings.TrimSpace(values[key]) == "" {
			secret, err := randomIdentitySecret(32)
			if err != nil {
				return KeycloakFiles{}, err
			}
			values[key] = secret
		}
	}
	values["BASEHARBOR_KEYCLOAK_ADMIN_USER"] = "baseharbor-admin"
	values["BASEHARBOR_KEYCLOAK_DB_USER"] = "keycloak"
	values["BASEHARBOR_KEYCLOAK_DB_NAME"] = "keycloak"
	if err := writeProtectedEnv(files.Env, values); err != nil {
		return KeycloakFiles{}, err
	}

	publicPolicy, err := serviceaccess.Resolve(app.Environment, "keycloak-public", serviceaccess.AuthenticationNative)
	if err != nil {
		return KeycloakFiles{}, err
	}
	publicPolicy.ServerName = keycloakPublicHost
	providerAlias := devaccess.ProviderAlias(files.Project, "identity")
	providerAdminAlias := devaccess.ProviderAlias(files.Project, "identity-admin")
	nativeMaterial, err := serviceaccess.EnsureTLSMaterial(
		ctx,
		issuer,
		publicPolicy,
		filepath.Join(dir, "native-tls", "pki"),
		keycloakPublicHost,
		providerAlias,
		providerAdminAlias,
		"keycloak",
		"127.0.0.1",
	)
	if err != nil {
		return KeycloakFiles{}, err
	}
	nativeMaterial, err = projectKeycloakTLSMaterial(filepath.Join(dir, "native-tls", "runtime"), nativeMaterial)
	if err != nil {
		return KeycloakFiles{}, err
	}
	publicAccess := serviceaccess.HTTPGatewayFiles{Dir: filepath.Join(dir, "native-tls"), Material: nativeMaterial}
	adminAccess := publicAccess

	files.PublicPort = publicPort
	files.AdminPort = publicPort
	files.PublicURL = fmt.Sprintf("https://%s:%d", keycloakPublicHost, publicPort)
	files.CanonicalPublicURL = files.PublicURL
	if devaccess.Enabled(app.Environment) {
		var host string
		if placement.Scope == capability.ScopeShared {
			host, err = devaccess.SharedHost(namespace, "identity")
		} else {
			host, err = devaccess.ApplicationHost(namespace, app.Name, "identity")
		}
		if err != nil {
			return KeycloakFiles{}, err
		}
		files.CanonicalPublicURL = devaccess.CanonicalURL(host)
	}
	files.AdminURL = fmt.Sprintf("https://127.0.0.1:%d", publicPort)
	files.PublicAccess = publicAccess
	files.AdminAccess = adminAccess

	compose := keycloakCompose(app, files)
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		return KeycloakFiles{}, fmt.Errorf("write Keycloak provider compose: %w", err)
	}
	if err := os.Chmod(files.Compose, 0o600); err != nil {
		return KeycloakFiles{}, err
	}
	return files, nil
}

func DestroyAllSharedKeycloakAt(ctx context.Context, runtime KeycloakRuntime, dataDir, namespace string) error {
	root := filepath.Join(filepath.Clean(dataDir), "providers", "keycloak", "shared")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	project := bhruntime.SharedProjectName(namespace)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		compose := filepath.Join(dir, "compose.yaml")
		env := filepath.Join(dir, "runtime.env")
		if _, err := os.Stat(compose); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := runtime.DestroyProject(ctx, project, compose, env); err != nil {
			return err
		}
	}
	return os.RemoveAll(root)
}

func keycloakStateIdentity(app application.Manifest, placement capability.ProviderPlacement, dataDir, namespace string) (string, string, error) {
	root := filepath.Join(filepath.Clean(dataDir), "providers", "keycloak")
	switch placement.Scope {
	case capability.ScopeShared:
		boundary := strings.TrimSpace(placement.SharingBoundary)
		if boundary == "" {
			boundary = "default"
		}
		dir := filepath.Join(root, "shared", boundary)
		return dir, bhruntime.SharedProjectName(namespace), nil
	case capability.ScopeApplication:
		dir := filepath.Join(root, "applications", app.Name, app.Environment)
		return dir, bhruntime.ApplicationProjectName(namespace, app.Name+"-identity", app.Environment), nil
	default:
		return "", "", fmt.Errorf("unsupported Keycloak placement scope %q", placement.Scope)
	}
}

func SetKeycloakCanonicalURL(files KeycloakFiles, canonicalURL string) error {
	canonicalURL = strings.TrimRight(strings.TrimSpace(canonicalURL), "/")
	if canonicalURL == "" {
		return errors.New("Keycloak canonical URL is required")
	}
	u, err := url.Parse(canonicalURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("Keycloak canonical URL %q must be an HTTPS URL without query or fragment", canonicalURL)
	}
	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return err
	}
	values["BASEHARBOR_KEYCLOAK_CANONICAL_URL"] = canonicalURL
	delete(values, "BASEHARBOR_KEYCLOAK_CANONICAL_ADMIN_URL")
	return writeProtectedEnv(files.Env, values)
}

func keycloakCompose(app application.Manifest, files KeycloakFiles) string {
	hostnameCommand := ""
	hostnameEnvironment := "      KC_HOSTNAME: ${BASEHARBOR_KEYCLOAK_CANONICAL_URL}\n"
	if devaccess.Enabled(app.Environment) {
		hostnameCommand = "      - --hostname-strict=false\n"
	}
	return fmt.Sprintf(`services:
  keycloak-db:
    image: docker.io/library/postgres:18-alpine
    restart: unless-stopped
    security_opt: ["no-new-privileges:true"]
    environment:
      POSTGRES_DB: ${BASEHARBOR_KEYCLOAK_DB_NAME}
      POSTGRES_USER: ${BASEHARBOR_KEYCLOAK_DB_USER}
      POSTGRES_PASSWORD: ${BASEHARBOR_KEYCLOAK_DB_PASSWORD}
    volumes:
      - keycloak-db-data:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${BASEHARBOR_KEYCLOAK_DB_USER} -d ${BASEHARBOR_KEYCLOAK_DB_NAME}"]
      interval: 2s
      timeout: 2s
      retries: 60
      start_period: 2s
    networks:
      - identity-internal

  keycloak:
    image: %s
    restart: unless-stopped
    user: "1000:0"
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    depends_on:
      keycloak-db:
        condition: service_healthy
    command:
      - start
      - --http-enabled=false
      - --https-port=%d
      - --https-certificate-file=/run/baseharbor/tls/server.pem
      - --https-certificate-key-file=/run/baseharbor/tls/server-key.pem
      - --https-certificates-reload-period=30s
%s      - --health-enabled=true
      - --metrics-enabled=true
    environment:
      KC_BOOTSTRAP_ADMIN_USERNAME: ${BASEHARBOR_KEYCLOAK_ADMIN_USER}
      KC_BOOTSTRAP_ADMIN_PASSWORD: ${BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD}
      KC_DB: postgres
      KC_DB_URL: jdbc:postgresql://keycloak-db:5432/${BASEHARBOR_KEYCLOAK_DB_NAME}
      KC_DB_USERNAME: ${BASEHARBOR_KEYCLOAK_DB_USER}
      KC_DB_PASSWORD: ${BASEHARBOR_KEYCLOAK_DB_PASSWORD}
%s    ports:
      - "127.0.0.1:${BASEHARBOR_KEYCLOAK_PUBLIC_PORT}:%d"
    volumes:
      - ./native-tls/runtime/server.pem:/run/baseharbor/tls/server.pem:ro
      - ./native-tls/runtime/server-key.pem:/run/baseharbor/tls/server-key.pem:ro
      - ./native-tls/runtime/ca.pem:/run/baseharbor/tls/ca.pem:ro
    healthcheck:
      test: ["CMD-SHELL", "bash -c 'exec 3<>/dev/tcp/127.0.0.1/8443'"]
      interval: 2s
      timeout: 3s
      retries: 90
      start_period: 10s
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /opt/keycloak/data/tmp:rw,noexec,nosuid,nodev
    networks:
      identity-consumer:
        aliases:
          - %q
      identity-internal:
        aliases:
          - keycloak
          - %q

volumes:
  keycloak-db-data:

networks:
  identity-consumer:
    name: %s
  identity-internal:
    name: %s
`, KeycloakImage, keycloakHTTPSPort, hostnameCommand, hostnameEnvironment, keycloakHTTPSPort, devaccess.ProviderAlias(files.Project, "identity"), devaccess.ProviderAlias(files.Project, "identity-admin"), files.ConsumerNetwork, files.InternalNetwork)
}

func projectKeycloakTLSMaterial(dir string, material serviceaccess.TLSMaterial) (serviceaccess.TLSMaterial, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return serviceaccess.TLSMaterial{}, err
	}
	project := func(source, name string) (string, error) {
		data, err := os.ReadFile(source)
		if err != nil {
			return "", err
		}
		if len(data) == 0 {
			return "", fmt.Errorf("Keycloak TLS material %s is empty", name)
		}
		target := filepath.Join(dir, name)
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return "", err
		}
		return target, nil
	}
	var err error
	material.CA, err = project(material.CA, "ca.pem")
	if err != nil {
		return serviceaccess.TLSMaterial{}, err
	}
	material.ServerCertificate, err = project(material.ServerCertificate, "server.pem")
	if err != nil {
		return serviceaccess.TLSMaterial{}, err
	}
	material.ServerKey, err = project(material.ServerKey, "server-key.pem")
	if err != nil {
		return serviceaccess.TLSMaterial{}, err
	}
	return material, nil
}

func randomIdentitySecret(bytes int) (string, error) {
	if bytes < 16 {
		return "", errors.New("identity secret length is too small")
	}
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func allocateIdentityPort(exclude map[int]struct{}) (int, error) {
	for attempt := 0; attempt < 16; attempt++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("allocate identity port: %w", err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if _, used := exclude[port]; used {
			continue
		}
		return port, nil
	}
	return 0, errors.New("allocate unique identity port")
}

func parseIdentityPort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("invalid Keycloak provider port")
	}
	return port, nil
}

func readProtectedEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, errors.New("invalid protected provider environment")
		}
		values[key] = value
	}
	return values, nil
}

func writeProtectedEnv(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortStrings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, values[key])
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
