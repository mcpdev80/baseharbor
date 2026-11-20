package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestNativeBackendBindingInspectionManifestRestoresMongoResources(t *testing.T) {
	manifest := application.New("selective-foundation", "dev", false, false, false)
	instance := capability.ProviderInstance{
		ID: "mongo-instance", Provider: capability.MongoDB,
		Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor,
		OwnerApplicationID: manifest.ApplicationID, OwnerEnvironment: manifest.Environment,
	}
	bindings := []capability.ProviderBinding{{
		ApplicationID: manifest.ApplicationID, Environment: manifest.Environment, ProviderInstanceID: instance.ID,
		Resource: capability.Resource{Application: manifest.Name, Kind: capability.DocumentDatabase, Name: "records", Provider: capability.ProviderMongoDB},
	}, {
		ApplicationID: manifest.ApplicationID, Environment: manifest.Environment, ProviderInstanceID: instance.ID,
		Resource: capability.Resource{Application: manifest.Name, Kind: capability.DocumentDatabase, Name: "audit", Provider: capability.ProviderMongoDB},
	}}

	inspection, ok := nativeBackendBindingInspectionManifest(manifest, instance, bindings)
	if !ok {
		t.Fatal("canonical retained MongoDB binding did not produce an inspection manifest")
	}
	resources, err := application.NativeBackendInstanceResources(inspection, application.RuntimeFiles{Project: "selective-foundation"}, instance)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) == 0 {
		t.Fatal("canonical retained MongoDB binding produced no native inspection resources")
	}
	found := map[string]bool{}
	for _, resource := range resources {
		if resource.Kind == "container" {
			found[resource.Name] = true
		}
	}
	for _, name := range []string{"selective-foundation-mongodb-audit-1", "selective-foundation-mongodb-records-1"} {
		if !found[name] {
			t.Fatalf("MongoDB resources do not include every canonical retained instance; missing %s in %+v", name, resources)
		}
	}
}

func TestNativeBackendBindingInspectionManifestRejectsUnboundMongo(t *testing.T) {
	manifest := application.New("selective-foundation", "dev", false, false, false)
	instance := capability.ProviderInstance{
		ID: "mongo-instance", Provider: capability.MongoDB,
		Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor,
		OwnerApplicationID: manifest.ApplicationID, OwnerEnvironment: manifest.Environment,
	}
	valid := capability.ProviderBinding{
		ApplicationID: manifest.ApplicationID, Environment: manifest.Environment, ProviderInstanceID: instance.ID,
		Resource: capability.Resource{Application: manifest.Name, Kind: capability.DocumentDatabase, Name: "records", Provider: capability.ProviderMongoDB},
	}
	for _, tc := range []struct {
		name string
		edit func(*capability.ProviderInstance, *capability.ProviderBinding)
	}{
		{name: "wrong instance", edit: func(_ *capability.ProviderInstance, b *capability.ProviderBinding) { b.ProviderInstanceID = "other" }},
		{name: "wrong app", edit: func(_ *capability.ProviderInstance, b *capability.ProviderBinding) { b.ApplicationID = "other" }},
		{name: "wrong environment", edit: func(_ *capability.ProviderInstance, b *capability.ProviderBinding) { b.Environment = "production" }},
		{name: "wrong resource kind", edit: func(_ *capability.ProviderInstance, b *capability.ProviderBinding) { b.Resource.Kind = capability.SQL }},
		{name: "wrong resource app", edit: func(_ *capability.ProviderInstance, b *capability.ProviderBinding) { b.Resource.Application = "other" }},
		{name: "wrong resource provider", edit: func(_ *capability.ProviderInstance, b *capability.ProviderBinding) {
			b.Resource.Provider = capability.ProviderPostgreSQL
		}},
		{name: "empty resource name", edit: func(_ *capability.ProviderInstance, b *capability.ProviderBinding) { b.Resource.Name = "" }},
		{name: "foreign owner", edit: func(i *capability.ProviderInstance, _ *capability.ProviderBinding) {
			i.Ownership = capability.OwnershipExternal
		}},
		{name: "foreign environment owner", edit: func(i *capability.ProviderInstance, _ *capability.ProviderBinding) { i.OwnerEnvironment = "production" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate, binding := instance, valid
			tc.edit(&candidate, &binding)
			if _, ok := nativeBackendBindingInspectionManifest(manifest, candidate, []capability.ProviderBinding{binding}); ok {
				t.Fatal("untrusted or mismatched MongoDB binding was accepted")
			}
		})
	}
	for _, tc := range []struct {
		name string
		edit func(*capability.ProviderBinding)
	}{
		{name: "conflicting app", edit: func(b *capability.ProviderBinding) { b.ApplicationID = "other" }},
		{name: "conflicting environment", edit: func(b *capability.ProviderBinding) { b.Environment = "production" }},
		{name: "unscoped legacy environment", edit: func(b *capability.ProviderBinding) { b.Environment = "" }},
	} {
		t.Run("valid plus "+tc.name, func(t *testing.T) {
			conflict := valid
			tc.edit(&conflict)
			if _, ok := nativeBackendBindingInspectionManifest(manifest, instance, []capability.ProviderBinding{valid, conflict}); ok {
				t.Fatal("partially inconsistent MongoDB binding set was accepted")
			}
		})
	}
}
