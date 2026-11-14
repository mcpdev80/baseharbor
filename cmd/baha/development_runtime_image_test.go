package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	controlplaneassets "github.com/mcpdev80/baseharbor/deploy/control-plane"
)

type developmentImageRuntime struct {
	registryPullRecordingCompose
	built          int
	binary, recipe []byte
	fail           error
}

func (r *developmentImageRuntime) BuildProjectFilesSelectedProgress(_ context.Context, _, dir string, _ map[string]string, _ []string, _ func(string), _ ...string) error {
	r.built++
	r.binary, _ = os.ReadFile(filepath.Join(dir, "baha"))
	r.recipe, _ = os.ReadFile(filepath.Join(dir, "Dockerfile"))
	return r.fail
}
func TestDevelopmentRuntimeImageUsesInstalledBinaryAndPreservesOverrides(t *testing.T) {
	previousVersion, previousCommit, previousExecutable := version, commit, developmentRuntimeExecutable
	t.Cleanup(func() {
		version, commit, developmentRuntimeExecutable = previousVersion, previousCommit, previousExecutable
	})
	version, commit = "dev", strings.Repeat("a", 40)
	binary := filepath.Join(t.TempDir(), "baha")
	if err := os.WriteFile(binary, []byte("candidate runtime binary"), 0700); err != nil {
		t.Fatal(err)
	}
	developmentRuntimeExecutable = func() (string, error) { return binary, nil }
	for _, tc := range []struct {
		name, image string
		fail        bool
		builds      int
	}{
		{"fresh default", "", false, 1},
		{"previous generated image", "localhost/baseharbor-runtime:dev-" + strings.Repeat("b", 64), false, 1},
		{"explicit candidate", "localhost/baseharbor-runtime:pre-release", false, 0},
		{"explicit registry", "registry.example/runtime@sha256:" + strings.Repeat("b", 64), false, 0},
		{"failed build", "", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_RUNTIME_IMAGE", tc.image)
			r := &developmentImageRuntime{}
			if tc.fail {
				r.fail = errors.New("build refused")
			}
			handled, err := ensureMatchingDevelopmentRuntimeImage(context.Background(), r, io.Discard)
			if r.built != tc.builds || handled != (tc.builds > 0) || (err != nil) != tc.fail {
				t.Fatalf("build selection: %t %v %d", handled, err, r.built)
			}
			if tc.fail || tc.builds == 0 {
				if os.Getenv("BASEHARBOR_RUNTIME_IMAGE") != tc.image {
					t.Fatal("failed build or explicit override changed image")
				}
				return
			}
			if string(r.binary) != "candidate runtime binary" || string(r.recipe) != controlplaneassets.BinaryDockerfile {
				t.Fatal("runtime did not use the exact installed binary/canonical recipe")
			}
			if !strings.HasPrefix(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"), "localhost/baseharbor-runtime:dev-") {
				t.Fatal("generated runtime not selected")
			}
		})
	}
}

func TestDevelopmentRuntimeImageSupportsDynamicallyLinkedDeveloperBuilds(t *testing.T) {
	for _, required := range []string{
		"FROM debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2",
		"apt-get install -y --no-install-recommends ca-certificates curl",
		"COPY baha /usr/local/bin/baha",
		"ENTRYPOINT [\"/usr/local/bin/baha\"]",
	} {
		if !strings.Contains(controlplaneassets.BinaryDockerfile, required) {
			t.Fatalf("runtime image recipe does not support the installed developer CLI: missing %q", required)
		}
	}
}
