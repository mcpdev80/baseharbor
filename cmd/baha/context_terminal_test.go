package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

// context_terminal_test.go — tests for card 0428-068 (per-terminal context).

// isUsageError reports whether err is a cli.UsageError (exit code 2).
func isUsageError(err error) bool {
	var usage *cli.UsageError
	return errors.As(err, &usage)
}

// --- (a) snippet emission ---------------------------------------------------

func TestContextUseEmitsExportSnippet(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := runContextUse(context.Background(), []string{"--shell", "bash", "docker-dev"}, &out, &errOut); err != nil {
		t.Fatalf("runContextUse: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "export BASEHARBOR_TARGET=\"docker-dev\"") {
		t.Fatalf("expected bash export snippet, got:\n%s", got)
	}
}

func TestContextUseFishSnippet(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := runContextUse(context.Background(), []string{"--shell", "fish", "prod"}, &out, &errOut); err != nil {
		t.Fatalf("runContextUse fish: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `set -gx BASEHARBOR_TARGET "prod"`) {
		t.Fatalf("expected fish set snippet, got:\n%s", got)
	}
}

func TestContextUseRejectsUnknownShell(t *testing.T) {
	var out, errOut bytes.Buffer
	err := runContextUse(context.Background(), []string{"--shell", "zsh3", "docker-dev"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
	if !isUsageError(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("expected 'unsupported shell' message, got %v", err)
	}
}

func TestContextUseRequiresTarget(t *testing.T) {
	var out, errOut bytes.Buffer
	err := runContextUse(context.Background(), []string{"--shell", "bash"}, &out, &errOut)
	if !isUsageError(err) {
		t.Fatalf("expected usage error for missing target, got %v", err)
	}
}

// --- precedence stages (c) ---------------------------------------------------

func TestContextPrecedenceExplicitFlagWins(t *testing.T) {
	configureTestTarget(t)
	ctx := withTargetOverride(context.Background(), "docker-dev")
	origin, target, err := effectiveContextOrigin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if target != "docker-dev" || origin != "explicit-flag" {
		t.Fatalf("explicit-flag should win, got target=%q origin=%q", target, origin)
	}
}

func TestContextPrecedenceShellEnvWinsOverDefault(t *testing.T) {
	configureTestTarget(t)
	t.Setenv(contextShellVar, "docker-dev")
	defer os.Unsetenv(contextShellVar)
	origin, target, err := effectiveContextOrigin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if target != "docker-dev" || origin != "shell-env" {
		t.Fatalf("shell-env should win over configured-default, got target=%q origin=%q", target, origin)
	}
}

func TestContextPrecedenceDefaultFallback(t *testing.T) {
	configureTestTarget(t)
	os.Unsetenv(contextShellVar)
	origin, target, err := effectiveContextOrigin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if target != "docker-dev" || origin != "configured-default" {
		t.Fatalf("configured-default should win, got target=%q origin=%q", target, origin)
	}
}

func TestContextShowReportsWinningStage(t *testing.T) {
	configureTestTarget(t)
	os.Unsetenv(contextShellVar)
	var out, errOut bytes.Buffer
	if err := runContextShow(context.Background(), nil, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "configured-default") {
		t.Fatalf("expected configured-default in output, got:\n%s", out.String())
	}
}

// --- clear -------------------------------------------------------------------

func TestContextClearUnsetsInProcess(t *testing.T) {
	t.Setenv(contextShellVar, "docker-dev")
	var out, errOut bytes.Buffer
	if err := runContextClear(context.Background(), []string{"--shell", "bash"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(contextShellVar) != "" {
		t.Fatal("context clear must unset BASEHARBOR_TARGET in process")
	}
	if !strings.Contains(out.String(), "Cleared per-terminal context") {
		t.Fatalf("expected clear hint, got:\n%s", out.String())
	}
}

// --- hook emission (b) -------------------------------------------------------

func TestCompletionHookEmitted(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var out bytes.Buffer
		command := completionCommand(rootCommand())
		if err := command.Run(context.Background(), []string{shell}, &out, &out); err != nil {
			t.Fatalf("%s completion: %v", shell, err)
		}
		if !strings.Contains(out.String(), "_baha_context_hook") {
			t.Fatalf("%s completion missing per-terminal hook:\n%s", shell, out.String())
		}
	}
}

func TestActivationSnippetRender(t *testing.T) {
	bash := activationSnippet("bash", "docker-dev", contextShellVar)
	if !strings.HasPrefix(bash, "export BASEHARBOR_TARGET=\"docker-dev\"") {
		t.Fatalf("bash snippet mismatch: %q", bash)
	}
	fish := activationSnippet("fish", "docker-dev", contextShellVar)
	if !strings.HasPrefix(fish, "set -gx BASEHARBOR_TARGET") {
		t.Fatalf("fish snippet mismatch: %q", fish)
	}
}

// --- shell-absent fallback (e) ----------------------------------------------

func TestShellAbsentFallbackHints(t *testing.T) {
	// Non-interactive / non-TTY: context use must not panic or fail closed; it
	// emits the snippet (the fallback is the snippet itself, no marker written).
	os.Unsetenv(contextShellVar)
	var out, errOut bytes.Buffer
	if err := runContextUse(context.Background(), []string{"docker-dev"}, &out, &errOut); err != nil {
		t.Fatalf("context use in non-interactive mode: %v", err)
	}
	if !strings.Contains(out.String(), "export BASEHARBOR_TARGET=\"docker-dev\"") {
		t.Fatalf("expected snippet fallback, got:\n%s", out.String())
	}
}

// --- context group registration ---------------------------------------------

func TestContextGroupRegistered(t *testing.T) {
	root := rootCommand()
	var found bool
	for _, child := range root.Children {
		if child.Name == "context" {
			found = true
			if len(child.Children) != 3 {
				t.Fatalf("context group must expose use/clear/show, got %d children", len(child.Children))
			}
		}
	}
	if !found {
		t.Fatal("context command group not registered in rootCommand()")
	}
}
