package credential

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRotationOrdersVerificationBeforeRetirement(t *testing.T) {
	var steps []string
	step := func(name string) func(context.Context) error {
		return func(context.Context) error { steps = append(steps, name); return nil }
	}
	err := (Rotation{
		Prepare: step("prepare"), Reconcile: step("reconcile"), Verify: step("verify"), Retire: step("retire"), Rollback: step("rollback"),
	}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"prepare", "reconcile", "verify", "retire"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
}

func TestRotationVerificationFailureKeepsRecoverablePath(t *testing.T) {
	var steps []string
	err := (Rotation{
		Prepare:   func(context.Context) error { steps = append(steps, "prepare"); return nil },
		Reconcile: func(context.Context) error { steps = append(steps, "reconcile"); return nil },
		Verify:    func(context.Context) error { steps = append(steps, "verify"); return errors.New("auth failed") },
		Retire:    func(context.Context) error { steps = append(steps, "retire"); return nil },
		Rollback:  func(context.Context) error { steps = append(steps, "rollback"); return nil },
	}).Run(context.Background())
	if err == nil {
		t.Fatal("verification failure accepted")
	}
	if want := []string{"prepare", "reconcile", "verify", "rollback"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
}

func TestRotationJournalResumesAtRetirementAfterVerifiedCutover(t *testing.T) {
	journal := FileRotationJournal{Path: filepath.Join(t.TempDir(), "rotation.json")}
	var steps []string
	retireAttempts := 0
	rotation := Rotation{
		Key:       "provider/postgresql/app/default",
		Journal:   journal,
		Prepare:   func(context.Context) error { steps = append(steps, "prepare"); return nil },
		Reconcile: func(context.Context) error { steps = append(steps, "reconcile"); return nil },
		Verify:    func(context.Context) error { steps = append(steps, "verify"); return nil },
		Retire: func(context.Context) error {
			steps = append(steps, "retire")
			retireAttempts++
			if retireAttempts == 1 {
				return errors.New("temporary revoke failure")
			}
			return nil
		},
		Rollback: func(context.Context) error { steps = append(steps, "rollback"); return nil },
	}
	if err := rotation.Run(context.Background()); err == nil {
		t.Fatal("first retirement failure accepted")
	}
	if phase, err := journal.Load(rotation.Key); err != nil || phase != RotationVerified {
		t.Fatalf("journal phase after retire failure = %q, %v", phase, err)
	}
	if err := rotation.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"prepare", "reconcile", "verify", "retire", "retire"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
	if phase, err := journal.Load(rotation.Key); err != nil || phase != "" {
		t.Fatalf("completed journal phase = %q, %v", phase, err)
	}
}

func TestRotationJournalClearsAfterRollbackBeforeVerification(t *testing.T) {
	journal := FileRotationJournal{Path: filepath.Join(t.TempDir(), "rotation.json")}
	rotation := Rotation{
		Key:       "provider/keycloak/admin",
		Journal:   journal,
		Prepare:   func(context.Context) error { return nil },
		Reconcile: func(context.Context) error { return nil },
		Verify:    func(context.Context) error { return errors.New("new login rejected") },
		Retire:    func(context.Context) error { return nil },
		Rollback:  func(context.Context) error { return nil },
	}
	if err := rotation.Run(context.Background()); err == nil {
		t.Fatal("verification failure accepted")
	}
	if phase, err := journal.Load(rotation.Key); err != nil || phase != "" {
		t.Fatalf("rolled-back journal phase = %q, %v", phase, err)
	}
}

func TestFileRotationJournalIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "rotation.json")
	journal := FileRotationJournal{Path: path}
	if err := journal.Save("provider/openbao/tls", RotationPrepared); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("journal mode = %o, want 600", got)
	}
}
