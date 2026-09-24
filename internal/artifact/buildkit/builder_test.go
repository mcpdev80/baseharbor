package buildkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/artifact"
)

func TestBuilderReturnsCanonicalDigestReference(t *testing.T) {
	root := t.TempDir()
	argsFile := filepath.Join(root, "args.txt")
	command := filepath.Join(root, "buildctl")
	script := `#!/bin/sh
set -eu
printf '%s
' "$@" > "$BASEHARBOR_TEST_ARGS"
metadata=""
previous=""
for arg in "$@"; do
  if [ "$previous" = "--metadata-file" ]; then
    metadata="$arg"
  fi
  previous="$arg"
done
test -n "$metadata"
printf '%s
' '{"containerimage.digest":"sha256:0123456789abcdef"}' > "$metadata"
`
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BASEHARBOR_TEST_ARGS", argsFile)
	builder := Builder{Command: command, Address: "unix:///tmp/buildkit.sock"}
	got, err := builder.Build(
		context.Background(),
		artifact.BuildRequest{
			Service:    "demo-app",
			Context:    root,
			Dockerfile: "Dockerfile",
		},
		"registry.example/demo:dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reference != "registry.example/demo:dev@sha256:0123456789abcdef" {
		t.Fatalf("reference = %q", got.Reference)
	}
	if got.Digest != "sha256:0123456789abcdef" {
		t.Fatalf("digest = %q", got.Digest)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(args)
	for _, want := range []string{
		"--addr",
		"unix:///tmp/buildkit.sock",
		"--frontend",
		"dockerfile.v0",
		"context=" + root,
		"dockerfile=" + root,
		"filename=Dockerfile",
		"type=image,name=registry.example/demo:dev,push=true,oci-mediatypes=true,name-canonical=true",
		"--metadata-file",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("buildctl arguments missing %q:
%s", want, text)
		}
	}
}

func TestBuilderRejectsDigestDestinationBeforeBuild(t *testing.T) {
	builder := Builder{Command: "/bin/true"}
	_, err := builder.Build(
		context.Background(),
		artifact.BuildRequest{Service: "demo-app", Context: t.TempDir()},
		"registry.example/demo@sha256:deadbeef",
	)
	if err == nil {
		t.Fatal("digest destination unexpectedly accepted")
	}
}
