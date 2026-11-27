package cli

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// TestCLIHelpSurfaceWalksTheRealCommandTree locks the shared golden helper
// itself: the visible tree, hidden nodes, duplicate registrations, both help
// routes and stable ordering must behave as the cmd/baha golden guard expects.
func TestCLIHelpSurfaceWalksTheRealCommandTree(t *testing.T) {
	hidden := &Command{Name: "secret", Hidden: true, Summary: "hidden surface"}
	duplicate := &Command{Name: "status", Summary: "shadowed by the first registration"}
	app := &Command{Name: "app", Summary: "Manage apps", Children: []*Command{
		{Name: "status", Summary: "First registration wins"},
		duplicate,
		hidden,
	}}
	root := &Command{Name: "baha", Usage: "baha <command>", Children: []*Command{app, {Name: "status", Summary: "Show status"}}}
	surface := root.HelpSurface()
	var got []string
	for _, entry := range surface {
		got = append(got, entry.Path)
	}
	want := []string{"baha", "baha app", "baha app status", "baha status"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected help surface paths: %v", got)
	}
	for _, entry := range surface {
		if entry.Text == "" {
			t.Fatalf("empty rendering for %s", entry.Path)
		}
	}
	if !strings.Contains(surface[2].Text, "First registration wins") {
		t.Fatalf("duplicate registration must render the first-match command: %s", surface[2].Text)
	}
	for _, entry := range surface {
		if strings.Contains(entry.Text, hidden.Summary) || strings.Contains(entry.Text, duplicate.Summary) {
			t.Fatalf("hidden or shadowed source must not appear in the surface: %s", entry.Text)
		}
	}
	// Both documented help routes must print the exact rendered text:
	// `<path> --help` and `help <path>`.
	for _, entry := range surface {
		args := strings.Fields(entry.Path)[1:]
		for _, route := range [][]string{append(append([]string{}, args...), "--help"), append([]string{"help"}, args...)} {
			var out bytes.Buffer
			if err := root.Execute(t.Context(), route, &out, &out); err != nil {
				t.Fatalf("help route %v failed: %v", route, err)
			}
			if out.String() != entry.Text {
				t.Fatalf("help route %v differs from the rendered surface:\n--- got ---\n%s\n--- want ---\n%s",
					route, out.String(), entry.Text)
			}
		}
	}
}

func TestCLIHelpSurfaceIsStableUnderRepeatedWalks(t *testing.T) {
	root := &Command{Name: "baha", Children: []*Command{{Name: "app", Children: []*Command{{Name: "status", Summary: "s"}}}}}
	first := root.HelpSurface()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			again := root.HelpSurface()
			if len(again) != len(first) {
				t.Errorf("surface size changed: %d vs %d", len(again), len(first))
			}
			for index := range again {
				if again[index].Path != first[index].Path || again[index].Text != first[index].Text {
					t.Errorf("surface entry %s differs between walks", first[index].Path)
				}
			}
		}()
	}
	wg.Wait()
}
