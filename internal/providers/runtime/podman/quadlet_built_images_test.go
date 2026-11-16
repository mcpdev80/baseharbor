package podman

import (
	"strings"
	"testing"
)

func TestQuadletNoBuildUsesCompletedLocalImageWithoutBuildDependency(t *testing.T) {
	p := QuadletProject{Files: map[string]string{
		"demo-app.container":   "[Container]\nImage=demo-app.build\nNetwork=demo.network\n",
		"demo-app.build":       "[Build]\nImageTag=localhost/demo-app:quadlet\n",
		"demo-proxy.container": "[Container]\nImage=caddy:2\n",
	}}
	got, err := quadletProjectWithBuiltImages(p)
	if err != nil {
		t.Fatal(err)
	}
	content := got.Files["demo-app.container"]
	if strings.Contains(content, "Image=demo-app.build") || !strings.Contains(content, "Image=localhost/demo-app:quadlet\nPull=never\n") || !strings.Contains(content, "Network=demo.network") {
		t.Fatalf("no-build container: %s", content)
	}
	if got.Files["demo-app.build"] != p.Files["demo-app.build"] || got.Files["demo-proxy.container"] != p.Files["demo-proxy.container"] || !strings.Contains(p.Files["demo-app.container"], "Image=demo-app.build") {
		t.Fatal("build source, registry service or original project changed")
	}
}

func TestQuadletNoBuildRejectsMissingBuildImageTag(t *testing.T) {
	_, err := quadletProjectWithBuiltImages(QuadletProject{Files: map[string]string{"demo.container": "[Container]\nImage=missing.build\n"}})
	if err == nil {
		t.Fatal("missing build tag accepted")
	}
}
