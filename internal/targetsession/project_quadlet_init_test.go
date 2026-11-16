package targetsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestInitGraphNeverActivatesDependentBeforeCurrentSuccessfulCompletion(t *testing.T) {
	for _, scenario := range []string{"success", "failed", "foreign-source", "implicit-restart", "unknown-init", "duplicate-init", "missing-capability"} {
		t.Run(scenario, func(t *testing.T) {
			transport := newProjectTestTransport(t, "podman")
			if scenario != "missing-capability" {
				transport.caps.Capabilities = append(transport.caps.Capabilities, Capability{Name: "runtime.quadlet.verify-completion", Available: true})
			}
			runtime, err := NewProjectRuntime(transport, transport.scope)
			if err != nil {
				t.Fatal(err)
			}
			initData := []byte("[Container]\nImage=example/init:fixture\n")
			appData := "[Unit]\nAfter=z-init.service\n[Container]\nImage=example/app:fixture\n"
			if scenario == "implicit-restart" {
				appData = "[Unit]\nWants=z-init.service\n" + appData
			}
			project, err := runtime.Stage(context.Background(), "owned", []ProjectFile{{Path: "a-app.container", Data: []byte(appData)}, {Path: "z-init.container", Data: initData}})
			if err != nil {
				t.Fatal(err)
			}
			var completed bool
			var activated []string
			transport.rewrite = func(response *Response) {
				call := transport.calls[len(transport.calls)-1]
				if call.Operation == "runtime.quadlet.verify-completion" {
					digest := sha256.Sum256(initData)
					result := map[string]any{"name": "z-init.container", "project_directory": project.directory, "content_sha256": hex.EncodeToString(digest[:]), "completed": true}
					if scenario == "foreign-source" {
						result["content_sha256"] = "foreign"
					}
					response.Result, _ = json.Marshal(result)
					response.Success = scenario != "failed"
					completed = scenario == "success"
				} else {
					var payload struct {
						Name      string `json:"name"`
						Enable    bool   `json:"enable"`
						Autostart *bool  `json:"autostart"`
					}
					if json.Unmarshal(call.Payload, &payload) != nil {
						t.Fatal("invalid publication")
					}
					if payload.Enable {
						if payload.Autostart == nil || *payload.Autostart {
							t.Fatal("init graph permits automatic activation without Core completion")
						}
						if payload.Name == "a-app.container" && !completed {
							t.Fatal("dependent activated before source-bound completion")
						}
						activated = append(activated, payload.Name)
					}
				}
			}
			initFiles := []string{"z-init.container"}
			if scenario == "unknown-init" {
				initFiles = []string{"foreign.container"}
			}
			if scenario == "duplicate-init" {
				initFiles = append(initFiles, initFiles[0])
			}
			timeout := 30 * time.Millisecond
			if scenario == "success" {
				timeout = 2 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			before := len(transport.calls)
			err = runtime.ApplyQuadletInitGraph(ctx, project, []string{"a-app.container", "z-init.container"}, initFiles)
			if (err == nil) != (scenario == "success") {
				t.Fatal("init qualification differs", err)
			}
			if scenario == "success" {
				if len(activated) != 2 || activated[0] != "z-init.container" || activated[1] != "a-app.container" {
					t.Fatal("init activation order differs", activated)
				}
			} else if scenario == "failed" || scenario == "foreign-source" {
				if len(activated) != 1 || activated[0] != "z-init.container" {
					t.Fatal("failed init replayed or dependent activated", activated)
				}
			} else if len(transport.calls) != before {
				t.Fatal("invalid init graph mutated runtime")
			}
		})
	}
}
