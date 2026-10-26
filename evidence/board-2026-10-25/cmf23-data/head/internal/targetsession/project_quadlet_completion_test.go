package targetsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestQuadletCompletionRequiresExactSourceReceiptWithoutMutation(t *testing.T) {
	for _, scenario := range []string{"success", "missing-capability", "foreign-name", "foreign-bundle", "foreign-source", "incomplete", "operation-failed", "disconnected"} {
		t.Run(scenario, func(t *testing.T) {
			transport := newProjectTestTransport(t, "podman")
			if scenario != "missing-capability" {
				transport.caps.Capabilities = append(transport.caps.Capabilities, Capability{Name: "runtime.quadlet.verify-completion", Available: true})
			}
			runtime, err := NewProjectRuntime(transport, transport.scope)
			if err != nil {
				t.Fatal(err)
			}
			data := []byte("[Container]\nImage=example/init:fixture\n")
			project, err := runtime.Stage(context.Background(), "owned", []ProjectFile{{Path: "init.container", Data: data}})
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(data)
			transport.rewrite = func(response *Response) {
				result := map[string]any{"name": "init.container", "project_directory": project.directory, "content_sha256": hex.EncodeToString(digest[:]), "completed": true}
				switch scenario {
				case "foreign-name":
					result["name"] = "foreign.container"
				case "foreign-bundle":
					result["project_directory"] = "bundles/.object-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				case "foreign-source":
					result["content_sha256"] = "foreign"
				case "incomplete":
					result["completed"] = false
				case "operation-failed":
					response.Success = false
				}
				response.Result, _ = json.Marshal(result)
			}
			transport.fail = scenario == "disconnected"
			before := len(transport.calls)
			err = runtime.VerifyQuadletCompletion(context.Background(), project, "init.container")
			if (err == nil) != (scenario == "success") {
				t.Fatal("completion qualification differs", err)
			}
			want := 1
			if scenario == "missing-capability" {
				want = 0
			}
			if len(transport.calls)-before != want {
				t.Fatal("completion observation retried or mutated")
			}
			for _, call := range transport.calls[before:] {
				if call.Operation != "runtime.quadlet.verify-completion" {
					t.Fatal("completion observation mutated runtime")
				}
			}
			before = len(transport.calls)
			for _, file := range []string{"../init.container", "foreign.container", "init.network", ""} {
				if runtime.VerifyQuadletCompletion(context.Background(), project, file) == nil {
					t.Fatal("unstaged source accepted")
				}
			}
			foreign := *project
			foreign.scope.NodeID = "foreign"
			if runtime.VerifyQuadletCompletion(context.Background(), &foreign, "init.container") == nil || len(transport.calls) != before {
				t.Fatal("foreign or invalid completion reached transport")
			}
		})
	}
}
