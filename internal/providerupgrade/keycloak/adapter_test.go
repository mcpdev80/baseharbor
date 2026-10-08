package keycloak

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
)

type fakeOps struct {
	state                                                                         State
	checkErr, backupErr, verifyBackupErr                                          error
	applySingleErr, stopErr, applyAllErr, replaceErr, memberReadyErr, allReadyErr error
	dbErr, realmErr, oidcErr, tokenErr, restoreErr                                error
	calls                                                                         []string
}

func (f *fakeOps) Inspect(context.Context) (State, error)                 { return f.state, nil }
func (f *fakeOps) CheckUpgradePath(context.Context, string, string) error { return f.checkErr }
func (f *fakeOps) CreateBackup(context.Context, string) (providerupgrade.BackupRef, error) {
	if f.backupErr != nil {
		return providerupgrade.BackupRef{}, f.backupErr
	}
	return providerupgrade.BackupRef{
		Provider:  providerupgrade.ProviderKeycloak,
		ID:        "kc-backup",
		Version:   f.state.Version,
		CreatedAt: time.Now(),
		Verified:  true,
		Metadata:  map[string]string{"database_verified": "true", "configuration_verified": "true"},
	}, nil
}
func (f *fakeOps) VerifyBackup(context.Context, providerupgrade.BackupRef) error {
	return f.verifyBackupErr
}
func (f *fakeOps) ApplySingle(context.Context, string, string, string) error {
	f.calls = append(f.calls, "single")
	return f.applySingleErr
}
func (f *fakeOps) StopAllMembers(context.Context) error {
	f.calls = append(f.calls, "stop-all")
	return f.stopErr
}
func (f *fakeOps) ApplyAllMembers(context.Context, string, string, string) error {
	f.calls = append(f.calls, "apply-all")
	return f.applyAllErr
}
func (f *fakeOps) ReplaceMember(_ context.Context, member, _, _, _ string) error {
	f.calls = append(f.calls, "replace:"+member)
	if f.replaceErr == nil {
		for i := range f.state.Members {
			if f.state.Members[i].Name == member {
				f.state.Members[i].Version = "26.7.6"
				f.state.Members[i].Ready = true
			}
		}
	}
	return f.replaceErr
}
func (f *fakeOps) WaitMemberReady(_ context.Context, member string) error {
	f.calls = append(f.calls, "ready:"+member)
	return f.memberReadyErr
}
func (f *fakeOps) WaitAllReady(context.Context) error {
	f.calls = append(f.calls, "ready-all")
	return f.allReadyErr
}
func (f *fakeOps) VerifyDatabase(context.Context) error      { return f.dbErr }
func (f *fakeOps) VerifyRealmState(context.Context) error    { return f.realmErr }
func (f *fakeOps) VerifyOIDCDiscovery(context.Context) error { return f.oidcErr }
func (f *fakeOps) VerifyTokenFlow(context.Context) error     { return f.tokenErr }
func (f *fakeOps) RestoreBackup(context.Context, providerupgrade.BackupRef, string) error {
	f.calls = append(f.calls, "restore")
	if f.restoreErr == nil {
		f.state = singleState("26.7.5")
	}
	return f.restoreErr
}

func singleState(version string) State {
	return State{
		Version: version, Owner: "baseharbor", Topology: TopologySingle, Healthy: true,
		DatabaseType: "postgresql",
		Members:      []Member{{Name: "keycloak-1", Version: version, Ready: true}},
	}
}

func haState(version string) State {
	return State{
		Version: version, Owner: "baseharbor", Topology: TopologyHA, Healthy: true,
		DatabaseType: "postgresql-ha",
		Members: []Member{
			{Name: "keycloak-1", Version: version, Ready: true},
			{Name: "keycloak-2", Version: version, Ready: true},
			{Name: "keycloak-3", Version: version, Ready: true},
		},
	}
}

func req(from, to string) providerupgrade.Request {
	return providerupgrade.Request{
		CurrentVersion: from, TargetVersion: to,
		TargetImage:  "quay.io/keycloak/keycloak:" + to,
		TargetDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
}

func backup(version string) providerupgrade.BackupRef {
	return providerupgrade.BackupRef{
		Provider: providerupgrade.ProviderKeycloak, ID: "kc-backup", Version: version, CreatedAt: time.Now(), Verified: true,
		Metadata: map[string]string{"database_verified": "true", "configuration_verified": "true"},
	}
}

func TestPreflightRejectsForeignUnhealthyAndUnknownPath(t *testing.T) {
	state := singleState("26.7.5")
	state.Owner = "external"
	if _, err := New(&fakeOps{state: state}).Preflight(context.Background(), req("26.7.5", "26.8.0")); err == nil {
		t.Fatal("foreign provider accepted")
	}

	state = singleState("26.7.5")
	state.Healthy = false
	if _, err := New(&fakeOps{state: state}).Preflight(context.Background(), req("26.7.5", "26.8.0")); err == nil {
		t.Fatal("unhealthy provider accepted")
	}

	ops := &fakeOps{state: singleState("26.7.5"), checkErr: errors.New("migration notes not approved")}
	_, err := New(ops).Preflight(context.Background(), req("26.7.5", "26.8.0"))
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorUnsupportedPath {
		t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err)
	}
}

func TestPreflightRejectsDowngrade(t *testing.T) {
	ops := &fakeOps{state: singleState("26.8.0")}
	_, err := New(ops).Preflight(context.Background(), req("26.8.0", "26.7.5"))
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorUnsupportedPath {
		t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err)
	}
}

func TestBackupRequiresDatabaseAndConfigurationEvidence(t *testing.T) {
	ops := &fakeOps{state: singleState("26.7.5")}
	b, err := New(ops).Backup(context.Background(), req("26.7.5", "26.8.0"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Metadata["database_verified"] != "true" || b.Metadata["configuration_verified"] != "true" {
		t.Fatal("missing backup evidence")
	}

	ops2 := &fakeOps{state: singleState("26.7.5")}
	ops2.backupErr = errors.New("database backup failed")
	if _, err := New(ops2).Backup(context.Background(), req("26.7.5", "26.8.0")); providerupgrade.ClassOf(err) != providerupgrade.ErrorBackupInvalid {
		t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err)
	}
}

func TestExecuteSingleUsesRecreatePath(t *testing.T) {
	ops := &fakeOps{state: singleState("26.7.5")}
	if err := New(ops).Execute(context.Background(), req("26.7.5", "26.8.0"), backup("26.7.5")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ops.calls, []string{"single", "ready-all"}) {
		t.Fatalf("calls=%v", ops.calls)
	}
}

func TestExecuteHAMinorUpgradeStopsOldClusterBeforeMigration(t *testing.T) {
	ops := &fakeOps{state: haState("26.7.5")}
	if err := New(ops).Execute(context.Background(), req("26.7.5", "26.8.0"), backup("26.7.5")); err != nil {
		t.Fatal(err)
	}
	want := []string{"stop-all", "apply-all", "ready-all"}
	if !reflect.DeepEqual(ops.calls, want) {
		t.Fatalf("calls=%v want=%v", ops.calls, want)
	}
}

func TestExecuteHAPatchUpgradeRollsOneMemberAtATime(t *testing.T) {
	ops := &fakeOps{state: haState("26.7.5")}
	if err := New(ops).Execute(context.Background(), req("26.7.5", "26.7.6"), backup("26.7.5")); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"replace:keycloak-1", "ready:keycloak-1",
		"replace:keycloak-2", "ready:keycloak-2",
		"replace:keycloak-3", "ready:keycloak-3",
	}
	if !reflect.DeepEqual(ops.calls, want) {
		t.Fatalf("calls=%v want=%v", ops.calls, want)
	}
}

func TestExecuteFailsClosedIfRollingMemberNotReady(t *testing.T) {
	ops := &fakeOps{state: haState("26.7.5"), memberReadyErr: errors.New("startup probe failed")}
	err := New(ops).Execute(context.Background(), req("26.7.5", "26.7.6"), backup("26.7.5"))
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorApplyFailed {
		t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err)
	}
	if len(ops.calls) != 2 {
		t.Fatalf("continued after member failure: %v", ops.calls)
	}
}

func TestExecuteRequiresVerifiedDatabaseBackupBeforeMutation(t *testing.T) {
	ops := &fakeOps{state: singleState("26.7.5")}
	b := backup("26.7.5")
	b.Metadata["database_verified"] = "false"
	err := New(ops).Execute(context.Background(), req("26.7.5", "26.8.0"), b)
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorBackupRequired {
		t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err)
	}
	if len(ops.calls) != 0 {
		t.Fatalf("mutation occurred: %v", ops.calls)
	}
}

func TestVerifyRequiresDatabaseRealmOIDCAndTokenSemantics(t *testing.T) {
	tests := map[string]func(*fakeOps){
		"database": func(f *fakeOps) { f.dbErr = errors.New("db failed") },
		"realm":    func(f *fakeOps) { f.realmErr = errors.New("realm drift") },
		"oidc":     func(f *fakeOps) { f.oidcErr = errors.New("discovery failed") },
		"token":    func(f *fakeOps) { f.tokenErr = errors.New("token failed") },
	}
	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			ops := &fakeOps{state: singleState("26.8.0")}
			configure(ops)
			err := New(ops).Verify(context.Background(), req("26.7.5", "26.8.0"))
			if providerupgrade.ClassOf(err) != providerupgrade.ErrorVerifyFailed {
				t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err)
			}
		})
	}
}

func TestVerifySuccess(t *testing.T) {
	if err := New(&fakeOps{state: singleState("26.8.0")}).Verify(context.Background(), req("26.7.5", "26.8.0")); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryRestoresDatabaseConfigurationAndIdentitySemantics(t *testing.T) {
	ops := &fakeOps{state: singleState("26.8.0")}
	if err := New(ops).Recover(context.Background(), req("26.7.5", "26.8.0"), backup("26.7.5")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ops.calls, []string{"restore"}) {
		t.Fatalf("calls=%v", ops.calls)
	}
}

func TestRecoveryRejectsMissingDatabaseBackup(t *testing.T) {
	ops := &fakeOps{state: singleState("26.8.0")}
	b := backup("26.7.5")
	delete(b.Metadata, "database_verified")
	err := New(ops).Recover(context.Background(), req("26.7.5", "26.8.0"), b)
	if providerupgrade.ClassOf(err) != providerupgrade.ErrorRecoveryFailed {
		t.Fatalf("class=%q err=%v", providerupgrade.ClassOf(err), err)
	}
}

func TestInventoryRejectsMemberDriftAndDuplicateMembers(t *testing.T) {
	for name, mutate := range map[string]func(*State){
		"member-version-drift": func(s *State) { s.Members[1].Version = "26.8.0" },
		"duplicate-member": func(s *State) { s.Members[1].Name = s.Members[0].Name },
		"missing-ha-member": func(s *State) { s.Members = s.Members[:2] },
	} {
		t.Run(name, func(t *testing.T) {
			state := haState("26.7.5")
			mutate(&state)
			_, err := New(&fakeOps{state: state}).Preflight(context.Background(), req("26.7.5", "26.7.6"))
			if providerupgrade.ClassOf(err) != providerupgrade.ErrorInvalidState {
				t.Fatalf("invalid HA inventory accepted: %v", err)
			}
		})
	}
}
