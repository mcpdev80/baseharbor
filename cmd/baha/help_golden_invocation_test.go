package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

// goldenHelpEntry is one golden command path with the live invocations that
// must print its golden rendering: `<sub path> --help` and, below the root,
// the routed `help <sub path>` form.
type goldenHelpEntry struct {
	path        string
	invocations [][]string
}

// goldenSurfaceOrder enumerates help entry points with the same first-match
// routing the CLI uses. The router reaches a child by name or alias only, so
// `--help` as a trailing argument exists for commands without children; group
// nodes are covered through their explicit `help <path>` route instead.
func goldenSurfaceOrder(root *cli.Command) []goldenHelpEntry {
	surface := root.HelpSurface()
	entries := make([]goldenHelpEntry, 0, len(surface))
	for _, entry := range surface {
		parts := strings.Fields(entry.Path)
		sub := parts[1:]
		invocations := [][]string{}
		if len(sub) > 0 {
			command := root
			for _, part := range sub {
				command = command.CommandChild(part)
			}
			if command != nil && len(command.Children) == 0 {
				invocations = append(invocations, append(append([]string{}, sub...), "--help"))
			}
		}
		invocations = append(invocations, append([]string{"help"}, sub...))
		entries = append(entries, goldenHelpEntry{path: entry.Path, invocations: invocations})
	}
	return entries
}

// TestCLIHelpGoldenMatchesEveryExecutableHelpInvocation executes the live help
// routes of every public command and compares them against the committed
// golden file, so the guard covers real execution and not only the in-process
// renderer.
func TestCLIHelpGoldenMatchesEveryExecutableHelpInvocation(t *testing.T) {
	t.Chdir(t.TempDir())
	golden := readHelpGolden(t)
	for _, entry := range goldenSurfaceOrder(rootCommand()) {
		want := goldenRecord(t, golden, entry.path)
		for _, args := range entry.invocations {
			t.Setenv("COLUMNS", "100")
			var out strings.Builder
			if err := runWithIOAsTestOperator(t.Context(), args, &out, &out); err != nil {
				t.Fatalf("help %v failed: %v", args, err)
			}
			if out.String() != want {
				t.Fatalf("executed `baha %s` differs from the golden record for %s:\n--- got ---\n%s\n--- want ---\n%s",
					strings.Join(args, " "), entry.path, out.String(), want)
			}
		}
	}
}
