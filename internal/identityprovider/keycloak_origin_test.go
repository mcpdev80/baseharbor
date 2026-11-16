package identityprovider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/devgateway"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestFirstSharedConsumerBindsServerOriginToActualGateway(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		for _, previous := range []string{"", "https://identity.localhost:45678", "https://installation.example:9443"} {
			t.Run(engine+"/"+previous, func(t *testing.T) {
				t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
				t.Setenv("BASEHARBOR_DEV_DOMAIN", "baha.localhost")
				gateway, err := devgateway.FilesFor("local")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(gateway.State), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(gateway.State, []byte(`{"version":1,"host_port":18443,"routes":[]}`), 0o600); err != nil {
					t.Fatal(err)
				}
				files := KeycloakFiles{Env: filepath.Join(t.TempDir(), "runtime.env"), PublicURL: "https://identity.localhost:45678", SharedSQL: &bhruntime.Files{}}
				if err := writeProtectedEnv(files.Env, map[string]string{"BASEHARBOR_KEYCLOAK_CANONICAL_URL": previous}); err != nil {
					t.Fatal(err)
				}
				if err := prepareApplicationKeycloakCanonicalURL(testKeycloakRuntime{engine: engine}, "local", files, "https://application.example"); err != nil {
					t.Fatal(err)
				}
				values, err := readProtectedEnv(files.Env)
				if err != nil {
					t.Fatal(err)
				}
				want := "https://auth.baha.localhost:18443"
				if previous == "https://installation.example:9443" {
					want = previous
				}
				if got := values["BASEHARBOR_KEYCLOAK_CANONICAL_URL"]; got != want {
					t.Fatalf("server origin = %q, want %q", got, want)
				}
			})
		}
	}
}
