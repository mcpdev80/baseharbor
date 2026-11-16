package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestGuidedBackupWriterStopFailuresRecoverAttemptedOwnersWithoutCancellation(t *testing.T) {
	for _, phase := range []string{"exposure", "workload", "broker"} {
		for _, recoveryFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/recovery-fails=%t", phase, recoveryFails), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				stopCause := errors.New("native stop interrupted after partial retirement")
				recoveryCause := errors.New("normal owner could not converge")
				var calls []string
				stop := func(name string) error {
					calls = append(calls, "stop-"+name)
					if phase == name {
						cancel()
						return fmt.Errorf("%s: %w", name, stopCause)
					}
					return nil
				}
				var requested backupWriterRecovery
				_, err := stopBackupWriters(ctx,
					func(context.Context) error { return stop("exposure") },
					func(context.Context) (bool, error) { return true, stop("workload") },
					func(context.Context) error { return stop("broker") },
					func(recoveryCtx context.Context, targets backupWriterRecovery) error {
						requested = targets
						if ctx.Err() == nil || recoveryCtx.Err() != nil {
							t.Fatal("recovery inherited cancelled request")
						}
						deadline, bounded := recoveryCtx.Deadline()
						if !bounded || time.Until(deadline) <= 0 || time.Until(deadline) > 3*time.Minute {
							t.Fatal("recovery is not bounded by existing three-minute contract")
						}
						if targets.broker {
							calls = append(calls, "recover-broker")
						}
						if targets.workload {
							calls = append(calls, "recover-workload")
						}
						if targets.exposure {
							calls = append(calls, "recover-exposure")
						}
						if recoveryFails {
							return fmt.Errorf("recovery: %w", recoveryCause)
						}
						return nil
					})
				if !errors.Is(err, stopCause) || errors.Is(err, recoveryCause) != recoveryFails {
					t.Fatal("lost original or recovery cause", err)
				}
				want := []string{"stop-exposure", "recover-exposure"}
				wantTargets := backupWriterRecovery{exposure: true}
				if phase == "workload" {
					want = []string{"stop-exposure", "stop-workload", "recover-workload", "recover-exposure"}
					wantTargets.workload = true
				}
				if phase == "broker" {
					want = []string{"stop-exposure", "stop-workload", "stop-broker", "recover-broker", "recover-workload", "recover-exposure"}
					wantTargets.workload = true
					wantTargets.broker = true
				}
				if !reflect.DeepEqual(calls, want) || requested != wantTargets {
					t.Fatalf("incorrect attempted-owner recovery: calls=%v targets=%+v", calls, requested)
				}
			})
		}
	}
}

func TestGuidedBackupWriterStopSuccessAndAbsentModulesDoNotTriggerFailureRecovery(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			var calls []string
			var exposure, broker func(context.Context) error
			if enabled {
				exposure = func(context.Context) error { calls = append(calls, "exposure"); return nil }
				broker = func(context.Context) error { calls = append(calls, "broker"); return nil }
			}
			targets, err := stopBackupWriters(t.Context(), exposure, func(context.Context) (bool, error) { calls = append(calls, "workload"); return enabled, nil }, broker, func(context.Context, backupWriterRecovery) error {
				t.Fatal("success invoked failure recovery")
				return nil
			})
			want := []string{"workload"}
			if enabled {
				want = []string{"exposure", "workload", "broker"}
			}
			if err != nil || !reflect.DeepEqual(calls, want) || targets != (backupWriterRecovery{broker: enabled, workload: enabled, exposure: enabled}) {
				t.Fatalf("success/absence stop result: calls=%v targets=%+v err=%v", calls, targets, err)
			}
		})
	}
}
