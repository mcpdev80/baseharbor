package coreupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteJournaledRecoversFailedProvider(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "journal.json")
	plan, err := Build("0.4.24", []Realization{{Kind: Secrets, Installation: "a", Scope: "shared", Instance: "vault", Owner: "baseharbor", Image: "openbao", Digest: digestA, Version: "2.7.0"}}, expected())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	fail := true
	hooks := Hooks{
		Preflight:     func(context.Context, Plan) error { return nil },
		RecoveryPoint: func(context.Context, Delta) error { return nil },
		Apply: func(context.Context, Delta) error {
			calls++
			if fail {
				return errors.New("stopped")
			}
			return nil
		},
		Verify: func(context.Context, Delta) error { return nil },
		Record: func(context.Context, Delta, string) error { return nil },
	}
	if err := ExecuteJournaled(context.Background(), plan, path, hooks); err == nil {
		t.Fatal("expected interrupted provider")
	}
	partial, err := LoadJournal(path, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if partial.Steps[JournalKey(plan.Deltas[0])] != "apply_failed" {
		t.Fatalf("lost failure: %+v", partial)
	}
	fail = false
	if err := ExecuteJournaled(context.Background(), plan, path, hooks); err != nil {
		t.Fatal(err)
	}
	final, err := LoadJournal(path, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if final.Steps[JournalKey(plan.Deltas[0])] != "verified" || calls != 2 {
		t.Fatalf("resume not verified: %+v calls %d", final, calls)
	}
}

func TestExecuteJournaledVerifiedStateRequiresLiveReadiness(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "journal.json")
	plan, err := Build("0.4.24", []Realization{{Kind: Identity, Installation: "a", Scope: "shared", Instance: "id", Owner: "baseharbor", Image: "keycloak", Digest: digestA, Version: "26.7.0"}}, expected())
	if err != nil {
		t.Fatal(err)
	}
	journal := Journal{Release: "0.4.24"}
	if err := journal.Record(path, plan.Deltas[0], "verified"); err != nil {
		t.Fatal(err)
	}
	applied := false
	hooks := Hooks{
		Preflight:     func(context.Context, Plan) error { return nil },
		RecoveryPoint: func(context.Context, Delta) error { return nil },
		Apply:         func(context.Context, Delta) error { applied = true; return nil },
		Verify:        func(context.Context, Delta) error { return errors.New("not ready") },
		Record:        func(context.Context, Delta, string) error { return nil },
	}
	if err := ExecuteJournaled(context.Background(), plan, path, hooks); err == nil || applied {
		t.Fatalf("stale journal led to apply: %v applied %v", err, applied)
	}
}

func TestExecuteJournaledRejectsForgedPreviouslyVerifiedPlan(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "journal.json")
	plan, err := Build("0.4.24", []Realization{{Kind: Identity, Installation: "a", Scope: "shared", Instance: "id", Owner: "baseharbor", Image: "keycloak", Digest: digestA, Version: "26.7.0"}}, expected())
	if err != nil {
		t.Fatal(err)
	}
	original := plan.Deltas[0]
	journal := Journal{Release: "0.4.24"}
	if err := journal.Record(path, original, "verified"); err != nil {
		t.Fatal(err)
	}
	plan.Deltas[0].Installed.Owner = "foreign"
	applied := false
	verified := false
	hooks := Hooks{
		Preflight:     func(context.Context, Plan) error { return nil },
		RecoveryPoint: func(context.Context, Delta) error { return nil },
		Apply:         func(context.Context, Delta) error { applied = true; return nil },
		Verify:        func(context.Context, Delta) error { verified = true; return nil },
		Record:        func(context.Context, Delta, string) error { return nil },
	}
	if err := ExecuteJournaled(context.Background(), plan, path, hooks); err == nil {
		t.Fatal("forged verified journal step accepted")
	}
	if applied || verified {
		t.Fatal("forged step reached runtime hooks")
	}
}

func TestExecuteJournaledReceiptFailureNeverCommitsVerified(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "journal.json")
	plan, err := Build("0.4.24", []Realization{{Kind: Secrets, Installation: "a", Scope: "shared", Instance: "vault", Owner: "baseharbor", Image: "openbao", Digest: digestA, Version: "2.7.0"}}, expected())
	if err != nil {
		t.Fatal(err)
	}
	failReceipt := true
	applied := 0
	hooks := Hooks{
		Preflight:     func(context.Context, Plan) error { return nil },
		RecoveryPoint: func(context.Context, Delta) error { return nil },
		Apply:         func(context.Context, Delta) error { applied++; return nil },
		Verify:        func(context.Context, Delta) error { return nil },
		Record: func(_ context.Context, _ Delta, state string) error {
			if failReceipt && state == "verified" {
				return errors.New("external receipt unavailable")
			}
			return nil
		},
	}
	if err := ExecuteJournaled(context.Background(), plan, path, hooks); err == nil {
		t.Fatal("accepted missing external receipt")
	}
	journal, err := LoadJournal(path, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if journal.Steps[JournalKey(plan.Deltas[0])] == "verified" {
		t.Fatal("persisted verified without external receipt")
	}
	failReceipt = false
	if err := ExecuteJournaled(context.Background(), plan, path, hooks); err != nil {
		t.Fatal(err)
	}
	journal, err = LoadJournal(path, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if journal.Steps[JournalKey(plan.Deltas[0])] != "verified" || applied != 2 {
		t.Fatalf("recovery did not replay safely: %+v, applied=%d", journal, applied)
	}
}

func TestInterruptedJournalCannotReplayWithoutRecoveryHook(t *testing.T) {
 root:=t.TempDir()
 if err:=os.Chmod(root,0700);err!=nil{t.Fatal(err)}
 path:=filepath.Join(root,"journal.json")
 plan,err:=Build("0.4.24",[]Realization{{Kind:Secrets,Installation:"a",Scope:"shared",Instance:"vault",Owner:"baseharbor",Image:"openbao",Digest:digestA,Version:"2.7.0"}},expected())
 if err!=nil{t.Fatal(err)}
 j:=Journal{Release:"0.4.24"}
 if err:=j.Record(path,plan.Deltas[0],"apply_failed");err!=nil{t.Fatal(err)}
 touched:=false
 hooks:=Hooks{
  Preflight:func(context.Context,Plan)error{return nil},
  Apply:func(context.Context,Delta)error{touched=true;return nil},
  Verify:func(context.Context,Delta)error{return nil},
  Record:func(context.Context,Delta,string)error{return nil},
 }
 if err:=ExecuteJournaled(context.Background(),plan,path,hooks);err==nil {t.Fatal("interrupted mutation replayed without recovery")}
 if touched{t.Fatal("unsafe runtime mutation occurred")}
}
