package targetsession

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

type projectTestTransport struct {
	t       *testing.T
	scope   targetenrollment.Scope
	caps    Capabilities
	calls   []Request
	rewrite func(*Response)
	fail    bool
}

func newProjectTestTransport(t *testing.T, runtime string) *projectTestTransport {
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "native-pki", NodeID: "node-live", Runtime: runtime}
	caps := Capabilities{ContractVersion: contractVersion, ProtocolVersion: protocolVersion,
		Node: Node{TenantID: scope.TenantID, TargetID: scope.TargetID, NodeID: scope.NodeID, Runtime: runtime, Identity: scope.Identity()}}
	for _, name := range []string{"artifact.bundle.stage", "runtime.compose.apply", "runtime.compose.destroy", "runtime.quadlet.apply", "runtime.quadlet.remove", "runtime.resource.list", "runtime.resource.inspect", "runtime.exec"} {
		caps.Capabilities = append(caps.Capabilities, Capability{Name: name, Available: true})
	}
	return &projectTestTransport{t: t, scope: scope, caps: caps}
}

func (f *projectTestTransport) LiveCapabilities(scope targetenrollment.Scope) (Capabilities, error) {
	if scope != f.scope {
		f.t.Fatal("foreign scope reached transport")
	}
	return f.caps, nil
}

func (f *projectTestTransport) Dispatch(_ context.Context, scope targetenrollment.Scope, request Request) (Response, error) {
	f.t.Helper()
	if scope != f.scope || request.TargetID != scope.TargetID {
		f.t.Fatal("foreign project dispatched")
	}
	data, err := json.Marshal(request)
	if err != nil || contracts.ValidateTargetAccessRecord("request", data) != nil {
		f.t.Fatal("noncanonical project request")
	}
	if !request.DeadlineAt.After(request.IssuedAt) || request.DeadlineAt.After(time.Now().Add(2*time.Minute)) {
		f.t.Fatal("project request deadline is unbounded")
	}
	f.calls = append(f.calls, request)
	if f.fail {
		return Response{}, errors.New("transport disconnected after admission")
	}
	response := Response{ContractVersion: contractVersion, ProtocolVersion: protocolVersion, RequestID: request.RequestID,
		CorrelationID: request.CorrelationID, Success: true, Result: json.RawMessage(`{"exit_code":0}`)}
	if request.Operation == "artifact.bundle.stage" {
		var bundle struct {
			BundleID string `json:"bundle_id"`
			Files    []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
				Mode   int    `json:"mode"`
			} `json:"files"`
		}
		if json.Unmarshal(request.Payload, &bundle) != nil {
			f.t.Fatal("invalid staged payload")
		}
		files := make([]map[string]string, 0, len(bundle.Files))
		for _, file := range bundle.Files {
			if file.Mode != 0600 {
				f.t.Fatal("unprotected remote artifact")
			}
			files = append(files, map[string]string{"path": "bundles/.object-" + strings.Repeat("a", 32) + "/" + file.Path, "sha256": file.SHA256})
		}
		response.Result, _ = json.Marshal(map[string]any{"bundle_id": bundle.BundleID, "files": files})
	}
	if f.rewrite != nil {
		f.rewrite(&response)
	}
	return response, nil
}

func TestProjectRuntimeStagesExactConfinedBytesAndNeverReplays(t *testing.T) {
	transport := newProjectTestTransport(t, "docker")
	runtime, err := NewProjectRuntime(transport, transport.scope)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("name: owned\nservices: {}\n")
	project, err := runtime.Stage(context.Background(), "owned-1", []ProjectFile{{Path: "compose.yaml", Data: content}, {Path: "runtime.env", Data: []byte("SECRET=private-fixture\n")}})
	if err != nil {
		t.Fatal(err)
	}
	content[0] = 'X'
	if string(project.files["compose.yaml"].data) != "name: owned\nservices: {}\n" {
		t.Fatal("caller changed staged bytes")
	}
	if err := runtime.ApplyCompose(context.Background(), project, []string{"compose.yaml"}, "runtime.env", false); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ApplyCompose(context.Background(), project, []string{"compose.yaml"}, "runtime.env", true); err != nil {
		t.Fatal(err)
	}
	if err := runtime.DestroyCompose(context.Background(), project, []string{"compose.yaml"}, "runtime.env"); err != nil {
		t.Fatal(err)
	}
	if len(transport.calls) != 4 {
		t.Fatal("unexpected invocation count")
	}
	var payload map[string]any
	if json.Unmarshal(transport.calls[2].Payload, &payload) != nil || payload["force_recreate"] != true {
		t.Fatal("repair did not use explicit recreation")
	}
	transport.fail = true
	before := len(transport.calls)
	if !errors.Is(runtime.ApplyCompose(context.Background(), project, []string{"compose.yaml"}, "", false), ErrUnavailable) || len(transport.calls) != before+1 {
		t.Fatal("interrupted mutation was retried or hidden")
	}
}

func TestProjectRuntimeRejectsForeignAndUnavailableNegotiation(t *testing.T) {
	for _, field := range []string{"tenant", "target", "node", "runtime", "identity", "contract", "capability"} {
		t.Run(field, func(t *testing.T) {
			transport := newProjectTestTransport(t, "docker")
			switch field {
			case "tenant":
				transport.caps.Node.TenantID = "22222222-2222-4222-8222-222222222222"
			case "target":
				transport.caps.Node.TargetID = "foreign"
			case "node":
				transport.caps.Node.NodeID = "foreign"
			case "runtime":
				transport.caps.Node.Runtime = "podman"
			case "identity":
				transport.caps.Node.Identity = "spiffe://foreign/core"
			case "contract":
				transport.caps.ContractVersion = "foreign/v1"
			case "capability":
				transport.caps.Capabilities[0].Available = false
			}
			if _, err := NewProjectRuntime(transport, transport.scope); err == nil || len(transport.calls) != 0 {
				t.Fatal("unbound negotiation admitted project")
			}
		})
	}
}

func TestProjectRuntimeRejectsSubstitutedStagingReceipts(t *testing.T) {
	for _, kind := range []string{"bundle", "digest", "path", "missing", "duplicate", "directory", "response", "correlation"} {
		t.Run(kind, func(t *testing.T) {
			transport := newProjectTestTransport(t, "docker")
			runtime, err := NewProjectRuntime(transport, transport.scope)
			if err != nil {
				t.Fatal(err)
			}
			transport.rewrite = func(response *Response) {
				if kind == "response" {
					response.RequestID = "foreign"
					return
				}
				if kind == "correlation" {
					response.CorrelationID = "foreign"
					return
				}
				var result map[string]any
				_ = json.Unmarshal(response.Result, &result)
				entries := result["files"].([]any)
				switch kind {
				case "bundle":
					result["bundle_id"] = "foreign"
				case "digest":
					entries[0].(map[string]any)["sha256"] = strings.Repeat("0", 64)
				case "path":
					entries[0].(map[string]any)["path"] = "bundles/foreign/compose.yaml"
				case "missing":
					result["files"] = []any{}
				case "duplicate":
					result["files"] = []any{entries[0], entries[0]}
				case "directory":
					entries[1].(map[string]any)["path"] = "bundles/.object-" + strings.Repeat("b", 32) + "/runtime.env"
				}
				response.Result, _ = json.Marshal(result)
			}
			if _, err := runtime.Stage(context.Background(), "owned-1", []ProjectFile{{Path: "compose.yaml", Data: []byte("services: {}")}, {Path: "runtime.env", Data: []byte("FIXTURE=yes")}}); err == nil {
				t.Fatal("substituted staging receipt admitted")
			}
		})
	}
}

func TestProjectRuntimeRejectsUnsafeOrUnstagedSelection(t *testing.T) {
	transport := newProjectTestTransport(t, "docker")
	runtime, _ := NewProjectRuntime(transport, transport.scope)
	for _, file := range []string{"..", "../foreign", "/foreign", "a/../b", "a\\b", ".manifest.json"} {
		if _, err := runtime.Stage(context.Background(), "owned-1", []ProjectFile{{Path: file, Data: []byte("x")}}); err == nil {
			t.Fatal("unsafe source path admitted", file)
		}
	}
	if len(transport.calls) != 0 {
		t.Fatal("unsafe bundle reached transport")
	}
	project, err := runtime.Stage(context.Background(), "owned-1", []ProjectFile{{Path: "compose.yaml", Data: []byte("services: {}")}})
	if err != nil {
		t.Fatal(err)
	}
	before := len(transport.calls)
	if runtime.ApplyCompose(context.Background(), project, []string{"foreign.yaml"}, "", false) == nil || runtime.ApplyCompose(context.Background(), project, []string{"compose.yaml"}, "foreign.env", false) == nil {
		t.Fatal("unstaged selection admitted")
	}
	foreign := *project
	foreign.scope.NodeID = "foreign"
	if runtime.DestroyCompose(context.Background(), &foreign, []string{"compose.yaml"}, "") == nil || len(transport.calls) != before {
		t.Fatal("foreign staged project dispatched")
	}
	transport.caps.Capabilities[1].Available = false
	if runtime.ApplyCompose(context.Background(), project, []string{"compose.yaml"}, "", false) == nil || len(transport.calls) != before {
		t.Fatal("lost live capability ignored")
	}
}

func TestProjectRuntimeQuadletUsesExactStagedContent(t *testing.T) {
	transport := newProjectTestTransport(t, "podman")
	runtime, _ := NewProjectRuntime(transport, transport.scope)
	content := []byte("[Container]\nImage=example@sha256:fixed\n")
	project, err := runtime.Stage(context.Background(), "owned-1", []ProjectFile{{Path: "owned.container", Data: content}})
	if err != nil {
		t.Fatal(err)
	}
	content[0] = 'X'
	if err := runtime.ApplyQuadlet(context.Background(), project, "owned.container"); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if json.Unmarshal(transport.calls[1].Payload, &payload) != nil || payload["content"] != "[Container]\nImage=example@sha256:fixed\n" {
		t.Fatal("Quadlet bytes differ")
	}
	if runtime.ApplyQuadlet(context.Background(), project, "foreign.container") == nil {
		t.Fatal("foreign Quadlet admitted")
	}
	if err := runtime.DestroyQuadlet(context.Background(), project, "owned.container"); err != nil {
		t.Fatal(err)
	}
	if runtime.ApplyCompose(context.Background(), project, []string{"owned.container"}, "", false) == nil {
		t.Fatal("Podman selection used Docker realization")
	}
}

func TestProjectRuntimeRejectsIncompleteApplyResultAndCancelledContext(t *testing.T) {
	transport := newProjectTestTransport(t, "docker")
	runtime, err := NewProjectRuntime(transport, transport.scope)
	if err != nil {
		t.Fatal(err)
	}
	project, err := runtime.Stage(context.Background(), "owned-1", []ProjectFile{{Path: "compose.yaml", Data: []byte("services: {}")}})
	if err != nil {
		t.Fatal(err)
	}
	transport.rewrite = func(response *Response) { response.Result = json.RawMessage(`{}`) }
	if runtime.ApplyCompose(context.Background(), project, []string{"compose.yaml"}, "", false) == nil {
		t.Fatal("missing runtime exit status accepted as success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := len(transport.calls)
	if !errors.Is(runtime.DestroyCompose(ctx, project, []string{"compose.yaml"}, ""), context.Canceled) || len(transport.calls) != before {
		t.Fatal("cancelled project mutation reached transport")
	}
}
