package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Card 0428-306 (hunt H-V36): the config/flag surface itself stayed consistent;
// this guard catches future --help drift. The exact rendered help of every
// public command must equal the committed golden file. Change --help output
// deliberately, regenerate the golden file with the command below, and review
// the golden diff like any other product surface change.
//
// BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestCLIHelpGoldenSurface
//
// The golden renders the tree outside an application repository: the root help
// section groups depend on inApplicationRepository, so tests run in an empty
// temporary directory for a deterministic layout. The sibling file
// help_golden_invocation_test.go executes both live help routes of every
// public command against this same golden.

const helpGoldenPath = "testdata/cli-help-golden.txt"

func helpGoldenPathOnDisk() string {
	return filepath.Join("..", "..", "cmd", "baha", helpGoldenPath)
}

func readHelpGolden(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(helpGoldenPathOnDisk())
	if err != nil {
		t.Fatalf("golden help file missing: %v", err)
	}
	return string(data)
}

func renderHelpGolden(t *testing.T) string {
	t.Helper()
	t.Setenv("COLUMNS", "100")
	surface := rootCommand().HelpSurface()
	if len(surface) < 40 {
		t.Fatalf("help surface unexpectedly small: %d commands", len(surface))
	}
	var golden strings.Builder
	fmt.Fprintf(&golden, "# Golden --help surface for card 0428-306 (hunt H-V36).\n")
	fmt.Fprintf(&golden, "# Rendered by (*cli.Command).HelpSurface at COLUMNS=100 outside an application repository.\n")
	fmt.Fprintf(&golden, "# Regenerate with:\n")
	fmt.Fprintf(&golden, "# BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestCLIHelpGoldenSurface\n")
	fmt.Fprintf(&golden, "# Help: %d commands\n", len(surface))
	for _, entry := range surface {
		digest := sha256.Sum256([]byte(entry.Text))
		fmt.Fprintf(&golden, "\n=== %s ===\nsha256 %s\n%s", entry.Path, hex.EncodeToString(digest[:]), entry.Text)
	}
	return golden.String()
}

// goldenRecord extracts the pinned text of one command path from the golden
// file. Missing records fail closed.
func goldenRecord(t *testing.T, golden, path string) string {
	t.Helper()
	marker := "\n=== " + path + " ===\n"
	start := strings.Index(golden, marker)
	if start < 0 {
		t.Fatalf("golden file lacks a record for %s", path)
	}
	rest := golden[start+len(marker):]
	newline := strings.Index(rest, "\n") // drop the sha256 line
	if newline < 0 {
		t.Fatalf("golden record for %s is malformed", path)
	}
	body := rest[newline+1:]
	if next := strings.Index(body, "\n=== "); next >= 0 {
		body = body[:next]
	}
	return body
}

// TestCLIHelpGoldenSurface compares the exact --help rendering of every public
// command against the committed golden file (regeneration route above). It is
// the card's primary golden-help guard.
func TestCLIHelpGoldenSurface(t *testing.T) {
	t.Chdir(t.TempDir())
	path := helpGoldenPathOnDisk()
	if os.Getenv("BASEHARBOR_UPDATE_MACHINE_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(renderHelpGolden(t)), 0644); err != nil {
			t.Fatal(err)
		}
	}
	golden := renderHelpGolden(t)
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden help file missing: %v", err)
	}
	if string(actual) != golden {
		t.Fatalf("golden help surface drift; regenerate with BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestCLIHelpGoldenSurface")
	}
}

// TestCLIHelpGoldenFileInventoryPinsEveryCommand is the cheap fail-closed
// witness: the committed golden file must carry exactly one record per current
// help entry point, so a silently deleted or duplicated golden row fails even
// before the full byte comparison runs.
func TestCLIHelpGoldenFileInventoryPinsEveryCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	golden := readHelpGolden(t)
	surface := rootCommand().HelpSurface()
	for _, entry := range surface {
		if !strings.Contains(golden, "\n=== "+entry.Path+" ===\n") {
			t.Fatalf("golden file lacks a record for %s", entry.Path)
		}
	}
	if got, want := strings.Count(golden, "\n=== "), len(surface); got != want {
		t.Fatalf("golden record count %d does not match the current help surface %d", got, want)
	}
}

func TestCLIHelpSurfaceCoversEveryDocumentedCommandPath(t *testing.T) {
	t.Chdir(t.TempDir())
	root := rootCommand()
	surface := map[string]string{}
	for _, entry := range root.HelpSurface() {
		if _, ok := surface[entry.Path]; ok {
			t.Fatalf("duplicate help path %s", entry.Path)
		}
		surface[entry.Path] = entry.Text
	}
	// Every command path the human documentation walks must also appear in the
	// golden help surface with a non-empty rendering.
	for _, row := range humanCommandDocumentationRows(root) {
		text, ok := surface[row.path]
		if !ok {
			t.Fatalf("documented command %s is absent from the golden help surface", row.path)
		}
		if text == "" {
			t.Fatalf("golden help text is empty for %s", row.path)
		}
	}
	// The root and a stable sample of nested paths must be present.
	for _, path := range []string{"baha", "baha app", "baha app init", "baha target", "baha node"} {
		if _, ok := surface[path]; !ok {
			t.Fatalf("help surface is missing %s", path)
		}
	}
}
