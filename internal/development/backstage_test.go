package development

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRenderBackstageCatalogRequiresExplicitOwner(t *testing.T) {
	_, err := RenderBackstageCatalog(application.New("catalog-api", "prod", true, false, false), BackstageCatalogOptions{})
	if err == nil {
		t.Fatal("expected explicit Backstage owner requirement")
	}
}

func TestRenderBackstageCatalogDoesNotLeakDeploymentSemantics(t *testing.T) {
	manifest := application.New("catalog-api", "prod", true, false, false)
	rendered, err := RenderBackstageCatalog(manifest, BackstageCatalogOptions{Owner: "platform-team"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"environment:", "prod", "target:", "provider:", "placement:"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("catalog metadata leaked deployment semantic %q: %s", forbidden, rendered)
		}
	}
	for _, required := range []string{"backstage.io/v1alpha1", "kind: Component", "owner: platform-team", "lifecycle: experimental"} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("catalog metadata missing %q: %s", required, rendered)
		}
	}
}
