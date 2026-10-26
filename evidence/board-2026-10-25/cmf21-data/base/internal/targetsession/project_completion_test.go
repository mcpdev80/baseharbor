package targetsession

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProjectCompletionRequiresSuccessfulOwnedNativeExit(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		t.Run(engine, func(t *testing.T) {
			for _, kind := range []string{"success", "running", "created", "dead", "nonzero", "missing-exit", "missing-start", "missing-finish", "reversed-time", "future", "foreign-project", "foreign-service", "absent", "replicas", "disconnected"} {
				t.Run(kind, func(t *testing.T) {
					transport := projectObservationTransport(t, engine)
					base := transport.rewrite
					transport.rewrite = func(response *Response) {
						base(response)
						op := transport.calls[len(transport.calls)-1].Operation
						if op == "runtime.resource.list" {
							if kind == "absent" {
								response.Result = json.RawMessage(`[]`)
							}
							if kind == "replicas" {
								response.Result = json.RawMessage(`[{"id":"aaaaaaaaaaaa","compose_project":"owned","compose_service":"postgres"},{"id":"aaaaaaaaaaaaa","compose_project":"owned","compose_service":"postgres"}]`)
							}
						}
						if op != "runtime.resource.inspect" {
							return
						}
						now := time.Now().UTC()
						state := map[string]any{"Running": false, "Status": "exited", "ExitCode": 0, "StartedAt": now.Add(-2 * time.Second), "FinishedAt": now.Add(-time.Second)}
						labels := map[string]string{"com.docker.compose.project": "owned", "com.docker.compose.service": "postgres"}
						switch kind {
						case "running":
							state["Running"] = true
						case "created":
							state["Status"] = "created"
						case "dead":
							state["Status"] = "dead"
						case "nonzero":
							state["ExitCode"] = 1
						case "missing-exit":
							delete(state, "ExitCode")
						case "missing-start":
							delete(state, "StartedAt")
						case "missing-finish":
							delete(state, "FinishedAt")
						case "reversed-time":
							state["FinishedAt"] = now.Add(-3 * time.Second)
						case "future":
							state["FinishedAt"] = now.Add(time.Hour)
						case "foreign-project":
							labels["com.docker.compose.project"] = "foreign"
						case "foreign-service":
							labels["com.docker.compose.service"] = "foreign"
						}
						response.Result, _ = json.Marshal([]any{map[string]any{"Id": strings.Repeat("a", 64), "Config": map[string]any{"Labels": labels}, "State": state}})
					}
					runtime, err := NewProjectRuntime(transport, transport.scope)
					if err != nil {
						t.Fatal(err)
					}
					if kind == "disconnected" {
						transport.fail = true
					}
					err = runtime.VerifyCompletedService(context.Background(), "owned", "postgres")
					if (err == nil) != (kind == "success") {
						t.Fatal("unverified completion accepted or successful native exit lost", err)
					}
					for _, call := range transport.calls {
						if call.Operation != "runtime.resource.list" && call.Operation != "runtime.resource.inspect" {
							t.Fatal("completion observation executed or replayed a mutation", call.Operation)
						}
					}
				})
			}
		})
	}
}
