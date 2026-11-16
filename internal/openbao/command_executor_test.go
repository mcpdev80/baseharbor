package openbao

import (
	"context"
	"testing"
)

type recordingCommandExecutor struct {
	args  []string
	input []byte
}

func (e *recordingCommandExecutor) Exec(_ context.Context, args ...string) (string, error) {
	e.args = append([]string(nil), args...)
	return "ok", nil
}

func (e *recordingCommandExecutor) ExecInput(_ context.Context, input []byte, args ...string) (string, error) {
	e.input = append([]byte(nil), input...)
	e.args = append([]string(nil), args...)
	return "ok", nil
}

func TestExecutorFromCommandIgnoresRuntimeProjectShape(t *testing.T) {
	command := &recordingCommandExecutor{}
	legacy := ExecutorFromCommand(command)

	out, err := legacy.ExecProject(
		context.Background(),
		"compose-project",
		"/tmp/compose.yaml",
		"/tmp/runtime.env",
		"openbao",
		"bao", "status",
	)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("out = %q", out)
	}
	if len(command.args) != 2 || command.args[0] != "bao" || command.args[1] != "status" {
		t.Fatalf("args = %#v", command.args)
	}

	_, err = legacy.ExecProjectInput(
		context.Background(),
		"other-project",
		"/different/compose.yaml",
		"/different/runtime.env",
		[]byte("secret-input"),
		"different-service",
		"bao", "write",
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(command.input) != "secret-input" {
		t.Fatalf("input = %q", command.input)
	}
}
