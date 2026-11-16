package targetsession

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func projectObservationTransport(t *testing.T, engine string) *projectTestTransport {
	transport := newProjectTestTransport(t, engine)
	transport.rewrite = func(response *Response) {
		switch transport.calls[len(transport.calls)-1].Operation {
		case "runtime.resource.list":
			response.Result = json.RawMessage(`[{"id":"aaaaaaaaaaaa","compose_project":"owned","compose_service":"postgres"},{"id":"bbbbbbbbbbbb","compose_project":"foreign","compose_service":"postgres"}]`)
		case "runtime.resource.inspect":
			response.Result = json.RawMessage(`[{"Id":"` + strings.Repeat("a", 64) + `","Config":{"Labels":{"com.docker.compose.project":"owned","com.docker.compose.service":"postgres"}},"State":{"Running":true,"Health":{"Status":"starting"}}}]`)
		case "runtime.exec":
			response.Result = json.RawMessage(`{"stdout":"1\n","exit_code":0}`)
		}
	}
	return transport
}

func TestProjectServiceProbeUsesInspectedImmutableIDOnBothRuntimes(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		t.Run(engine, func(t *testing.T) {
			transport := projectObservationTransport(t, engine)
			runtime, _ := NewProjectRuntime(transport, transport.scope)
			observed, err := runtime.ObserveProject(context.Background(), "owned")
			if err != nil || len(observed) != 1 || !observed[0].Running || observed[0].Health != "starting" {
				t.Fatal("actual health state was lost", observed, err)
			}
			output, err := runtime.ExecService(context.Background(), "owned", "postgres", "sh", "-ec", `PGPASSWORD="$POSTGRES_PASSWORD" psql -tAc 'SELECT 1'`)
			if err != nil || output != "1\n" {
				t.Fatal(output, err)
			}
			var payload struct {
				ID string `json:"resource_id"`
			}
			if json.Unmarshal(transport.calls[len(transport.calls)-1].Payload, &payload) != nil || payload.ID != strings.Repeat("a", 64) {
				t.Fatal("probe selected a name or mutable inventory reference")
			}
			for _, call := range transport.calls {
				if strings.Contains(string(call.Payload), "bbbb") {
					t.Fatal("foreign container was inspected or executed")
				}
			}
		})
	}
}

func TestProjectServiceProbeRejectsOwnershipRacesAndIncompleteEvidence(t *testing.T) {
	for _, kind := range []string{"project", "service", "id", "missing-state", "stopped", "replicas", "exit-missing", "exit-failure"} {
		t.Run(kind, func(t *testing.T) {
			transport := projectObservationTransport(t, "docker")
			valid := transport.rewrite
			transport.rewrite = func(response *Response) {
				valid(response)
				operation := transport.calls[len(transport.calls)-1].Operation
				if operation == "runtime.resource.inspect" {
					text := string(response.Result)
					switch kind {
					case "project":
						text = strings.ReplaceAll(text, `"owned"`, `"foreign"`)
					case "service":
						text = strings.ReplaceAll(text, `"postgres"`, `"foreign"`)
					case "id":
						text = strings.ReplaceAll(text, strings.Repeat("a", 64), strings.Repeat("b", 64))
					case "missing-state":
						text = strings.ReplaceAll(text, `"Running":true,`, "")
					case "stopped":
						text = strings.ReplaceAll(text, `"Running":true`, `"Running":false`)
					}
					response.Result = json.RawMessage(text)
				}
				if operation == "runtime.resource.list" && kind == "replicas" {
					response.Result = json.RawMessage(`[{"id":"aaaaaaaaaaaa","compose_project":"owned","compose_service":"postgres"},{"id":"aaaaaaaaaaaaa","compose_project":"owned","compose_service":"postgres"}]`)
				}
				if operation == "runtime.exec" {
					if kind == "exit-missing" {
						response.Result = json.RawMessage(`{"stdout":"1"}`)
					}
					if kind == "exit-failure" {
						response.Result = json.RawMessage(`{"exit_code":1,"stderr":"private-fixture"}`)
					}
				}
			}
			runtime, _ := NewProjectRuntime(transport, transport.scope)
			output, err := runtime.ExecService(context.Background(), "owned", "postgres", "true")
			if err == nil || output != "" || strings.Contains(err.Error(), "private-fixture") {
				t.Fatal("unverified service probe accepted or disclosed remote diagnostics", output, err)
			}
			for _, call := range transport.calls {
				if kind != "exit-missing" && kind != "exit-failure" && call.Operation == "runtime.exec" {
					t.Fatal("ownership/state failure reached execution")
				}
			}
		})
	}
}

func TestProjectBundleIDsMatchNodePublicationContract(t *testing.T) {
	transport := newProjectTestTransport(t, "docker")
	runtime, _ := NewProjectRuntime(transport, transport.scope)
	for _, id := range []string{"has.dot", strings.Repeat("a", 65), "../foreign"} {
		if _, err := runtime.Stage(context.Background(), id, []ProjectFile{{Path: "compose.yaml", Data: []byte("services: {}")}}); err == nil {
			t.Fatal("bundle outside Node publication contract admitted")
		}
	}
	if len(transport.calls) != 0 {
		t.Fatal("invalid bundle reached transport")
	}
}
