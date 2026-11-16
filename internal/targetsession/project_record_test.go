package targetsession

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectRecordRestoresExactCoreBytesWithoutRestaging(t *testing.T) {
	transport := newProjectTestTransport(t, "docker")
	runtime, _ := NewProjectRuntime(transport, transport.scope)
	files := []ProjectFile{{Path: "compose.yaml", Data: []byte("services: {}")}, {Path: "runtime.env", Data: []byte("TOKEN=private-fixture")}}
	project, err := runtime.Stage(context.Background(), "owned-1", files)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := json.Marshal(project.Record())
	if err != nil || strings.Contains(string(persisted), "private-fixture") {
		t.Fatal("project receipt contains protected source bytes", err)
	}
	var record ProjectRecord
	if json.Unmarshal(persisted, &record) != nil {
		t.Fatal("durable project record could not be decoded")
	}
	restarted, _ := NewProjectRuntime(transport, transport.scope)
	restored, err := restarted.RestoreProject(record, files)
	if err != nil {
		t.Fatal(err)
	}
	files[1].Data[0] = 'X'
	if string(restored.files["runtime.env"].data) != "TOKEN=private-fixture" {
		t.Fatal("restored project retained mutable caller bytes")
	}
	if err := restarted.ApplyCompose(context.Background(), restored, []string{"compose.yaml"}, "runtime.env", true); err != nil {
		t.Fatal(err)
	}
	if len(transport.calls) != 2 || transport.calls[1].Operation != "runtime.compose.apply" {
		t.Fatal("restoration created another immutable bundle or replayed staging")
	}
}

func TestProjectRecordRejectsForeignOrChangedProtectedState(t *testing.T) {
	for _, kind := range []string{"version", "scope", "bundle", "directory", "path", "digest", "missing", "duplicate", "changed-source"} {
		t.Run(kind, func(t *testing.T) {
			transport := newProjectTestTransport(t, "docker")
			runtime, _ := NewProjectRuntime(transport, transport.scope)
			files := []ProjectFile{{Path: "compose.yaml", Data: []byte("services: {}")}}
			project, err := runtime.Stage(context.Background(), "owned-1", files)
			if err != nil {
				t.Fatal(err)
			}
			record := project.Record()
			switch kind {
			case "version":
				record.Version++
			case "scope":
				record.Scope.TargetID = "foreign"
			case "bundle":
				record.BundleID = "../foreign"
			case "directory":
				record.Directory = "/foreign"
			case "path":
				record.Files[0].Path = "../compose.yaml"
			case "digest":
				record.Files[0].SHA256 = strings.Repeat("0", 64)
			case "missing":
				record.Files = nil
			case "duplicate":
				record.Files = append(record.Files, record.Files[0])
				files = append(files, files[0])
			case "changed-source":
				files[0].Data = []byte("services: {foreign: {}}")
			}
			if _, err := runtime.RestoreProject(record, files); err == nil {
				t.Fatal("changed project state admitted")
			}
			if len(transport.calls) != 1 {
				t.Fatal("invalid state dispatched a remote operation")
			}
		})
	}
}
