package goadapter

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestGoAdapterPlansCommonCapabilityMatrix(t *testing.T) {
	manifest := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "catalog",
		Environment: "dev",
		Services: application.Services{
			SQL:           true,
			Cache:         true,
			ObjectStorage: true,
			Secrets:       true,
		},
		Secrets: application.SecretRequirements{
			Required: []application.SecretRequirement{{Name: "APP_SECRET"}},
		},
		Exposures: []application.HTTPExposureRequirement{{
			Name: "web", Service: "app", Port: 8080, Protocol: "http", Visibility: "public",
		}},
		Telemetry: application.TelemetryRequirements{
			OTLP: &application.OTLPRequirement{Signals: []string{"traces"}},
		},
		Workload: application.WorkloadConfig{Compose: "compose.yaml", Services: []string{"app"}},
	}
	contract, err := application.PortableContractFromManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	profile := development.StackProfile{
		SchemaVersion: development.StackProfileVersion,
		Name:          "go-api",
		Components:    []development.Component{{ID: "app", Role: "backend", Adapter: AdapterID}},
	}
	actions, err := (Adapter{}).Plan(contract, profile, profile.Components[0])
	if err != nil {
		t.Fatal(err)
	}
	requireAction(t, actions, development.ActionDependency, capability.SQL, "github.com/jackc/pgx/v5")
	requireAction(t, actions, development.ActionDependency, capability.KeyValue, "github.com/redis/go-redis/v9")
	requireAction(t, actions, development.ActionDependency, capability.ObjectStorageS3, "github.com/aws/aws-sdk-go-v2/service/s3")
	requireAction(t, actions, development.ActionDependency, capability.TelemetryOTLP, "go.opentelemetry.io/otel")
	requireAction(t, actions, development.ActionBinding, capability.Secrets, "APP_SECRET")
}

func TestGoAdapterBootstrapIsEcosystemNative(t *testing.T) {
	profile := development.StackProfile{
		SchemaVersion: development.StackProfileVersion,
		Name:          "go-api",
		Components:    []development.Component{{ID: "app", Role: "backend", Adapter: AdapterID}},
	}
	contract := application.PortableContract{
		Application: "catalog",
		Capabilities: []capability.Requirement{
			{Kind: capability.ExposureHTTP, Name: "web"},
			{Kind: capability.SQL, Name: "default"},
		},
	}
	registry, err := development.NewRegistry(Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := development.BuildPlan(contract, profile, registry)
	if err != nil {
		t.Fatal(err)
	}
	files, err := (Adapter{}).Bootstrap(plan, profile.Components[0])
	if err != nil {
		t.Fatal(err)
	}
	content := map[string]string{}
	for _, file := range files {
		content[file.Path] = string(file.Content)
	}
	for _, required := range []string{"go.mod", "main.go", "Dockerfile", "compose.yaml", ".env.example"} {
		if _, ok := content[required]; !ok {
			t.Fatalf("generated files missing %s", required)
		}
	}
	if strings.Contains(content["main.go"], "baseharbor/") || strings.Contains(content["main.go"], "baha.") {
		t.Fatal("generated application must not depend on a BaseHarbor application SDK")
	}
	if !strings.Contains(content["go.mod"], "github.com/jackc/pgx/v5 "+pgxVersion) {
		t.Fatal("generated go.mod does not contain pinned pgx dependency")
	}
	if !strings.Contains(content["compose.yaml"], "/healthz") {
		t.Fatal("generated compose file does not include workload healthcheck")
	}
}

func requireAction(t *testing.T, actions []development.Action, kind development.ActionKind, capabilityKind capability.Kind, name string) {
	t.Helper()
	for _, action := range actions {
		if action.Kind == kind && action.Capability == capabilityKind && action.Name == name {
			return
		}
	}
	t.Fatalf("missing action kind=%s capability=%s name=%s: %#v", kind, capabilityKind, name, actions)
}
