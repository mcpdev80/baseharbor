package identityprovider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	KeycloakImage      = "quay.io/keycloak/keycloak:26.7.4"
	KeycloakService    = "keycloak"
	keycloakPublicHost = "identity.localhost"
)

type KeycloakRuntime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

type KeycloakFiles struct {
	Dir             string
	Compose         string
	Env             string
	Project         string
	ConsumerNetwork string
	InternalNetwork string
	PublicPort      int
	AdminPort       int
	PublicURL       string
	AdminURL        string
	PublicAccess    serviceaccess.HTTPGatewayFiles
	AdminAccess     serviceaccess.HTTPGatewayFiles
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
	if values["BASEHARBOR_KEYCLOAK_ADMIN_PORT"] == "" {
		port, err := allocateIdentityPort(map[int]struct{}{publicPort: {}})
		if err != nil {
			return KeycloakFiles{}, err
		}
		values["BASEHARBOR_KEYCLOAK_ADMIN_PORT"] = strconv.Itoa(port)
	}
	adminPort, err := parseIdentityPort(values["BASEHARBOR_KEYCLOAK_ADMIN_PORT"])
	if err != nil {
		return KeycloakFiles{}, err
	}
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
	publicSpec := serviceaccess.HTTPGatewaySpec{
		ServiceName:      "keycloak-public",
		Upstream:         "http://keycloak:8080",
		PublishedPortEnv: "BASEHARBOR_KEYCLOAK_PUBLIC_PORT",
		ContainerPort:    publicPort,
		Networks:         []string{"identity-consumer", "identity-internal"},
		DenyPaths:        []string{"/admin"},
	}
	publicAccess, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, publicPolicy, filepath.Join(dir, "public"), publicSpec)
	if err != nil {
		return KeycloakFiles{}, err
	}

	adminPolicy, err := serviceaccess.Resolve(app.Environment, "keycloak-admin", serviceaccess.AuthenticationNative)
	if err != nil {
		return KeycloakFiles{}, err
	}
	adminPolicy.ServerName = "localhost"
	adminSpec := serviceaccess.HTTPGatewaySpec{
		ServiceName:      "keycloak-admin",
		Upstream:         "http://keycloak:8080",
		PublishedPortEnv: "BASEHARBOR_KEYCLOAK_ADMIN_PORT",
		ContainerPort:    9443,
		Networks:         []string{"identity-internal"},
	}
	adminAccess, err := serviceaccess.EnsureHTTPGateway(ctx, issuer, adminPolicy, filepath.Join(dir, "admin"), adminSpec)
	if err != nil {
		return KeycloakFiles{}, err
	}

	files.PublicPort = publicPort
	files.AdminPort = adminPort
	files.PublicURL = fmt.Sprintf("https://%s:%d", keycloakPublicHost, publicPort)
	files.AdminURL = fmt.Sprintf("https://127.0.0.1:%d", adminPort)
	files.PublicAccess = publicAccess
	files.AdminAccess = adminAccess

	compose := keycloakCompose(files, publicSpec, adminSpec)
	if err := os.WriteFile(files.Compose, []byte(compose), 0o600); err != nil {
		return KeycloakFiles{}, fmt.Errorf("write Keycloak provider compose: %w", err)
	}
	if err := os.Chmod(files.Compose, 0o600); err != nil {
		return KeycloakFiles{}, err
	}
	return files, nil
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

func keycloakCompose(files KeycloakFiles, publicSpec, adminSpec serviceaccess.HTTPGatewaySpec) string {
	publicGateway := serviceaccess.HTTPGatewayComposeService(files.PublicAccess, publicSpec)
	adminGateway := serviceaccess.HTTPGatewayComposeService(files.AdminAccess, adminSpec)
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
    networks:
      - identity-internal

  keycloak:
    image: %s
    restart: unless-stopped
    user: "1000:0"
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    depends_on:
      - keycloak-db
    command:
      - start
      - --http-enabled=true
      - --http-port=8080
      - --proxy-headers=xforwarded
      - --health-enabled=true
      - --metrics-enabled=true
    environment:
      KC_BOOTSTRAP_ADMIN_USERNAME: ${BASEHARBOR_KEYCLOAK_ADMIN_USER}
      KC_BOOTSTRAP_ADMIN_PASSWORD: ${BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD}
      KC_DB: postgres
      KC_DB_URL: jdbc:postgresql://keycloak-db:5432/${BASEHARBOR_KEYCLOAK_DB_NAME}
      KC_DB_USERNAME: ${BASEHARBOR_KEYCLOAK_DB_USER}
      KC_DB_PASSWORD: ${BASEHARBOR_KEYCLOAK_DB_PASSWORD}
      KC_HOSTNAME: https://%s:${BASEHARBOR_KEYCLOAK_PUBLIC_PORT}
      KC_HTTP_MANAGEMENT_SCHEME: http
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /opt/keycloak/data/tmp:rw,noexec,nosuid,nodev
    networks:
      - identity-internal

%s
%s
volumes:
  keycloak-db-data:

networks:
  identity-consumer:
    name: %s
  identity-internal:
    name: %s
`, KeycloakImage, keycloakPublicHost, publicGateway, adminGateway, files.ConsumerNetwork, files.InternalNetwork)
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
