package runtime

import (
	"os"
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
