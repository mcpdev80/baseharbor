package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNestedHelp(t *testing.T) {
	root := &Command{Name: "baha", Usage: "baha <command>", Children: []*Command{{Name: "app", Summary: "Manage apps", Usage: "baha app <command>", Children: []*Command{{Name: "create", Summary: "Create app", Usage: "baha app create NAME"}}}}}
	var out bytes.Buffer
	if err := root.Execute(context.Background(), []string{"app", "create", "--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "baha app create NAME") || !strings.Contains(got, "Create app") {
		t.Fatalf("unexpected help output: %s", got)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	root := &Command{Name: "baha", Children: []*Command{{Name: "app"}}}
	err := root.Execute(context.Background(), []string{"nope"}, &bytes.Buffer{}, &bytes.Buffer{})
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", ExitCode(err))
	}
}

func TestRuntimeErrorExitCode(t *testing.T) {
	if got := ExitCode(errors.New("boom")); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
}

func TestUnknownCommandSuggestsNearestMatch(t *testing.T) {
	root := &Command{Name: "baha", Children: []*Command{{Name: "status"}, {Name: "doctor"}}}
	err := root.Execute(context.Background(), []string{"statsu"}, &bytes.Buffer{}, &bytes.Buffer{})
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if !strings.Contains(usage.Hint, "status") {
		t.Fatalf("expected status suggestion, got %q", usage.Hint)
	}
}

func TestHelpWrapsLongDescriptionsAtConfiguredWidth(t *testing.T) {
	t.Setenv("COLUMNS", "50")
	root := &Command{
		Name: "baha",
		Usage: "baha <command>",
		Children: []*Command{{
			Name: "status",
			Summary: "Show a deliberately long application status description that should wrap cleanly on narrow terminals",
		}},
	}
	var out bytes.Buffer
	root.Help(&out)
	if !strings.Contains(out.String(), "\n          ") {
		t.Fatalf("expected wrapped continuation indentation, got:\n%s", out.String())
	}
}
