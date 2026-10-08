package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestClientConsentFailsClosedWithoutTerminal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	pipe, writer, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	defer pipe.Close()
	if _, err := writer.Write([]byte("yes\n")); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	err := requireManagedClientConsent(context.Background(), pipe, &out, "dev", "webshop", "dev", "postgres", "default", "managed-runtime")
	if err == nil {
		t.Fatal("unattended approval must be rejected")
	}
	if !strings.Contains(out.String(), "managed-runtime") {
		t.Fatalf("missing transparent method in prompt: %s", out.String())
	}
}

func TestClientConsentRejectsIncompleteScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := requireManagedClientConsent(context.Background(), nil, &bytes.Buffer{}, "", "webshop", "dev", "postgres", "default", "managed-runtime")
	if err == nil {
		t.Fatal("empty target cannot be approved")
	}
}

func TestClientConsentRememberedOnlyForExactScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	ctx := context.Background()
	if err := requireManagedClientConsent(ctx, strings.NewReader("yes\n"), &out, "local", "webshop", "dev", "postgres", "default", "managed-runtime"); err != nil {
		t.Fatal(err)
	}
	pipe, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	writer.Close()
	if err := requireManagedClientConsent(ctx, pipe, &bytes.Buffer{}, "local", "webshop", "dev", "postgres", "default", "managed-runtime"); err != nil {
		t.Fatalf("scoped approval not reused: %v", err)
	}
	if err := requireManagedClientConsent(ctx, pipe, &bytes.Buffer{}, "local", "webshop", "dev", "postgres", "reporting", "managed-runtime"); err == nil {
		t.Fatal("different instance must need fresh approval")
	}
}
