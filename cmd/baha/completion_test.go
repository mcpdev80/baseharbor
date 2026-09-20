package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
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
	filtered, opts, err := extractGlobalOutputOptions([]string{"--quiet", "--no-color", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Quiet || !opts.NoColor || opts.Verbose {
		t.Fatalf("unexpected options: %+v", opts)
	}
	if strings.Join(filtered, " ") != "status" {
		t.Fatalf("unexpected forwarded args: %v", filtered)
	}

	if _, _, err := extractGlobalOutputOptions([]string{"--quiet", "--verbose", "status"}); err == nil {
		t.Fatal("quiet + verbose must fail")
	}
}
