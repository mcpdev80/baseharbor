package orgconfig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeToolNotFoundIdentitySurvivesRedaction proves the CODEX round-2
// orgconfig gap: the exec.ErrNotFound identity must survive the redacting
// wrappers on this path ("%v" collapses it, a "%w: %v" detail wrapper hides it
// behind a message). Only that identity may ever qualify the fail-closed
// missing-tool classification; a tool that exists but fails for another
// reason must never claim it.
func TestNativeToolNotFoundIdentitySurvivesRedaction(t *testing.T) {
	notFound := &exec.Error{Name: "oras", Err: exec.ErrNotFound}
	chains := map[string]error{
		"raw":                 notFound,
		"percent-w-redaction": fmt.Errorf("%w: executable file not found in $PATH", notFound),
		"joined":              fmt.Errorf("context: %w", errors.Join(errors.New("oras: no stderr"), notFound)),
	}
	for name, err := range chains {
		t.Run(name, func(t *testing.T) {
			if !IsNativeToolNotFound(err) {
				t.Fatalf("%s: the fail-closed path must recognise the missing-tool identity, got %v", name, err)
			}
			if classified := classifyNativeToolFailure("oras", err); !errors.Is(classified, ErrNativeToolNotFound) {
				t.Fatalf("%s: classification must yield the actionable missing-oras identity, got %v", name, classified)
			}
		})
	}
	t.Run("other-tool-failure-never-claims-it", func(t *testing.T) {
		other := errors.New("oras: exit status 1")
		if IsNativeToolNotFound(other) {
			t.Fatal("a failing-but-present tool must not claim the missing identity")
		}
		classified := classifyNativeToolFailure("oras", other)
		if errors.Is(classified, ErrNativeToolNotFound) {
			t.Fatalf("a generic tool failure must land in the merely-unavailable class, got %v", classified)
		}
		if !errors.Is(classified, ErrSourceUnavailable) {
			t.Fatalf("a generic tool failure must stay in the unavailable class, got %v", classified)
		}
		if strings.Contains(classified.Error(), "exit status 1") {
			t.Fatalf("the raw failure text must stay redacted: %v", classified)
		}
	})
}

// TestUnverifiableOCIDigestReplyIsHardRejection proves an OCI org source the
// tool resolved UNPRUEFBAR — an answer without an immutable sha256 digest —
// carries the distinct hard-rejection identity so no variable (mutable)
// fallback can ever qualify on it, even with the endpoint opt-in on.
func TestUnverifiableOCIDigestReplyIsHardRejection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv(OrgRegistryOptInEnv, "true")
	// The fake answers "resolve" with a NON-digest reply (its resolveErr slot
	// is a stderr echo plus exit 1; a successful non-digest stdout never
	// reaches the pull because the digest gate fails first). To produce the
	// UNPRUEFBAR shape, replace the generated script's resolve branch output
	// with a plain tag reference and exit 0.
	_ = installFakeOras(t, root, "", "", "")
	// Make the fake's "resolve" answer a successful plain-tag reply (exit 0,
	// no digest): the tool ran and the reply is UNPRUEFBAR.
	orasPath := filepath.Join(root, "bin", "oras")
	script, err := os.ReadFile(orasPath)
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(string(script),
		"    echo 'sha256:"+strings.Repeat("c", 64)+"' ;;",
		"    echo 'registry.example.invalid/platform/baseharbor-org:stable' ;;", 1)
	if patched == string(script) {
		t.Fatal("fake oras script did not contain the expected digest reply")
	}
	if err := os.WriteFile(orasPath, []byte(patched), 0o700); err != nil {
		t.Fatal(err)
	}

	_, _, err = Resolve(context.Background(), Source{
		Kind: SourceOCI, Location: "registry.example.invalid/platform/baseharbor-org", Requested: "stable",
	})
	if err == nil {
		t.Fatal("an unverifiable resolution must fail closed")
	}
	if !IsUnverifiableOCIResolution(err) {
		t.Fatalf("the unverifiable identity must survive to the caller, got %v", err)
	}
	if !isHardRegistryRejection(err) {
		t.Fatalf("an unverifiable resolution is a hard rejection, never the class a fallback covers, got %v", err)
	}
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("the identity must keep unwrapping to the unavailable class for caller classification, got %v", err)
	}
}
