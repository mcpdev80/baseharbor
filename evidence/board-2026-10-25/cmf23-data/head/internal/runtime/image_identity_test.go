package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedImageIdentitySelectsOCIIndexOverFirstPlatformManifest(t *testing.T) {
	index := "sha256:" + strings.Repeat("a", 64)
	platform := "sha256:" + strings.Repeat("b", 64)
	identity := ImageIdentity{Reference: "docker.io/library/postgres@" + index, ImageID: "config-id"}
	observed := "library/postgres@" + platform + "\ndocker.io/library/postgres@" + index + "\n"
	if got := (Compose{}).verifiedImageRepositoryDigest(t.Context(), identity, observed); got != "docker.io/library/postgres@"+index {
		t.Fatalf("index pin replaced by platform manifest: %q", got)
	}
}

func TestPlatformOnlyImageStoreMustResolveExactPinnedReferenceToRunningImage(t *testing.T) {
	index := "sha256:" + strings.Repeat("a", 64)
	platform := "sha256:" + strings.Repeat("b", 64)
	imageID := strings.Repeat("c", 64)
	identity := ImageIdentity{Reference: "docker.io/library/postgres@" + index, ImageID: "sha256:" + imageID}
	observed := "docker.io/library/postgres@" + platform
	command := filepath.Join(t.TempDir(), "podman")
	script := "#!/bin/sh\n[ \"$1 $2 $3 $4\" = 'image inspect --format {{.Id}}' ] || exit 1\n[ \"$5\" = '" + identity.Reference + "' ] || exit 1\nprintf '%s\\n' \"$RESOLVED_IMAGE_ID\"\n"
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	backend := NewCLIBackend(command)
	t.Setenv("RESOLVED_IMAGE_ID", imageID)
	if got := backend.verifiedImageRepositoryDigest(t.Context(), identity, observed); got != "docker.io/library/postgres@"+index {
		t.Fatalf("verified index-to-running-image relationship lost: %q", got)
	}
	t.Setenv("RESOLVED_IMAGE_ID", strings.Repeat("d", 64))
	if got := backend.verifiedImageRepositoryDigest(t.Context(), identity, observed); got != observed {
		t.Fatalf("different image ID admitted as requested index: %q", got)
	}
	t.Setenv("RESOLVED_IMAGE_ID", imageID)
	if got := backend.verifiedImageRepositoryDigest(t.Context(), identity, "quay.io/foreign/postgres@"+platform); got != "" {
		t.Fatalf("foreign repository supplied immutable identity: %q", got)
	}
}
