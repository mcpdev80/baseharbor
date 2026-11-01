package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// insecureSSLModeTokens are PostgreSQL TLS settings that downgrade or remove
// encryption on the openbao<->postgres path. They must never appear, neither in
// the Go source (DSN builder / HCL) nor in the generated HCL / init-script URLs.
// verify-ca and verify-full are the only sanctioned modes (verify-ca is used
// because HAProxy takes the server hostname, so verify-full is intentionally not
// used here).
var insecureSSLModeTokens = []string{
	"sslmode=off",
	"sslmode=trust",
	"sslmode=verify-none",
	"PGSSLMODE=off",
	"PGSSLMODE=trust",
	"PGSSLMODE=disable",
	"PGSSLMODE=verify-none",
}

// TestOpenBaoSSLModeGuard is a regression guard: it fails if any insecure
// PostgreSQL TLS mode is ever reintroduced on the openbao<->postgres path,
// while explicitly allowing the sanctioned verify-ca / verify-full modes.
//
// It runs in isolation as a pure source-text / DSN check with no Docker daemon
// dependency, so it never blocks on the pending endpoint fix.
func TestOpenBaoSSLModeGuard(t *testing.T) {
	srcDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Read every Go source file in this package so the DSN/HCL builder and the
	// init-script templates embedded in openbao_runtime_prepare.go are covered.
	files, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, f := range files {
		name := f.Name()
		// Skip this guard's own source: the forbidden tokens are intentionally
		// listed as string literals here, which would otherwise trip the check.
		if strings.EqualFold(name, "openbao_sslmode_guard_test.go") {
			continue
		}
		if strings.HasSuffix(name, ".go") {
			data, err := os.ReadFile(filepath.Join(srcDir, name))
			if err != nil {
				t.Fatal(err)
			}
			sources = append(sources, string(data))
		}
	}
	if len(sources) == 0 {
		t.Fatal("no Go sources read for SSL mode guard")
	}
	combined := strings.Join(sources, "\n")

	// Forbidden modes must never appear anywhere in the package sources.
	for _, tok := range insecureSSLModeTokens {
		if strings.Contains(combined, tok) {
			t.Fatalf("insecure PostgreSQL TLS mode %q must never appear on the openbao<->postgres path:\n%s", tok, combined)
		}
	}

	// The sanctioned mode must actually be present (guards against accidental
	// removal of the secure wiring). verify-ca is used because HAProxy takes the
	// server hostname; verify-full is also acceptable if it ever appears.
	if !strings.Contains(combined, "sslmode=verify-ca") &&
		!strings.Contains(combined, "sslmode=verify-full") {
		t.Fatal("sanctioned PostgreSQL TLS mode verify-ca/verify-full is missing; openbao storage may no longer enforce TLS")
	}

	// The generated openbao-init.sh template must pin a TLS mode too.
	if !strings.Contains(combined, "PGSSLMODE=verify-ca") {
		t.Fatal("generated openbao-init.sh template must pin PGSSLMODE=verify-ca")
	}
}
