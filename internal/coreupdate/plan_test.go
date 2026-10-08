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
  {Kind:SQL,Image:"postgres",Digest:digestB,Version:"18.1"},
  {Kind:Secrets,Image:"openbao",Digest:digestB,Version:"2.7.1"},
  {Kind:Identity,Image:"keycloak",Digest:digestB,Version:"26.8.0"},
 }
}

func TestBuildFailsClosedWithoutFullPinnedRelease(t *testing.T) {
 for _, desired := range [][]Desired{nil, expected()[:2], {{Kind:SQL,Image:"postgres:latest",Version:"18"}}} {
  if _,err:=Build("0.4.24",nil,desired); err==nil { t.Fatalf("accepted unpinned/incomplete release: %+v",desired) }
 }
}

func TestPlanPreservesForeignAndUnchangedRealizations(t *testing.T) {
 installed:=[]Realization{
  {Kind:SQL,Installation:"a",Scope:"shared",Instance:"sql",Owner:"baseharbor",Image:"postgres",Digest:digestB,Version:"18.1"},
  {Kind:SQL,Installation:"a",Scope:"isolated",Instance:"app1",Owner:"baseharbor",Image:"postgres",Digest:digestA,Version:"17.9"},
  {Kind:Secrets,Installation:"a",Scope:"shared",Instance:"secrets",Owner:"external",Image:"openbao",Digest:digestA,Version:"2.6.0"},
 }
 plan,err:=Build("0.4.24",installed,expected())
 if err!=nil { t.Fatal(err) }
 if len(plan.Deltas)!=2 || plan.Deltas[0].Classification!=NoChange || plan.Deltas[1].Classification!=Unsupported {
  t.Fatalf("incorrect update classification: %#v",plan.Deltas)
 }
 if err:=Execute(context.Background(),plan,Hooks{Preflight:func(context.Context,Plan)error{return nil},Apply:func(context.Context,Delta)error{return nil},Verify:func(context.Context,Delta)error{return nil},Record:func(context.Context,Delta,string)error{return nil}});err==nil {
  t.Fatal("unsupported PostgreSQL major update was applied")
 }
}

func TestExecutionRequiresRecoveryAndVerifiedResult(t *testing.T) {
 plan,err:=Build("0.4.24",[]Realization{{Kind:Secrets,Installation:"a",Scope:"shared",Instance:"vault",Owner:"baseharbor",Image:"openbao",Digest:digestA,Version:"2.7.0"}},expected())
 if err!=nil {t.Fatal(err)}
 count:=0
 hooks:=Hooks{
  Preflight:func(context.Context,Plan)error{count++;return nil},
  Apply:func(context.Context,Delta)error{count++;return nil},
  Verify:func(context.Context,Delta)error{return errors.New("unready")},
  Record:func(context.Context,Delta,string)error{return nil},
 }
 if err:=Execute(context.Background(),plan,hooks);err==nil || count!=0 {t.Fatalf("missing recovery applied: %v count=%d",err,count)}
 hooks.RecoveryPoint=func(context.Context,Delta)error{count++;return nil}
 if err:=Execute(context.Background(),plan,hooks);err==nil || !strings.Contains(err.Error(),"verification") || count!=3 {t.Fatalf("verification failure missing: %v count=%d",err,count)}
}

func TestBuildRejectsMalformedDigestAndDetectsDifferentImage(t *testing.T) {
 bad := expected()
 bad[0].Digest = "sha256:" + strings.Repeat("z",64)
 if _,err:=Build("0.4.24",nil,bad);err==nil { t.Fatal("accepted non-hex digest") }
 installed := []Realization{{Kind:SQL,Installation:"a",Scope:"shared",Instance:"sql",Owner:"baseharbor",Image:"untrusted-postgres",Digest:digestB,Version:"18.1"}}
 plan,err := Build("0.4.24",installed,expected())
 if err!=nil { t.Fatal(err) }
 if len(plan.Deltas)!=1 || plan.Deltas[0].Classification==NoChange {
  t.Fatalf("same digest with different image must not be treated as unchanged: %+v",plan)
 }
}
