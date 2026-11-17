package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAppNewNameNormalizationIsVisibleAndRejectsNavigationWords(t *testing.T) {
	var output bytes.Buffer
	name, err := normalizeAppNewWizardName("My App!!", &output)
	if err != nil {
		t.Fatal(err)
	}
	if name != "my-app" {
		t.Fatalf("normalized name = %q, want my-app", name)
	}
	if !strings.Contains(output.String(), "Application name normalized:") ||
		!strings.Contains(output.String(), "\"My App!!\"") ||
		!strings.Contains(output.String(), "\"my-app\"") {
		t.Fatalf("normalization was silent: %q", output.String())
	}
	for _, reserved := range []string{"cancel", "CANCEL", "back", "exit", "quit"} {
		output.Reset()
		if _, err := normalizeAppNewWizardName(reserved, &output); err == nil {
			t.Fatalf("reserved wizard name %q accepted", reserved)
		}
		if output.Len() != 0 {
			t.Fatalf("rejected name %q emitted success output: %s", reserved, output.String())
		}
	}
}

func TestAppNewCreateOnlyShowsExplicitInitAndUpSteps(t *testing.T) {
	var output bytes.Buffer
	printAppNewNextSteps(&output, "/tmp/example-project")
	s := output.String()
	for _, expected := range []string{
		"Next: cd /tmp/example-project",
		"Next: baha app init --quick",
		"Next: baha up",
	} {
		if !strings.Contains(s, expected) {
			t.Fatalf("missing create-only step %q in %q", expected, s)
		}
	}
	if strings.Index(s, "baha app init --quick") > strings.Index(s, "Next: baha up") {
		t.Fatalf("up suggested before init: %q", s)
	}
}
