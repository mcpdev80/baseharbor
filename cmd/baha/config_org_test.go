package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

const organizationParityYAML = `apiVersion: baseharbor.organization/v1
organization: parity
targets:
  dev:
    reference: company-dev
providers:
  company-postgres:
    reference: external-provider:company-postgres
policies:
  security:
    reference: policy:company-security
defaults:
  target: dev
  providers:
    database.sql:
      provider: company-postgres
      scope: external
  policies:
    - policy: security
      mandatory: true
`

func TestOrganizationCLIJSONAndMCPShareEffectiveResolution(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	source := filepath.Join(root, "org")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "organization.yaml"), []byte(organizationParityYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{
		"config", "organization", "set",
		"--source", "local",
		"--location", source,
		"--environment", "dev",
		"-o", "json",
	}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var cli organizationView
	if err := json.Unmarshal(out.Bytes(), &cli); err != nil {
		t.Fatal(err)
	}
	assertOrganizationParityEffective(t, cli.Effective)

	server := newMCPServer(application.DefaultStore())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "org-parity-test", Version: "v1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "baseharbor.organization.inspect",
		Arguments: map[string]any{"environment": "dev"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("organization inspect returned error: %#v", result.Content)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var machine organizationView
	if err := json.Unmarshal(encoded, &machine); err != nil {
		t.Fatalf("decode MCP organization view: %v: %s", err, encoded)
	}
	assertOrganizationParityEffective(t, machine.Effective)

	if cli.Effective.Target.Value != machine.Effective.Target.Value ||
		cli.Effective.Providers["database.sql"] != machine.Effective.Providers["database.sql"] ||
		cli.State.Resolution.ResolvedDigest != machine.State.Resolution.ResolvedDigest {
		t.Fatalf("CLI/MCP organization semantics diverged: cli=%+v mcp=%+v", cli, machine)
	}
}

func assertOrganizationParityEffective(t *testing.T, effective orgconfig.Effective) {
	t.Helper()
	if effective.Organization != "parity" || effective.Environment != "dev" {
		t.Fatalf("unexpected effective organization: %+v", effective)
	}
	if effective.Target == nil || effective.Target.Name != "dev" || effective.Target.Value != "company-dev" {
		t.Fatalf("unexpected target: %+v", effective.Target)
	}
	provider := effective.Providers["database.sql"]
	if provider.Provider != "company-postgres" ||
		provider.Reference != "external-provider:company-postgres" ||
		provider.Scope != "external" ||
		provider.Source != "organization.defaults" {
		t.Fatalf("unexpected provider default: %+v", provider)
	}
	if len(effective.Policies) != 1 || effective.Policies[0].Policy != "security" ||
		effective.Policies[0].Reference != "policy:company-security" ||
		!effective.Policies[0].Mandatory ||
		effective.Policies[0].Source != "organization.defaults" {
		t.Fatalf("unexpected mandatory policy reference: %+v", effective.Policies)
	}
}

func TestOrganizationCheckDoesNotReplaceAndUpdateRequiresApproval(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	source := filepath.Join(root, "organization.yaml")
	write := func(name string) {
		t.Helper()
		body := bytes.ReplaceAll([]byte(organizationParityYAML), []byte("organization: parity"), []byte("organization: "+name))
		if err := os.WriteFile(source, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("parity")

	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{
		"config", "organization", "set", "--source", "local", "--location", source, "-o", "json",
	}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	write("parity-next")

	out.Reset()
	if err := runWithIO(context.Background(), []string{"config", "organization", "check", "-o", "json"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var check organizationCheckView
	if err := json.Unmarshal(out.Bytes(), &check); err != nil {
		t.Fatal(err)
	}
	if !check.Status.Changed || check.Available.Organization != "parity-next" {
		t.Fatalf("check did not report new immutable config: %+v", check)
	}

	active, err := orgconfig.LoadActive()
	if err != nil {
		t.Fatal(err)
	}
	if active.Config.Organization != "parity" {
		t.Fatalf("check silently replaced active config: %+v", active.Config)
	}

	if err := runWithIO(context.Background(), []string{"config", "organization", "update"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("organization update must require explicit --yes")
	}
	out.Reset()
	if err := runWithIO(context.Background(), []string{"config", "organization", "update", "--yes", "-o", "json"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	active, err = orgconfig.LoadActive()
	if err != nil {
		t.Fatal(err)
	}
	if active.Config.Organization != "parity-next" {
		t.Fatalf("explicit update did not activate resolved config: %+v", active.Config)
	}
}
