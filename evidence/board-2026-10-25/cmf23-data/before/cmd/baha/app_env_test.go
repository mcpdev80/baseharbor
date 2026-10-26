package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestAppEnvMasksSecretsByDefaultAndRevealsExplicitly(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	t.Setenv(application.ProviderScopeEnv(capability.ProviderValkey), string(capability.ScopeApplication))
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	m := application.New("demo", "dev", true, true, false)
	store := testDeploymentStore(t, target, m)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}
	registerTestDeployment(t, target, m, "", "")
	files, err := application.EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}

	private := `AMQP_URL=amqps://alice:NEVER_EXPOSE@host/
RABBITMQ_ORDERS_URL=amqps://alice:NEVER_EXPOSE@host/
MONGODB_URL=mongodb://alice:NEVER_EXPOSE@host/
MONGO_URL=mongodb://alice:NEVER_EXPOSE@host/
CUSTOM_PROVIDER_CREDENTIAL=NEVER_EXPOSE
TLS_PRIVATE_KEY=NEVER_EXPOSE
`
	data, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.ApplicationEnv, append(data, []byte(private)...), 0600); err != nil {
		t.Fatal(err)
	}
	session := workspaceMutationClient(t)
	response, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app.environment", Arguments: map[string]any{"name": "demo"}})
	if err != nil || response.IsError {
		t.Fatalf("environment query failed: %#v %v", response, err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("NEVER_EXPOSE")) || !bytes.Contains(encoded, []byte("masked")) {
		t.Fatalf("MCP leaked provider credential: %s", encoded)
	}

	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "env", "demo"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if strings.Contains(text, "NEVER_EXPOSE") {
		t.Fatal("CLI default leaked provider credential")
	}
	if !strings.Contains(text, "DATABASE_URL=<masked>") || !strings.Contains(text, "REDIS_URL=<masked>") {
		t.Fatalf("default output did not mask service credentials: %s", text)
	}
	if strings.Contains(text, "postgresql://") || strings.Contains(text, "redis://") || strings.Contains(text, "rediss://") {
		t.Fatalf("default output leaked service URLs: %s", text)
	}

	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "env", "demo", "--reveal", "--format", "json"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "postgresql://") || !strings.Contains(out.String(), "rediss://") {
		t.Fatalf("explicit reveal did not return native service URLs: %s", out.String())
	}

	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "env", "demo", "--path"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	expectedPath, err := filepath.Abs(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != expectedPath {
		t.Fatalf("unexpected application env path: %q want %q", strings.TrimSpace(out.String()), expectedPath)
	}
}

func TestParseAppEnvArgsRejectsUnsafeCombination(t *testing.T) {
	if _, _, _, _, err := parseAppEnvArgs([]string{"demo", "--path", "--reveal"}); err == nil {
		t.Fatal("expected --path with --reveal to fail")
	}
	if _, _, _, _, err := parseAppEnvArgs([]string{"demo", "--format", "toml"}); err == nil {
		t.Fatal("expected unsupported format to fail")
	}
}
