package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestProviderContinuationRequiresEveryCanonicalMember(t *testing.T) {
	for _, kind := range []bhruntime.ProviderKind{bhruntime.ProviderDocker, bhruntime.ProviderPodman} {
		t.Run(string(kind), func(t *testing.T) {
			resources := canonicalProviderServiceResources("owned", []string{"db-1", "db-2", "db-3", "router"})
			containers := []bhruntime.RuntimeContainer{}
			for _, service := range []string{"db-1", "db-2", "db-3", "router"} {
				name := "owned-" + service + "-1"
				if kind == bhruntime.ProviderPodman {
					name = "owned-" + service
				}
				containers = append(containers, bhruntime.RuntimeContainer{Name: name, Project: "owned", Service: service, Running: true, Health: "healthy"})
			}
			if count, state := observeCanonicalProviderContainers(kind, "owned", resources, containers); count != 4 || state != application.ProviderRunning {
				t.Fatalf("complete module unavailable: %d %s", count, state)
			}
			for _, test := range []struct {
				name  string
				alter func([]bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer
			}{
				{"missing", func(c []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer { return c[:3] }},
				{"stopped", func(c []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer { c[3].Running = false; return c }},
				{"starting", func(c []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer { c[3].Health = "starting"; return c }},
				{"unhealthy", func(c []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer { c[3].Health = "unhealthy"; return c }},
				{"foreign-project", func(c []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer { c[3].Project = "foreign"; return c }},
				{"foreign-service", func(c []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer { c[3].Service = "foreign"; return c }},
				{"foreign-name", func(c []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer { c[3].Name = "foreign"; return c }},
				{"duplicate", func(c []bhruntime.RuntimeContainer) []bhruntime.RuntimeContainer { return append(c, c[3]) }},
			} {
				t.Run(test.name, func(t *testing.T) {
					actual := test.alter(append([]bhruntime.RuntimeContainer{}, containers...))
					if _, state := observeCanonicalProviderContainers(kind, "owned", resources, actual); state != application.ProviderUnverifiable {
						t.Fatalf("partial/foreign/unhealthy module falsely available: %s", state)
					}
				})
			}
			if _, state := observeCanonicalProviderContainers(kind, "owned", resources, nil); state != application.ProviderStoppedOwned {
				t.Fatal("owned absence is not stopped")
			}
			for _, resource := range [][]bhruntime.ProjectResource{nil, {{Kind: "volume", Name: "data"}}, {resources[0], resources[0]}} {
				if _, state := observeCanonicalProviderContainers(kind, "owned", resource, containers); state != application.ProviderUnverifiable {
					t.Fatal("missing/duplicate canonical inventory became a complete provider")
				}
			}
			// An independently declared sibling is not part of this selected module.
			containers = append(containers, bhruntime.RuntimeContainer{Name: "owned-sibling-1", Project: "owned", Service: "sibling", Running: false})
			if _, state := observeCanonicalProviderContainers(kind, "owned", resources, containers); state != application.ProviderRunning {
				t.Fatal("unselected sibling changed selected-module availability")
			}
		})
	}
}

func TestProviderContinuationRequiresSuccessfulOwnedInitCompletion(t *testing.T) {
	for _, kind := range []bhruntime.ProviderKind{bhruntime.ProviderDocker, bhruntime.ProviderPodman} {
		name := "owned-postgres-init-1"
		if kind == bhruntime.ProviderPodman {
			name = "owned-postgres-init"
		}
		original := bhruntime.RuntimeContainer{Name: name, Project: "owned", Service: "postgres-init", State: "exited", ExitCode: 0}
		resources := canonicalProviderServiceResources("owned", []string{"postgres-init"})
		if count, complete := observeCanonicalProviderCompletion(kind, "owned", resources, []bhruntime.RuntimeContainer{original}); count != 1 || !complete {
			t.Fatal("successful canonical init completion rejected")
		}
		for _, mutation := range []string{"missing", "running", "failed", "unknown-state", "state-error", "foreign-project", "foreign-service", "foreign-name", "duplicate"} {
			changed := original
			containers := []bhruntime.RuntimeContainer{changed}
			switch mutation {
			case "missing":
				containers = nil
			case "running":
				changed.Running, changed.State = true, "running"
			case "failed":
				changed.ExitCode = 1
			case "unknown-state":
				changed.State = ""
			case "state-error":
				changed.Error = "native failure"
			case "foreign-project":
				changed.Project = "foreign"
			case "foreign-service":
				changed.Service = "foreign"
			case "foreign-name":
				changed.Name = "foreign"
			case "duplicate":
				containers = append(containers, changed)
			}
			if len(containers) > 0 {
				containers[0] = changed
			}
			if _, complete := observeCanonicalProviderCompletion(kind, "owned", resources, containers); complete {
				t.Fatalf("unverified init completion accepted: %s %s", kind, mutation)
			}
		}
	}
}

type scriptedContainerLister struct {
	lists [][]bhruntime.RuntimeContainer
	calls int
}

func (s *scriptedContainerLister) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	i := s.calls
	if i >= len(s.lists) {
		i = len(s.lists) - 1
	}
	s.calls++
	return s.lists[i], nil
}

func TestZeroContainerListingIsConfirmedByFreshListing(t *testing.T) {
	resources := canonicalProviderServiceResources("owned", []string{"db"})
	running := []bhruntime.RuntimeContainer{{Name: "owned-db-1", Project: "owned", Service: "db", Running: true, Health: "healthy"}}
	flaky := &scriptedContainerLister{lists: [][]bhruntime.RuntimeContainer{running}}
	if count, state := observeCanonicalProviderContainersConfirmed(context.Background(), flaky, bhruntime.ProviderDocker, "owned", resources, nil); count != 1 || state != application.ProviderRunning {
		t.Fatalf("transiently unlisted running container changed the verdict: %d %s (calls=%d)", count, state, flaky.calls)
	}
	if flaky.calls != 1 {
		t.Fatalf("confirmation must re-list exactly once: calls=%d", flaky.calls)
	}
	absent := &scriptedContainerLister{lists: [][]bhruntime.RuntimeContainer{{}}}
	if count, state := observeCanonicalProviderContainersConfirmed(context.Background(), absent, bhruntime.ProviderDocker, "owned", resources, nil); count != 0 || state != application.ProviderStoppedOwned {
		t.Fatalf("consistently absent resources must remain stopped-but-owned: %d %s", count, state)
	}
	if absent.calls != 1 {
		t.Fatalf("stable absence must also confirm once: calls=%d", absent.calls)
	}
}
