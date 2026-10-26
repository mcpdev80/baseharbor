package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestWritePathPromptContextShowsCurrentDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(original) }()

	var out bytes.Buffer
	if err := writePathPromptContext(&out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Path base") {
		t.Fatalf("missing path base heading: %q", got)
	}
	if !strings.Contains(got, dir) {
		t.Fatalf("missing current directory %q: %q", dir, got)
	}
	if !strings.Contains(got, "relative paths are resolved from this directory") {
		t.Fatalf("missing relative-path explanation: %q", got)
	}
}
