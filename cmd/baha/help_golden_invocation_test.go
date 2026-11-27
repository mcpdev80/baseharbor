package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

func goldenHelpInvocations(command *cli.Command, path []string) [][]string {
	if command == nil || command.Hidden {
		return nil
	}
	current := path
	if command.Name != "" {
		current = append(append([]string{}, path...), command.Name)
	}
	invocations := [][]string{append(append([]string{}, current...), "--help")}
	if len(current) > 1 {
		invocations = append(invocations, append([]string{"help"}, current[1:]...))
	}
	for _, child := range command.Children {
		invocations = append(invocations, goldenHelpInvocations(child, current)...)
	}
	return invocations
}

// TestCLIHelpGoldenMatchesEveryExecutableHelpInvocation executes both help
// routes of every public command through the real entry point and compares
// against the committed golden file, so the guard covers live execution and
// not only the in-process renderer.
func TestCLIHelpGoldenMatchesEveryExecutableHelpInvocation(t *testing.T) {
	t.Chdir(t.TempDir())
	golden := readHelpGolden(t)
	for _, args := range goldenHelpInvocations(rootCommand(), nil) {
		t.Setenv("COLUMNS", "100")
		var out strings.Builder
		if err := runWithIOAsTestOperator(t.Context(), args, &out, &out); err != nil {
			t.Fatalf("help %v failed: %v", args, err)
		}
		path := strings.Join(args[1:], " ")
		if len(args) == 2 && args[1] == "--help" {
			path = strings.Join(args[:1], " ")
		}
		want := goldenRecord(t, golden, path)
		if out.String() != want {
			t.Fatalf("executed `baha %s` differs from the golden record for %s:\n--- got ---\n%s\n--- want ---\n%s",
				strings.Join(args, " "), path, out.String(), want)
		}
	}
}
