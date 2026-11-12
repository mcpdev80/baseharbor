package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestQuickAppInitCarriesExplicitComposeSecretSources(t *testing.T) {
	detection := appProjectDetection{
		Name:             "demo",
		WorkloadServices: []string{"demo-app"},
		SecretCandidates: []string{"DEMO_MANAGEMENT_PASSWORD", "DEMO_MANAGEMENT_USERNAME"},
		SecretSourcePolicies: map[string]string{
			"DEMO_MANAGEMENT_USERNAME": "target-development-username",
			"DEMO_MANAGEMENT_PASSWORD": "target-development-password",
		},
	}
	manifest, err := manifestFromDetectedProject(detection, true)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Services.Secrets {
		t.Fatal("explicit source declarations did not enable managed secrets")
	}
	got := map[string]application.SecretSource{}
	for _, requirement := range manifest.Secrets.Required {
		got[requirement.Name] = requirement.Source
	}
	want := map[string]application.SecretSource{
		"DEMO_MANAGEMENT_USERNAME": application.SecretSourceTargetDevelopmentUsername,
		"DEMO_MANAGEMENT_PASSWORD": application.SecretSourceTargetDevelopmentPassword,
	}
	if len(got) != len(want) {
		t.Fatalf("quick init secret requirements = %#v", got)
	}
	for name, source := range want {
		if got[name] != source {
			t.Fatalf("quick init source for %s = %q, want %q", name, got[name], source)
		}
	}
}

func TestDetectAppProjectReadsComposeManagedSecretSources(t *testing.T) {
	root := t.TempDir()
	compose := `services:
  demo-app:
    build: .
    ports:
      - "8080:8080"
    environment:
      DEMO_MANAGEMENT_USERNAME: ${DEMO_MANAGEMENT_USERNAME:-developer}
      DEMO_MANAGEMENT_PASSWORD: ${DEMO_MANAGEMENT_PASSWORD:-}
x-baseharbor:
  secrets:
    - name: DEMO_MANAGEMENT_USERNAME
      source: target-development-username
    - name: DEMO_MANAGEMENT_PASSWORD
      source: target-development-password
`
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	detection, err := detectAppProject(root)
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"DEMO_MANAGEMENT_USERNAME": "target-development-username",
		"DEMO_MANAGEMENT_PASSWORD": "target-development-password",
	} {
		if detection.SecretSourcePolicies[name] != source {
			t.Fatalf("detected source for %s = %q, want %q", name, detection.SecretSourcePolicies[name], source)
		}
	}
}
