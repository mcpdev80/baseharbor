package credential

import (
	"context"
	"errors"
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
	if err != nil { t.Fatal(err) }
	if want := []string{"prepare", "reconcile", "verify", "retire"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
}

func TestRotationVerificationFailureKeepsRecoverablePath(t *testing.T) {
	var steps []string
	err := (Rotation{
		Prepare: func(context.Context) error { steps = append(steps, "prepare"); return nil },
		Reconcile: func(context.Context) error { steps = append(steps, "reconcile"); return nil },
		Verify: func(context.Context) error { steps = append(steps, "verify"); return errors.New("auth failed") },
		Retire: func(context.Context) error { steps = append(steps, "retire"); return nil },
		Rollback: func(context.Context) error { steps = append(steps, "rollback"); return nil },
	}).Run(context.Background())
	if err == nil { t.Fatal("verification failure accepted") }
	if want := []string{"prepare", "reconcile", "verify", "rollback"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
}
