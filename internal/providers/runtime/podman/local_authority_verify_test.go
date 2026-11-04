package podman

import (
	"context"
	"os"
	"testing"
	"time"
)

// verifyLocalAuthorityInfo must fail closed on malformed JSON — an opaque engine
// cannot be trusted.
func TestVerifyLocalAuthorityRejectsMalformedJSON(t *testing.T) {
	if err := verifyLocalAuthorityInfo([]byte("not json at all")); err == nil {
		t.Fatal("malformed podman info: expected a closed error, got nil")
	}
}

// a payload larger than the 1 MiB guard fails closed — oversized responses are
// not accepted.
func TestVerifyLocalAuthorityRejectsOversized(t *testing.T) {
	big := make([]byte, 1<<20+1)
	for i := range big {
		big[i] = 'x'
	}
	if err := verifyLocalAuthorityInfo(big); err == nil {
		t.Fatal("oversized podman info: expected a closed error, got nil")
	}
}

// the OS must be linux — a non-linux host is not a verified runtime authority.
func TestVerifyLocalAuthorityRequiresLinux(t *testing.T) {
	info := `{"host":{"os":"macos","serviceIsRemote":false,"security":{"rootless":true}}}`
	if err := verifyLocalAuthorityInfo([]byte(info)); err == nil {
		t.Fatal("non-linux os: expected a closed error, got nil")
	}
}

// a remote service is not a local authority — fail closed.
func TestVerifyLocalAuthorityRejectsRemoteService(t *testing.T) {
	info := `{"host":{"os":"linux","serviceIsRemote":true,"security":{"rootless":true}}}`
	if err := verifyLocalAuthorityInfo([]byte(info)); err == nil {
		t.Fatal("remote podman service: expected a closed error, got nil")
	}
}

// a root (non-rootless) engine is not a verified local rootless authority.
func TestVerifyLocalAuthorityRejectsRootEngine(t *testing.T) {
	info := `{"host":{"os":"linux","serviceIsRemote":false,"security":{"rootless":false}}}`
	if err := verifyLocalAuthorityInfo([]byte(info)); err == nil {
		t.Fatal("root podman engine: expected a closed error, got nil")
	}
}

// the happy path: a verified native rootless Linux podman authority passes.
func TestVerifyLocalAuthorityAcceptsRootlessLinux(t *testing.T) {
	info := `{"host":{"os":"linux","serviceIsRemote":false,"security":{"rootless":true}}}`
	if err := verifyLocalAuthorityInfo([]byte(info)); err != nil {
		t.Fatalf("verified rootless linux authority: unexpected error: %v", err)
	}
}

// a missing serviceIsRemote field fails closed — the authority is ambiguous.
func TestVerifyLocalAuthorityRequiresRemoteFlag(t *testing.T) {
	info := `{"host":{"os":"linux","security":{"rootless":true}}}`
	if err := verifyLocalAuthorityInfo([]byte(info)); err == nil {
		t.Fatal("missing serviceIsRemote: expected a closed error, got nil")
	}
}

// a missing rootless field fails closed.
func TestVerifyLocalAuthorityRequiresRootlessFlag(t *testing.T) {
	info := `{"host":{"os":"linux","serviceIsRemote":false}}}`
	if err := verifyLocalAuthorityInfo([]byte(info)); err == nil {
		t.Fatal("missing rootless: expected a closed error, got nil")
	}
}

// VerifyLocalRuntimeAuthority returns a closed error when podman is not found
// on PATH — no runtime, no local authority claim. The PATH override is restored
// after this test so it cannot leak into sibling tests in the package.
func TestVerifyLocalRuntimeAuthorityClosesWithoutPodman(t *testing.T) {
	original := os.Getenv("PATH")
	defer os.Setenv("PATH", original)
	t.Setenv("PATH", "/nonexistent-empty-path")

	if err := (Provider{}).VerifyLocalRuntimeAuthority(context.Background()); err == nil {
		t.Fatal("podman absent: expected a closed error, got nil")
	}
}

// a context deadline is honored and surfaces as a closed error.
func TestVerifyLocalRuntimeAuthorityHonorsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	if err := (Provider{}).VerifyLocalRuntimeAuthority(ctx); err == nil {
		t.Fatal("expired context: expected a closed error, got nil")
	}
}
