package development

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRenderBackstageCatalogIsStaticContractProjection(t *testing.T) {
	manifest := application.Manifest{Version: application.CurrentVersion, Name: "Catalog API", Environment: "dev"}
	contract := application.PortableContract{Application: "Catalog API", Capabilities: []capability.Requirement{{Kind: capability.SQL}, {Kind: capability.ExposureHTTP}}}
	out := RenderBackstageCatalog(manifest, contract)
	for _, want := range []string{"apiVersion: backstage.io/v1alpha1", "kind: Component", "name: catalog-api", "baseharbor.dev/contract-version: v1", "baseharbor.dev/environment: dev", "- database-sql", "- exposure-http"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "owner:") {
		t.Fatal("BaseHarbor must not infer Backstage ownership")
	}
	if strings.Contains(out, "${{") {
		t.Fatal("static catalog emission must not include template expressions")
	}
}
