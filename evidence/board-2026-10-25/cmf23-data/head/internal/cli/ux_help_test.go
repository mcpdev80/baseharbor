package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCommandSuggestsMistypedSubcommand(t *testing.T) {
	root := &Command{
		Name: "baha",
		Children: []*Command{
			{Name: "status", Summary: "show status", Run: func(context.Context, []string, io.Writer, io.Writer) error { return nil }},
		},
	}
	var out bytes.Buffer
	err := root.Execute(context.Background(), []string{"statsu"}, &out, &out)
	if err == nil {
		t.Fatal("expected unknown command error")
	}
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected usage error, got %T", err)
	}
	if !strings.Contains(usage.Hint, "status") {
		t.Fatalf("missing command suggestion: %q", usage.Hint)
	}
}

func TestCommandSuggestsMistypedFlag(t *testing.T) {
	command := &Command{
		Name:  "up",
		Usage: "baha up [-e ENV|--environment ENV]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return &UsageError{Message: "unknown argument " + args[0], Hint: "Run 'baha up --help' for usage."}
		},
	}
	var out bytes.Buffer
	err := command.Execute(context.Background(), []string{"--enviroment"}, &out, &out)
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected usage error, got %T", err)
	}
	if !strings.Contains(usage.Hint, "--environment") {
		t.Fatalf("missing flag suggestion: %q", usage.Hint)
	}
}

func TestHelpWrapsDescriptionsToColumns(t *testing.T) {
	t.Setenv("COLUMNS", "50")
	command := &Command{
		Name:    "baha",
		Summary: "BaseHarbor CLI",
		Long:    "This is deliberately long help text that should wrap instead of overflowing a narrow terminal window.",
		Usage:   "baha COMMAND",
	}
	var out bytes.Buffer
	command.Help(&out)
	for _, line := range strings.Split(out.String(), "\n") {
		if len([]rune(line)) > 60 {
			t.Fatalf("help line did not wrap: %q", line)
		}
	}
}
