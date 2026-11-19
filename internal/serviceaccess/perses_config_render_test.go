package serviceaccess

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRenderPersesConfigIsDeterministicAndCredentialFree(t *testing.T) {
	policy := capability.DefaultPersesSecurityPolicy()
	first, err := RenderPersesConfig(policy)
	if err != nil {
		t.Fatalf("the validated posture must render: %v", err)
	}
	second, err := RenderPersesConfig(policy)
	if err != nil || first != second {
		t.Fatalf("the rendering must be deterministic: %v", err)
	}
	for _, want := range []string{
		"enable_auth: true",
		"encryption_key_file: " + strconvQuote(PersesEncryptionKeyContainerPath),
		"deny_private_networks: true",
		"disable_local: true",
		"enable_native: false",
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("the rendering must state %q:\n%s", want, first)
		}
	}
	for _, forbidden := range []string{"encryption_key: ", "password", "token", "secret:"} {
		if strings.Contains(strings.ToLower(first), forbidden) {
			t.Fatalf("the rendering must never inline credential material (%q):\n%s", forbidden, first)
		}
	}
}

func TestRenderPersesConfigFailsClosedForInvalidPosture(t *testing.T) {
	anonymous := capability.DefaultPersesSecurityPolicy()
	anonymous.AuthenticationEnabled = false
	if _, err := RenderPersesConfig(anonymous); err == nil {
		t.Fatal("an anonymous posture must have no rendering")
	}
	keyless := capability.DefaultPersesSecurityPolicy()
	keyless.EncryptionKeyDeclared = false
	if _, err := RenderPersesConfig(keyless); err == nil {
		t.Fatal("a keyless posture must have no rendering")
	}
}

func TestPersesComposeServicesProvenanceFailClosed(t *testing.T) {
	plan, err := BuildPersesGateway(persesRequirements(AuthenticationMTLS))
	if err != nil {
		t.Fatalf("build the plan: %v", err)
	}
	files := HTTPGatewayFiles{
		Dir:       t.TempDir(),
		Caddyfile: filepath.Join(t.TempDir(), "config", "Caddyfile"),
		Material:  TLSMaterial{ServerName: "perses-ui-access"},
	}
	ctx := context.Background()

	// No registered renderer: nothing may render, so the deployment cannot
	// silently serve the upstream anonymous default.
	previous := persesConfigRenderer
	SetPersesConfigRenderer(nil)
	if _, err := PersesComposeServices(ctx, plan, files, capability.PersesDefaultRelease); err == nil {
		t.Fatal("rendering without a configuration producer must fail closed")
	}

	dir := t.TempDir()
	hostPath := filepath.Join(dir, "config.yaml")
	keyPath := filepath.Join(dir, "secrets", "encryption-key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, make([]byte, 32), 0o644); err != nil {
		t.Fatal(err)
	}
	plan.Policy.EncryptionKeyFile = keyPath
	SetPersesConfigRenderer(func(_ context.Context, current PersesGatewayPlan) (PersesRenderedConfig, error) {
		contents, err := RenderPersesConfig(current.Policy)
		if err != nil {
			return PersesRenderedConfig{}, err
		}
		if err := os.WriteFile(hostPath, []byte(contents), 0o444); err != nil {
			return PersesRenderedConfig{}, err
		}
		return PersesRenderedConfig{HostPath: hostPath, Contents: contents}, nil
	})
	defer SetPersesConfigRenderer(previous)

	rendered, err := PersesComposeServices(ctx, plan, files, capability.PersesDefaultRelease)
	if err != nil {
		t.Fatalf("the services must render with a registered renderer: %v", err)
	}
	if !strings.Contains(rendered, strconvQuote(composeBindSource(filepath.Dir(hostPath))+":"+PersesConfigMountDir+":ro")) {
		t.Fatalf("the rendering must mount the materialized configuration read-only:\n%s", rendered)
	}
	if !strings.Contains(rendered, "--config="+PersesConfigContainerPath) {
		t.Fatalf("the command must name the mounted configuration path:\n%s", rendered)
	}

	// Drift on the mounted file fails the deployment closed: a realization
	// that rewrites the configuration with a shell (or loses it) must never
	// reach a rendered service definition.
	drifted := func(_ context.Context, current PersesGatewayPlan) (PersesRenderedConfig, error) {
		contents, err := RenderPersesConfig(current.Policy)
		if err != nil {
			return PersesRenderedConfig{}, err
		}
		return PersesRenderedConfig{HostPath: hostPath, Contents: contents}, nil
	}
	SetPersesConfigRenderer(func(ctx context.Context, current PersesGatewayPlan) (PersesRenderedConfig, error) {
		out, err := drifted(ctx, current)
		if err != nil {
			return out, err
		}
		if writeErr := os.WriteFile(hostPath, []byte("security:\n  enable_auth: false\n"), 0o600); writeErr != nil {
			return PersesRenderedConfig{}, writeErr
		}
		return out, nil
	})
	if _, err := PersesComposeServices(ctx, plan, files, capability.PersesDefaultRelease); err == nil {
		t.Fatal("a drifted configuration file must never render")
	}
	if err := os.Remove(hostPath); err != nil {
		t.Fatalf("remove the file: %v", err)
	}
	if _, err := PersesComposeServices(ctx, plan, files, capability.PersesDefaultRelease); err == nil {
		t.Fatal("an unmaterialized configuration path must never render")
	}
	// A renderer that declares a path it never wrote fails closed as well.
	SetPersesConfigRenderer(func(_ context.Context, current PersesGatewayPlan) (PersesRenderedConfig, error) {
		contents, err := RenderPersesConfig(current.Policy)
		return PersesRenderedConfig{HostPath: filepath.Join(dir, "absent.yaml"), Contents: contents}, err
	})
	if _, err := PersesComposeServices(ctx, plan, files, capability.PersesDefaultRelease); err == nil {
		t.Fatal("a renderer whose declared path is not materialized must fail closed")
	}
}

func strconvQuote(value string) string { return `"` + value + `"` }
