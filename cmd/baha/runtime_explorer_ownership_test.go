package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

func TestRuntimeExplorerDoesNotClaimUnregisteredSiblingAsProvider(t *testing.T) {
	resolved := retainedNativeBackendFixture(t)
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	for _, tc := range []struct {
		service string
		managed bool
	}{
		{"mongodb-first", true},
		{"demo-app", true},
		{"foreign-sibling", false},
		{"broker", false},
	} {
		container := bhruntime.RuntimeContainer{ID: tc.service, Name: files.Project + "-" + tc.service + "-1", Project: files.Project, Service: tc.service}
		evidence, err := (deploymentRuntimeOwnershipResolver{}).ResolveContainer(context.Background(), resolved.Target.Name, bhruntime.ProviderDocker, container)
		if err != nil {
			t.Fatal(err)
		}
		if (evidence.Ownership == runtimeexplorer.OwnershipManaged) != tc.managed {
			t.Fatalf("service %s owner falsely inferred from project label: %#v", tc.service, evidence)
		}
	}
}

func TestDeclaredNativeRuntimeContainerKeepsDockerAndQuadletIdentity(t *testing.T) {
	m := application.WithDocumentDatabaseInstances(application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "api"), "records")
	files := application.RuntimeFilesFor(application.Store{Namespace: "isolated"}, m)
	for _, kind := range []bhruntime.ProviderKind{bhruntime.ProviderDocker, bhruntime.ProviderPodman} {
		name := files.Project + "-mongodb-records-1"
		if kind == bhruntime.ProviderPodman {
			name = files.Project + "-mongodb-records"
		}
		known, err := declaredNativeRuntimeContainer(m, files, kind, bhruntime.RuntimeContainer{Name: name, Project: files.Project, Service: "mongodb-records"})
		if err != nil || !known {
			t.Fatalf("native identity %s: %v %v", kind, known, err)
		}
	}
}
