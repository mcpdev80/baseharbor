package openbao

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// Persist through the production protected registry rather than retaining an
// in-memory receipt. This is provider qualification, not an Application apply.
func persistNativeSQLProject(t *testing.T, root string, manifest application.Manifest, project targetsession.ProjectRecord) targetsession.ProjectRecord {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "sql-deployment-registry"))
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
	return *restored.Applied.RemoteProject
}
