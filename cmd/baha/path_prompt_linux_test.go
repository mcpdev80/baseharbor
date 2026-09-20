//go:build linux

package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompleteDirectoryPathUniqueMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "certificates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}

	typed := filepath.Join(root, "cert")
	got, _ := completeDirectoryPath(typed)
	want := filepath.ToSlash(filepath.Join(root, "certificates")) + "/"
	if got != want {
		t.Fatalf("completeDirectoryPath(%q) = %q, want %q", typed, got, want)
	}
}

func TestCompleteDirectoryPathCommonPrefixAndMatches(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"cert-prod", "cert-stage", "other"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	typed := filepath.Join(root, "ce")
	got, matches := completeDirectoryPath(typed)
	want := filepath.ToSlash(filepath.Join(root, "cert-"))
	if got != want {
		t.Fatalf("completeDirectoryPath(%q) = %q, want %q", typed, got, want)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %#v, want 2 entries", matches)
	}
	for _, match := range matches {
		if !strings.HasSuffix(match, "/") {
			t.Fatalf("match %q is not rendered as a directory", match)
		}
	}
}

func TestCompleteDirectoryPathIgnoresFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cert.pem"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	typed := filepath.Join(root, "cert")
	got, matches := completeDirectoryPath(typed)
	if got != typed || len(matches) != 0 {
		t.Fatalf("got %q, %#v; files must not be offered for a directory prompt", got, matches)
	}
}


func TestPromptPathWithCompletionShowsPathBaseForInteractiveNonFileReader(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(original) }()

	input := strings.NewReader("recovery.json\n")
	reader := bufio.NewReader(input)
	var out bytes.Buffer

	got, err := promptPathWithCompletion(reader, &out, "New recovery output file", input)
	if err != nil {
		t.Fatal(err)
	}
	if got != "recovery.json" {
		t.Fatalf("path = %q, want recovery.json", got)
	}
	text := out.String()
	if !strings.Contains(text, "New recovery output file:"+shellDisplayPath(dir)+"$ ") {
		t.Fatalf("shell-like path prompt missing: %q", text)
	}
}


func TestDrawPathPromptKeepsStaticCWDForRelativeInput(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(original) }()

	var out bytes.Buffer
	promptPath := shellDisplayPath(root)
	if err := drawPathPrompt(&out, "OpenBao-recovery-key", promptPath, "../../key", true); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := "OpenBao-recovery-key:" + promptPath + "$ ../../key"
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestDrawPathPromptKeepsStaticCWDForAbsoluteInput(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	promptPath := shellDisplayPath(root)
	if err := drawPathPrompt(&out, "OpenBao-recovery-key", promptPath, "/tmp/key", true); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := "OpenBao-recovery-key:" + promptPath + "$ /tmp/key"
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}


func TestShellDisplayPathUsesHomeShorthand(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := shellDisplayPath(filepath.Join(home, "dev", "project"))
	if got != "~/dev/project" {
		t.Fatalf("shellDisplayPath = %q, want ~/dev/project", got)
	}
}
