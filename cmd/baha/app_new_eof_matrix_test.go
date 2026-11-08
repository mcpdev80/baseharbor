package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No name, location, or stack choice may be interpreted as consent when the
// interactive input stream ends (for example Ctrl-D in canonical terminal mode).
func TestAppNewWizardEOFCancelsEarlyPromptsWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input func(string) string
	}{
		{name: "application-name", input: func(string) string { return "" }},
		{name: "project-location", input: func(string) string { return "cancel-project\n" }},
		{name: "stack-selection", input: func(parent string) string { return "cancel-project\n" + parent + "\n" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			t.Setenv("HOME", t.TempDir())
			configureTestTarget(t)
			parent := t.TempDir()
			previous := appNewInput
			t.Cleanup(func() { appNewInput = previous })
			appNewInput = strings.NewReader(tc.input(parent))
			var output bytes.Buffer
			if err := runAppNewWizard(context.Background(), &output, &output); err != nil {
				t.Fatalf("EOF at %s should cancel cleanly: %v", tc.name, err)
			}
			if !strings.Contains(output.String(), "No changes were made.") {
				t.Fatalf("EOF at %s missing cancel message: %s", tc.name, output.String())
			}
			if _, err := os.Stat(filepath.Join(parent, "cancel-project")); !os.IsNotExist(err) {
				t.Fatalf("EOF at %s created a project: %v", tc.name, err)
			}
		})
	}
}
