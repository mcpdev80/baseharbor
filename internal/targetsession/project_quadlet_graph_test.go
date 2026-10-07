package targetsession

import (
	"context"
	"encoding/json"
	"testing"
)

func TestQuadletGraphPublishesDependenciesBeforeContainersAndRemovesInReverse(t *testing.T) {
	transport := newProjectTestTransport(t, "podman")
	runtime, _ := NewProjectRuntime(transport, transport.scope)
	project, err := runtime.Stage(context.Background(), "owned", []ProjectFile{
		{Path: "owned.container", Data: []byte("[Container]\nImage=example:fixture\n")},
		{Path: "owned.network", Data: []byte("[Network]\nNetworkName=owned\n")},
		{Path: "owned.volume", Data: []byte("[Volume]\nVolumeName=owned\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"owned.container", "owned.volume", "owned.network"}
	if err := runtime.ApplyQuadletGraph(context.Background(), project, files); err != nil {
		t.Fatal(err)
	}
	for i, expected := range []string{"owned.network", "owned.volume", "owned.container", "owned.container"} {
		var payload struct {
			Name      string `json:"name"`
			Enable    bool   `json:"enable"`
			Directory string `json:"project_directory"`
		}
		if json.Unmarshal(transport.calls[i+1].Payload, &payload) != nil || payload.Name != expected ||
			payload.Enable != (i == 3) || payload.Directory != project.directory {
			t.Fatal("wrong dependency order, activation or bundle binding", payload)
		}
	}
	if err := runtime.DestroyQuadletGraph(context.Background(), project, files); err != nil {
		t.Fatal(err)
	}
	for i, expected := range []string{"owned.container", "owned.volume", "owned.network"} {
		var payload struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(transport.calls[i+5].Payload, &payload) != nil || payload.Name != expected {
			t.Fatal("wrong removal order")
		}
	}
	before := len(transport.calls)
	for _, invalid := range [][]string{{"owned.network", "foreign.container"}, {"owned.container", "owned.container"}, {"../owned.container"}, {"owned.env"}} {
		if runtime.ApplyQuadletGraph(context.Background(), project, invalid) == nil || runtime.DestroyQuadletGraph(context.Background(), project, invalid) == nil {
			t.Fatal("invalid graph admitted")
		}
	}
	if len(transport.calls) != before {
		t.Fatal("invalid graph caused a partial mutation")
	}
	transport.fail = true
	if runtime.ApplyQuadletGraph(context.Background(), project, files) == nil || len(transport.calls) != before+1 {
		t.Fatal("interrupted graph was replayed or continued")
	}
}
