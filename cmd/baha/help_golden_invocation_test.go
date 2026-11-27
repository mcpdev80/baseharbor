package main

import (
	"strings"
	"testing"
)

// TestCLIHelpGoldenMatchesEveryExecutableHelpInvocation executes every live
// help route of every public command — the `<sub path> --help` flag form and,
// below the root, the routed `help <sub path>` form — and compares the output
// against the committed golden file. The route inventory comes from the shared
// HelpSurface walk itself (entry.Invocations), so this guard never
// re-implements tree filtering. Guards run in the out-of-repository layout
// (see outsideApplicationRepository), matching the golden.
func TestCLIHelpGoldenMatchesEveryExecutableHelpInvocation(t *testing.T) {
	t.Setenv("COLUMNS", "100")
	golden := readHelpGolden(t)
	outsideApplicationRepository(t, func() {
		root := rootCommand()
		for _, entry := range root.HelpSurface() {
			want := goldenRecord(t, golden, entry.Path)
			for _, args := range entry.Invocations {
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
// help inventory against the human command-index generator: every distinct
// command path the documentation publishes must exist in the golden help
// surface with a non-empty rendering. The index is generated from the shipped
// tree, so a help page that drifted silently, or a golden row someone deleted,
// fails here on its exact published path — independent of the byte-comparison
// guard and without any hardcoded path sample.
func TestCLIHelpSurfaceCoversEveryDocumentedCommandPath(t *testing.T) {
	root := rootCommand()
	surface := map[string]string{}
	for _, entry := range root.HelpSurface() {
		if _, ok := surface[entry.Path]; ok {
			t.Fatalf("duplicate help path %s", entry.Path)
		}
		surface[entry.Path] = entry.Text
	}
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
}
