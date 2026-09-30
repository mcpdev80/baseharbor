package objectstorage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type stdinCaptureRuntime struct {
	input []byte
	args  []string
	err   error
}

func (f *stdinCaptureRuntime) ConfigProject(context.Context, string, string, string) error {
	return nil
}
func (f *stdinCaptureRuntime) UpProject(context.Context, string, string, string) error { return nil }
func (f *stdinCaptureRuntime) DestroyProject(context.Context, string, string, string) error {
	return nil
}
func (f *stdinCaptureRuntime) ExecProject(context.Context, string, string, string, string, ...string) (string, error) {
	return "", nil
}
func (f *stdinCaptureRuntime) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	f.input = append([]byte(nil), input...)
	f.args = append([]string(nil), args...)
	return "", f.err
}

func TestSeaweedShellKeepsSensitiveCommandOutOfArgumentsAndErrors(t *testing.T) {
	runtime := &stdinCaptureRuntime{err: errors.New("runtime failed")}
	driver := NewDriver(runtime, application.Manifest{}, application.RuntimeFiles{}, nil)
	files := ProviderFiles{Compose: "/provider/compose.yaml", Env: "/provider/runtime.env"}
	command := "s3.configure -access_key=TESTACCESS -secret_key=TESTSECRET -user=test -apply"

	err := driver.runSeaweedShell(context.Background(), files, command)
	if err == nil {
		t.Fatal("runSeaweedShell() error = nil")
	}
	if got := strings.Join(runtime.args, " "); got != "weed shell" {
		t.Fatalf("process args = %q, want only weed shell", got)
	}
	if string(runtime.input) != command+"\n" {
		t.Fatalf("stdin = %q", runtime.input)
	}
	if strings.Contains(err.Error(), "TESTACCESS") || strings.Contains(err.Error(), "TESTSECRET") {
		t.Fatalf("error leaked credential material: %v", err)
	}
}
