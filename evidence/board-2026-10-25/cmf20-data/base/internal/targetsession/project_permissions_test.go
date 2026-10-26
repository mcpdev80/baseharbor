package targetsession

import (
	"context"
	"testing"
)

func TestProjectRestorationRetainsExactProtectedFilePermissions(t *testing.T) {
	f := newProjectTestTransport(t, "docker")
	r, err := NewProjectRuntime(f, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	files := []ProjectFile{{Path: "tls/key.pem", Data: []byte("native-key-fixture"), Mode: 0644}, {Path: "runtime.env", Data: []byte("PASSWORD=fixture"), Mode: 0600}}
	p, err := r.Stage(context.Background(), "protected", files)
	if err != nil {
		t.Fatal(err)
	}
	record := p.Record()
	restarted, err := NewProjectRuntime(f, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	before := len(f.calls)
	if _, err := restarted.RestoreProject(record, files); err != nil {
		t.Fatal(err)
	}
	files[0].Mode = 0600
	if _, err := restarted.RestoreProject(record, files); err == nil {
		t.Fatal("changed runtime bind permissions restored")
	}
	if len(f.calls) != before {
		t.Fatal("restoration dispatched another staging operation")
	}
	for _, mode := range []uint32{0666, 0777, 0640, 04000 | 0700} {
		if _, err := r.Stage(context.Background(), "invalid", []ProjectFile{{Path: "key.pem", Data: []byte("fixture"), Mode: mode}}); err == nil {
			t.Fatal("unsupported permissions dispatched", mode)
		}
	}
	if len(f.calls) != before {
		t.Fatal("invalid permissions reached the node")
	}
}
