package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestRemoteApplyPayloadTransportsOnlyPinnedGitOrOCIModel(t *testing.T) {
	for _, test := range []struct {
		name   string
		source development.SourceDefinition
	}{
		{"git", development.SourceDefinition{
			ID: "app", Type: development.SourceRepository,
			Repository: "https://git.example/demo.git", Ref: strings.Repeat("a", 40),
		}},
		{"oci", development.SourceDefinition{
			ID: "app", Type: development.SourceOCI,
			Image: "ghcr.io/example/demo@sha256:" + strings.Repeat("b", 64),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := development.SourceModel{
				SchemaVersion: development.SourceModelVersion, Application: "demo",
				Sources: []development.SourceDefinition{test.source},
				Components: []development.ComponentSource{{Component: "app", Source: "app"}},
			}
			payload, err := remoteApplyPayloadWithSource(json.RawMessage(`{"name":"demo","environment":"dev"}`), &model)
			if err != nil {
				t.Fatal(err)
			}
			var received machineApplyInput
			if err := json.Unmarshal(payload, &received); err != nil {
				t.Fatal(err)
			}
			if received.Name != "demo" || received.Environment != "dev" || received.SourceModel == nil ||
				received.SourceModel.Sources[0] != test.source {
				t.Fatalf("pinned source identity was not preserved across machine request: %+v", received)
			}
			if strings.Contains(string(payload), "XDG_CONFIG_HOME") || strings.Contains(string(payload), "/home/marcel") {
				t.Fatalf("developer-local workspace path leaked into machine request: %s", payload)
			}
		})
	}
}

func TestRemoteApplyPayloadRejectsMutableOrAmbiguousSource(t *testing.T) {
	source := development.SourceModel{
		SchemaVersion: development.SourceModelVersion, Application: "demo",
		Sources: []development.SourceDefinition{{ID: "app", Type: development.SourceRepository,
			Repository: "https://git.example/demo.git", Ref: "main"}},
		Components: []development.ComponentSource{{Component: "app", Source: "app"}},
	}
	if _, err := remoteApplyPayloadWithSource(json.RawMessage(`{"name":"demo"}`), &source); err == nil {
		t.Fatal("mutable branch accepted as immutable remote release")
	}
	source.Sources[0].Ref = strings.Repeat("a", 40)
	if _, err := remoteApplyPayloadWithSource(json.RawMessage(`{"source_model":{},"name":"demo"}`), &source); err == nil {
		t.Fatal("duplicate remote source selection accepted")
	}
}
