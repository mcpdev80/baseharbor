package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

// TestServeTrustsTargetManagedOperatorCA is the positive regression: the managed
// CA the Core bootstrap wrote into the selected Target state root must reach the
// operator API verifier without any OS system-trust change.
func TestServeTrustsTargetManagedOperatorCA(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	t.Setenv(operatorauth.EnvTrustFile, "")
	caPath := writeManagedOperatorCAAt(t, filepath.Join(root, "targets", "vm-core", "trust"))
	got, err := resolveServeOperatorOIDCTrustFile(context.Background(), controlplaneruntime.Config{
		ConnectorAuthorityTarget: "vm-core",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != caPath {
		t.Fatalf("expected the owned Target managed CA %s, got %q", caPath, got)
	}
}

// TestServePrefersExplicitOperatorCAFile pins the documented precedence: an
// explicit BASEHARBOR_OPERATOR_OIDC_CA_FILE wins over state material.
func TestServePrefersExplicitOperatorCAFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	stateCA := writeManagedOperatorCAAt(t, filepath.Join(root, "targets", "vm-core", "trust"))
	explicit := filepath.Join(t.TempDir(), "explicit-ca.pem")
	if _, err := os.Stat(stateCA); err != nil {
		t.Fatal(err)
	}
	// Mint a distinct CA for the explicit path so precedence is observable.
	explicitCA, _ := writeTestCA(t, explicit)
	if explicitCA == nil {
		t.Fatal("explicit CA mint failed")
	}
	t.Setenv(operatorauth.EnvTrustFile, explicit)
	got, err := resolveServeOperatorOIDCTrustFile(context.Background(), controlplaneruntime.Config{
		ConnectorAuthorityTarget: "vm-core",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != explicit {
		t.Fatalf("explicit trust file must win, got %q", got)
	}
}

// TestServeRejectsUnusableExplicitOperatorCA keeps the explicit override
// fail-closed: a missing file or non-certificate content is an error, never a
// silent fallback to another trust source.
func TestServeRejectsUnusableExplicitOperatorCA(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	writeManagedOperatorCAAt(t, filepath.Join(root, "targets", "vm-core", "trust"))

	for _, tc := range []struct{ name, kind string }{
		{"missing", "missing"},
		{"not a certificate", "garbage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ca.pem")
			if tc.kind == "garbage" {
				if err := os.WriteFile(path, []byte("not a pem certificate\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv(operatorauth.EnvTrustFile, path)
			if _, err := resolveServeOperatorOIDCTrustFile(context.Background(), controlplaneruntime.Config{
				ConnectorAuthorityTarget: "vm-core",
			}); err == nil {
				t.Fatal("an unusable explicit operator CA must fail closed")
			}
		})
	}
}

// TestServeKeepsSystemTrustWithoutManagedCA proves the unchanged default: when no
// managed CA material exists, resolution returns no file so verification stays on
// the system trust pool.
func TestServeKeepsSystemTrustWithoutManagedCA(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	t.Setenv(operatorauth.EnvTrustFile, "")
	got, err := resolveServeOperatorOIDCTrustFile(context.Background(), controlplaneruntime.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("no managed CA must preserve system-only trust, got %q", got)
	}
}

// TestServeStateWideManagedOperatorCAFallback covers an installation state layout
// without a Target-scoped CA file.
func TestServeStateWideManagedOperatorCAFallback(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	t.Setenv(operatorauth.EnvTrustFile, "")
	want, err := filepath.Abs(filepath.Join(root, operatorauth.DefaultManagedCAFile))
	if err != nil {
		t.Fatal(err)
	}
	writeManagedOperatorCAAt(t, filepath.Join(root, "trust"))
	got, err := resolveServeOperatorOIDCTrustFile(context.Background(), controlplaneruntime.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join("trust", "managed-operator-ca.pem")) || got == "" {
		t.Fatalf("expected the state-wide managed CA %s, got %q", want, got)
	}
	if _, err := deployment.TargetStateRoot("vm-core"); err != nil {
		t.Fatal(err)
	}
}

func writeManagedOperatorCAAt(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "managed-operator-ca.pem")
	writeTestCA(t, path)
	return path
}
