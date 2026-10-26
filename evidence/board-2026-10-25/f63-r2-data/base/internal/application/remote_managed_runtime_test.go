package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

type managedRuntimeTransport struct {
	t     *testing.T
	scope targetenrollment.Scope
	calls []targetsession.Request
	fail  bool
}

func (f *managedRuntimeTransport) LiveCapabilities(scope targetenrollment.Scope) (targetsession.Capabilities, error) {
	if scope != f.scope {
		return targetsession.Capabilities{}, errors.New("foreign scope")
	}
	result := targetsession.Capabilities{ContractVersion: "baseharbor.target-access/v1", ProtocolVersion: "1", Node: targetsession.Node{TenantID: scope.TenantID, NodeID: scope.NodeID, TargetID: scope.TargetID, Runtime: scope.Runtime, Identity: scope.Identity()}}
	for _, name := range []string{"artifact.bundle.stage", "runtime.compose.apply", "runtime.compose.destroy", "runtime.quadlet.apply", "runtime.quadlet.remove", "runtime.quadlet.verify-completion", "runtime.quadlet.reset-volume", "runtime.resource.list", "runtime.resource.inspect", "runtime.exec"} {
		result.Capabilities = append(result.Capabilities, targetsession.Capability{Name: name, Available: true})
	}
	return result, nil
}

func (f *managedRuntimeTransport) Dispatch(_ context.Context, scope targetenrollment.Scope, request targetsession.Request) (targetsession.Response, error) {
	f.t.Helper()
	encoded, _ := json.Marshal(request)
	if scope != f.scope || contracts.ValidateTargetAccessRecord("request", encoded) != nil {
		f.t.Fatal("unbound or invalid request")
	}
	f.calls = append(f.calls, request)
	if f.fail {
		return targetsession.Response{}, errors.New("disconnected")
	}
	result := json.RawMessage(`{"exit_code":0}`)
	if request.Operation == "artifact.bundle.stage" {
		var payload struct {
			BundleID string `json:"bundle_id"`
			Files    []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
			} `json:"files"`
		}
		if json.Unmarshal(request.Payload, &payload) != nil {
			f.t.Fatal("invalid bundle")
		}
		receipt := map[string]any{"bundle_id": payload.BundleID}
		var files []map[string]string
		for _, file := range payload.Files {
			files = append(files, map[string]string{"path": "bundles/.object-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/" + file.Path, "sha256": file.SHA256})
		}
		receipt["files"] = files
		result, _ = json.Marshal(receipt)
	}
	return targetsession.Response{ContractVersion: request.ContractVersion, ProtocolVersion: request.ProtocolVersion, RequestID: request.RequestID, CorrelationID: request.CorrelationID, Success: true, Result: result}, nil
}

func TestRemoteManagedRuntimeSnapshotsProtectedFilesAndRestoresWithoutPublication(t *testing.T) {
	for _, kind := range []string{"docker", "podman"} {
		t.Run(kind, func(t *testing.T) {
			files, manifest := remoteRuntimeProjectionFixture(t)
			scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "selected", NodeID: "selected-node", Runtime: kind}
			transport := &managedRuntimeTransport{t: t, scope: scope}
			runtime, err := NewRemoteManagedRuntime(transport, scope, files, manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Apply(context.Background(), false); err == nil || len(transport.calls) != 0 {
				t.Fatal("unpublished project activated")
			}
			original, _ := os.ReadFile(files.Env)
			if err := os.WriteFile(files.Env, []byte("changed-after-preparation=forbidden\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Publish(context.Background(), func(record targetsession.ProjectRecord) error { return record.Validate() }); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(transport.calls[0].Payload), "changed-after-preparation") {
				t.Fatal("mutable Core source replaced snapshot")
			}
			record := runtime.Record()
			if record.Validate() != nil {
				t.Fatal("missing protected receipt")
			}
			before := len(transport.calls)
			if runtime.Publish(context.Background(), func(record targetsession.ProjectRecord) error { return record.Validate() }) == nil || len(transport.calls) != before {
				t.Fatal("published snapshot staged twice")
			}
			if err := runtime.Apply(context.Background(), false); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(files.Env, original, 0600); err != nil {
				t.Fatal(err)
			}
			restored, err := NewRemoteManagedRuntime(transport, scope, files, manifest)
			if err != nil {
				t.Fatal(err)
			}
			before = len(transport.calls)
			if err := restored.Restore(record); err != nil || len(transport.calls) != before {
				t.Fatal("restoration restaged project", err)
			}
			if err := restored.Apply(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			if err := restored.Destroy(context.Background()); err != nil {
				t.Fatal(err)
			}
			for _, call := range transport.calls[before:] {
				if call.Operation == "artifact.bundle.stage" {
					t.Fatal("repair or destroy republished immutable source")
				}
			}
			validRecord := record
			record.Scope.NodeID = "foreign"
			if restored.Restore(record) == nil {
				t.Fatal("foreign node receipt adopted")
			}
			before = len(transport.calls)
			if restored.Apply(context.Background(), true) == nil || len(transport.calls) != before {
				t.Fatal("failed restoration retained executable state")
			}
			if err := restored.Restore(validRecord); err != nil {
				t.Fatal(err)
			}
			transport.fail = true
			before = len(transport.calls)
			if restored.Apply(context.Background(), true) == nil || len(transport.calls) != before+1 {
				t.Fatal("disconnected mutation replayed or succeeded")
			}
		})
	}
}

func TestRemoteManagedRuntimeRejectsChangedDefinitionBeforeDispatch(t *testing.T) {
	files, manifest := remoteRuntimeProjectionFixture(t)
	if err := os.WriteFile(files.Compose, []byte("services: {foreign: {image: foreign}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "selected", NodeID: "selected-node", Runtime: "podman"}
	transport := &managedRuntimeTransport{t: t, scope: scope}
	if _, err := NewRemoteManagedRuntime(transport, scope, files, manifest); err == nil || len(transport.calls) != 0 {
		t.Fatal("modified generated provider definition reached node")
	}
}

func TestRemoteManagedRuntimeRequiresDurableCommitBeforeActivation(t *testing.T) {
	for _, kind := range []string{"docker", "podman"} {
		t.Run(kind, func(t *testing.T) {
			files, manifest := remoteRuntimeProjectionFixture(t)
			scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "selected", NodeID: "selected-node", Runtime: kind}
			transport := &managedRuntimeTransport{t: t, scope: scope}
			runtime, err := NewRemoteManagedRuntime(transport, scope, files, manifest)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.Publish(context.Background(), nil) == nil || len(transport.calls) != 0 {
				t.Fatal("publication without persistence reached Node")
			}
			failedCommit := errors.New("protected registry unavailable")
			var receipt targetsession.ProjectRecord
			err = runtime.Publish(context.Background(), func(record targetsession.ProjectRecord) error {
				receipt = record
				before := len(transport.calls)
				if record.Validate() != nil || runtime.Apply(context.Background(), false) == nil || len(transport.calls) != before {
					t.Fatal("activation permitted before durable commit")
				}
				return failedCommit
			})
			if !errors.Is(err, failedCommit) || receipt.Validate() != nil {
				t.Fatal("failed commit lost exact receipt or cause", err)
			}
			before := len(transport.calls)
			if runtime.Apply(context.Background(), false) == nil || runtime.Destroy(context.Background()) == nil ||
				runtime.Publish(context.Background(), func(targetsession.ProjectRecord) error { return nil }) == nil || len(transport.calls) != before {
				t.Fatal("failed commit permitted activation, teardown or publication replay")
			}
		})
	}
}
