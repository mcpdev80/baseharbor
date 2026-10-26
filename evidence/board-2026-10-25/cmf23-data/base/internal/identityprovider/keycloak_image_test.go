package identityprovider

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"go.yaml.in/yaml/v3"
)

func TestKeycloakBootstrapUsesImmutableReleaseInventory(t *testing.T) {
	data, err := os.ReadFile("../coreupdate/releases/v0.4.24.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Providers []struct{ Kind, Image, Digest string } `json:"providers"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, provider := range catalog.Providers {
		if provider.Kind == "identity" {
			want = provider.Image + "@" + provider.Digest
		}
	}
	if want == "" || !strings.Contains(want, "@sha256:") {
		t.Fatal("release has no immutable Identity image")
	}
	for _, ha := range []bool{false, true} {
		t.Run(fmt.Sprintf("ha=%v", ha), func(t *testing.T) {
			app := application.WithHA(application.New("demo", "prod", false, false, false), ha)
			files := KeycloakFiles{Project: "owned", ConsumerNetwork: "consumer", InternalNetwork: "internal"}
			var doc struct {
				Services map[string]struct{ Image string } `yaml:"services"`
			}
			if err := yaml.Unmarshal([]byte(keycloakCompose(app, files)), &doc); err != nil {
				t.Fatal(err)
			}
			members := 1
			if ha {
				members = 3
			}
			for n := 1; n <= members; n++ {
				if got := doc.Services[fmt.Sprintf("keycloak-%d", n)].Image; got != want {
					t.Fatalf("member %d uses mutable or mismatched image %q, want %q", n, got, want)
				}
			}
		})
	}
}
