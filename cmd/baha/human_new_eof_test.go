package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuidedNewTargetEOFDoesNotMutateConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "partial-name", input: "eof-target"},
		{name: "after-name", input: "eof-target\n"},
		{name: "partial-runtime", input: "eof-target\npod"},
		{name: "after-runtime", input: "eof-target\npodman\n"},
		{name: "after-scope", input: "eof-target\n\n\n"},
		{name: "before-consent", input: "eof-target\n\n\n\n"},
		{name: "partial-yes-at-consent", input: "eof-target\n\n\n\ny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := filepath.Join(t.TempDir(), "new-config")
			t.Setenv("XDG_CONFIG_HOME", config)
			t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "new-state"))
			var out bytes.Buffer
			err := guidedNewLocalTargetWithInput(context.Background(), strings.NewReader(tc.input), &out, &out)
			if err != nil {
				t.Fatalf("EOF must cancel without error: %v; output: %s", err, out.String())
			}
			if !strings.Contains(out.String(), "Cancelled. No changes were made.") {
				t.Fatalf("missing cancellation feedback: %s", out.String())
			}
			if _, err := os.Stat(config); !os.IsNotExist(err) {
				t.Fatalf("EOF unexpectedly touched target configuration: %v", err)
			}
		})
	}
}

func TestGuidedNewProviderEOFDoesNotCreateFiles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "partial-id", input: "company/provider"},
		{name: "after-id", input: "company/provider\n"},
		{name: "partial-yes-at-consent", input: "company/provider\ny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			var out bytes.Buffer
			err := guidedCreateProviderWithInput(context.Background(), strings.NewReader(tc.input), &out, &out)
			if err != nil {
				t.Fatalf("EOF must cancel without error: %v; output: %s", err, out.String())
			}
			if !strings.Contains(out.String(), "Cancelled. No files created.") {
				t.Fatalf("missing cancellation feedback: %s", out.String())
			}
			entries, err := os.ReadDir(".")
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("EOF unexpectedly wrote provider files: %v", entries)
			}
		})
	}
}
