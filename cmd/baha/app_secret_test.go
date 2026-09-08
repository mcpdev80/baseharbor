package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseSecretSetArgsRequiresStdin(t *testing.T) {
	name, key, err := parseSecretSetArgs([]string{"demo", "API_TOKEN", "--stdin"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || key != "API_TOKEN" {
		t.Fatalf("unexpected parsed values %q %q", name, key)
	}
	if _, _, err := parseSecretSetArgs([]string{"demo", "API_TOKEN"}); err == nil {
		t.Fatal("expected missing --stdin to fail")
	}
	if _, _, err := parseSecretSetArgs([]string{"demo", "API_TOKEN", "secret-on-command-line"}); err == nil {
		t.Fatal("expected an extra positional secret value to fail")
	}
}

func TestParseSecretDeleteArgsDefaultsToPreview(t *testing.T) {
	name, key, yes, err := parseSecretDeleteArgs([]string{"demo", "API_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || key != "API_TOKEN" || yes {
		t.Fatalf("unexpected preview parse result: %q %q %v", name, key, yes)
	}
	_, _, yes, err = parseSecretDeleteArgs([]string{"demo", "API_TOKEN", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if !yes {
		t.Fatal("expected --yes to enable confirmed deletion")
	}
}

func TestReadSecretValuePreservesInputExactly(t *testing.T) {
	input := []byte("line one\nline two\n")
	got, err := readSecretValue(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("secret input changed: got %q want %q", got, input)
	}
}

func TestReadSecretValueRejectsEmptyAndOversizedInput(t *testing.T) {
	if _, err := readSecretValue(strings.NewReader("")); err == nil {
		t.Fatal("expected empty input to fail")
	}
	if _, err := readSecretValue(strings.NewReader(strings.Repeat("x", (1<<20)+1))); err == nil {
		t.Fatal("expected oversized input to fail")
	}
}
