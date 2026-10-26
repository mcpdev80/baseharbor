// ROLLE: REVIEW. Append via overlay to cmf20_r1_test.go; product sources stay unchanged.
func TestCMF21StructuredCLIJSONRetainsMaterializationContext(t *testing.T) {
	resolved := cmf19WizardFixture(t)
	resolved.Target.RuntimeProvider = "cmf21-unsupported-runtime"
	observed, diagnostic := observeApplicationProviderFactsWithDiagnostics(context.Background(), resolved)
	_, registry := cmf19Registry(t, resolved)
	id := registry.Instances[0].ID
	fact, found := observed[id]
	if !found || fact.Containers.Value != nil || fact.Containers.Classification != "unknown" {
		t.Fatalf("projection must have unknown container quantity: %+v", fact)
	}
	plan := application.ProviderPlan{Version: application.ProviderPlanVersion, Target: resolved.Target.Name, Complete: true,
		Changes: []application.ProviderChange{{InstanceID: id, Ownership: capability.OwnershipBaseHarbor, Action: "stop"}}}
	retirement := withMaterializationDiagnostics(verifyApplicationRuntimeRetirement(resolved, plan, observed), diagnostic)
	if retirement == nil { t.Fatal("preview authorized retirement") }
	// Force the ordinary retained-owner rejection while retaining the bound
	// instance that produces the materialization diagnostic.
	store, registry := cmf19Registry(t, resolved)
	retained := registry.Instances[0]
	retained.ID = application.ApplicationProviderInstanceIDForScopedPostgreSQL(resolved.Manifest.ApplicationID, resolved.Manifest.Environment, "retained")
	if err := registry.Register(retained); err != nil { t.Fatal(err) }
	if err := store.Save(registry); err != nil { t.Fatal(err) }
	ownership := checkRetainedApplicationProviderOwnership(context.Background(), resolved)
	if ownership == nil { t.Fatal("unverified retained owner authorized") }
	for name, resultErr := range map[string]error{"retirement": retirement, "ownership": ownership} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			// Match main.go's real CLI classification and JSON encoder.
			if err := writeJSON(&out, machine.ResultError(classifyMachineCLIError(resultErr))); err != nil { t.Fatal(err) }
			var decoded struct { Error struct {
				Cause string `json:"cause"`
				Details struct { Diagnostics []struct {
					Code string `json:"code"`
					Cause string `json:"cause"`
					Resource string `json:"resource"`
					Details struct { Target string `json:"target"`; Instance string `json:"instance"`; ApplicationID string `json:"application_id"` } `json:"details"`
				} `json:"materialization_diagnostics"` } `json:"details"`
			} `json:"error"` }
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil { t.Fatal(err) }
			want := "runtime_retirement_unverified"
			if name == "ownership" { want = "retained_provider_ownership_unverified" }
			if decoded.Error.Cause != want { t.Fatalf("outer cause: %s", out.String()) }
			if len(decoded.Error.Details.Diagnostics) == 0 { t.Fatalf("missing structured diagnostic: %s", out.String()) }
			for _, d := range decoded.Error.Details.Diagnostics {
			if d.Code != string(machine.ErrorRuntimeUnavailable) || d.Cause != causeRuntimeInventoryNotYetMaterialized || d.Resource != id || d.Details.Instance != id || d.Details.Target != resolved.Target.Name || d.Details.ApplicationID != resolved.Manifest.ApplicationID {
				t.Fatalf("lost typed context: %s", out.String())
			}
			}
			t.Logf("serialized CLI machine result: %s", out.String())
		})
	}
}
