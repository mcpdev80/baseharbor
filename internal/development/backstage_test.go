package development

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRenderBackstageCatalogIsStaticContractProjection(t *testing.T) {
	manifest := application.Manifest{Version: application.CurrentVersion, Name: "Catalog API", Environment: "prod"}
	out, err := RenderBackstageCatalog(manifest, BackstageCatalogOptions{Owner: "platform-team"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"apiVersion: backstage.io/v1alpha1",
		"kind: Component",
		"name: catalog-api",
		"baseharbor.dev/application: catalog-api",
		"lifecycle: experimental",
		"owner: platform-team",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog output missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"environment", "target", "runtime", "provider", "${" + "{"} {
		if strings.Contains(strings.ToLower(out), forbidden) {
			t.Fatalf("catalog output leaked forbidden %q:\n%s", forbidden, out)
		}
	}
}

func TestRenderBackstageCatalogRequiresExplicitOwner(t *testing.T) {
	_, err := RenderBackstageCatalog(application.Manifest{Name: "app"}, BackstageCatalogOptions{})
	if err == nil {
		t.Fatal("expected explicit owner requirement")
	}
}
