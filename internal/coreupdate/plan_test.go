package coreupdate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func expected() []Desired {
	return []Desired{
		{Kind: SQL, Image: "postgres", Digest: digestB, Version: "18.1"},
		{Kind: Secrets, Image: "openbao", Digest: digestB, Version: "2.7.1"},
		{Kind: Identity, Image: "keycloak", Digest: digestB, Version: "26.8.0"},
	}
}

func TestBuildFailsClosedWithoutFullPinnedRelease(t *testing.T) {
	for _, desired := range [][]Desired{nil, expected()[:2], {{Kind: SQL, Image: "postgres:latest", Version: "18"}}} {
		if _, err := Build("0.4.24", nil, desired); err == nil {
			t.Fatalf("accepted unpinned/incomplete release: %+v", desired)
		}
	}
}

func TestPlanPreservesForeignAndUnchangedRealizations(t *testing.T) {
	installed := []Realization{
		{Kind: SQL, Installation: "a", Scope: "shared", Instance: "sql", Owner: "baseharbor", Image: "postgres", Digest: digestB, Version: "18.1"},
		{Kind: SQL, Installation: "a", Scope: "isolated", Instance: "app1", Owner: "baseharbor", Image: "postgres", Digest: digestA, Version: "17.9"},
		{Kind: Secrets, Installation: "a", Scope: "shared", Instance: "secrets", Owner: "external", Image: "openbao", Digest: digestA, Version: "2.6.0"},
	}
	plan, err := Build("0.4.24", installed, expected())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Deltas) != 2 || plan.Deltas[0].Classification != NoChange || plan.Deltas[1].Classification != Unsupported {
		t.Fatalf("incorrect update classification: %#v", plan.Deltas)
	}
	if err := Execute(context.Background(), plan, Hooks{Preflight: func(context.Context, Plan) error { return nil }, Apply: func(context.Context, Delta) error { return nil }, Verify: func(context.Context, Delta) error { return nil }, Record: func(context.Context, Delta, string) error { return nil }}); err == nil {
		t.Fatal("unsupported PostgreSQL major update was applied")
	}
}

func TestExecutionRequiresRecoveryAndVerifiedResult(t *testing.T) {
	plan, err := Build("0.4.24", []Realization{{Kind: Secrets, Installation: "a", Scope: "shared", Instance: "vault", Owner: "baseharbor", Image: "openbao", Digest: digestA, Version: "2.7.0"}}, expected())
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	hooks := Hooks{
		Preflight: func(context.Context, Plan) error { count++; return nil },
		Apply:     func(context.Context, Delta) error { count++; return nil },
		Verify:    func(context.Context, Delta) error { return errors.New("unready") },
		Record:    func(context.Context, Delta, string) error { return nil },
	}
	if err := Execute(context.Background(), plan, hooks); err == nil || count != 0 {
		t.Fatalf("missing recovery applied: %v count=%d", err, count)
	}
	hooks.RecoveryPoint = func(context.Context, Delta) error { count++; return nil }
	if err := Execute(context.Background(), plan, hooks); err == nil || !strings.Contains(err.Error(), "verification") || count != 3 {
		t.Fatalf("verification failure missing: %v count=%d", err, count)
	}
}

func TestBuildRejectsMalformedDigestAndDetectsDifferentImage(t *testing.T) {
	bad := expected()
	bad[0].Digest = "sha256:" + strings.Repeat("z", 64)
	if _, err := Build("0.4.24", nil, bad); err == nil {
		t.Fatal("accepted non-hex digest")
	}
	installed := []Realization{{Kind: SQL, Installation: "a", Scope: "shared", Instance: "sql", Owner: "baseharbor", Image: "untrusted-postgres", Digest: digestB, Version: "18.1"}}
	plan, err := Build("0.4.24", installed, expected())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Deltas) != 1 || plan.Deltas[0].Classification == NoChange {
		t.Fatalf("same digest with different image must not be treated as unchanged: %+v", plan)
	}
}

func TestBuildRejectsUnverifiableInstalledDigestBeforeMutation(t *testing.T) {
	installed := []Realization{{Kind: Secrets, Installation: "core-a", Scope: "shared", Instance: "vault", Owner: "baseharbor", Image: "openbao", Version: "2.7.0"}}
	plan, err := Build("0.4.24", installed, expected())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Deltas) != 1 || plan.Deltas[0].Classification != Unsupported {
		t.Fatalf("unverified installed image cannot be safely reconciled: %+v", plan)
	}
	called := false
	hooks := Hooks{
		Preflight: func(context.Context, Plan) error { called = true; return nil },
		Apply:     func(context.Context, Delta) error { called = true; return nil },
		Verify:    func(context.Context, Delta) error { called = true; return nil },
		Record:    func(context.Context, Delta, string) error { called = true; return nil },
	}
	if err := Execute(context.Background(), plan, hooks); err == nil || called {
		t.Fatalf("unsafe plan executed: err=%v called=%v", err, called)
	}
}

func TestProviderVersionComparisonFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		current, target string
		downgrade       bool
	}{
		{"26.8.0", "26.7.1", true},
		{"18.1", "17.9", true},
		{"2.7.1", "2.7.1", false},
		{"2.7", "2.7.0", false},
		{"2.7.0", "2.8.0", false},
		{"26.8.0-rc1", "26.8.0", true},
		{"", "26.8.0", true},
	} {
		if got := providerDowngrade(tc.current, tc.target); got != tc.downgrade {
			t.Errorf("providerDowngrade(%q,%q)=%v, want %v", tc.current, tc.target, got, tc.downgrade)
		}
	}
	for _, kind := range []ProviderKind{SQL, Secrets, Identity} {
		installed := []Realization{{Kind: kind, Installation: "a", Scope: "shared", Instance: "owned", Owner: "baseharbor", Image: "provider", Digest: digestA, Version: "99.0.0"}}
		plan, err := Build("0.4.24", installed, expected())
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Deltas) != 1 || plan.Deltas[0].Classification != Unsupported {
			t.Fatalf("%s downgrade accepted: %+v", kind, plan)
		}
	}
}

func TestExecuteRejectsForgedPlanBeforeHooks(t *testing.T) {
	base := Delta{
		Installed:      Realization{Kind: SQL, Installation: "a", Scope: "shared", Instance: "sql", Owner: "baseharbor", Image: "postgres", Digest: digestA, Version: "18.1"},
		Desired:        Desired{Kind: SQL, Image: "postgres", Digest: digestB, Version: "18.2"},
		Classification: BackupRequired,
	}
	called := false
	hooks := Hooks{
		Preflight:     func(context.Context, Plan) error { called = true; return nil },
		RecoveryPoint: func(context.Context, Delta) error { called = true; return nil },
		Apply:         func(context.Context, Delta) error { called = true; return nil },
		Verify:        func(context.Context, Delta) error { called = true; return nil },
		Record:        func(context.Context, Delta, string) error { called = true; return nil },
	}
	cases := []struct {
		name   string
		mutate func(*Delta)
	}{
		{"foreign_owner", func(d *Delta) { d.Installed.Owner = "external" }},
		{"provider_mismatch", func(d *Delta) { d.Desired.Kind = Secrets }},
		{"missing_pin", func(d *Delta) { d.Desired.Digest = "" }},
		{"forged_no_change", func(d *Delta) { d.Classification = NoChange }},
		{"version_downgrade", func(d *Delta) { d.Desired.Version = "17.9" }},
		{"postgres_major_upgrade", func(d *Delta) { d.Desired.Version = "19.0" }},
		{"forged_safe_reconcile", func(d *Delta) { d.Classification = SafeReconcile }},
		{"forged_migration", func(d *Delta) { d.Classification = MigrationRequired }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			tc.mutate(&d)
			called = false
			err := Execute(context.Background(), Plan{Release: "0.4.24", Deltas: []Delta{d}}, hooks)
			if err == nil || called {
				t.Fatalf("unsafe plan invoked hooks: %v called=%v", err, called)
			}
		})
	}
	called = false
	if err := Execute(context.Background(), Plan{Release: "0.4.24", Deltas: []Delta{base, base}}, hooks); err == nil || called {
		t.Fatalf("duplicate plan invoked hooks: %v called=%v", err, called)
	}
}
