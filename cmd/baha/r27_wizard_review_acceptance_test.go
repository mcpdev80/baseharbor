package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The wizard must not create a project on EOF at any of the early prompts,
// including a torn/unterminated affirmative. The full scripted successful
// input is intentionally not sent: every case stops before final consent.
func TestR27WizardEarlyPromptEOFMatrixNoMutation(t *testing.T) {
	for _, tc := range []struct {
		name, input string
	}{
		{"name-EOF", ""},
		{"partial-name-EOF", "torn-app"},
		{"after-name", "torn-app\n"},
		{"partial-directory-EOF", "torn-app\n/tmp/incomplete"},
		{"after-directory", "torn-app\n%s\n"},
		{"after-stack", "torn-app\n%s\n1\n"},
		{"after-mode", "torn-app\n%s\n1\ncreate-only\n"},
		{"after-needs", "torn-app\n%s\n1\ncreate-only\nenter\n"},
		{"partial-consent", "torn-app\n%s\n1\ncreate-only\nenter\ny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			configureTestTarget(t)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			t.Setenv("HOME", t.TempDir())
			parent := t.TempDir()
			old := appNewInput
			defer func() { appNewInput = old }()
			appNewInput = strings.NewReader(strings.ReplaceAll(tc.input, "%s", parent))
			var output bytes.Buffer
			if err := runAppNewWizard(context.Background(), &output, &output); err != nil {
				t.Fatalf("EOF must cancel without usage error: %v\n%s", err, output.String())
			}
			if _, err := os.Stat(filepath.Join(parent, "torn-app")); !os.IsNotExist(err) {
				t.Fatalf("wizard EOF created a project: %v\n%s", err, output.String())
			}
			if !strings.Contains(output.String(), "No changes were made.") {
				t.Fatalf("EOF did not report clean cancellation:\n%s", output.String())
			}
		})
	}
}

// Both explicit CLI entry points must present the same create-only contract.
// It is intentionally a two-stage workflow, NOT proof that init/apply ran.
func TestR27CreateOnlyNextStepsAcrossAppAndTopLevelAlias(t *testing.T) {
	for _, tc := range []struct {
		name string
		run func(context.Context, []string, *bytes.Buffer) error
	}{
		{"app-new", func(ctx context.Context, args []string, out *bytes.Buffer) error {
			return appNewCommand().Run(ctx, args, out, out)
		}},
		{"top-level-new-application", func(ctx context.Context, args []string, out *bytes.Buffer) error {
			return runHumanNewSelection(ctx, "application", args, out, out)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			t.Setenv("HOME", t.TempDir())
			parent := t.TempDir()
			const name = "r27-review-create"
			args := []string{name, "--directory", parent, "--stack", "go"}
			var output bytes.Buffer
			if err := tc.run(context.Background(), args, &output); err != nil {
				t.Fatalf("create-only entry point failed: %v\n%s", err, output.String())
			}
			root := filepath.Join(parent, name)
			if _, err := os.Stat(filepath.Join(root, "baseharbor.yaml")); err != nil {
				t.Fatalf("create-only did not create portable contract: %v", err)
			}
			for _, expected := range []string{
				"Next: cd ", "Next: baha app init --quick", "Next: baha up",
			} {
				if !strings.Contains(output.String(), expected) {
					t.Fatalf("missing %q next action:\n%s", expected, output.String())
				}
			}
			if strings.Contains(output.String(), "Deployment READY") {
				t.Fatalf("create-only falsely claimed deployed resources:\n%s", output.String())
			}
		})
	}
}

// The presentation-only top-level chooser must not turn a broken/empty
// response into an application creation or an affirmative.
func TestR27TopLevelNewChoiceEOFCancels(t *testing.T) {
	for _, input := range []string{"", "1"} {
		t.Run("input-"+input, func(t *testing.T) {
			old := appNewInput
			defer func() { appNewInput = old }()
			appNewInput = strings.NewReader(input)
			var output bytes.Buffer
			if err := humanNewCommand().Run(context.Background(), nil, &output, &output); err != nil {
				t.Fatalf("top-level choice EOF should cancel: %v", err)
			}
			if !strings.Contains(output.String(), "No changes were made.") {
				t.Fatalf("missing cancellation message:\n%s", output.String())
			}
		})
	}
}
