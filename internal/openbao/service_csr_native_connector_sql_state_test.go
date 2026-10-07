package openbao

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func TestNativeSQLRegistryKeepsRuntimeStorageEnvironment(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "native-runtime-storage")
	t.Setenv("XDG_DATA_HOME", storage)
	project := targetsession.ProjectRecord{Version: 1,
		Scope:    targetenrollment.Scope{TenantID: "00000000-0000-0000-0000-000000000001", TargetID: "node-target", NodeID: "node-a", Runtime: "podman"},
		BundleID: "owned-sql", Directory: "bundles/.object-" + strings.Repeat("a", 32),
		Files: []targetsession.ProjectFileRecord{{Path: "runtime.env", SHA256: strings.Repeat("b", 64), Mode: 0600}},
	}
	restored := persistNativeSQLProject(t, t.TempDir(), application.New("demo", "dev", true, false, false), project)
	if os.Getenv("XDG_DATA_HOME") != storage || !reflect.DeepEqual(restored, project) {
		t.Fatal("registry restoration changed native runtime storage selection")
	}
}

// Persist through the production protected registry rather than retaining an
// in-memory receipt. This is provider qualification, not an Application apply.
func persistNativeSQLProject(t *testing.T, root string, manifest application.Manifest, project targetsession.ProjectRecord) targetsession.ProjectRecord {
	t.Helper()
	var restored deployment.DeploymentRecord
	if !t.Run("protected-sql-registry", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", filepath.Join(root, "sql-deployment-registry"))
		restored = roundTripNativeSQLProject(t, manifest, project)
	}) {
		t.Fatal("native SQL registry round-trip failed")
	}
	return *restored.Applied.RemoteProject
}

func roundTripNativeSQLProject(t *testing.T, manifest application.Manifest, project targetsession.ProjectRecord) deployment.DeploymentRecord {
	t.Helper()
	identity, err := deployment.NewDeploymentIdentity(project.Scope.TargetID, manifest.ApplicationID, manifest.Name, manifest.Environment)
	if err != nil {
		t.Fatal("native SQL deployment identity failed", err)
	}
	record := deployment.DeploymentRecord{
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "native-provider-qualification"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: project.Scope.Runtime, RemoteProject: &project},
		Observed: deployment.ObservedDeployment{State: "provider_verified"},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal("native SQL project registry persistence failed", err)
	}
	restored, err := deployment.LoadDeploymentRecord(identity)
	if err != nil || restored.Applied.RemoteProject == nil || !reflect.DeepEqual(*restored.Applied.RemoteProject, project) {
		t.Fatal("native SQL durable registry restoration changed the project", err)
	}
	return restored
}
