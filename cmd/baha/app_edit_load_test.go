package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

const appEditLoadYAML = "version: 1\napp:\n  name: demo\n  environment: dev\nservices:\n  sql:\n    instances:\n      primary: {}\n      analytics: {}\n"

func appEditLoadAuthority(t *testing.T, source string, target string, prefs []application.ProviderPreference) GuidedReadContext {
	t.Helper()
	manifest, err := application.ParseYAML(source)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := application.PortableContractFromManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return GuidedReadContext{
		ApplicationID: "existing-system-owned-id",
		Name:          manifest.Name,
		Environment:   manifest.Environment,
		Target:        target,
		Capabilities:  contract.Capabilities,
		Preferences:   prefs,
		Footprint:     application.Footprint{Target: target},
		Complete:      true,
	}
}

func TestAppEditLoadRichYAMLRoundTrip(t *testing.T) {
	source := []byte("# authored manifest; do not reformat\n" + appEditLoadYAML + "# retained comment\n")
	view := appEditLoadAuthority(t, string(source), "dev", nil)
	if len(view.Capabilities) != 2 {
		t.Fatalf("expected both named SQL requirements: %+v", view.Capabilities)
	}
	view.Preferences = []application.ProviderPreference{
		{Requirement: view.Capabilities[0], InstanceID: "sql-one"},
		{Requirement: view.Capabilities[1]},
	}
	before := bytes.Clone(source)
	stateBefore := append([]capability.Requirement(nil), view.Capabilities...)
	result, err := appEditLoad(context.Background(), "dev", source, func(context.Context, string) (GuidedReadContext, error) {
		return view, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(appEditLoadSource(result), before) || !reflect.DeepEqual(result.Capabilities, view.Capabilities) ||
		!reflect.DeepEqual(result.Preferences, view.Preferences) || result.ApplicationID != view.ApplicationID {
		t.Fatalf("lost YAML intent or preferences: %+v", result)
	}
	result.Source[0] = '!'
	result.Capabilities[0].Name = "altered"
	result.Preferences[0].InstanceID = "altered"
	if !bytes.Equal(source, before) || !reflect.DeepEqual(view.Capabilities, stateBefore) ||
		view.Preferences[0].InstanceID != "sql-one" {
		t.Fatal("hydration modified source or authority")
	}
}

func TestAppEditLoadTwoTargetsAutomaticExplicit(t *testing.T) {
	source := []byte(appEditLoadYAML)
	base := appEditLoadAuthority(t, string(source), "dev", nil)
	dev := base
	dev.Preferences = []application.ProviderPreference{{Requirement: base.Capabilities[0]}}
	prod := base
	prod.Target = "prod"
	prod.Footprint.Target = "prod"
	prod.Preferences = []application.ProviderPreference{{Requirement: base.Capabilities[0], InstanceID: "prod-db"}}
	contexts := map[string]GuidedReadContext{"dev": dev, "prod": prod}
	read := func(_ context.Context, target string) (GuidedReadContext, error) {
		return contexts[target], nil
	}
	a, err := appEditLoad(context.Background(), "dev", source, read)
	if err != nil {
		t.Fatal(err)
	}
	b, err := appEditLoad(context.Background(), "prod", source, read)
	if err != nil {
		t.Fatal(err)
	}
	if a.Preferences[0].InstanceID != "" || b.Preferences[0].InstanceID != "prod-db" {
		t.Fatal("Target preferences crossed")
	}
	b.Preferences[0].InstanceID = "changed"
	if contexts["prod"].Preferences[0].InstanceID != "prod-db" {
		t.Fatal("read changed another Target")
	}
}

func TestAppEditLoadRejectsForeignAppEnvironmentIntent(t *testing.T) {
	base := appEditLoadAuthority(t, appEditLoadYAML, "dev", nil)
	cases := []struct {
		name   string
		source string
	}{
		{"app", "version: 1\napp:\n  name: other\n  environment: dev\n"},
		{"environment", "version: 1\napp:\n  name: demo\n  environment: prod\n"},
		{"intent", "version: 1\napp:\n  name: demo\n  environment: dev\nservices:\n  sql:\n    instances:\n      primary: {}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := appEditLoad(context.Background(), "dev", []byte(tc.source), func(context.Context, string) (GuidedReadContext, error) {
				return base, nil
			})
			var typed *appEditLoadError
			if !errors.As(err, &typed) || typed.Kind != "incompatible" {
				t.Fatalf("expected incompatible, got %v", err)
			}
		})
	}
}

func TestAppEditLoadTypedSourceErrors(t *testing.T) {
	for _, kind := range []string{"missing", "ambiguous", "incompatible", "unverifiable"} {
		_, err := appEditLoad(context.Background(), "dev", []byte(appEditLoadYAML), func(context.Context, string) (GuidedReadContext, error) {
			return GuidedReadContext{}, &GuidedReadError{Kind: kind, Cause: errors.New("secret=private")}
		})
		var typed *appEditLoadError
		if !errors.As(err, &typed) || typed.Kind != kind || bytes.Contains([]byte(err.Error()), []byte("private")) {
			t.Fatalf("unsafe source error: %v", err)
		}
	}
}

func TestAppEditLoadActualFileUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseharbor.yaml")
	if err := os.WriteFile(path, []byte(appEditLoadYAML), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	statBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	view := appEditLoadAuthority(t, string(before), "dev", nil)
	result, err := appEditLoad(context.Background(), "dev", before, func(context.Context, string) (GuidedReadContext, error) {
		return view, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	statAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !bytes.Equal(result.Source, before) ||
		!statBefore.ModTime().Equal(statAfter.ModTime()) || statBefore.Mode() != statAfter.Mode() {
		t.Fatal("read modified source file")
	}
}

func TestAppEditLoadManagedSecretsHydration(t *testing.T) {
	source := []byte(appEditLoadYAML + "  secrets:\n    enabled: true\n")
	manifest, err := application.ParseYAML(string(source))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := application.PortableContractFromManifest(manifest)
	if err != nil || !contract.Secrets.Managed {
		t.Fatalf("managed secret contract missing: %+v %v", contract, err)
	}
	secret := capability.Requirement{Kind: capability.Secrets, Name: "default"}
	base := appEditLoadAuthority(t, string(source), "dev", nil)
	base.Footprint.Requirements = []application.RequirementResolution{{Requirement: secret}}
	original := bytes.Clone(source)
	for _, tc := range []struct {
		target   string
		provider string
	}{{"dev", ""}, {"prod", "explicit-secrets"}} {
		view := base
		view.Target = tc.target
		view.Footprint.Target = tc.target
		view.Preferences = []application.ProviderPreference{{Requirement: secret, InstanceID: tc.provider}}
		state := append([]application.ProviderPreference(nil), view.Preferences...)
		loaded, err := appEditLoad(context.Background(), tc.target, source, func(context.Context, string) (GuidedReadContext, error) {
			return view, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, requirement := range loaded.Capabilities {
			if requirement == secret {
				count++
			}
		}
		if count != 1 || !reflect.DeepEqual(loaded.Preferences, state) || !bytes.Equal(appEditLoadSource(loaded), original) {
			t.Fatalf("lost managed secrets: %+v", loaded)
		}
		loaded.Preferences[0].InstanceID = "modified"
		if !reflect.DeepEqual(view.Preferences, state) || !bytes.Equal(source, original) {
			t.Fatal("read mutated input authority or YAML")
		}
	}
}

func TestAppEditLoadRejectsManagedSecretsMismatch(t *testing.T) {
	withSecrets := []byte(appEditLoadYAML + "  secrets:\n    enabled: true\n")
	secret := capability.Requirement{Kind: capability.Secrets, Name: "default"}
	tests := []struct {
		source    []byte
		footprint bool
	}{
		{withSecrets, false},
		{[]byte(appEditLoadYAML), true},
	}
	for _, tc := range tests {
		view := appEditLoadAuthority(t, string(tc.source), "dev", nil)
		if tc.footprint {
			view.Footprint.Requirements = []application.RequirementResolution{{Requirement: secret}}
		}
		_, err := appEditLoad(context.Background(), "dev", tc.source, func(context.Context, string) (GuidedReadContext, error) {
			return view, nil
		})
		var typed *appEditLoadError
		if !errors.As(err, &typed) || typed.Kind != "incompatible" {
			t.Fatalf("expected managed-secrets mismatch, got %v", err)
		}
	}
}
