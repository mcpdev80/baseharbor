package podman

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestQuadletServiceImagesDeduplicateAndPreserveSelectedLocalBuilds(t *testing.T) {
	p := QuadletProject{Files: map[string]string{
		"demo-a.container":      "[Container]\nImage=quay.io/keycloak/keycloak:26.8.0\n",
		"demo-b.container":      "[Container]\nImage=quay.io/keycloak/keycloak:26.8.0\n",
		"demo-local.container":  "[Container]\nImage=localhost/baseharbor-runtime:test\nPull=never\n",
		"demo-built.container":  "[Container]\nImage=demo-built.build\n",
		"demo-unused.container": "[Container]\nImage=example.invalid/unused:latest\n",
	}}
	units := []string{"demo-a.service", "demo-b.service", "demo-local.service", "demo-built.service"}
	if got := quadletServiceRegistryImages(p, units); !reflect.DeepEqual(got, []string{"quay.io/keycloak/keycloak:26.8.0"}) {
		t.Fatalf("images = %v", got)
	}
}

func TestQuadletPrepareImagesPullsMissingImageOnceAndSkipsCachedImage(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("IMAGE_TEST_LOG", log)
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$IMAGE_TEST_LOG\"\nif [ \"$1 $2\" = 'image exists' ]; then [ \"$3\" = 'cached:1' ]; else [ \"$1\" = 'pull' ]; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	p := QuadletProject{Files: map[string]string{
		"demo-a.container": "[Container]\nImage=missing:1\n",
		"demo-b.container": "[Container]\nImage=missing:1\n",
		"demo-c.container": "[Container]\nImage=cached:1\n",
	}}
	if err := quadletPrepareServiceImages(context.Background(), p, []string{"demo-a.service", "demo-b.service", "demo-c.service"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(data)), "image exists cached:1\nimage exists missing:1\npull missing:1"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}
