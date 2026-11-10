package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func minimalRepositorySelection(t *testing.T) application.RepositoryEnvironmentSelection {
	t.Helper()
	root := t.TempDir()
	authored := "version: 1\napp:\n  name: plain\n  environment: dev\n"
	if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte(authored), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: alpine:3.22\n    command: [sleep, infinity]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	selection, err := application.ResolveRepositoryEnvironment(root, "")
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func TestMinimalRepositoryIdentityInitializationAndReadSurfaces(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	authored, _ := os.ReadFile(selection.ManifestPath)
	for _, command := range []string{"plan", "status", "doctor", "stop", "destroy"} {
		_, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, command)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.CauseCode != "application_initialization_required" {
			t.Fatalf("%s: %v", command, err)
		}
	}
	if _, err := os.Stat(filepath.Join(selection.RepositoryRoot, ".baseharbor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only surface initialized identity: %v", err)
	}
	initialized, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, "init")
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Manifest.ApplicationID == "" || initialized.Manifest.Services.SQL || !application.HasExplicitWorkload(initialized.Manifest) {
		t.Fatalf("incorrect minimal realization: %+v", initialized.Manifest)
	}
	for _, command := range []string{"init", "apply", "up", "plan", "status", "doctor", "stop", "destroy"} {
		next, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, command)
		if err != nil || next.Manifest.ApplicationID != initialized.Manifest.ApplicationID {
			t.Fatalf("%s identity drift: %v", command, err)
		}
	}
	after, _ := os.ReadFile(selection.ManifestPath)
	if string(after) != string(authored) {
		t.Fatal("authored intent rewritten")
	}
}

func TestMinimalRepositoryMissingSourceDoesNotInitialize(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	if err := os.Remove(filepath.Join(selection.RepositoryRoot, "compose.yaml")); err != nil {
		t.Fatal(err)
	}
	_, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, "apply")
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.CauseCode != "workload_missing" {
		t.Fatalf("missing actionable workload error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(selection.RepositoryRoot, ".baseharbor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source allocated state: %v", err)
	}
}

func TestMinimalInitializationRejectsCorruptTargetBeforeAllocating(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateRoot, "provider-registry.json"), []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, "apply"); err == nil {
		t.Fatal("corrupt Target accepted for identity mutation")
	}
	if _, err := os.Stat(filepath.Join(selection.RepositoryRoot, ".baseharbor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt Target allocated identity: %v", err)
	}
}

func TestProviderPreflightRevalidatesUploadedSourceWithCoreOwnedIdentity(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	// This resembles an immutable workstation upload: authored YAML has no
	// machine-owned ID and no local Core identity store in its source root.
	if selection.Manifest.ApplicationID != "" {
		t.Fatal("authored fixture unexpectedly claims an application identity")
	}
	owned := application.WithWorkloadComponents(application.New("plain", "dev", false, false, false), "api")
	resolved := resolvedApplication{
		Target:   target,
		Manifest: owned,
		DeploymentIdentity: deployment.DeploymentIdentity{
			Target: target.Name, ApplicationID: owned.ApplicationID,
			Application: owned.Name, Environment: owned.Environment,
		},
	}
	bound, err := bindAuthoritativeProviderRevalidationIdentity(resolved, selection)
	if err != nil || bound.Manifest.ApplicationID != owned.ApplicationID {
		t.Fatalf("Core-owned source identity was not preserved: %+v, %v", bound.Manifest, err)
	}
	validated, err := resolveRepositoryLifecycleIntent(context.Background(), target, bound, "plan")
	if err != nil || validated.Manifest.ApplicationID != owned.ApplicationID {
		t.Fatalf("read-only Core source preflight lost the identity: %+v, %v", validated.Manifest, err)
	}
	if _, err := os.Stat(filepath.Join(selection.RepositoryRoot, ".baseharbor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only source validation created local identity state: %v", err)
	}
	for name, mutate := range map[string]func(*application.RepositoryEnvironmentSelection){
		"foreign-application": func(s *application.RepositoryEnvironmentSelection) { s.Manifest.Name = "foreign" },
		"foreign-environment": func(s *application.RepositoryEnvironmentSelection) { s.Manifest.Environment = "prod" },
		"foreign-identity": func(s *application.RepositoryEnvironmentSelection) {
			s.Manifest.ApplicationID = application.MustNewApplicationID()
		},
	} {
		t.Run(name, func(t *testing.T) {
			foreign := selection
			mutate(&foreign)
			if _, err := bindAuthoritativeProviderRevalidationIdentity(resolved, foreign); err == nil {
				t.Fatal("provider source revalidation admitted foreign ownership")
			}
		})
	}
}

func TestVerifiedHTTPSourceProviderPreflightUsesSameCoreIdentityProjection(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	m := application.WithWorkloadComponents(application.New("plain", "dev", false, false, false), "api")
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{}
	resolved.Target = target
	resolved.TargetStateRoot = root
	resolved.Manifest = m
	resolved.DeploymentIdentity = deployment.DeploymentIdentity{
		Target: target.Name,
		ApplicationID: m.ApplicationID,
		Application: m.Name,
		Environment: m.Environment,
	}
	prepared, err := prepareVerifiedHTTPSource(context.Background(), resolved, selection)
	if err != nil {
		t.Fatalf("Core rejected its own verified uploaded source: %v", err)
	}
	if prepared.Manifest.ApplicationID != m.ApplicationID || !prepared.FromRepository || !prepared.SourceAvailable {
		t.Fatalf("prepared source lost the authorized Core identity: %+v", prepared)
	}
	if err := revalidateApplicationProviderIntent(context.Background(), prepared); err != nil {
		t.Fatalf("identical uploaded source was rejected as stale provider consent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(selection.RepositoryRoot, ".baseharbor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source normalization wrote workstation-owned identity state: %v", err)
	}
	foreign := selection
	foreign.Manifest.Name = "foreign"
	if _, err := prepareVerifiedHTTPSource(context.Background(), resolved, foreign); err == nil {
		t.Fatal("foreign source was accepted as the registered Core application")
	}
}
