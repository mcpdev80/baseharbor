package targetsession

import (
	"context"
	"encoding/json"
	"testing"
)

func TestComposePhaseRejectsUndeclaredSelectionsBeforeDispatch(t *testing.T) {
	transport := newProjectTestTransport(t, "docker")
	runtime, err := NewProjectRuntime(transport, transport.scope)
	if err != nil {
		t.Fatal(err)
	}
	project, err := runtime.Stage(context.Background(), "owned", []ProjectFile{{Path: "compose.yaml", Data: []byte("name: owned\nservices:\n  sql: {image: postgres}\n  app: {image: app}\n")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, services := range [][]string{nil, {}, {"sql", "sql"}, {"foreign"}, {"--build"}, {"../app"}} {
		before := len(transport.calls)
		if runtime.ApplyComposeSelected(context.Background(), project, []string{"compose.yaml"}, "", services, false) == nil || len(transport.calls) != before {
			t.Fatal("invalid phase reached transport", services)
		}
	}
	if err := runtime.ApplyComposeSelected(context.Background(), project, []string{"compose.yaml"}, "", []string{"sql"}, false); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Services []string `json:"services"`
	}
	if json.Unmarshal(transport.calls[len(transport.calls)-1].Payload, &payload) != nil || len(payload.Services) != 1 || payload.Services[0] != "sql" {
		t.Fatal("phase broadened activation")
	}
	transport.fail = true
	before := len(transport.calls)
	if runtime.ApplyComposeSelected(context.Background(), project, []string{"compose.yaml"}, "", []string{"app"}, true) == nil || len(transport.calls) != before+1 {
		t.Fatal("lost phase response replayed mutation")
	}
}
