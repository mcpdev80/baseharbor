package deployment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func remoteDeploymentFixture(t *testing.T) DeploymentRecord {
	t.Helper()
	return DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: testDeploymentIdentity(t, "node-target", "demo", "dev"),
		Applied: AppliedDeployment{RuntimeProvider: "podman", RemoteProject: &targetsession.ProjectRecord{
			Version:  1,
			Scope:    targetenrollment.Scope{TenantID: "00000000-0000-0000-0000-000000000001", TargetID: "node-target", NodeID: "node-a", Runtime: "podman"},
			BundleID: "owned-project", Directory: "bundles/.object-" + strings.Repeat("a", 32),
			Files: []targetsession.ProjectFileRecord{{Path: "runtime.env", SHA256: strings.Repeat("b", 64), Mode: 0600}},
		}},
	}
}

func TestRemoteProjectDeploymentRoundTripAndCorruption(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	record := remoteDeploymentFixture(t)
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDeploymentRecord(record.Identity)
	if err != nil || !reflect.DeepEqual(got.Applied.RemoteProject, record.Applied.RemoteProject) {
		t.Fatalf("durable project binding changed: %#v %v", got.Applied.RemoteProject, err)
	}
	root, _ := DeploymentRoot(record.Identity)
	file := filepath.Join(root, "deployment.json")
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("deployment commitment is not protected", err)
	}
	record.Applied.RemoteProject.Scope.TargetID = "foreign"
	data, _ := json.Marshal(record)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadDeploymentRecord(record.Identity)
	state, classified := DeploymentRecordState(err)
	if !classified || state.Kind != "corrupt" {
		t.Fatal("foreign stored project was admitted", err)
	}
}

func TestRemoteProjectDeploymentRejectsInvalidBindingBeforeWrite(t *testing.T) {
	for _, kind := range []string{"target", "runtime", "scope", "digest", "path", "mode", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			record := remoteDeploymentFixture(t)
			project := record.Applied.RemoteProject
			switch kind {
			case "target":
				project.Scope.TargetID = "foreign"
			case "runtime":
				project.Scope.Runtime = "docker"
			case "scope":
				project.Scope.TenantID = ""
			case "digest":
				project.Files[0].SHA256 = "not-a-commitment"
			case "path":
				project.Files[0].Path = "../runtime.env"
			case "mode":
				project.Files[0].Mode = 0777
			case "duplicate":
				project.Files = append(project.Files, project.Files[0])
			}
			if err := SaveDeploymentRecord(record); err == nil {
				t.Fatal("invalid binding persisted")
			}
			root, _ := DeploymentRoot(record.Identity)
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatal("rejected binding created deployment state", err)
			}
		})
	}
}
