package repositoryinspect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type conformanceCorpus struct {
	Cases []conformanceCase `yaml:"cases"`
}

type conformanceCase struct {
	ID          string            `yaml:"id"`
	Description string            `yaml:"description"`
	Files       map[string]string `yaml:"files"`
	Expected    struct {
		State                WorkloadSourceResolutionState  `yaml:"state"`
		Reason               WorkloadSourceResolutionReason `yaml:"reason"`
		SourceKind           WorkloadSourceKind             `yaml:"source_kind"`
		SourcePath           string                         `yaml:"source_path"`
		Components           []string                       `yaml:"components"`
		Infrastructure       map[string]string              `yaml:"infrastructure"`
		OpaqueResources      []string                       `yaml:"opaque_resources"`
		ConfigRefs           []string                       `yaml:"config_refs"`
		ForbiddenStrings     []string                       `yaml:"forbidden_strings"`
		InspectErrorContains string                         `yaml:"inspect_error_contains"`
	} `yaml:"expected"`
}

func TestAdoptionConformanceCorpus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "adoption-conformance", "cases.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus conformanceCorpus
	if err := yaml.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) < 10 {
		t.Fatalf("conformance corpus too small: %d cases", len(corpus.Cases))
	}

	for _, tc := range corpus.Cases {
		tc := tc
		t.Run(tc.ID, func(t *testing.T) {
			root := t.TempDir()
			for path, content := range tc.Files {
				full := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			result, err := Inspect(t.Context(), root)
			if tc.Expected.InspectErrorContains != "" {
				if err == nil {
					t.Fatalf("expected inspect error containing %q", tc.Expected.InspectErrorContains)
				}
				if !strings.Contains(err.Error(), tc.Expected.InspectErrorContains) {
					t.Fatalf("inspect error = %q, want substring %q", err, tc.Expected.InspectErrorContains)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if got := result.WorkloadSourceResolution.State; got != tc.Expected.State {
				t.Fatalf("resolution state = %q, want %q", got, tc.Expected.State)
			}
			if got := result.WorkloadSourceResolution.Reason; got != tc.Expected.Reason {
				t.Fatalf("resolution reason = %q, want %q", got, tc.Expected.Reason)
			}
			if tc.Expected.SourceKind != "" {
				if result.WorkloadSourceResolution.Selected == nil {
					t.Fatal("expected selected source")
				}
				if got := result.WorkloadSourceResolution.Selected.Kind; got != tc.Expected.SourceKind {
					t.Fatalf("source kind = %q, want %q", got, tc.Expected.SourceKind)
				}
			}
			if tc.Expected.SourcePath != "" {
				if result.WorkloadSourceResolution.Selected == nil {
					t.Fatal("expected selected source")
				}
				if got := result.WorkloadSourceResolution.Selected.Path; got != tc.Expected.SourcePath {
					t.Fatalf("source path = %q, want %q", got, tc.Expected.SourcePath)
				}
			}

			if len(tc.Expected.Components) > 0 {
				if result.WorkloadEvidence == nil {
					t.Fatal("expected workload evidence")
				}
				var got []string
				for _, component := range result.WorkloadEvidence.Components {
					got = append(got, component.ID)
				}
				sort.Strings(got)
				want := append([]string(nil), tc.Expected.Components...)
				sort.Strings(want)
				if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
					t.Fatalf("components = %#v, want %#v", got, want)
				}
			}

			if len(tc.Expected.Infrastructure) > 0 {
				if result.WorkloadEvidence == nil {
					t.Fatal("expected workload evidence")
				}
				byID := map[string]WorkloadComponent{}
				for _, component := range result.WorkloadEvidence.Components {
					byID[component.ID] = component
				}
				for id, want := range tc.Expected.Infrastructure {
					got, ok := byID[id]
					if !ok {
						t.Fatalf("missing component %q", id)
					}
					if got.InfrastructureClass != want {
						t.Fatalf("%s infrastructure = %q, want %q", id, got.InfrastructureClass, want)
					}
				}
			}

			if len(tc.Expected.OpaqueResources) > 0 {
				if result.WorkloadEvidence == nil {
					t.Fatal("expected workload evidence")
				}
				var got []string
				for _, ref := range result.WorkloadEvidence.Opaque {
					got = append(got, ref.Resource)
				}
				for _, want := range tc.Expected.OpaqueResources {
					if !containsString(got, want) {
						t.Fatalf("opaque resources = %#v, missing %q", got, want)
					}
				}
			}

			if len(tc.Expected.ConfigRefs) > 0 {
				if result.WorkloadEvidence == nil {
					t.Fatal("expected workload evidence")
				}
				var got []string
				for _, component := range result.WorkloadEvidence.Components {
					got = append(got, component.ConfigRefs...)
				}
				for _, want := range tc.Expected.ConfigRefs {
					if !containsString(got, want) {
						t.Fatalf("config refs = %#v, missing %q", got, want)
					}
				}
			}

			if len(tc.Expected.ForbiddenStrings) > 0 {
				payload, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				text := string(payload)
				for _, forbidden := range tc.Expected.ForbiddenStrings {
					if strings.Contains(text, forbidden) {
						t.Fatalf("forbidden value %q leaked into inspection result", forbidden)
					}
				}
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
