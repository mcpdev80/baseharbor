package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// runWithIOAndOperator mirrors runWithIO but injects a verified operator
// principal (and a read-granting membership provider), so human-only reveal
// paths (which now require an authenticated operator) can be exercised in tests.
func runWithIOAndOperator(ctx context.Context, args []string, out, errOut io.Writer, principal *identity.Principal) error {
	_ = includeInstalledORASOnPath()
	filtered, opts, _, target, err := extractGlobalOutputOptions(args)
	if err != nil {
		return err
	}
	ctx = withTargetOverride(ctx, target)
	ctx = cli.WithOutputOptions(ctx, opts)
	ctx = operatorauth.WithVerifiedTestOperatorPrincipalForced(ctx, principal)
	ctx = operatorauth.WithInteractive(ctx, os.Stdin, out, errOut)
	ctx = localCLIProviderConsentContext(ctx, filtered)
	ctx = withLocalInvocation(ctx, "cli")
	if operation := cliLocalWorkloadOperation(filtered); operation != "" {
		ctx = localWorkloadScope(ctx, operation)
	}
	return rootCommand().Execute(ctx, filtered, out, errOut)
}

// The verified-operator fixture composition lives on exactly one path now
// (card 0428-231): operatorauth.WithVerifiedTestOperator and
// operatorauth.ReadGrantingMembershipProvider in internal/operatorauth/
// test_operator.go. runWithIOAndOperator above keeps its own explicit
// principal because it takes the principal as an argument and layers the
// interactive surface on top.

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
	if err := runWithIOAsTestOperator(context.Background(), []string{"app", "env", "demo"}, &out, &out); err != nil {
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
	// Without an authenticated operator the reveal must fail closed now (human-
	// authorization seam): a bare runtime/trusted-local identity cannot reveal.
	if err := authorizeHumanReveal(context.Background(), "test-target", "dev", "demo"); err == nil {
		t.Fatalf("reveal without an authenticated operator should fail closed: %s", out.String())
	} else if !strings.Contains(err.Error(), "authenticated BaseHarbor operator") {
		t.Fatalf("reveal denial was not the expected operator-authentication failure: %v", err)
	}

	// With a verified operator the reveal returns the native service URLs.
	operator := operatorauth.TestPrincipal("https://issuer.example", "operator-reveal")
	out.Reset()
	if err := runWithIOAndOperator(context.Background(), []string{"app", "env", "demo", "--reveal", "--format", "json"}, &out, &out, operator); err != nil {
		t.Fatalf("explicit reveal with a verified operator failed: %v", err)
	}
	if !strings.Contains(out.String(), "postgresql://") || !strings.Contains(out.String(), "rediss://") {
		t.Fatalf("explicit reveal did not return native service URLs: %s", out.String())
	}

	out.Reset()
	if err := runWithIOAsTestOperator(context.Background(), []string{"app", "env", "demo", "--path"}, &out, &out); err != nil {
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

func TestHumanRevealRequiresAuthorizedMembership(t *testing.T) {
	for _, tc := range []struct {
		name        string
		group       string
		memberships []tenancy.Membership
		wantCause   string
	}{
		{name: "no membership", group: "visitor", wantCause: "claim_membership_unresolved"},
		{name: "membership without permission", group: "guest", memberships: []tenancy.Membership{{TenantID: "tenant-a", Role: "guest"}}, wantCause: "tenant_permission_denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			principal := operatorauth.TestPrincipal("https://issuer.example", "operator-without-reveal-access")
			principal.Groups = []string{tc.group}
			ctx := operatorauth.WithVerifiedTestOperatorPrincipal(context.Background(), principal)
			ctx = operatorauth.WithMembershipProvider(ctx, &firstRunMembershipProvider{memberships: tc.memberships})

			err := authorizeHumanReveal(ctx, "target-a", "dev", "application-a")
			if err == nil {
				t.Fatal("verified operator without reveal permission revealed application secrets")
			}
			if got := machine.Classify(err).CauseCode; got != tc.wantCause {
				t.Fatalf("reveal denial cause = %q, want %q: %v", got, tc.wantCause, err)
			}
		})
	}
}

// TestAppCredsRevealRequiresVerifiedOperator proves the second human-reveal seam:
// `baha app creds --reveal` also fails closed without an authenticated operator
// and succeeds with a verified operator, mirroring the app env path.
func TestAppCredsRevealRequiresVerifiedOperator(t *testing.T) {
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

	// Ensure a postgres binding with a password is present.
	data, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "DATABASE_URL=") == false {
		t.Skip("postgres binding not materialized in this runtime fixture")
	}

	operator := operatorauth.TestPrincipal("https://issuer.example", "operator-creds")

	out := bytes.Buffer{}
	// Without an operator the creds reveal must fail closed.
	if err := runWithIO(context.Background(), []string{"app", "creds", "postgres", "--app", "demo", "--reveal"}, &out, &out); err == nil {
		t.Fatalf("creds reveal without an operator should fail closed: %s", out.String())
	} else if !strings.Contains(err.Error(), "operator authentication") && !strings.Contains(err.Error(), "authenticated BaseHarbor operator") {
		t.Fatalf("creds reveal was denied for an unexpected reason: %v", err)
	}

	// With a verified operator the creds reveal returns the password.
	out.Reset()
	if err := runWithIOAndOperator(context.Background(), []string{"app", "creds", "postgres", "--app", "demo", "--reveal"}, &out, &out, operator); err != nil {
		t.Fatalf("creds reveal with a verified operator failed: %v", err)
	}
	if !strings.Contains(out.String(), "PASSWORD=") {
		t.Fatalf("creds reveal with a verified operator did not return a password line: %s", out.String())
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
