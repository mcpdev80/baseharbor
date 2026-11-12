package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
)

func TestTargetDevelopmentCredentialSecretValue(t *testing.T) {
	credentials := devaccess.Credentials{Username: "developer", Password: "random-target-value"}
	for source, want := range map[application.SecretSource]string{
		application.SecretSourceTargetDevelopmentUsername: "developer",
		application.SecretSourceTargetDevelopmentPassword: "random-target-value",
	} {
		got, err := targetDevelopmentCredentialSecretValue(source, credentials)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("source %q returned wrong value", source)
		}
	}
	if _, err := targetDevelopmentCredentialSecretValue("unknown", credentials); err == nil {
		t.Fatal("unknown source should fail closed")
	}
	if _, err := targetDevelopmentCredentialSecretValue(application.SecretSourceTargetDevelopmentPassword, devaccess.Credentials{Username: "developer"}); err == nil {
		t.Fatal("empty Target password should fail closed")
	}
}

func TestManifestWithoutManagedSecretSourcesKeepsUserSecrets(t *testing.T) {
	manifest := application.Manifest{Secrets: application.SecretRequirements{Required: []application.SecretRequirement{
		{Name: "APP_SECRET"},
		{Name: "TARGET_USER", Source: application.SecretSourceTargetDevelopmentUsername},
	}}}
	filtered := manifestWithoutManagedSecretSources(manifest)
	if len(filtered.Secrets.Required) != 1 || filtered.Secrets.Required[0].Name != "APP_SECRET" {
		t.Fatalf("filtered required secrets = %#v", filtered.Secrets.Required)
	}
	if len(manifest.Secrets.Required) != 2 {
		t.Fatal("filter mutated the original application manifest")
	}
}
