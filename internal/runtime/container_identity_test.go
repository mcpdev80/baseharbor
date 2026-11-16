package runtime

import (
	"context"
	"testing"
)

func TestContainerIdentityKeepsNativeNamesAndRequiresExactLabels(t *testing.T) {
	resource := ProjectResource{Kind: "container", Name: "owned-keycloak-db-0-1"}
	for _, tc := range []struct {
		kind      ProviderKind
		container RuntimeContainer
		want      bool
	}{
		{ProviderDocker, RuntimeContainer{Name: resource.Name, Project: "owned", Service: "keycloak-db-0"}, true},
		{ProviderPodman, RuntimeContainer{Name: "owned-keycloak-db-0", Project: "owned", Service: "keycloak-db-0"}, true},
		{ProviderDocker, RuntimeContainer{Name: "owned-keycloak-db-0", Project: "owned", Service: "keycloak-db-0"}, false},
		{ProviderPodman, RuntimeContainer{Name: "owned-keycloak-db-0-2", Project: "owned", Service: "keycloak-db-0"}, false},
		{ProviderPodman, RuntimeContainer{Name: resource.Name, Project: "foreign", Service: "keycloak-db-0"}, false},
		{ProviderPodman, RuntimeContainer{Name: resource.Name, Project: "owned", Service: "other"}, false},
	} {
		if got := ContainerMatchesProjectResource(tc.kind, tc.container, "owned", resource); got != tc.want {
			t.Fatalf("native container identity %#v: got %v want %v", tc, got, tc.want)
		}
	}
}

// TestParsedExpectedContainerNamesDerivesCanonicalNamesFromRenderedModel proves
// the fix for 0427-128: a workload that pins a container_name resolves to that
// single name, a default service resolves to the ordinal -1 (single replica),
// and a default service with deploy.replicas resolves to -1..-N. The rendered
// Compose model is the source of truth, never the default scheme alone.
func TestParsedExpectedContainerNamesDerivesCanonicalNamesFromRenderedModel(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rendered string
		want     []string
	}{
		{
			name:     "pinned container_name",
			rendered: `{"services":{"web":{"container_name":"linkding-web"}}}`,
			want:     []string{"linkding-web"},
		},
		{
			name:     "default single replica",
			rendered: `{"services":{"db":{"image":"postgres:16"}}}`,
			want:     []string{"owned-db-1"},
		},
		{
			name:     "default scaled by deploy.replicas",
			rendered: `{"services":{"db":{"image":"postgres:16","deploy":{"replicas":3}}}}`,
			want:     []string{"owned-db-1", "owned-db-2", "owned-db-3"},
		},
		{
			name:     "unknown service returns no planned names",
			rendered: `{"services":{}}`,
			want:     nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsedExpectedContainerNames(tc.rendered, "owned", serviceOf(tc.name))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("expected names = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("expected names = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// serviceOf disambiguates the service queried per sub-test without a second map.
func serviceOf(name string) string {
	switch name {
	case "pinned container_name":
		return "web"
	case "default single replica", "default scaled by deploy.replicas", "unknown service falls back to canonical ordinal":
		return "db"
	}
	return "web"
}

// staticConfigProvider is a RuntimeConfigProvider that returns a fixed render.
type staticConfigProvider struct{ rendered string }

func (p staticConfigProvider) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return p.rendered, nil
}

// TestExpectedContainerNameSetFallsBackToCanonicalOrdinal proves that a missing
// service in the render resolves to the single canonical ordinal, keeping the
// default-scheme path intact for callers without a rendered model.
func TestExpectedContainerNameSetFallsBackToCanonicalOrdinal(t *testing.T) {
	got, err := expectedContainerNameSet(t.Context(), staticConfigProvider{rendered: `{"services":{}}`}, "", "owned", "missing", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "owned-missing-1" {
		t.Fatalf("expected names = %v, want [owned-missing-1]", got)
	}
}
