package targetsession

import (
	"context"
	"encoding/json"
	"testing"
)

func TestQuadletResetChecksCapabilityBeforeTeardownAndBindsExactVolumeSource(t *testing.T) {
	transport := newProjectTestTransport(t, "podman")
	runtime, _ := NewProjectRuntime(transport, transport.scope)
	content := "[Volume]\nVolumeName=owned-data\nLabel=com.docker.compose.project=owned\n"
	project, err := runtime.Stage(context.Background(), "owned", []ProjectFile{{Path: "owned.container", Data: []byte("[Container]\nImage=example:fixture\n")}, {Path: "owned.volume", Data: []byte(content)}})
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"owned.container", "owned.volume"}
	before := len(transport.calls)
	if runtime.ResetQuadletGraph(context.Background(), project, files) == nil || len(transport.calls) != before {
		t.Fatal("missing reset capability partially destroyed project")
	}
	transport.caps.Capabilities = append(transport.caps.Capabilities, Capability{Name: "runtime.quadlet.reset-volume", Available: true})
	if err := runtime.ResetQuadletGraph(context.Background(), project, files); err != nil {
		t.Fatal(err)
	}
	calls := transport.calls[before:]
	if len(calls) != 5 || calls[0].Operation != "runtime.quadlet.apply" || calls[1].Operation != "runtime.quadlet.apply" || calls[2].Operation != "runtime.quadlet.remove" || calls[3].Operation != "runtime.quadlet.remove" || calls[4].Operation != "runtime.quadlet.reset-volume" {
		t.Fatal("reset did not follow exact teardown", calls)
	}
	var payload struct {
		Name      string `json:"name"`
		Content   string `json:"content"`
		Directory string `json:"project_directory"`
	}
	if json.Unmarshal(calls[4].Payload, &payload) != nil || payload.Name != "owned.volume" || payload.Content != content || payload.Directory != project.directory {
		t.Fatal("reset substituted published volume")
	}
}
