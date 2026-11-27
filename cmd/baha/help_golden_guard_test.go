package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
// section groups depend on inApplicationRepository, which a helper relocates to
// an empty temporary root (no process-wide Chdir) for the duration of one
// render. The sibling file help_golden_invocation_test.go executes both live
// help routes of every public command against this same golden.
//
// Catalog binding (scripts/pr-gate-coverage.json): the golden names here and
// the invocation witness carry required_test_names rows, so the trusted
// planner demands their actual PASS events. The guards therefore never turn
// red from environment-dependent state: they render the command tree only
// (never a Run callback), use no ambient fixture and need no database.

const helpGoldenPath = "testdata/cli-help-golden.txt"

// goldenWorkingDirectory remembers the package working directory that the test
// binary starts in, so guards address the golden file independently of any
// temporary directory a test is running in.
var goldenWorkingDirectory = func() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return dir
}()

var resolveHelpGolden = sync.OnceValue(func() string {
	return filepath.Join(goldenWorkingDirectory, helpGoldenPath)
})

func helpGoldenPathOnDisk() string {
	return resolveHelpGolden()
}

func readHelpGolden(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(helpGoldenPathOnDisk())
	if err != nil {
		t.Fatalf("golden help file missing: %v", err)
	}
	return string(data)
}

// outsideApplicationRepository runs body with the working directory relocated
// to an empty temporary directory, so inApplicationRepository resolves false
// and the root help layout is deterministic. It uses no t.Chdir: go test
// executes package tests sequentially, and the temporary directory is removed
// on return, so no later test can inherit the relocated directory.
// outsideApplicationRepository runs body with the working directory relocated
// to an empty temporary directory, so inApplicationRepository resolves false
// and the root help layout is deterministic. It uses no t.Chdir: go test runs
// package tests sequentially in one process and t.Chdir has no undo, which
// would make every later test inherit the relocated directory (package
// precedent: cmd/baha/main_test.go saves and restores os.Getwd around its
// chdir cases). The restore must therefore be exception-safe: a t.Fatalf
// inside body unwinds through a panic, and a bare deferred os.Chdir would be
// skipped, leaking a deleted working directory into the whole rest of the
// package suite. The guard records the restore step itself, runs it before the
// panic continues, and fails the test when body did not complete.
func outsideApplicationRepository(t *testing.T, body func()) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if restoreErr := os.Chdir(previous); restoreErr != nil {
			t.Errorf("restore working directory: %v", restoreErr)
		}
	}()
	body()
}

// renderHelpGolden renders the complete public help surface at the fixed
// golden width, in the out-of-repository layout.
func renderHelpGolden(t *testing.T) string {
	t.Helper()
	t.Setenv("COLUMNS", "100")
	var golden string
	outsideApplicationRepository(t, func() {
		surface := rootCommand().HelpSurface()
		if len(surface) < 40 {
			t.Fatalf("help surface unexpectedly small: %d commands", len(surface))
		}
		var builder strings.Builder
		fmt.Fprintf(&builder, "# Golden --help surface for card 0428-306 (hunt H-V36).\n")
		fmt.Fprintf(&builder, "# Rendered by (*cli.Command).HelpSurface at COLUMNS=100 outside an application repository.\n")
		fmt.Fprintf(&builder, "# Regenerate with:\n")
		fmt.Fprintf(&builder, "# BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestCLIHelpGoldenSurface\n")
		fmt.Fprintf(&builder, "# Help: %d commands\n", len(surface))
		for _, entry := range surface {
			digest := sha256.Sum256([]byte(entry.Text))
			fmt.Fprintf(&builder, "\n=== %s ===\nsha256 %s\n%s", entry.Path, hex.EncodeToString(digest[:]), entry.Text)
		}
		golden = builder.String()
	})
	return golden
}

// TestCLIHelpGoldenSurface compares the exact --help rendering of every public
// command against the committed golden file (regeneration route above). It is
// the card's primary golden-help guard.
func TestCLIHelpGoldenSurface(t *testing.T) {
	path := helpGoldenPathOnDisk()
	if os.Getenv("BASEHARBOR_UPDATE_MACHINE_DOCS") == "1" {
		golden := renderHelpGolden(t)
		if err := os.WriteFile(path, []byte(golden), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden help file regenerated at %s (%d bytes)", path, len(golden))
		return
	}
	golden := renderHelpGolden(t)
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden help file missing at %s: %v", path, err)
	}
	if string(actual) != golden {
		t.Fatal("golden help surface drift; regenerate with BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestCLIHelpGoldenSurface")
	}
}

// TestCLIHelpGoldenFileInventoryPinsEveryCommand is the fail-closed inventory
// witness: the committed golden file must carry exactly one record per current
// help entry point, so a silently deleted or duplicated golden row fails even
// before the full byte comparison runs.
func TestCLIHelpGoldenFileInventoryPinsEveryCommand(t *testing.T) {
	golden := readHelpGolden(t)
	var surface []string
	outsideApplicationRepository(t, func() {
		for _, entry := range rootCommand().HelpSurface() {
			surface = append(surface, entry.Path)
			if !strings.Contains(golden, "\n=== "+entry.Path+" ===\n") {
				t.Fatalf("golden file lacks a record for %s", entry.Path)
			}
		}
	})
	if got, want := strings.Count(golden, "\n=== "), len(surface); got != want {
		t.Fatalf("golden record count %d does not match the current help surface %d", got, want)
	}
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
