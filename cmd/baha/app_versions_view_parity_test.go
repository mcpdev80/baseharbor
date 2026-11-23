package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestUnifiedViewPinnedImagesMatchRenderers is the separation regression for
// the unified component rows: the pinned backend images the view reports must
// stay identical to the images the shipped runtime inventory recognizes. A
// provider image move that forgets this view fails here instead of drifting
// into two parallel realities.
func TestUnifiedViewPinnedImagesMatchRenderers(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "application", "shared_backends_inventory.go"))
	if err != nil {
		t.Fatalf("read provider inventory: %v", err)
	}
	dir, err := os.ReadFile(filepath.Join("..", "..", "internal", "application", "mongodb_runtime.go"))
	if err != nil {
		t.Fatalf("read mongodb renderer: %v", err)
	}
	rabbit, err := os.ReadFile(filepath.Join("..", "..", "internal", "application", "rabbitmq_ha.go"))
	if err != nil {
		t.Fatalf("read rabbitmq renderer: %v", err)
	}
	postgresRenderer, err := os.ReadFile(filepath.Join("..", "..", "internal", "application", "shared_backends_runtime.go"))
	if err != nil {
		t.Fatalf("read backend renderer: %v", err)
	}
	source := strings.Join([]string{string(raw), string(dir), string(rabbit), string(postgresRenderer)}, "\n")
	cases := map[string]string{
		"postgres": "docker.io/library/postgres:18-alpine",
		"valkey":   "docker.io/valkey/valkey:9.1.2-alpine",
		"mongodb":  application.MongoDBImage,
	}
	for kind, image := range cases {
		if !strings.Contains(source, image) {
			t.Fatalf("%s: the provider inventory does not recognize %q; the view must not claim an image the product does not manage", kind, image)
		}
		if kind == "mongodb" {
			continue
		}
		if got := pinnedRuntimeImage(kind); got != image {
			t.Fatalf("view pins %q for %s but the inventory recognizes %q", got, kind, image)
		}
	}
	if got := pinnedRuntimeImage("rabbitmq"); !strings.Contains(string(rabbit), got) {
		t.Fatalf("view pins %q but the rabbitmq renderer ships something else", got)
	}
	if got := pinnedRuntimeImage("unknown"); got != "" {
		t.Fatalf("an unknown kind must yield no image, not an invented one: %q", got)
	}
}
