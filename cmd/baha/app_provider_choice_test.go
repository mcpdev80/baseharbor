package main

import (
 "reflect"
 "strings"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/capability"
)

func TestAppProviderChoiceAutomaticAndExplicit(t *testing.T) {
 sel:=newAppCapabilitySelection(appCapabilitySelectionInput{Detected:[]capability.Kind{capability.SQL}})
 reg:=capability.NewRegistry()
 reg.Instances=[]capability.ProviderInstance{{ID:"sql-one",Provider:capability.Provider{Kind:capability.ProviderPostgreSQL,Capabilities:[]capability.Kind{capability.SQL}},Scope:capability.ScopeShared}}
 choices,err:=appProviderChoices("dev","app",sel,reg)
 if err!=nil||len(choices)!=1 {t.Fatalf("choices=%v err=%v",choices,err)}
 c:=choices[0]
 if c.Preference().InstanceID!=""||c.ProposedInstanceID!="sql-one" {t.Fatalf("automatic selection: %+v",c)}
 instance,err:=c.Effective()
 if err!=nil||instance.ID!="sql-one" {t.Fatalf("effective=%+v err=%v",instance,err)}
 c,err=c.Select("sql-one")
 if err!=nil||c.Preference().InstanceID!="sql-one" {t.Fatalf("explicit=%+v err=%v",c,err)}
 if !strings.Contains(c.View(),"postgresql")||!strings.Contains(c.View(),"Reason:") {t.Fatalf("missing reason: %s",c.View())}
 c,err=c.Select("")
 if err!=nil||c.Preference().InstanceID!="" {t.Fatal("Automatic should reset preference")}
}

func TestAppProviderChoiceAmbiguousAndIncompatible(t *testing.T) {
 sel:=newAppCapabilitySelection(appCapabilitySelectionInput{Detected:[]capability.Kind{capability.SQL}})
 reg:=capability.NewRegistry()
 provider:=capability.Provider{Kind:capability.ProviderPostgreSQL,Capabilities:[]capability.Kind{capability.SQL}}
 reg.Instances=[]capability.ProviderInstance{
  {ID:"sql-a",Provider:provider,Scope:capability.ScopeShared},
  {ID:"sql-b",Provider:provider,Scope:capability.ScopeExternal},
  {ID:"cache",Provider:capability.Provider{Kind:capability.ProviderValkey,Capabilities:[]capability.Kind{capability.KeyValue}},Scope:capability.ScopeShared},
  {ID:"other-app",Provider:provider,Scope:capability.ScopeApplication,OwnerApplicationID:"other"},
 }
 choices,err:=appProviderChoices("dev","mine",sel,reg)
 if err!=nil||len(choices[0].Candidates)!=2 {t.Fatalf("choices=%+v err=%v",choices,err)}
 c:=choices[0]
 if _,err=c.Effective();err==nil||!strings.Contains(err.Error(),"multiple") {t.Fatalf("ambiguity must fail: %v",err)}
 for _,id:=range []string{"postgresql","cache","other-app","missing"} {
  if _,err=c.Select(id);err==nil {t.Fatalf("incompatible or ambiguous ID %q accepted",id)}
 }
 c,err=c.Select("sql-b")
 if err!=nil {t.Fatal(err)}
 chosen,err:=c.Effective()
 if err!=nil||chosen.ID!="sql-b" {t.Fatalf("explicit effective=%+v err=%v",chosen,err)}
}

func TestAppProviderChoiceNoProviderAndTargetSwitch(t *testing.T) {
 sel:=newAppCapabilitySelection(appCapabilitySelectionInput{Detected:[]capability.Kind{capability.Secrets}})
 empty:=capability.NewRegistry()
 first,err:=appProviderChoices("dev","app",sel,empty)
 if err!=nil {t.Fatal(err)}
 if _,err:=first[0].Effective();err==nil||!strings.Contains(err.Error(),"No compatible") {t.Fatalf("missing provider not reported: %v",err)}
 reg:=capability.NewRegistry()
 reg.Instances=[]capability.ProviderInstance{{ID:"secret-prod",Scope:capability.ScopeExternal,Provider:capability.Provider{Kind:capability.ProviderOpenBao,Capabilities:[]capability.Kind{capability.Secrets}}}}
 prod,err:=appProviderChoices("prod","app",sel,reg)
 if err!=nil {t.Fatal(err)}
 if prod[0].Preference().Target!="prod"||prod[0].Preference().InstanceID!="" {t.Fatal("preferences leaked across targets")}
 pref,err:=prod[0].Select("secret-prod")
 if err!=nil||!reflect.DeepEqual(pref.Preference(),appProviderPreference{Target:"prod",Kind:capability.Secrets,InstanceID:"secret-prod"}) {t.Fatalf("target preference %+v %v",pref.Preference(),err)}
 if first[0].Preference().Target!="dev"||first[0].Preference().InstanceID!="" {t.Fatal("dev preference mutated")}
}

func TestAppProviderChoiceSelectionRequirementsRemainPortable(t *testing.T) {
 sel:=newAppCapabilitySelection(appCapabilitySelectionInput{Current:[]capability.Requirement{{Kind:capability.SQL,Name:"primary"},{Kind:capability.SQL,Name:"analytics"}}})
 registry:=capability.NewRegistry()
 registry.Instances=[]capability.ProviderInstance{{ID:"sql",Scope:capability.ScopeShared,Provider:capability.Provider{Kind:capability.ProviderPostgreSQL,Capabilities:[]capability.Kind{capability.SQL}}}}
 before:=sel.Requirements()
 choices,err:=appProviderChoices("test","app",sel,registry)
 if err!=nil||len(choices)!=2 {t.Fatalf("choices=%+v err=%v",choices,err)}
 _,err=choices[0].Select("sql")
 if err!=nil||!reflect.DeepEqual(sel.Requirements(),before) {t.Fatal("portable requirement changed")}
}
