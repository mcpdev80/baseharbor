package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestCompletionEnvironmentValues(t *testing.T) {
	root := rootCommand()
	candidates := completeCommandLine(root, []string{"up", "-e", ""})
	got := map[string]bool{}
	for _, candidate := range candidates {
		got[candidate.Value] = true
	}
	for _, want := range []string{"dev", "test", "prod"} {
		if !got[want] {
			t.Fatalf("missing environment completion %q: %#v", want, candidates)
		}
	}
}

func TestCompletionHidesInternalCommand(t *testing.T) {
	root := rootCommand()
	candidates := completeCommandLine(root, []string{""})
	for _, candidate := range candidates {
		if candidate.Value == "__complete" {
			t.Fatal("internal completion command leaked into user completion")
		}
	}
}

func TestCompletionCommandGeneratesSupportedShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var out bytes.Buffer
		command := completionCommand(rootCommand())
		if err := command.Run(context.Background(), []string{shell}, &out, &out); err != nil {
			t.Fatalf("%s completion: %v", shell, err)
		}
		if !strings.Contains(out.String(), "baha") {
			t.Fatalf("%s completion output does not reference baha: %q", shell, out.String())
		}
	}
}

func TestGlobalOutputOptions(t *testing.T) {
	filtered, opts, showVersion, _, err := extractGlobalOutputOptions([]string{"--quiet", "--no-color", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Quiet || !opts.NoColor || opts.Verbose || showVersion {
		t.Fatalf("unexpected options: %+v", opts)
	}
	if strings.Join(filtered, " ") != "status" {
		t.Fatalf("unexpected forwarded args: %v", filtered)
	}

	if _, _, _, _, err := extractGlobalOutputOptions([]string{"--quiet", "--verbose", "status"}); err == nil {
		t.Fatal("quiet + verbose must fail")
	}
}

func TestCompletionIncludesConfiguredApplicationsReadOnly(t *testing.T) {
	store := application.Store{Root: t.TempDir()}
	if _, err := store.Create(application.New("mailflow", "dev", true, false, false)); err != nil {
		t.Fatal(err)
	}
	candidates := completeCommandLine(rootCommand(), []string{"app", "status", "mail"}, store)
	found := false
	for _, candidate := range candidates {
		if candidate.Value == "mailflow" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("configured application missing from completion: %#v", candidates)
	}
}

func TestGlobalOutputOptionsIncludePlainNoInputAndVersion(t *testing.T) {
	filtered, opts, showVersion, _, err := extractGlobalOutputOptions([]string{"--plain", "--no-input", "--version"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 || !opts.Plain || !opts.NonInteractive || !showVersion {
		t.Fatalf("filtered=%v opts=%+v showVersion=%v", filtered, opts, showVersion)
	}
}

func TestBashCompletionUsesCurrentCursorWord(t *testing.T) {
	script := bashCompletionScript()
	for _, wanted := range []string{
		"COMP_CWORD",
		"COMP_WORDS[@]:1:COMP_CWORD-1",
		"COMP_WORDS[COMP_CWORD]",
		"command baha __complete",
	} {
		if !strings.Contains(script, wanted) {
			t.Fatalf("bash completion script missing %q:\n%s", wanted, script)
		}
	}
}

func TestCompletionAfterSubcommandSpaceReturnsChildren(t *testing.T) {
	root := rootCommand()
	candidates := completeCommandLine(root, []string{"app", ""}, application.Store{Root: t.TempDir()})
	values := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		values[candidate.Value] = true
	}
	for _, wanted := range []string{"status", "doctor", "apply", "up"} {
		if !values[wanted] {
			t.Fatalf("completion after 'baha app ' missing %q: %#v", wanted, candidates)
		}
	}
}
