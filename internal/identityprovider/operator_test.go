package identityprovider

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/devgateway"
)

type testKeycloakRuntime struct{ engine string }

func (r testKeycloakRuntime) Engine() string { return r.engine }
func (r testKeycloakRuntime) PreferredLocalHTTPSPort() int {
	if r.engine == "podman" {
		return 8443
	}
	return 443
}
func (r testKeycloakRuntime) ConfigProject(context.Context, string, string, string) error { return nil }
func (r testKeycloakRuntime) UpProject(context.Context, string, string, string) error     { return nil }
func (r testKeycloakRuntime) DestroyProject(context.Context, string, string, string) error {
	return nil
}

func TestManagedOperatorCanonicalBaseURL(t *testing.T) {
	tests := []struct {
		name          string
		engine        string
		preferredPort int
	}{
		{name: "docker", engine: "docker", preferredPort: 443},
		{name: "podman", engine: "podman", preferredPort: 8443},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			t.Setenv("BASEHARBOR_DEV_DOMAIN", "baha.localhost")
			got, err := managedOperatorCanonicalBaseURL(testKeycloakRuntime{engine: tt.engine}, "demo")
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(got)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Scheme != "https" || parsed.Hostname() != "auth.baha.localhost" {
				t.Fatalf("canonical base URL = %q, want HTTPS host auth.baha.localhost", got)
			}
			port := tt.preferredPort
			if raw := parsed.Port(); raw != "" {
				port, err = strconv.Atoi(raw)
				if err != nil {
					t.Fatalf("canonical base URL has invalid port %q", raw)
				}
			}
			if port != tt.preferredPort && port < 18443 {
				t.Fatalf("canonical base URL port = %d, want preferred %d or fallback >= 18443", port, tt.preferredPort)
			}
		})
	}
}

func TestManagedOperatorCanonicalBaseURLUsesPersistedGatewayFallback(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("BASEHARBOR_DEV_DOMAIN", "baha.localhost")

	files, err := devgateway.FilesFor("demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(files.State), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.State, []byte("{\"version\":1,\"host_port\":18443,\"routes\":[]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := managedOperatorCanonicalBaseURL(testKeycloakRuntime{engine: "docker"}, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://auth.baha.localhost:18443" {
		t.Fatalf("operator canonical base URL = %q, want persisted gateway fallback", got)
	}
}
