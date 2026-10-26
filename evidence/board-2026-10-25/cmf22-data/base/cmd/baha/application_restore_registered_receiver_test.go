package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func restoreRegisteredReceiverFixture(t *testing.T, retained bool) (resolvedApplication, map[string]string) {
	t.Helper()
	r := externalReceiverPlanFixture(t)
	identity, err := serviceissuer.New(t).Issue(t.Context(), serviceaccess.CertificateRequest{CommonName: "restore-otlp-client"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	paths := map[string]string{}
	for name, material := range map[string][]byte{
		"credential": []byte(`{"token":"restore-private-original-token"}`),
		"trust":      identity.IssuingCA, "certificate": identity.Certificate, "key": identity.PrivateKey,
	} {
		paths[name] = filepath.Join(root, name)
		if err := os.WriteFile(paths[name], material, 0600); err != nil {
			t.Fatal(err)
		}
	}
	reference := func(name string) string { return (&url.URL{Scheme: "file", Path: paths[name]}).String() }
	if err := application.RegisterExternalProviderAt(r.TargetStateRoot, externalprovider.Registration{
		ID: "restore-selected", ProviderID: "company/restore-receiver", Provider: capability.ExternalOTLP,
		Endpoint: "https://receiver-before.example.test", CredentialRef: reference("credential"),
		Trust: externalprovider.Trust{Mode: externalprovider.TrustMTLS, CAReference: reference("trust"), ClientCertificate: reference("certificate"), ClientKey: reference("key")},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.OTLPEndpointEnv, "")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "restore-selected")
	if err := os.Unsetenv(telemetry.ExternalHeadersEnv); err != nil {
		t.Fatal(err)
	}
	f := application.RuntimeFilesFor(r.Store, r.Manifest)
	if retained {
		if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
			t.Fatal(err)
		}
		driver := telemetry.NewDriverAt(nil, r.Manifest, f, nil, r.TargetStateRoot, r.Target.Name)
		if err := driver.Bind(t.Context(), capability.Resource{Provider: capability.ProviderExternalOTLP}, capability.Binding{}); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.RemoveAll(f.Dir); err != nil {
			t.Fatal(err)
		}
		registry := capability.RegistryStore{Path: filepath.Join(r.TargetStateRoot, "provider-registry.json")}
		if err := registry.Update(func(current *capability.Registry) error {
			current.Bindings = nil // Registration authority survives; delivery does not.
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	r.DeploymentIdentity, err = deployment.NewDeploymentIdentity(r.Target.Name, r.Manifest.ApplicationID, r.Manifest.Name, r.Manifest.Environment)
	if err != nil {
		t.Fatal(err)
	}
	r.DeploymentStateRoot, err = deployment.DeploymentRoot(r.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	return r, paths
}

func TestRestoreBindsRegisteredReceiverMaterialWithoutSurvivingDelivery(t *testing.T) {
	for _, retained := range []bool{false, true} {
		for _, change := range []string{"credential", "trust", "certificate", "key"} {
			name := "fresh/"
			if retained {
				name = "retained/"
			}
			t.Run(name+change, func(t *testing.T) {
				r, paths := restoreRegisteredReceiverFixture(t, retained)
				ctx := machineNoninteractiveContext(t.Context())
				data := applicationRestoreData{manifest: r.Manifest, archiveRevision: strings.Repeat("a", 64)}
				planned, err := buildApplicationRestorePlan(ctx, r, data)
				if err != nil {
					t.Fatal(err)
				}
				unchanged, err := buildApplicationRestorePlan(ctx, r, data)
				if err != nil || unchanged.Scope.PlanID != planned.Scope.PlanID {
					t.Fatal("unchanged selected registration did not retain its restore plan")
				}
				files := application.RuntimeFilesFor(r.Store, r.Manifest)
				before := map[string][]byte{}
				for _, path := range []string{files.Env, files.ApplicationEnv, filepath.Join(r.TargetStateRoot, "provider-registry.json")} {
					before[path], _ = os.ReadFile(path)
				}
				material, err := os.ReadFile(paths[change])
				if err != nil {
					t.Fatal(err)
				}
				if change == "credential" {
					material = []byte(`{"token":"restore-private-changed-token"}`)
				} else {
					material = append(material, '\n') // Still valid PEM, same protected reference.
				}
				if err := os.WriteFile(paths[change], material, 0600); err != nil {
					t.Fatal(err)
				}
				current, err := buildApplicationRestorePlan(ctx, r, data)
				if err != nil {
					t.Fatal(err)
				}
				if current.Scope.PlanID == planned.Scope.PlanID {
					t.Fatal("changed selected OTLP credential/CA/client identity retained restore consent")
				}
				found := false
				for _, effect := range planned.Effects {
					found = found || effect.Aspect == "external-telemetry-recovery" && strings.Contains(effect.After, "https://receiver-before.example.test")
				}
				if !found || planned.Scope.Attention != "Achtung" || planned.Scope.ConfirmationsRequired != 2 {
					t.Fatal("selected canonical receiver or current consequence acknowledgements missing")
				}
				ack := &application.ProviderChangeConsent{PlanID: planned.Scope.PlanID, First: true, Second: true, DeleteData: true}
				if err := authorizeApplicationRestore(withProviderChangeConsent(ctx, ack), r, data, io.Discard); err == nil {
					t.Fatal("selected OTLP material drift admitted stale restore acknowledgement")
				}
				for path, original := range before {
					after, _ := os.ReadFile(path)
					if !bytes.Equal(original, after) {
						t.Fatal("restore plan or denial changed protected delivery/registry bytes")
					}
				}
				public, _ := json.Marshal(current)
				for _, private := range []string{"restore-private-", "BEGIN CERTIFICATE", "PRIVATE KEY"} {
					if strings.Contains(string(public), private) {
						t.Fatal("restore plan exposed selected material")
					}
				}
			})
		}
	}
}
