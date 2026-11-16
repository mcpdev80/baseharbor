package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestSelectAccessInstance(t *testing.T) {
	got, err := selectAccessInstance([]string{"primary"}, "", "postgres")
	if err != nil || got != "primary" {
		t.Fatalf("single instance: got %q err=%v", got, err)
	}
	got, err = selectAccessInstance([]string{"primary", "analytics"}, "analytics", "postgres")
	if err != nil || got != "analytics" {
		t.Fatalf("named instance: got %q err=%v", got, err)
	}
	if _, err := selectAccessInstance([]string{"primary", "analytics"}, "", "postgres"); err == nil {
		t.Fatal("expected ambiguous instance selection to fail clearly")
	}
	if _, err := selectAccessInstance([]string{"primary"}, "missing", "postgres"); err == nil {
		t.Fatal("expected unknown instance to fail")
	}
}

func TestParseAccessTarget(t *testing.T) {
	appName, instance, err := parseAccessTarget([]string{"analytics", "--app", "mailflow"}, "psql")
	if err != nil {
		t.Fatal(err)
	}
	if appName != "mailflow" || instance != "analytics" {
		t.Fatalf("unexpected target %q/%q", appName, instance)
	}
}

func TestParseCredsArgsMasksByDefault(t *testing.T) {
	kind, appName, instance, reveal, err := parseCredsArgs([]string{"postgres", "primary", "--app=mailflow"})
	if err != nil {
		t.Fatal(err)
	}
	if kind != "postgres" || appName != "mailflow" || instance != "primary" || reveal {
		t.Fatalf("unexpected parse result")
	}
}

func TestParseLogsArgs(t *testing.T) {
	appName, service, follow, err := parseLogsArgs([]string{"api", "-f", "--app", "mailflow"})
	if err != nil {
		t.Fatal(err)
	}
	if appName != "mailflow" || service != "api" || !follow {
		t.Fatalf("unexpected log target")
	}
}

// TestAppCredsRevealWritesPerRevealAuditEvent pins the access-wizard
// fail-closed fix: the human `baha app creds --reveal` path must (a) keep the
// value masked by default, (b) still print the real value on explicit
// `--reveal`, (c) write a per-reveal audit event (operation
// "connection-reveal") to the target-state store, and (d) never leak the
// secret through the JSON path. It mirrors the app_env fixture (serviceissuer)
// and reads the target-state audit store the same way recordApplicationAudit
// writes it.
func TestAppCredsRevealWritesPerRevealAuditEvent(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	m := application.New("demo", "dev", true, false, false)
	store := testDeploymentStore(t, target, m)
	if _, err := store.Create(m); err != nil {
		t.Fatal(err)
	}
	registerTestDeployment(t, target, m, "", "")
	_, err = application.EnsureRuntime(context.Background(), serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}

	targetStateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("target state root: %v", err)
	}

	// Default: masked, no secret in output.
	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "creds", "postgres", "default", "--app", "demo"}, &out, &out); err != nil {
		t.Fatalf("creds default failed: %v", err)
	}
	if text := out.String(); strings.Contains(text, "postgresql://") || strings.Contains(text, "PASSWORD=<masked>") == false {
		t.Fatalf("default creds output must mask the password: %q", text)
	}

	// Explicit human reveal still prints the real value AND records an audit
	// event with operation "connection-reveal".
	out.Reset()
	if err := runWithIO(context.Background(), []string{"app", "creds", "postgres", "default", "--app", "demo", "--reveal"}, &out, &out); err != nil {
		t.Fatalf("creds --reveal failed: %v", err)
	}
	if !strings.Contains(out.String(), "PASSWORD=") || !strings.Contains(out.String(), "URI=") {
		t.Fatalf("explicit reveal must print the connection values: %q", out.String())
	}
	events, err := loadTargetAuditEvents(targetStateRoot)
	if err != nil {
		t.Fatalf("target audit store unreadable: %v", err)
	}
	if !containsAuditOperation(events, "connection-reveal") {
		t.Fatalf("no per-reveal connection-reveal audit event recorded: %+v", events)
	}

	// JSON --reveal stays blocked (contract §7).
	out.Reset()
	err = runWithIO(context.Background(), []string{"app", "creds", "postgres", "default", "--app", "demo", "--reveal", "--json"}, &out, &out)
	if err == nil {
		t.Fatal("expected JSON --reveal to be blocked")
	}
	if strings.Contains(out.String(), "PASSWORD=") {
		t.Fatalf("JSON --reveal leaked the secret: %q", out.String())
	}
}

func loadTargetAuditEvents(root string) ([]evidence.AuditEvent, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	return evidence.Load(root)
}

func containsAuditOperation(events []evidence.AuditEvent, operation string) bool {
	for _, e := range events {
		if e.Operation == operation {
			return true
		}
	}
	return false
}
