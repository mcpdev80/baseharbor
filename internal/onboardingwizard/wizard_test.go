package onboardingwizard

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// wizard_test.go holds the internal-slice regressions for the guided
// onboarding flow (card t-20261018-r27-onboarding-wizard): the decision
// invariants are enforced by the type itself, independently of any driver.

func TestModeValid(t *testing.T) {
	for _, mode := range []Mode{ModeBootstrap, ModeAttach, ModeImport, ModeRemove} {
		if !mode.Valid() {
			t.Fatalf("mode %q must be valid", mode)
		}
	}
	for _, mode := range []Mode{"", "teleport", "BOOTSTRAP"} {
		if mode.Valid() {
			t.Fatalf("mode %q must be invalid", mode)
		}
	}
}

func TestDecisionValidateMatrix(t *testing.T) {
	cases := []struct {
		name string
		dec  Decision
		err  error
	}{
		{"unknown", Decision{}, ErrInvalidChoice},
		{"attach-without-target", Decision{Mode: ModeAttach}, ErrIncompleteConfiguration},
		{"attach", Decision{Mode: ModeAttach, Target: "t"}, nil},
		{"attach-bad-persistence", Decision{Mode: ModeAttach, Target: "t", Persistence: "forever"}, ErrInvalidChoice},
		{"bootstrap-without-persistence", Decision{Mode: ModeBootstrap, App: AppNew}, ErrInvalidChoice},
		{"bootstrap-without-app", Decision{Mode: ModeBootstrap, Persistence: PersistenceSessionOnly}, ErrInvalidChoice},
		{"bootstrap", Decision{Mode: ModeBootstrap, App: AppNew, Persistence: PersistenceSecure}, nil},
		{"import-both", Decision{Mode: ModeImport, ImportBundle: "b", ImportFile: "f"}, ErrConflict},
		{"import-none", Decision{Mode: ModeImport}, ErrIncompleteConfiguration},
		{"import-bundle", Decision{Mode: ModeImport, ImportBundle: "b"}, nil},
		{"remove", Decision{Mode: ModeRemove, Target: "t"}, nil},
		{"remove-without-target", Decision{Mode: ModeRemove}, ErrIncompleteConfiguration},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.dec.Validate()
			if tc.err == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.err) {
				t.Fatalf("Validate() = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestRunCancelAndEOF(t *testing.T) {
	if _, err := Run(context.Background(), Input{In: strings.NewReader("cancel\n"), Out: io.Discard}); !errors.Is(err, ErrCancel) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := Run(context.Background(), Input{In: strings.NewReader(""), Out: io.Discard}); err == nil {
		t.Fatal("empty input produced a decision")
	}
	if _, err := Run(context.Background(), Input{In: strings.NewReader("new\n"), Out: io.Discard}); err == nil {
		t.Fatal("truncated input produced a decision")
	}
}

func TestRunContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, Input{In: strings.NewReader("attach\nyes\n"), Out: io.Discard}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context: %v", err)
	}
}

func TestRunNilOutputFailsClosed(t *testing.T) {
	if _, err := Run(context.Background(), Input{In: strings.NewReader("attach\n")}); !errors.Is(err, ErrIncompleteConfiguration) {
		t.Fatalf("nil output: %v", err)
	}
}

func TestReviewSheetListsEveryChoice(t *testing.T) {
	var out strings.Builder
	if _, err := Run(context.Background(), Input{
		In:       strings.NewReader("attach\nnew\nno\nyes\n"),
		Out:      &out,
		Target:   "a",
		Discover: func(context.Context) (InstallState, error) { return InstallState{Targets: []string{"a", "b"}}, nil },
	}); err != nil {
		t.Fatalf("wizard failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Review:", "mode:    attach", "target:  a", "store:   session-only"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("review sheet lacks %q:\n%s", want, out.String())
		}
	}
}

func TestTrustedLocalRunsSilently(t *testing.T) {
	var out strings.Builder
	dec, err := Run(context.Background(), Input{
		In:     strings.NewReader(""),
		Out:    &out,
		Target: "homelab",
		Discover: func(context.Context) (InstallState, error) {
			return InstallState{HasInstallation: true, TrustedLocal: true, Targets: []string{"homelab"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("trusted repeat operation must continue silently, got %v\n%s", err, out.String())
	}
	if dec.Mode != ModeAttach || dec.Target != "homelab" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
	if out.Len() != 0 {
		t.Fatalf("repeat operation produced output: %q", out.String())
	}
}

func TestRemoveIsLocalOnlyStatement(t *testing.T) {
	var out strings.Builder
	dec, err := Run(context.Background(), Input{
		In:  strings.NewReader("remove\nyes\n"),
		Out: &out,
		Discover: func(context.Context) (InstallState, error) {
			return InstallState{HasInstallation: true, Targets: []string{"homelab"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("remove wizard failed: %v\n%s", err, out.String())
	}
	if dec.Mode != ModeRemove || dec.Target != "homelab" {
		t.Fatalf("expected local removal, got %+v", dec)
	}
	if !strings.Contains(out.String(), "revokes nothing remotely") {
		t.Fatalf("local cleanup must state the revocation boundary:\n%s", out.String())
	}
}

func TestExistingAppFoundIsNotAsked(t *testing.T) {
	var out strings.Builder
	dec, err := Run(context.Background(), Input{
		In:       strings.NewReader("new\nyes\nyes\n"),
		Out:      &out,
		Discover: func(context.Context) (InstallState, error) { return InstallState{ExistingAppFound: true}, nil },
	})
	if err != nil {
		t.Fatalf("wizard failed: %v\n%s", err, out.String())
	}
	if dec.App != AppExisting {
		t.Fatalf("existing application was not attached: %+v", dec)
	}
}

func TestImportBundleAndProfileAreDistinctAndExclusive(t *testing.T) {
	var out strings.Builder
	dec, err := Run(context.Background(), Input{
		In:       strings.NewReader("import\nbundle\n/dev/null/bundle.json\nyes\n"),
		Out:      &out,
		Discover: func(context.Context) (InstallState, error) { return InstallState{}, nil },
	})
	if err != nil {
		t.Fatalf("bundle import wizard failed: %v\n%s", err, out.String())
	}
	if dec.Mode != ModeImport || dec.ImportBundle != "/dev/null/bundle.json" || dec.ImportFile != "" {
		t.Fatalf("unexpected decision: %+v", dec)
	}
}

func TestFirstOwnerWizardDefaultCustomAndCancel(t *testing.T) {
	for _, test := range []struct {
		input  string
		slug   string
		name   string
		cancel bool
	}{
		{"owner\ndefault\nyes\n", "baseharbor", "BaseHarbor", false},
		{"owner\ncustom\nacme\nAcme GmbH\nyes\n", "acme", "Acme GmbH", false},
		{"owner\ndefault\nno\n", "", "", true},
	} {
		var output strings.Builder
		decision, err := Run(context.Background(), Input{
			In:  strings.NewReader(test.input),
			Out: &output,
			Discover: func(context.Context) (InstallState, error) {
				return InstallState{HasInstallation: true, Targets: []string{"local"}}, nil
			},
		})
		if test.cancel {
			if !errors.Is(err, ErrCancel) {
				t.Fatalf("cancel must leave Core untouched: %v", err)
			}
			continue
		}
		if err != nil || decision.Mode != ModeOwner || decision.TenantSlug != test.slug || decision.TenantName != test.name {
			t.Fatalf("owner wizard decision: %+v, %v\n%s", decision, err, output.String())
		}
	}
}

func TestExistingCoreDefaultsToOneUseInvitation(t *testing.T) {
	var output strings.Builder
	decision, err := Run(context.Background(), Input{
		In:  strings.NewReader("\nyes\n"),
		Out: &output,
		Discover: func(context.Context) (InstallState, error) {
			return InstallState{HasInstallation: true, Targets: []string{"docker-local"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("existing Core invitation failed: %v", err)
	}
	if decision.Mode != ModeInvite || decision.Target != "docker-local" {
		t.Fatalf("existing Core must default to invitation: %+v", decision)
	}
	if !strings.Contains(output.String(), "one-use workstation connection file") {
		t.Fatalf("default invitation choice was not offered: %s", output.String())
	}
}
func TestInvitationNeedsExistingCoreTarget(t *testing.T) {
	if err := (Decision{Mode: ModeInvite}).Validate(); err == nil {
		t.Fatal("invitation without a locally resolved Core Target was permitted")
	}
}
