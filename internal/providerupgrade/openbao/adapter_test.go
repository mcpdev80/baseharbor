package openbao

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

type fakeOps struct {
	state State
	checkErr, backupErr, verifyBackupErr, applyErr, healthErr, unsealErr error
	managerErr, authErr, appErr, restoreErr error
	applyCalls, restoreCalls int
}

func (f *fakeOps) Inspect(context.Context) (State, error) { return f.state, nil }
func (f *fakeOps) CheckUpgradePath(context.Context, string, string) error { return f.checkErr }
func (f *fakeOps) CreateBackup(context.Context, string) (providerupgrade.BackupRef, error) {
	if f.backupErr != nil { return providerupgrade.BackupRef{}, f.backupErr }
	return providerupgrade.BackupRef{Provider: providerupgrade.ProviderOpenBao, ID: "bao-backup", Version: f.state.Version, CreatedAt: time.Now(), Verified: true}, nil
}
func (f *fakeOps) VerifyBackup(context.Context, providerupgrade.BackupRef) error { return f.verifyBackupErr }
func (f *fakeOps) ApplyTarget(context.Context, string, string, string) error { f.applyCalls++; return f.applyErr }
func (f *fakeOps) WaitHealthy(context.Context) error { return f.healthErr }
func (f *fakeOps) EnsureUnsealed(context.Context) error { return f.unsealErr }
func (f *fakeOps) VerifyManagerAuth(context.Context) error { return f.managerErr }
func (f *fakeOps) VerifyAuthConfiguration(context.Context) error { return f.authErr }
func (f *fakeOps) VerifyApplicationAccess(context.Context) error { return f.appErr }
func (f *fakeOps) RestoreBackup(context.Context, providerupgrade.BackupRef, string) error {
	f.restoreCalls++
	if f.restoreErr == nil {
		f.state.Version = "2.6.0"
		f.state.Initialized = true
		f.state.Sealed = false
		f.state.Healthy = true
	}
	return f.restoreErr
}

func goodState() State {
	return State{Version: "2.6.0", Initialized: true, Sealed: false, Healthy: true, Owner: "baseharbor", Topology: "single"}
}

func request() providerupgrade.Request {
	return providerupgrade.Request{CurrentVersion: "2.6.0", TargetVersion: "2.7.0", TargetImage: "docker.io/openbao/openbao:2.7.0", TargetDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
}

func goodBackup() providerupgrade.BackupRef {
	return providerupgrade.BackupRef{Provider: providerupgrade.ProviderOpenBao, ID: "bao-backup", Version: "2.6.0", CreatedAt: time.Now(), Verified: true}
}

func TestPreflightRejectsForeignAndUnhealthy(t *testing.T) {
	for name, mutate := range map[string]func(*State){
		"foreign": func(s *State) { s.Owner = "external" },
		"sealed": func(s *State) { s.Sealed = true },
		"uninitialized": func(s *State) { s.Initialized = false },
		"unhealthy": func(s *State) { s.Healthy = false },
	} {
		t.Run(name, func(t *testing.T) {
			state := goodState(); mutate(&state)
			_, err := New(&fakeOps{state: state}).Preflight(context.Background(), request())
			if err == nil { t.Fatal("expected fail-closed preflight") }
		})
	}
}

func TestPreflightRejectsDowngradeAndUnknownPath(t *testing.T) {
	ops := &fakeOps{state: goodState()}
	req := request(); req.TargetVersion = "2.5.0"
	if _, err := New(ops).Preflight(context.Background(), req); providerupgrade.ClassOf(err) != providerupgrade.ErrorUnsupportedPath {
		t.Fatalf("downgrade class = %q, err=%v", providerupgrade.ClassOf(err), err)
	}
	ops.checkErr = errors.New("upgrade notes not approved")
	_, err := New(ops).Preflight(context.Background(), request())
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorUnsupportedPath { t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err) }
}

func TestExecuteRequiresVerifiedBackupBeforeMutation(t *testing.T) {
	ops := &fakeOps{state: goodState()}
	backup := goodBackup(); backup.Verified = false
	err := New(ops).Execute(context.Background(), request(), backup)
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorBackupRequired { t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err) }
	if ops.applyCalls != 0 { t.Fatalf("mutation occurred without backup: %d", ops.applyCalls) }
}

func TestExecuteStopsOnHealthOrUnsealFailure(t *testing.T) {
	for name, configure := range map[string]func(*fakeOps){
		"health": func(f *fakeOps) { f.healthErr = errors.New("not ready") },
		"unseal": func(f *fakeOps) { f.unsealErr = errors.New("still sealed") },
	} {
		t.Run(name, func(t *testing.T) {
			ops := &fakeOps{state: goodState()}; configure(ops)
			err := New(ops).Execute(context.Background(), request(), goodBackup())
			if providerupgrade.ClassOf(err) != providerupgrade.ErrorApplyFailed { t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err) }
		})
	}
}

func TestVerifyRequiresAuthAndApplicationSecretAccess(t *testing.T) {
	for name, configure := range map[string]func(*fakeOps){
		"manager": func(f *fakeOps) { f.managerErr = errors.New("login failed") },
		"auth-config": func(f *fakeOps) { f.authErr = errors.New("approle missing") },
		"application-secret": func(f *fakeOps) { f.appErr = errors.New("secret read failed") },
	} {
		t.Run(name, func(t *testing.T) {
			state := goodState(); state.Version = "2.7.0"
			ops := &fakeOps{state: state}; configure(ops)
			err := New(ops).Verify(context.Background(), request())
			if providerupgrade.ClassOf(err) != providerupgrade.ErrorVerifyFailed { t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err) }
		})
	}
}

func TestVerifySucceedsOnlyAfterSemanticChecks(t *testing.T) {
	state := goodState(); state.Version = "2.7.0"
	if err := New(&fakeOps{state: state}).Verify(context.Background(), request()); err != nil { t.Fatal(err) }
}

func TestRecoveryRestoresOriginalVersionAndSemantics(t *testing.T) {
	state := goodState(); state.Version = "2.7.0"
	ops := &fakeOps{state: state}
	if err := New(ops).Recover(context.Background(), request(), goodBackup()); err != nil { t.Fatal(err) }
	if ops.restoreCalls != 1 { t.Fatalf("restore calls=%d", ops.restoreCalls) }
}

func TestRecoveryFailsWhenSemanticAccessIsNotRestored(t *testing.T) {
	state := goodState(); state.Version = "2.7.0"
	ops := &fakeOps{state: state, appErr: errors.New("app role no longer works")}
	err := New(ops).Recover(context.Background(), request(), goodBackup())
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorRecoveryFailed { t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err) }
}
