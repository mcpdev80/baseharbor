package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

func externalReceiverPlanFixture(t *testing.T) resolvedApplication {
	t.Helper()
	r := corelessResolvedFixture(t)
	r.FromRepository = false
	r.Store = application.Store{Root: t.TempDir(), Namespace: r.Target.Name}
	r.Manifest = application.WithOTLPTelemetry(r.Manifest, "traces")
	t.Setenv(application.OTLPEndpointEnv, "https://receiver-before.example.test")
	t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer fixture-sensitive-before")
	if err := application.RegisterExternalProviderAt(r.TargetStateRoot, externalprovider.Registration{
		ID: "default", ProviderID: "company/otlp", Endpoint: application.ExternalOTLPEndpoint(),
		Provider: capability.ExternalOTLP,
		Trust:    externalprovider.Trust{Mode: externalprovider.TrustSystem},
	}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	f := application.RuntimeFilesFor(r.Store, r.Manifest)
	if err := os.MkdirAll(f.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{f.Env, f.ApplicationEnv} {
		if err := os.WriteFile(p, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := application.MaterializeExternalOTLPBinding(r.Manifest, f, application.ExternalOTLPEndpoint(), os.Getenv(telemetry.ExternalHeadersEnv)); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestExternalReceiverChangeRequiresCurrentPlanBeforeAnyDeliveryMutation(t *testing.T) {
	for _, change := range []string{"endpoint", "authorization", "anonymous", "missing-authorization", "credential-url"} {
		t.Run(change, func(t *testing.T) {
			r := externalReceiverPlanFixture(t)
			f := application.RuntimeFilesFor(r.Store, r.Manifest)
			paths := []string{f.Env, f.ApplicationEnv, filepath.Join(r.TargetStateRoot, "provider-registry.json")}
			before := map[string][]byte{}
			for _, path := range paths {
				before[path], _ = os.ReadFile(path)
			}
			switch change {
			case "endpoint":
				t.Setenv(application.OTLPEndpointEnv, "https://receiver-after.example.test")
				t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer fixture-sensitive-after")
			case "authorization":
				t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer fixture-sensitive-after")
			case "anonymous":
				t.Setenv(telemetry.ExternalHeadersEnv, "")
			case "missing-authorization":
				t.Setenv(application.OTLPEndpointEnv, "https://receiver-after.example.test")
				if err := os.Unsetenv(telemetry.ExternalHeadersEnv); err != nil {
					t.Fatal(err)
				}
			case "credential-url":
				t.Setenv(application.OTLPEndpointEnv, "https://user:fixture-sensitive-url@receiver-after.example.test/?access=fixture-sensitive-query")
			}
			plan, err := buildResolvedApplicationPlan(t.Context(), r)
			if err != nil || plan.ProviderPlan.Impact == nil {
				t.Fatalf("receiver/material change lacked concrete consequence disclosure: %v", err)
			}
			impact := plan.ProviderPlan.Impact
			if impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
				t.Fatal("unknown delivery consequences lacked two current confirmations")
			}
			encoded, _ := json.Marshal(plan)
			if strings.Contains(string(encoded), "fixture-sensitive") {
				t.Fatal("consequence plan disclosed authorization")
			}
			for _, consent := range []*application.ProviderChangeConsent{nil, {PlanID: impact.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}, {PlanID: impact.PlanID, First: true, Second: true}} {
				err := authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), consent), r, nil, io.Discard)
				if consent != nil && consent.PlanID == impact.PlanID && consent.Second && change != "endpoint" && change != "missing-authorization" && change != "credential-url" {
					if err != nil {
						t.Fatalf("supported explicitly acknowledged delivery permanently refused: %v", err)
					}
				} else {
					var failure *machine.Error
					if !errors.As(err, &failure) {
						t.Fatalf("invalid consent/material crossed execution admission: %v", err)
					}
					if consent != nil && consent.PlanID == impact.PlanID && consent.Second {
						if failure.CauseCode != "external_provider_material_preflight_failed" || failure.Details == nil {
							t.Fatal("invalid material lost its concrete acknowledged consequence plan")
						}
					} else if failure.Code != machine.ErrorApprovalRequired && failure.Code != machine.ErrorConflict {
						t.Fatal("technical preflight hid the current two-stage acknowledgement")
					}
				}
				for _, path := range paths {
					after, _ := os.ReadFile(path)
					if !bytes.Equal(before[path], after) {
						t.Fatal("read-only planning or admission changed current delivery")
					}
				}
			}
			oldID := impact.PlanID
			t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer another-selected-value")
			fresh, err := buildResolvedApplicationPlan(t.Context(), r)
			if err != nil || fresh.ProviderPlan.Impact == nil || fresh.ProviderPlan.Impact.PlanID == oldID {
				t.Fatal("changed selected material did not invalidate prior consent")
			}
			stale := application.ProviderChangeConsent{PlanID: oldID, First: true, Second: true}
			err = authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), &stale), r, nil, io.Discard)
			var staleFailure *machine.Error
			if !errors.As(err, &staleFailure) || staleFailure.Code != machine.ErrorConflict {
				t.Fatalf("prior material consent remained usable: %v", err)
			}
		})
	}
}

func TestExternalSameReceiverOmittedAuthorizationHasNoInventedChange(t *testing.T) {
	r := externalReceiverPlanFixture(t)
	if err := os.Unsetenv(telemetry.ExternalHeadersEnv); err != nil {
		t.Fatal(err)
	}
	change, err := applicationExternalReceiverChange(r)
	if err != nil || change != nil {
		t.Fatal("same-receiver retained authorization invented a destructive replacement")
	}
}

func TestRegisteredReceiverReplacementAndMaterialDriftRequireFreshConsent(t *testing.T) {
	r := externalReceiverPlanFixture(t)
	f := application.RuntimeFilesFor(r.Store, r.Manifest)
	credential := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(credential, []byte(`{"token":"destination-private-token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := application.RegisterExternalProviderAt(r.TargetStateRoot, externalprovider.Registration{ID: "destination-alias", ProviderID: "company/destination", Provider: capability.ExternalOTLP, Endpoint: "https://destination.example.test", CredentialRef: "file://" + credential, Trust: externalprovider.Trust{Mode: externalprovider.TrustSystem}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.OTLPEndpointEnv, "")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "destination-alias")
	if err := os.Unsetenv(telemetry.ExternalHeadersEnv); err != nil {
		t.Fatal(err)
	}
	plan, err := buildResolvedApplicationPlan(t.Context(), r)
	if err != nil || plan.ProviderPlan.Impact == nil {
		t.Fatalf("registered destination has no current impact: %v", err)
	}
	impact := plan.ProviderPlan.Impact
	if impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
		t.Fatal("unknown destination bypassed attention")
	}
	consent := &application.ProviderChangeConsent{PlanID: impact.PlanID, First: true, Second: true}
	before, _ := os.ReadFile(f.Env)
	if err := authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), consent), r, nil, io.Discard); err != nil {
		t.Fatalf("supported registered destination permanently denied: %v", err)
	}
	after, _ := os.ReadFile(f.Env)
	if !bytes.Equal(before, after) {
		t.Fatal("admission mutated delivery")
	}
	// Use the existing external ID for this second check: replacing the secure
	// file at the same reference must still change consent, not just its ID.
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	driver := telemetry.NewDriverAt(nil, r.Manifest, f, nil, r.TargetStateRoot, r.Target.Name)
	if err := driver.Bind(t.Context(), capability.Resource{Provider: capability.ProviderExternalOTLP}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credential, []byte(`{"token":"first-rotated-private-token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err = buildResolvedApplicationPlan(t.Context(), r)
	if err != nil || plan.ProviderPlan.Impact == nil {
		t.Fatal("same-ID credential change omitted impact")
	}
	old := plan.ProviderPlan.Impact.PlanID
	if err := os.WriteFile(credential, []byte(`{"token":"rotated-private-token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	fresh, err := buildResolvedApplicationPlan(t.Context(), r)
	if err != nil || fresh.ProviderPlan.Impact == nil || fresh.ProviderPlan.Impact.PlanID == old {
		t.Fatal("same-reference credential drift retained consent")
	}
	encoded, _ := json.Marshal(fresh)
	if strings.Contains(string(encoded), "private-token") {
		t.Fatal("private material escaped into plan")
	}
	stale := &application.ProviderChangeConsent{PlanID: old, First: true, Second: true}
	if err := authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), stale), r, nil, io.Discard); err == nil {
		t.Fatal("stale material consent admitted")
	}
}

func TestManagedCollectorAndRegisteredReceiverReplacementRequireCurrentConsent(t *testing.T) {
	for _, direction := range []string{"managed-to-registered", "registered-to-managed"} {
		t.Run(direction, func(t *testing.T) {
			r := externalReceiverPlanFixture(t)
			f := application.RuntimeFilesFor(r.Store, r.Manifest)
			if direction == "managed-to-registered" {
				t.Setenv(application.OTLPEndpointEnv, "")
				t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "")
				if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
					t.Fatal(err)
				}
				if err := application.MaterializeOTLPBinding(r.Manifest, f, capability.ProviderOTelCollector, "https://managed.example.test", "https://managed.example.test"); err != nil {
					t.Fatal(err)
				}
				t.Setenv(application.OTLPEndpointEnv, "https://receiver-before.example.test")
				t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer explicitly-selected")
			} else {
				t.Setenv(application.OTLPEndpointEnv, "")
				t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "")
				t.Setenv(telemetry.ExternalHeadersEnv, "")
			}
			paths := []string{f.Env, f.ApplicationEnv, filepath.Join(r.TargetStateRoot, "provider-registry.json"), filepath.Join(r.TargetStateRoot, "external-providers.json")}
			before := map[string][]byte{}
			for _, p := range paths {
				before[p], _ = os.ReadFile(p)
			}
			plan, err := buildResolvedApplicationPlan(t.Context(), r)
			if err != nil || plan.ProviderPlan.Impact == nil {
				t.Fatalf("actual receiver replacement omitted consequences: %v", err)
			}
			impact := plan.ProviderPlan.Impact
			if impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
				t.Fatal("receiver replacement lacked current two-stage consequences")
			}
			for _, consent := range []*application.ProviderChangeConsent{nil, {PlanID: impact.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}, {PlanID: impact.PlanID, First: true, Second: true}} {
				err := authorizeApplicationProviderChanges(withProviderChangeConsent(machineNoninteractiveContext(t.Context()), consent), r, nil, io.Discard)
				if consent != nil && consent.PlanID == impact.PlanID && consent.Second {
					if err != nil {
						t.Fatalf("implemented receiver adapter permanently denied: %v", err)
					}
				} else if err == nil {
					t.Fatal("receiver replacement bypassed current consent")
				}
				for _, p := range paths {
					after, _ := os.ReadFile(p)
					if !bytes.Equal(before[p], after) {
						t.Fatal("planning/admission changed current or foreign delivery")
					}
				}
			}
		})
	}
}
