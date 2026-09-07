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
