package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestUnverifiableResolveNeverQualifiesMutableFallback proves the CODEX
// round-2 gap: an UNPRUEFBAR (unverifiable) OCI source image — the native tool
// answered but carried no immutable sha256 digest — must never qualify the
// variable (mutable) fallback, not even with the development opt-in on.
// Pulling the raw tag would fetch content that was never proven; the correct
// shape of this host (witnessed on baha-docker: oras absent) is exactly the
// missing-tool fail-closed, never a raw `docker pull` of the edge tag.
func TestUnverifiableResolveNeverQualifiesMutableFallback(t *testing.T) {
	cases := []struct {
		name string
		out  string
	}{
		{name: "empty-reply", out: ""},
		{name: "no-digest-in-reply", out: "registry.example.invalid/org/app:edge"},
		{name: "reply-drops-the-pin", out: "Resolved registry.example.invalid/org/app:edge"},
		{name: "malformed-digest", out: "registry.example.invalid/org/app@sha256:tooshort"},
		// A well-formed digest that the resolved reference does not actually
		// carry: the defensive pin check must classify it unverifiable too.
		{name: "pin-mismatch", out: "registry.example.invalid/org/app@sha256:" + strings.Repeat("b", 64) + " sha256:" + strings.Repeat("a", 64)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, optIn := range []string{"", "true"} {
				t.Setenv("BASEHARBOR_ALLOW_MUTABLE_DEVELOPMENT_IMAGE", optIn)
				out := tc.out
				installNativeRunner(t, func(context.Context, string, ...string) (string, error) {
					return out, nil
				})
				compose := &callerRecordingCompose{}
				err := pullResolvedRuntimeImage(context.Background(), compose, "registry.example.invalid/org/app:edge", "edge")
				if err == nil {
					t.Fatalf("opt-in %q: an unverifiable resolution (%s) must fail closed", optIn, tc.name)
				}
				if !isHardRejection(err) {
					t.Fatalf("opt-in %q: the unverifiable identity must surface as a hard rejection for %s, got %v", optIn, tc.name, err)
				}
				if !errors.Is(err, ErrRegistrySourceUnavailable) {
					t.Fatalf("opt-in %q: the identity must unwrap to the unavailable class, got %v", optIn, err)
				}
				if resolveUnavailable(err) {
					t.Fatalf("opt-in %q: an unverifiable resolve must never be the merely-unavailable class the fallback covers", optIn)
				}
				if len(compose.pulled) != 0 {
					t.Fatalf("opt-in %q: an unverifiable resolve (%s) must cause NULL pulls, got %v", optIn, tc.name, compose.pulled)
				}
			}
		})
	}
}

// TestUnreachableResolveKeepsOptInFallbackScope proves the exclusion is
// narrower than fail-closed-everywhere: a genuinely unreachable endpoint (a
// tool error, not an unverifiable reply) stays in the merely-unavailable class
// and keeps the documented development opt-in behaviour byte-for-byte.
func TestUnreachableResolveKeepsOptInFallbackScope(t *testing.T) {
	installNativeRunner(t, func(context.Context, string, ...string) (string, error) {
		return "", errors.New("oras: dial tcp: connection refused")
	})
	t.Setenv("BASEHARBOR_ALLOW_MUTABLE_DEVELOPMENT_IMAGE", "true")
	compose := &callerRecordingCompose{}
	err := pullResolvedRuntimeImage(context.Background(), compose, "registry.example.invalid/org/app:edge", "edge")
	if err != nil {
		t.Fatalf("an unreachable endpoint with the explicit opt-in keeps the legacy fallback, got %v", err)
	}
	if isHardRejection(err) {
		t.Fatal("an unreachable endpoint is not a hard rejection")
	}
	if len(compose.pulled) != 1 || compose.pulled[0] != "registry.example.invalid/org/app:edge" {
		t.Fatalf("the opt-in fallback must pull exactly the original tag reference, got %v", compose.pulled)
	}
}

type localRuntimeImageCompose struct {
	callerRecordingCompose
	imageID   string
	inspected []string
}

func (c *localRuntimeImageCompose) DirectOutput(_ context.Context, args ...string) (string, error) {
	c.inspected = append([]string(nil), args...)
	return c.imageID, nil
}

func TestUnreachableLocalCandidateUsesPreloadedRuntimeImage(t *testing.T) {
	installNativeRunner(t, func(context.Context, string, ...string) (string, error) {
		return "", errors.New("oras: dial tcp: connection refused")
	})
	t.Setenv("BASEHARBOR_ALLOW_MUTABLE_DEVELOPMENT_IMAGE", "true")
	compose := &localRuntimeImageCompose{imageID: "sha256:local-candidate"}
	var progress []string
	err := pullResolvedRuntimeImageProgress(context.Background(), compose,
		"localhost/baseharbor-runtime:pre-release", "", func(detail string) {
			progress = append(progress, detail)
		})
	if err != nil {
		t.Fatalf("preloaded local image should be usable with the explicit opt-in: %v", err)
	}
	if len(compose.pulled) != 0 {
		t.Fatalf("preloaded local image must not be pulled from a nonexistent localhost registry: %v", compose.pulled)
	}
	wantInspect := []string{"image", "inspect", "--format", "{{.Id}}", "localhost/baseharbor-runtime:pre-release"}
	if strings.Join(compose.inspected, "\x00") != strings.Join(wantInspect, "\x00") {
		t.Fatalf("local-image inspection args = %q, want %q", compose.inspected, wantInspect)
	}
	if len(progress) != 2 || progress[1] != "using the preloaded local development runtime image" {
		t.Fatalf("local-image progress = %q", progress)
	}
}

func TestLocalImageShortcutRequiresLocalReferenceAndSuccessfulInspect(t *testing.T) {
	compose := &localRuntimeImageCompose{imageID: "sha256:local-candidate"}
	if localRuntimeImagePresent(context.Background(), compose, "registry.example.invalid/app:edge") {
		t.Fatal("remote registry reference was treated as a preloaded local image")
	}
	compose.imageID = "  "
	if localRuntimeImagePresent(context.Background(), compose, "localhost/baseharbor-runtime:pre-release") {
		t.Fatal("empty local image identity was treated as present")
	}
}

// TestIsHardRejectionCoversUnverifiableIdentity pins the classifier contract
// the fallback gates rely on: every unverifiable resolution is a hard
// rejection, and the identity survives the wrapping through Resolve.
func TestIsHardRejectionCoversUnverifiableIdentity(t *testing.T) {
	installNativeRunner(t, func(context.Context, string, ...string) (string, error) {
		return "registry.example.invalid/org/app:edge", nil
	})
	_, err := resolveRegistrySourceImpl(context.Background(), "registry.example.invalid/org/app:edge", "edge")
	if err == nil {
		t.Fatal("a reply without a digest must fail closed")
	}
	if !isHardRejection(err) {
		t.Fatalf("the unverifiable identity must surface as a hard rejection, got %v", err)
	}
	if resolveUnavailable(err) {
		t.Fatal("resolveUnavailable must exclude an unverifiable chain")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("the message must name the missing immutable digest, got %v", err)
	}
	// The alias identity must NOT be reachable by errors.Is: the sentinel
	// unwraps through the unavailable class, and every merely-unavailable
	// chain would otherwise answer true. The node-identity walk is the gate.
	notUnverifiable := fmt.Errorf("%w: %s", ErrRegistrySourceUnavailable, "connection refused")
	if isHardRejection(notUnverifiable) {
		t.Fatal("a merely-unavailable chain must not be a hard rejection")
	}

}
