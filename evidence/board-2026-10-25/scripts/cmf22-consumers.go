// ROLLE: REVIEW. Authorization/receipt consumers must not promote preview projections.
func TestCMF22ProjectionCannotBecomeVerifiedReceiptOrImpact(t *testing.T) {
 resolved := cmf19WizardFixture(t)
 resolved.Target.RuntimeProvider = "cmf22-unsupported-runtime"
 observed, diagnostic := observeApplicationProviderFactsWithDiagnostics(context.Background(), resolved)
 if diagnostic == nil {t.Fatal("fixture must contain materialization diagnostic")}
 _, registry := cmf19Registry(t,resolved)
 id:=registry.Instances[0].ID
 fact:=observed[id]
 if providerFactIsRuntimeMeasurement(fact) {t.Fatal("preview qualifies as measurement")}
 snapshot,_,err:=applicationLifecycleSnapshot(resolved,observed)
 if err!=nil {t.Fatal(err)}
 impact:=observeApplicationProviderImpact(context.Background(),resolved,snapshot)
 if impact.Providers[id].Realization.Verified {t.Fatal("preview-only realization marked verified")}
 for _,operation:=range []string{"stop","remove-runtime","add","resume","start","replace","update"} {
  t.Run(operation,func(t *testing.T) {
   after:=application.ProviderRealization{InstanceID:id,Target:resolved.Target.Name,State:application.ProviderStoppedOwned}
   plan:=application.ProviderImpactPlan{Target:resolved.Target.Name,Impacts:[]application.ProviderImpact{{InstanceID:id,Operation:operation,After:&after}}}
   outcomes:=appApplyObservedOutcomes(plan,applicationApplyLifecycleEvidence{Before:observed,After:observed,Registry:registry})
   for _,outcome:=range outcomes {if outcome.Verified || outcome.Status=="applied" {t.Fatalf("projection produced verified receipt: %+v",outcome)}}
  })
 }
}
