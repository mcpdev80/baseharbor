package development

import (
	"strings"
	"testing"
)

func TestValidateImmutableRemoteSources(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  SourceKind
		ref   string
		image string
		ok    bool
	}{
		{"pinned-git", SourceRepository, strings.Repeat("a", 40), "", true},
		{"git-branch", SourceRepository, "main", "", false},
		{"git-uppercase", SourceRepository, strings.Repeat("A", 40), "", false},
		{"git-forged-sha", SourceRepository, strings.Repeat("z", 40), "", false},
		{"pinned-oci", SourceOCI, "", "ghcr.io/example/app@sha256:" + strings.Repeat("b", 64), true},
		{"oci-tag", SourceOCI, "", "ghcr.io/example/app:v1.2", false},
		{"oci-short-digest", SourceOCI, "", "ghcr.io/example/app@sha256:deadbeef", false},
		{"oci-uppercase", SourceOCI, "", "ghcr.io/example/app@sha256:" + strings.Repeat("F", 64), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := SourceDefinition{ID: "app", Type: tc.kind, Repository: "https://git.example/app.git", Ref: tc.ref, Image: tc.image}
			if tc.kind == SourceOCI {
				source.Repository = ""
			}
			model := SourceModel{SchemaVersion: SourceModelVersion, Application: "demo",
				Sources:    []SourceDefinition{source},
				Components: []ComponentSource{{Component: "app", Source: "app"}},
			}
			err := ValidateImmutableRemoteSources(model)
			if (err == nil) != tc.ok {
				t.Fatalf("immutable source result = %v; want allowed=%v", err, tc.ok)
			}
		})
	}
}

func TestRemoteGitRejectsUnsafeOrNonRemoteLocations(t *testing.T) {
	for _, tc := range []struct {
		url string
		ok  bool
	}{
		{"https://github.com/org/app.git", true},
		{"ssh://git@example.org/org/app.git", true},
		{"git@example.org:org/app.git", true},
		{"/tmp/checkout", false},
		{"file:///tmp/checkout", false},
		{"http://example.org/app.git", false},
		{"git://example.org/app.git", false},
		{"https://user:password@example.org/app.git", false},
		{"https://example.org/app.git?token=secret", false},
		{"https://example.org/app.git#main", false},
	} {
		if err := validateRemoteGitLocation(tc.url); (err == nil) != tc.ok {
			t.Errorf("Git source %q validation = %v, want allowed=%t", tc.url, err, tc.ok)
		}
	}
}
