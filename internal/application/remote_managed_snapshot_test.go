package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func TestRemoteManagedSnapshotRestoresAfterCoreSourceChanges(t *testing.T) {
	for _, kind := range []string{"docker", "podman"} {
		t.Run(kind, func(t *testing.T) {
			files, manifest := remoteRuntimeProjectionFixture(t)
			scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "selected", NodeID: "selected-node", Runtime: kind}
			transport := &managedRuntimeTransport{t: t, scope: scope}
			runtime, err := NewRemoteManagedRuntime(transport, scope, files, manifest)
			if err != nil {
				t.Fatal(err)
			}
			state := t.TempDir()
			if err := os.Chmod(state, 0700); err != nil {
				t.Fatal(err)
			}
			var record targetsession.ProjectRecord
			if err := runtime.Publish(context.Background(), func(receipt targetsession.ProjectRecord) error {
				record = receipt
				return runtime.SaveSnapshot(state, receipt)
			}); err != nil {
				t.Fatal(err)
			}
			if err := runtime.SaveSnapshot(state, record); err != nil {
				t.Fatal("identical publication retry failed", err)
			}
			if err := os.WriteFile(files.Env, []byte("changed-checkout-must-not-publish\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(files.Compose, []byte("services: {foreign: {image: alpine}}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before := len(transport.calls)
			restored, err := RestoreRemoteManagedSnapshot(transport, scope, state, record)
			if err != nil || len(transport.calls) != before {
				t.Fatal("restoration dispatched or used changed source", err)
			}
			if err := restored.Apply(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			if err := restored.Destroy(context.Background()); err != nil {
				t.Fatal(err)
			}
			for _, request := range transport.calls[before:] {
				if request.Operation == "artifact.bundle.stage" || strings.Contains(string(request.Payload), "changed-checkout") || strings.Contains(string(request.Payload), "foreign:") {
					t.Fatal("repair republished changed source")
				}
			}
			changed := record
			changed.Directory = "bundles/.object-" + strings.Repeat("b", 32)
			if err := runtime.SaveSnapshot(state, changed); err == nil {
				t.Fatal("different retained publication was replaced")
			}
			if restored.Publish(context.Background(), func(targetsession.ProjectRecord) error { return nil }) == nil {
				t.Fatal("restored project was republished")
			}
		})
	}
}

func TestRemoteManagedSnapshotRejectsUnprotectedOrSubstitutedState(t *testing.T) {
	for _, change := range []string{"directory-mode", "file-mode", "symlink", "bytes", "node", "receipt", "trailing", "units", "init"} {
		t.Run(change, func(t *testing.T) {
			files, manifest := remoteRuntimeProjectionFixture(t)
			scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "selected", NodeID: "selected-node", Runtime: "podman"}
			transport := &managedRuntimeTransport{t: t, scope: scope}
			runtime, err := NewRemoteManagedRuntime(transport, scope, files, manifest)
			if err != nil {
				t.Fatal(err)
			}
			state := t.TempDir()
			if err := os.Chmod(state, 0700); err != nil {
				t.Fatal(err)
			}
			var record targetsession.ProjectRecord
			if err := runtime.Publish(context.Background(), func(receipt targetsession.ProjectRecord) error {
				record = receipt
				return runtime.SaveSnapshot(state, receipt)
			}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(state, remoteManagedSnapshotFile)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var snapshot remoteManagedSnapshot
			if err := json.Unmarshal(data, &snapshot); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "directory-mode":
				err = os.Chmod(state, 0755)
			case "file-mode":
				err = os.Chmod(path, 0644)
			case "symlink":
				foreign := filepath.Join(t.TempDir(), "foreign.json")
				if err := os.WriteFile(foreign, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(foreign, path)
			case "node":
				scope.NodeID = "foreign-node"
			case "receipt":
				record.Directory = "bundles/.object-" + strings.Repeat("b", 32)
			case "trailing":
				err = os.WriteFile(path, append(data, []byte(" {}")...), 0600)
			default:
				switch change {
				case "bytes":
					snapshot.Files[0].Data = []byte("substituted")
				case "units":
					snapshot.Units = nil
				case "init":
					snapshot.InitUnits = []string{snapshot.Units[0]}
				}
				data, err = json.Marshal(snapshot)
				if err == nil {
					err = os.WriteFile(path, data, 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			before := len(transport.calls)
			if _, err := RestoreRemoteManagedSnapshot(transport, scope, state, record); err == nil || len(transport.calls) != before {
				t.Fatal("unprotected/substituted state restored or dispatched", err)
			}
		})
	}
}
