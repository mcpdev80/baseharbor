package main

import (
	"strings"
	"testing"
)

// TestCLIHelpGoldenMatchesEveryExecutableHelpInvocation executes both live
// help routes of every public command — `<sub path> --help` and
// `help <sub path>`, the two routes Command.Execute implements — and compares
// the output against the committed golden file. The golden rendering comes
// from the same shared HelpSurface walk, so no route inventory is
// re-implemented here. Guards run in the out-of-repository layout (see
// outsideApplicationRepository), matching the golden.
func TestCLIHelpGoldenMatchesEveryExecutableHelpInvocation(t *testing.T) {
	t.Setenv("COLUMNS", "100")
	golden := readHelpGolden(t)
	outsideApplicationRepository(t, func() {
		root := rootCommand()
		for _, entry := range root.HelpSurface() {
			want := goldenRecord(t, golden, entry.Path)
			sub := strings.Fields(entry.Path)[1:]
			routes := [][]string{append(append([]string{}, sub...), "--help")}
			if len(sub) > 0 {
				routes = append(routes, append([]string{"help"}, sub...))
			}
			for _, args := range routes {
				var out strings.Builder
				if err := runWithIOAsTestOperator(t.Context(), args, &out, &out); err != nil {
					t.Fatalf("help %v failed: %v", args, err)
				}
				if out.String() != want {
					t.Fatalf("executed `baha %s` differs from the golden record for %s:\n--- got ---\n%s\n--- want ---\n%s",
						strings.Join(args, " "), entry.Path, out.String(), want)
				}
			}
		}
	})
}

// TestCLIHelpSurfaceCoversEveryDocumentedCommandPath cross-checks the golden
// help inventory against the existing human command-index generator: every
// distinct command path the documentation publishes must exist in the golden
// help surface with a non-empty rendering. Alias rows resolve to their
// canonical command through the shared documentation inventory itself.
func TestCLIHelpSurfaceCoversEveryDocumentedCommandPath(t *testing.T) {
	root := rootCommand()
	surface := map[string]string{}
	outsideApplicationRepository(t, func() {
		for _, entry := range rootCommand().HelpSurface() {
			if _, ok := surface[entry.Path]; ok {
				t.Fatalf("duplicate help path %s", entry.Path)
			}
			surface[entry.Path] = entry.Text
		}
	})
	for _, row := range humanCommandDocumentationRows(root) {
		path := row.path
		if strings.HasPrefix(row.summary, "Alias: ") {
			path = strings.TrimPrefix(row.summary, "Alias: ")
		}
		text, ok := surface[path]
		if !ok {
			t.Fatalf("documented command %s is absent from the golden help surface", path)
		}
		if text == "" {
			t.Fatalf("golden help text is empty for %s", path)
		}
	}
	for _, path := range []string{"baha", "baha app", "baha app init", "baha target", "baha node"} {
		if _, ok := surface[path]; !ok {
			t.Fatalf("help surface is missing %s", path)
		}
	}
}
