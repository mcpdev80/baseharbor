package serviceaccess

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaddyfileRequiresClientCertificateWhenRequested(t *testing.T) {
	got := caddyfile("http://prometheus:9090", 8443, AuthenticationMTLS, "", "")
	for _, want := range []string{"auto_https disable_redirects", "tls /certs/server.pem /certs/server-key.pem", "mode require_and_verify", "reverse_proxy http://prometheus:9090"} {
		if !strings.Contains(got, want) {
			t.Fatalf("gateway config missing %q:\n%s", want, got)
		}
	}
}

func TestHTTPClientDoesNotLoadOptionalClientIdentity(t *testing.T) {
	dir := t.TempDir()
	policy, err := Resolve("dev", "test-http-client", AuthenticationNative)
	if err != nil {
		t.Fatal(err)
	}
	material, err := EnsureTLSMaterial(context.Background(), newTestIssuer(t), policy, dir, "localhost", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	material.ClientCertificate = filepath.Join(dir, "missing-client.pem")
	material.ClientKey = filepath.Join(dir, "missing-client-key.pem")
	if _, err := NewHTTPClient(material, false); err != nil {
		t.Fatalf("optional client identity must be ignored when mTLS is not required: %v", err)
	}
}

func TestGatewayComposePublishesOnlyTLSPort(t *testing.T) {
	files := HTTPGatewayFiles{
		Caddyfile: "/tmp/access/Caddyfile",
		Material: TLSMaterial{
			CA:                "/tmp/access/ca.pem",
			ServerCertificate: "/tmp/access/server.pem",
			ServerKey:         "/tmp/access/server-key.pem",
		},
	}
	got := HTTPGatewayComposeService(files, HTTPGatewaySpec{
		ServiceName:      "prometheus-access",
		PublishedPortEnv: "BASEHARBOR_PROMETHEUS_PORT",
		ContainerPort:    8443,
		Networks:         []string{"provider"},
	})
	if !strings.Contains(got, "127.0.0.1:$"+"{BASEHARBOR_PROMETHEUS_PORT}:8443") {
		t.Fatalf("gateway does not publish TLS loopback port:\n%s", got)
	}
	if strings.Contains(got, ":9090") {
		t.Fatalf("gateway unexpectedly publishes upstream clear-text port:\n%s", got)
	}
}

func TestCaddyfileRequiresBearerTokenWhenSelected(t *testing.T) {
	got := caddyfile("http://prometheus:9090", 8443, AuthenticationToken, "", "")
	for _, want := range []string{
		"Bearer {$BASEHARBOR_ACCESS_TOKEN}",
		"respond @unauthorized 401",
		"reverse_proxy http://prometheus:9090",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("token gateway config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "client_auth") {
		t.Fatalf("token gateway unexpectedly requires mTLS:\n%s", got)
	}
}

func TestCaddyfileSupportsHashedBasicAuth(t *testing.T) {
	got := caddyfile("http://prometheus:9090", 8443, AuthenticationNative, "developer", "$2a$10$example")
	for _, want := range []string{
		"basic_auth",
		"developer $2a$10$example",
		"reverse_proxy http://prometheus:9090",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("basic auth gateway config missing %q:\n%s", want, got)
		}
	}
}

func TestGatewayComposeRunsCaddyWithLeastPrivilege(t *testing.T) {
	files := HTTPGatewayFiles{
		Caddyfile: "/tmp/access/Caddyfile",
		Material: TLSMaterial{
			CA:                "/tmp/access/ca.pem",
			ServerCertificate: "/tmp/access/server.pem",
			ServerKey:         "/tmp/access/server-key.pem",
		},
	}
	got := HTTPGatewayComposeService(files, HTTPGatewaySpec{ServiceName: "openbao-access", ContainerPort: 8443})
	for _, want := range []string{
		"cap_drop: [\"ALL\"]",
		"entrypoint: [\"/bin/sh\", \"-ec\"]",
		"/run/baseharbor:rw,exec,nosuid,nodev,mode=1777",
		"cat /usr/bin/caddy > /run/baseharbor/caddy",
		"chmod 0755 /run/baseharbor/caddy",
		"exec /run/baseharbor/caddy run --watch --config /etc/caddy/Caddyfile --adapter caddyfile",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("gateway compose missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "cap_add:") {
		t.Fatalf("gateway on unprivileged port must not add capabilities:\n%s", got)
	}

	privilegedPort := HTTPGatewayComposeService(files, HTTPGatewaySpec{ServiceName: "https-access", ContainerPort: 443})
	if !strings.Contains(privilegedPort, "cap_add: [\"NET_BIND_SERVICE\"]") {
		t.Fatalf("gateway on privileged port must add NET_BIND_SERVICE:\n%s", privilegedPort)
	}
}

func TestHTTPGatewayMultipleUpstreams(t *testing.T) {
	got := caddyfileWithUpstreamsTLS(
		[]string{"http://member-a:8080", "http://member-b:8080"},
		"",
		"",
		8443,
		AuthenticationNone,
		"",
		"",
	)
	for _, want := range []string{
		"reverse_proxy http://member-a:8080 http://member-b:8080",
		"lb_policy round_robin",
		"lb_try_duration 5s",
		"fail_duration 30s",
		"max_fails 2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("gateway config missing %q:\n%s", want, got)
		}
	}
}

func TestNormalizeGatewayUpstreamsRejectsInvalidScheme(t *testing.T) {
	if _, err := normalizedGatewayUpstreams("", []string{"member-a:8080"}); err == nil {
		t.Fatal("invalid upstream scheme accepted")
	}
}

func TestHTTPGatewayKeepsPrivateStateSeparateFromReadableRuntimeConfig(t *testing.T) {
	dir := t.TempDir()
	policy, err := Resolve("prod", "gateway-permissions", AuthenticationNative)
	if err != nil {
		t.Fatal(err)
	}
	files, err := EnsureHTTPGateway(context.Background(), newTestIssuer(t), policy, dir, HTTPGatewaySpec{
		ServiceName:   "gateway-permissions",
		Upstream:      "http://upstream:8080",
		ContainerPort: 8443,
		Networks:      []string{"provider"},
	})
	if err != nil {
		t.Fatal(err)
	}
	stateInfo, err := os.Stat(files.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := stateInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("service-access state mode = %o, want 700", got)
	}
	configDir := filepath.Dir(files.Caddyfile)
	configInfo, err := os.Stat(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := configInfo.Mode().Perm(); got != 0o755 {
		t.Fatalf("gateway runtime config dir mode = %o, want 755", got)
	}
	configInfo, err = os.Stat(files.Caddyfile)
	if err != nil {
		t.Fatal(err)
	}
	if got := configInfo.Mode().Perm(); got != 0o644 {
		t.Fatalf("Caddyfile mode = %o, want 644", got)
	}
	if filepath.Dir(configDir) != files.Dir {
		t.Fatalf("Caddy runtime config escaped private service-access state: %s", files.Caddyfile)
	}
}

func TestProjectGatewayMaterialProjectsClientIdentity(t *testing.T) {
	dir := t.TempDir()
	source := t.TempDir()
	write := func(name, value string) string {
		t.Helper()
		path := filepath.Join(source, name)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	material := TLSMaterial{
		CA:                write("ca.pem", "ca"),
		ServerCertificate: write("server.pem", "server-cert"),
		ServerKey:         write("server-key.pem", "server-key"),
		ClientCertificate: write("client.pem", "client-cert"),
		ClientKey:         write("client-key.pem", "client-key"),
	}
	projected, err := projectGatewayMaterial(dir, material)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{material.ServerKey, material.ClientKey} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("authoritative private material %s mode = %o, want 600", path, got)
		}
	}
	runtimeDir := filepath.Join(dir, "runtime")
	for _, path := range []string{
		projected.CA,
		projected.ServerCertificate,
		projected.ServerKey,
		projected.ClientCertificate,
		projected.ClientKey,
	} {
		if filepath.Dir(path) != runtimeDir {
			t.Fatalf("projected material %s is outside runtime projection %s", path, runtimeDir)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Fatalf("projected material %s mode = %o, want 644", path, got)
		}
	}
}


func TestHTTPGatewayRelativeUpstreamTLSBindIsExplicit(t *testing.T) {
	compose := HTTPGatewayComposeService(
		HTTPGatewayFiles{
			Caddyfile: "./service-access/config/Caddyfile",
			Material: TLSMaterial{
				ServerCertificate: "./service-access/runtime/server.pem",
				ServerKey:         "./service-access/runtime/server-key.pem",
				ServerName:        "otel-collector",
			},
		},
		HTTPGatewaySpec{
			ServiceName:               "otel-collector-access",
			UpstreamTrustFile:         "members/service-access/runtime/ca.pem",
			UpstreamClientCertificate: "members/service-access/runtime/client-cert.pem",
			UpstreamClientKey:         "members/service-access/runtime/client-key.pem",
			ContainerPort:             4318,
		},
	)
	if !strings.Contains(compose, "./members/service-access/runtime:/upstream:ro") {
		t.Fatalf("relative upstream TLS directory is not rendered as an explicit bind mount:\n%s", compose)
	}
}
