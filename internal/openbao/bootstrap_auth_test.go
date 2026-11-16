package openbao

import (
	"context"
	"errors"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type appRoleLoginExecutor struct {
	responses []error
	calls     int
}

func (e *appRoleLoginExecutor) ExecProject(context.Context, string, string, string, string, ...string) (string, error) {
	return "", errors.New("unexpected ExecProject call")
}

func (e *appRoleLoginExecutor) ExecProjectInput(context.Context, string, string, string, []byte, string, ...string) (string, error) {
	e.calls++
	index := e.calls - 1
	if index < len(e.responses) && e.responses[index] != nil {
		return "", e.responses[index]
	}
	return `{"auth":{"client_token":"manager-token"}}`, nil
}

func TestLoginManagerRetriesTransientTransportEOF(t *testing.T) {
	executor := &appRoleLoginExecutor{responses: []error{errors.New("Error writing data to auth/approle/login: EOF")}}
	token, err := loginManager(context.Background(), executor, bhruntime.Files{}, AdminCredentials{RoleID: "role", SecretID: "secret"})
	if err != nil {
		t.Fatalf("loginManager() error = %v", err)
	}
	if token != "manager-token" {
		t.Fatalf("loginManager() token = %q, want manager-token", token)
	}
	if executor.calls != 2 {
		t.Fatalf("login attempts = %d, want 2", executor.calls)
	}
}

func TestLoginManagerRetriesConnectionReset(t *testing.T) {
	executor := &appRoleLoginExecutor{responses: []error{errors.New("write: connection reset by peer")}}
	token, err := loginManager(context.Background(), executor, bhruntime.Files{}, AdminCredentials{RoleID: "role", SecretID: "secret"})
	if err != nil {
		t.Fatalf("loginManager() error = %v", err)
	}
	if token != "manager-token" {
		t.Fatalf("loginManager() token = %q, want manager-token", token)
	}
	if executor.calls != 2 {
		t.Fatalf("login attempts = %d, want 2", executor.calls)
	}
}

func TestLoginManagerDoesNotRetryAuthenticationFailure(t *testing.T) {
	executor := &appRoleLoginExecutor{responses: []error{
		errors.New("Error writing data to auth/approle/login: Code: 400, invalid role_id or secret_id"),
	}}
	_, err := loginManager(context.Background(), executor, bhruntime.Files{}, AdminCredentials{RoleID: "role", SecretID: "secret"})
	if err == nil || !strings.Contains(err.Error(), "Code: 400") {
		t.Fatalf("loginManager() error = %v, want original authentication failure", err)
	}
	if executor.calls != 1 {
		t.Fatalf("login attempts = %d, want fail-fast after 1", executor.calls)
	}
}
