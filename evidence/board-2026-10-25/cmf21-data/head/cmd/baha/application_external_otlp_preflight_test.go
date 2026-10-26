package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestExternalOTLPTraceConsumerDoesNotAcquireManagedTempo(t *testing.T) {
	t.Setenv(application.OTLPProviderEnv, "")
	t.Setenv(application.OTLPEndpointEnv, "")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "native-receiver")
	m, err := application.ParseYAML(nativeOTLPHandoffIntent)
	if err != nil {
		t.Fatal(err)
	}
	m.ApplicationID = application.MustNewApplicationID()
	root := filepath.Join(t.TempDir(), "unallocated-target")
	if err := application.RegisterExternalProviderAt(root, nativeOTLPReceiverRegistration(t.TempDir(), "receiver.fixture.test", 4318)); err != nil {
		t.Fatal(err)
	}
	original := nativeOTLPMaterialSnapshot(t, root)
	resolved := resolvedApplication{Manifest: m, TargetStateRoot: root}
	var runtime bhruntime.RuntimeProvider
	prepared, err := prepareManagedTraces(context.Background(), runtime, resolved, nil)
	if err != nil || prepared != nil {
		t.Fatalf("external OTLP trace export acquired managed storage: %+v %v", prepared, err)
	}
	assertNativeOTLPMaterialSnapshot(t, original)
	if len(nativeOTLPMaterialSnapshot(t, root)) != len(original) {
		t.Fatal("external trace selection allocated managed storage")
	}
	if requiresManagedServiceIssuer(m) {
		t.Fatal("external trace export acquired an unrelated managed PKI dependency")
	}
	var issuer serviceaccess.Issuer
	checks := appendManagedProviderPreflights(nil, &runtime, resolved, &managedProviderPreflightState{}, &issuer)
	for _, check := range checks {
		if check.Name == "managed traces provider" {
			t.Fatal("external OTLP selected a managed Tempo preflight")
		}
	}
	if !application.HasTraceSignal(m) || !application.HasOTLPTelemetry(m) {
		t.Fatal("external selection lost the authored trace export intent")
	}
	// Removing the external binding restores the existing managed prerequisites.
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "")
	if !requiresManagedServiceIssuer(m) {
		t.Fatal("managed return lost its required issuer")
	}
	checks = appendManagedProviderPreflights(nil, &runtime, resolved, &managedProviderPreflightState{}, &issuer)
	found := false
	for _, check := range checks {
		found = found || check.Name == "managed traces provider"
	}
	if !found {
		t.Fatal("managed return lost actual Tempo preflight")
	}
}
