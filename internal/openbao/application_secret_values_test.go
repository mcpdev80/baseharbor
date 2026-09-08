package openbao

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type secretInputExecutor struct {
	args  []string
	input []byte
}

func (f *secretInputExecutor) ExecProject(context.Context, string, string, string, string, ...string) (string, error) {
	return "", nil
}

func (f *secretInputExecutor) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	f.args = append([]string(nil), args...)
	f.input = append([]byte(nil), input...)
	return "", nil
}

func TestValidateApplicationSecretKey(t *testing.T) {
	valid := []string{"API_TOKEN", "smtp.password", "key-1", "a"}
	for _, key := range valid {
		if err := validateApplicationSecretKey(key); err != nil {
			t.Fatalf("expected %q to be valid: %v", key, err)
		}
	}

	invalid := []string{"", "-bad", ".bad", "nested/key", "bad key", applicationSecretSentinel, "__baseharbor_probe", strings.Repeat("a", 129)}
	for _, key := range invalid {
		if err := validateApplicationSecretKey(key); err == nil {
			t.Fatalf("expected %q to be invalid", key)
		}
	}
}

func TestParseApplicationSecretKeyListFiltersSentinelAndSorts(t *testing.T) {
	keys, err := parseApplicationSecretKeyList(`["zeta","_baseharbor","Alpha","middle"]`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Alpha", "middle", "zeta"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("unexpected keys: got %#v want %#v", keys, want)
	}
}

func TestParseApplicationSecretKeyListRejectsUnexpectedNamespaceEntries(t *testing.T) {
	for _, input := range []string{
		`{"data":{"keys":["alpha"]}}`,
		`["nested/"]`,
		`["bad key"]`,
	} {
		if _, err := parseApplicationSecretKeyList(input); err == nil {
			t.Fatalf("expected input %s to be rejected", input)
		}
	}
}

func TestApplicationSecretKeyPathIsScoped(t *testing.T) {
	identity := ApplicationIdentity{Name: "mailflow", Environment: "prod"}
	if got := applicationSecretKeyPath(identity, "API_TOKEN"); got != "apps/mailflow/prod/API_TOKEN" {
		t.Fatalf("unexpected secret path %q", got)
	}
}

func TestExecWithTokenInputKeepsTokenAndSecretOutOfArguments(t *testing.T) {
	executor := &secretInputExecutor{}
	files := bhruntime.Files{Compose: "compose.yaml", Env: "runtime.env"}
	secret := []byte("super-sensitive-value\nwith-second-line")
	if _, err := execWithTokenInput(context.Background(), executor, files, "application-token", "exec bao kv put -mount=baseharbor apps/demo/dev/API_TOKEN value=-", secret); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(executor.args, " ")
	for _, sensitive := range []string{"application-token", "super-sensitive-value", "with-second-line"} {
		if strings.Contains(joined, sensitive) {
			t.Fatalf("sensitive value %q leaked into command arguments %q", sensitive, joined)
		}
	}
	wantInput := append([]byte("application-token\n"), secret...)
	if !bytes.Equal(executor.input, wantInput) {
		t.Fatal("sensitive stdin payload was changed before reaching the runtime boundary")
	}
}
