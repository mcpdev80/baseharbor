
// CMF20 reviewer additions; use an overlay, never edit the candidate.
func TestCMF20PreviewProjectionMustNotVerifyRuntimeRetirement(t *testing.T) {
 resolved := cmf19WizardFixture(t)
 resolved.Target.RuntimeProvider = "cmf20-unsupported-runtime"
 facts, diagnostic := observeApplicationProviderFactsWithDiagnostics(context.Background(), resolved)
 if !containsCauseCode(diagnostic, causeRuntimeInventoryNotYetMaterialized) { t.Fatalf("fixture must yield typed diagnostic: %v", diagnostic) }
 _, registry := cmf19Registry(t, resolved)
 id := registry.Instances[0].ID
 if len(facts) == 0 { t.Fatal("fixture must contain the projection") }
 consumed := observeApplicationProviderFacts(context.Background(), resolved)
 plan := application.ProviderPlan{Changes: []application.ProviderChange{{InstanceID:id, Action:"stop"}}}
 if err := verifyApplicationRuntimeRetirement(resolved, plan, consumed); err == nil {
  t.Fatalf("registry-only projection passed actual runtime-retirement verification after wrapper discarded diagnostic: fact=%+v diagnostic=%v", consumed[id], diagnostic)
 }
}

func TestCMF20OwnerFileFailuresStayFailClosed(t *testing.T) {
 for _, mode := range []string{"missing-compose", "missing-env", "symlink-dir", "symlink-compose", "unprotected-dir", "unprotected-compose", "noncanonical-compose", "unreadable-dir"} {
  t.Run(mode,func(t *testing.T) {
   resolved := cmf19Fixture(t)
   cmf19BindAll(t,resolved)
   files := cmf19MaterializeCanonicalFiles(t,resolved)
   switch mode {
   case "missing-compose": if err:=os.Remove(files.Compose);err!=nil{t.Fatal(err)}
   case "missing-env": if err:=os.Remove(files.Env);err!=nil{t.Fatal(err)}
   case "symlink-dir":
    target:=files.Dir+"-real"
    if err:=os.Rename(files.Dir,target);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(target,files.Dir);err!=nil{t.Fatal(err)}
   case "symlink-compose":
    target:=files.Compose+"-real"
    if err:=os.Rename(files.Compose,target);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(target,files.Compose);err!=nil{t.Fatal(err)}
   case "unprotected-dir": if err:=os.Chmod(files.Dir,0777);err!=nil{t.Fatal(err)}
   case "unprotected-compose": if err:=os.Chmod(files.Compose,0666);err!=nil{t.Fatal(err)}
   case "noncanonical-compose": if err:=os.WriteFile(files.Compose,[]byte("services: {}\n"),0600);err!=nil{t.Fatal(err)}
   case "unreadable-dir":
    if err:=os.Chmod(files.Dir,0000);err!=nil{t.Fatal(err)}
    t.Cleanup(func(){os.Chmod(files.Dir,0700)})
   }
   resolved.Target.RuntimeProvider="cmf20-unsupported-runtime"
   facts:=map[string]application.ProviderFact{}
   diagnostic:=observeNativeBackendProviderFacts(context.Background(),resolved,facts)
   if len(facts)!=0 || diagnostic!=nil {t.Fatalf("owner-file failure manufactured projection: facts=%+v diagnostic=%v",facts,diagnostic)}
  })
 }
}
