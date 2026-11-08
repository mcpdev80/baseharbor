package main

import (
	"strings"
	"testing"
)

func TestQuickInitFailureExplainsObservedRequirementsAndDryRun(t *testing.T) {
	_, err := manifestFromDetectedProject(appProjectDetection{Name: "sample", SecretCandidates: []string{"SECRET_NAME"}}, true)
	if err == nil {
		t.Fatal("ambiguous quick init unexpectedly accepted")
	}
	for _, expected := range []string{"SQL=false", "workload-services=0", "secret-candidates=1"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("quick init diagnostic missing %q: %v", expected, err)
		}
	}
}

func TestListHelpExplainsFailClosedTargetSelection(t *testing.T) {
	for _, child := range rootCommand().Children {
		if child.Name != "list" {
			continue
		}
		for _, expected := range []string{"selected Target", "fails closed", "baha list --all-targets", "TARGET column", "baha target list", "baha target activate NAME"} {
			if !strings.Contains(child.Long, expected) {
				t.Fatalf("list help missing %q: %s", expected, child.Long)
			}
		}
		return
	}
	t.Fatal("top-level list command not registered")
}
