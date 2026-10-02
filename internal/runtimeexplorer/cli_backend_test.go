package runtimeexplorer

import (
	"context"
	"io"
	"strings"
	"testing"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

type fakeDirectRuntime struct {
	args []string
}

func (f *fakeDirectRuntime) Kind() runtimecontract.ProviderKind {
	return runtimecontract.ProviderDocker
}
func (f *fakeDirectRuntime) ListRuntimeContainers(context.Context) ([]runtimecontract.RuntimeContainer, error) {
	return nil, nil
}
func (f *fakeDirectRuntime) DirectOutput(_ context.Context, args ...string) (string, error) {
	f.args = append([]string(nil), args...)
	return "ok", nil
}
func (f *fakeDirectRuntime) DirectStream(_ context.Context, args ...string) (io.ReadCloser, error) {
	f.args = append([]string(nil), args...)
	return io.NopCloser(strings.NewReader("ok")), nil
}

func TestCLIContainerBackendUsesBoundedContainerCommands(t *testing.T) {
	runtime := &fakeDirectRuntime{}
	backend, err := NewCLIContainerBackend(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.OperateContainer(context.Background(), "abc", OperationRestart, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runtime.args, " "); got != "container restart abc" {
		t.Fatalf("restart command = %q", got)
	}
	if _, err := backend.OperateContainer(context.Background(), "abc", OperationExec, []string{"printf", "ok"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runtime.args, " "); got != "container exec abc printf ok" {
		t.Fatalf("exec command = %q", got)
	}
}
