package logs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type rotationRuntime struct {
	events []string
	fail   bool
}

func (*rotationRuntime) ConfigProject(context.Context, string, string, string) error { return nil }
func (*rotationRuntime) UpProject(context.Context, string, string, string) error {
	return errors.New("whole project restart forbidden")
}
func (*rotationRuntime) StopProject(context.Context, string, string, string) error    { return nil }
func (*rotationRuntime) DestroyProject(context.Context, string, string, string) error { return nil }
func (r *rotationRuntime) UpProjectFilesSelectedForceRecreateNoBuild(_ context.Context, _, _ string, env map[string]string, services []string, _ ...string) error {
	if env["credential"] != "replacement" {
		return errors.New("replacement environment missing")
	}
	for _, service := range services {
		r.events = append(r.events, "restart:"+service)
	}
	return nil
}
func (r *rotationRuntime) ExecProject(_ context.Context, _, _, _, gateway string, args ...string) (string, error) {
	r.events = append(r.events, "probe:"+args[len(args)-1])
	if gateway != "loki-access" {
		return "", errors.New("wrong gateway")
	}
	if r.fail {
		return "", errors.New("member unready")
	}
	return "", nil
}

func TestLokiRotationWaitsForEachMemberBeforeNextRestart(t *testing.T) {
	files := ProviderFiles{Dir: t.TempDir()}
	files.Env = filepath.Join(files.Dir, "runtime.env")
	if err := os.WriteFile(files.Env, []byte("credential=replacement\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &rotationRuntime{}
	if err := rollLokiStorageMembers(context.Background(), r, Placement{Project: "owned"}, files); err != nil {
		t.Fatal(err)
	}
	want := []string{"restart:loki-1", "probe:http://loki-1:3100/ready", "restart:loki-2", "probe:http://loki-2:3100/ready", "restart:loki-3", "probe:http://loki-3:3100/ready"}
	if !reflect.DeepEqual(r.events, want) {
		t.Fatalf("events=%v", r.events)
	}
	r = &rotationRuntime{fail: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rollLokiStorageMembers(ctx, r, Placement{Project: "owned"}, files); err == nil {
		t.Fatal("accepted unready member")
	}
	if len(r.events) != 2 {
		t.Fatalf("continued after readiness failure: %v", r.events)
	}
}

func TestLokiPKIRotationLeavesStorageMembersRunning(t *testing.T) {
	files := ProviderFiles{Dir: t.TempDir()}
	files.Env = filepath.Join(files.Dir, "runtime.env")
	if err := os.WriteFile(files.Env, []byte("credential=replacement\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &rotationRuntime{}
	if err := reconcileLokiAccessRuntime(context.Background(), r, Placement{}, files, "loki-access", true); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.events, []string{"restart:loki-access", "restart:alloy"}) {
		t.Fatalf("events=%v", r.events)
	}
}
