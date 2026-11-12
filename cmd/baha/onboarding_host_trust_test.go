package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
)

func TestImportedTrustRequiresExplicitConsentAndPreservesConnectionOnFailure(t *testing.T) {
	configureTestTarget(t)
	ca, err := newCoreSigningTestIssuer(t).TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	old := installImportedPublicTrust
	oldInput := onboardingInput
	t.Cleanup(func() { installImportedPublicTrust = old; onboardingInput = oldInput })
	for _, tc := range []struct {
		name, input                       string
		options                           onboardTrustOptions
		noninteractive, wantInstall, fail bool
	}{
		{name: "decline", input: "n\n"}, {name: "default-no", input: "\n"}, {name: "eof"},
		{name: "skip", input: "y\n", options: onboardTrustOptions{skip: true}},
		{name: "no-input", input: "y\n", noninteractive: true},
		{name: "interactive", input: "yes\n", wantInstall: true},
		{name: "explicit", options: onboardTrustOptions{install: true}, noninteractive: true, wantInstall: true},
		{name: "failed-install", options: onboardTrustOptions{install: true}, wantInstall: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			installImportedPublicTrust = func(_ context.Context, _ string, data []byte, issuer string) (hosttrust.Status, []hosttrust.BrowserStatus, error) {
				calls++
				if string(data) != string(ca.PEM) || issuer != "core-target/dev-docker-1" {
					t.Fatal("wrong trust material")
				}
				if tc.fail {
					return hosttrust.Status{}, nil, errors.New("permissions denied")
				}
				return hosttrust.Status{Trusted: true}, []hosttrust.BrowserStatus{{State: "trusted", Database: "test-browser"}}, nil
			}
			onboardingInput = strings.NewReader(tc.input)
			ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{NonInteractive: tc.noninteractive})
			var out bytes.Buffer
			err := offerImportedHostTrust(ctx, bufio.NewReader(onboardingInput), &out, "dev-docker-1", ca.PEM, tc.options)
			if (calls == 1) != tc.wantInstall {
				t.Fatalf("unexpected trust mutation count: %d", calls)
			}
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "connection saved") || !strings.Contains(err.Error(), "do not import") || !strings.Contains(err.Error(), "--target dev-docker-1 trust install --yes") {
					t.Fatalf("bad recovery: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if tc.name == "interactive" && (!strings.Contains(out.String(), "CA SHA-256") || !strings.Contains(out.String(), "no private keys")) {
				t.Fatal("CA installation not explained")
			}
		})
	}
}

func TestOnboardImportInvalidInvitationNeverInstallsTrust(t *testing.T) {
	configureTestTarget(t)
	path := filepath.Join(t.TempDir(), "invite.json")
	if err := os.WriteFile(path, []byte(`{"contract_version":"invalid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	old := installImportedPublicTrust
	t.Cleanup(func() { installImportedPublicTrust = old })
	installImportedPublicTrust = func(context.Context, string, []byte, string) (hosttrust.Status, []hosttrust.BrowserStatus, error) {
		t.Fatal("unverified invitation installed CA")
		return hosttrust.Status{}, nil, nil
	}
	if err := runOnboardImport(context.Background(), []string{path, "--trust-host-ca"}, new(bytes.Buffer)); err == nil {
		t.Fatal("invalid invitation accepted")
	}
	if _, _, err := stripOnboardTrustOptions([]string{path, "--trust-host-ca", "--skip-host-trust"}); err == nil {
		t.Fatal("conflicting consent accepted")
	}
}

func TestRemoteTrustBundleUsesVerifiedConnectionWithoutLocalCore(t *testing.T) {
	ctx, _, _ := remoteJourneyFixture(t)
	trust, managed, err := currentManagedTrustBundle(ctx)
	if err != nil || !managed || len(trust.PEM) == 0 || !strings.HasPrefix(trust.IssuerReference, "core/") {
		t.Fatalf("remote public trust unavailable: %v %v", managed, err)
	}
	state, err := managedTrustCoreState(ctx)
	if err != nil || state != "installed" {
		t.Fatalf("remote trust tried local runtime: %s %v", state, err)
	}
}
