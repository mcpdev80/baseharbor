package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeProcessEnvironmentOverridesInheritedValue(t *testing.T) {
	t.Setenv("SECRET_KEY", "shell-value")
	env, err := mergeProcessEnvironment(map[string]string{"SECRET_KEY": "openbao-value"})
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, "SECRET_KEY=") {
			matches++
			if entry != "SECRET_KEY=openbao-value" {
				t.Fatalf("unexpected SECRET_KEY entry %q", entry)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("SECRET_KEY entries=%d want 1", matches)
	}
	if got := os.Getenv("SECRET_KEY"); got != "shell-value" {
		t.Fatalf("merge mutated process environment: %q", got)
	}
}

func TestMergeProcessEnvironmentRejectsNUL(t *testing.T) {
	if _, err := mergeProcessEnvironment(map[string]string{"SECRET_KEY": "bad\x00value"}); err == nil {
		t.Fatal("expected NUL-containing environment value to be rejected")
	}
}

func TestComposeUpArgsAlwaysBuildCurrentRepositorySource(t *testing.T) {
	got := strings.Join(composeUpArgs(nil), " ")
	if got != "up -d --build" {
		t.Fatalf("compose up args=%q want %q", got, "up -d --build")
	}

	got = strings.Join(composeUpArgs([]string{"api", "worker"}), " ")
	if got != "up -d --build --no-deps api worker" {
		t.Fatalf("selected compose up args=%q want %q", got, "up -d --build --no-deps api worker")
	}
}

func TestComposeBuildArgsForcePlainProgress(t *testing.T) {
	got := strings.Join(composeBuildArgs([]string{"api"}), " ")
	if got != "build --progress plain api" {
		t.Fatalf("compose build args=%q want %q", got, "build --progress plain api")
	}
}


func TestComposeUpProjectFilesUsesResolvedWorkloadPortEnvironment(t *testing.T) {
	root := t.TempDir()
	compose := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(compose, []byte("services:\n  app:\n    image: example/app\n    ports:\n      - \"\${HTTP_PORT:-8080}:8080\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(root, "docker")
	script := "#!/bin/sh\nif [ \"$HTTP_PORT\" != \"8082\" ]; then echo \"HTTP_PORT=$HTTP_PORT\" >&2; exit 42; fi\nexit 0\n"
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	backend := NewCLIBackend(command, "compose")
	if err := backend.UpProjectFilesSelected(context.Background(), "demo", root, map[string]string{"HTTP_PORT": "8082"}, nil, compose); err != nil {
		t.Fatal(err)
	}
}
