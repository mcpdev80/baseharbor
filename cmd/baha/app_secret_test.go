package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

func TestParseSecretSetArgsSupportsInteractiveOrExplicitInput(t *testing.T) {
	name, key, err := parseSecretSetArgs([]string{"demo", "API_TOKEN", "--stdin"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || key != "API_TOKEN" {
		t.Fatalf("unexpected parsed values %q %q", name, key)
	}
	name, key, err = parseSecretSetArgs([]string{"demo", "API_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || key != "API_TOKEN" {
		t.Fatalf("unexpected interactive parsed values %q %q", name, key)
	}
	if _, _, err := parseSecretSetArgs([]string{"demo", "API_TOKEN", "--stdin", "--file", "secret.txt"}); err == nil {
		t.Fatal("expected multiple input sources to fail")
	}
	if _, _, err := parseSecretSetArgs([]string{"demo", "API_TOKEN", "secret-on-command-line"}); err == nil {
		t.Fatal("expected an extra positional secret value to fail")
	}
}

func TestParseSecretSetArgsSupportsFileInput(t *testing.T) {
	name, key, err := parseSecretSetArgs([]string{"demo", "TLS_KEY_FILE", "--file", "/tmp/key.pem"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || key != "TLS_KEY_FILE" {
		t.Fatalf("unexpected parsed values %q %q", name, key)
	}
	if got := secretSetFilePath([]string{"TLS_KEY_FILE", "--file=/tmp/key.pem"}); got != "/tmp/key.pem" {
		t.Fatalf("unexpected file path %q", got)
	}
}

func TestParseSecretSetArgsSupportsRepositoryShorthand(t *testing.T) {
	name, key, err := parseSecretSetArgs([]string{"API_TOKEN", "--stdin"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "" || key != "API_TOKEN" {
		t.Fatalf("unexpected repository shorthand values %q %q", name, key)
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

func TestParseSecretDeleteArgsSupportsRepositoryShorthand(t *testing.T) {
	name, key, yes, err := parseSecretDeleteArgs([]string{"API_TOKEN", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "" || key != "API_TOKEN" || !yes {
		t.Fatalf("unexpected repository shorthand values: %q %q %v", name, key, yes)
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

func TestReadSecretSetValueReadsFileExactly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.pem")
	want := []byte("-----BEGIN TEST-----\nabc\n-----END TEST-----\n")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readSecretSetValue([]string{"TLS_CERT_FILE", "--file", path}, strings.NewReader("ignored"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file secret input changed: got %q want %q", got, want)
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

func TestReadSecretSetValueInteractiveUsesHiddenPromptWithoutPrintingValue(t *testing.T) {
	oldInput := appSecretInput
	oldHidden := appSecretReadHidden
	appSecretInput = os.Stdin
	appSecretReadHidden = func(input io.Reader, out io.Writer, key string) ([]byte, error) {
		if key != "API_TOKEN" {
			t.Fatalf("key = %q", key)
		}
		fmt.Fprint(out, "API_TOKEN value: ")
		return []byte("super-secret"), nil
	}
	t.Cleanup(func() {
		appSecretInput = oldInput
		appSecretReadHidden = oldHidden
	})

	var out bytes.Buffer
	value, err := readSecretSetValueInteractive(context.Background(), []string{"API_TOKEN"}, appSecretInput, &out, "API_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	defer zeroBytes(value)
	if string(value) != "super-secret" {
		t.Fatal("hidden prompt did not return expected value")
	}
	if strings.Contains(out.String(), "super-secret") {
		t.Fatalf("secret value leaked to output: %s", out.String())
	}
}

func TestReadSecretSetValueInteractiveNoInputRequiresExplicitSource(t *testing.T) {
	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{NonInteractive: true})
	_, err := readSecretSetValueInteractive(ctx, []string{"API_TOKEN"}, strings.NewReader("secret"), &bytes.Buffer{}, "API_TOKEN")
	if err == nil || !strings.Contains(err.Error(), "--stdin") {
		t.Fatalf("expected deterministic no-input remediation, got %v", err)
	}
}
