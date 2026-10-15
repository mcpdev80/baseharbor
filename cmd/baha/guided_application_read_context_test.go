package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func guidedTreeDigest(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, err = h.Write([]byte(rel)); err != nil {
			return err
		}
		if !entry.IsDir() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, err = h.Write(b)
			return err
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func TestGuidedReadContextRefusesMissingAndWrongTarget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolved resolvedApplication
		kind     string
	}{
		{"missing", resolvedApplication{}, "missing"},
		{"wrong target", resolvedApplication{Target: deployment.ResolvedTarget{Name: "test"}, DeploymentIdentity: deployment.DeploymentIdentity{Target: "prod", ApplicationID: "owned"}, Manifest: application.Manifest{Name: "app", ApplicationID: "owned"}}, "wrong_target"},
		{"unverified id", resolvedApplication{Target: deployment.ResolvedTarget{Name: "test"}, DeploymentIdentity: deployment.DeploymentIdentity{Target: "test", ApplicationID: "different"}, Manifest: application.Manifest{Name: "app", ApplicationID: "owned"}}, "unverifiable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readGuidedApplicationContext(context.Background(), tc.resolved)
			var typed *GuidedReadError
			if !errors.As(err, &typed) || typed.Kind != tc.kind {
				t.Fatalf("error = %v, expected %s", err, tc.kind)
			}
		})
	}
}

func TestGuidedReadNewAndEditDoNotWriteState(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "edit"}[edit], func(t *testing.T) {
			r := corelessResolvedFixture(t)
			if edit {
				r.DeploymentIdentity = deployment.DeploymentIdentity{Target: r.Target.Name, ApplicationID: r.Manifest.ApplicationID, DeploymentID: "existing"}
			}
			before, err := guidedTreeDigest(r.TargetStateRoot)
			if err != nil {
				t.Fatal(err)
			}
			sourceBefore, err := guidedTreeDigest(r.RepositoryRoot)
			if err != nil {
				t.Fatal(err)
			}
			// An actual source fixture contains nested credential material; it must
			// never appear in the adapter's returned JSON or public errors.
			secret := "SENTINEL_PASSWORD_DO_NOT_LEAK"
			source := filepath.Join(r.RepositoryRoot, "compose.yaml")
			compose, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, append(compose, []byte("    environment:\n      DATABASE_PASSWORD: "+secret+"\n      PRIVATE_KEY: SENTINEL_PRIVATE_KEY_DO_NOT_LEAK\n")...), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err = guidedTreeDigest(r.TargetStateRoot)
			if err != nil {
				t.Fatal(err)
			}
			sourceBefore, err = guidedTreeDigest(r.RepositoryRoot)
			if err != nil {
				t.Fatal(err)
			}
			state, err := readGuidedApplicationContext(context.Background(), r)
			if err != nil {
				t.Fatalf("actual read: %v", err)
			}
			if state.Target != r.Target.Name || state.Name != r.Manifest.Name || state.SnapshotRevision == "" || state.PlanRevision == "" {
				t.Fatalf("incomplete projection: %+v", state)
			}
			after, err := guidedTreeDigest(r.TargetStateRoot)
			if err != nil {
				t.Fatal(err)
			}
			sourceAfter, err := guidedTreeDigest(r.RepositoryRoot)
			if err != nil {
				t.Fatal(err)
			}
			if before != after || sourceBefore != sourceAfter {
				t.Fatalf("read changed stored data: %s => %s; source: %s => %s", before, after, sourceBefore, sourceAfter)
			}
			payload, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"SENTINEL_PASSWORD_DO_NOT_LEAK", "SENTINEL_PRIVATE_KEY_DO_NOT_LEAK"} {
				if strings.Contains(string(payload), secret) {
					t.Fatalf("secret %q leaked", secret)
				}
			}
			bad := r
			bad.DeploymentIdentity.Target = "different"
			bad.DeploymentIdentity.ApplicationID = r.Manifest.ApplicationID
			_, publicErr := readGuidedApplicationContext(context.Background(), bad)
			if publicErr == nil {
				t.Fatal("mismatched Target accepted")
			}
			for _, value := range []string{"SENTINEL_PASSWORD_DO_NOT_LEAK", "SENTINEL_PRIVATE_KEY_DO_NOT_LEAK"} {
				if strings.Contains(publicErr.Error(), value) {
					t.Fatal("public error leaked source secret")
				}
			}
		})
	}
}

func TestGuidedErrorDoesNotExposeUnderlyingSecret(t *testing.T) {
	sentinel := "SENTINEL_PRIVATE_KEY_DO_NOT_LEAK"
	err := &GuidedReadError{Kind: "unverifiable", Cause: errors.New(sentinel)}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatal("authority error exposed secret data")
	}
	var typed *GuidedReadError
	if !errors.As(err, &typed) {
		t.Fatal("typed error was lost")
	}
}

func TestGuidedNestedOutputHasNoSecretValues(t *testing.T) {
	sentinel := "SENTINEL_PASSWORD_DO_NOT_LEAK"
	state := GuidedReadContext{
		Name: "real", Target: "development",
		Capabilities: []capability.Requirement{{Kind: capability.SQL, Name: "primary"}},
		Footprint: application.Footprint{
			Target: "development",
			Requirements: []application.RequirementResolution{
				{Requirement: capability.Requirement{Kind: capability.SQL, Name: "primary"}, Binding: "automatic"},
			},
		},
		Preferences: []application.ProviderPreference{{Requirement: capability.Requirement{Kind: capability.SQL, Name: "primary"}}},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "primary") {
		t.Fatal("nested projection was not populated")
	}
	if strings.Contains(string(encoded), sentinel) {
		t.Fatal("secret leaked through nested output")
	}
}

// Test the adapter with real changed application inputs rather than hashing
// an unrelated artificial structure.
func TestGuidedReadRevisionChangesWithApplicationIntent(t *testing.T) {
	r := corelessResolvedFixture(t)
	first, err := readGuidedApplicationContext(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.Manifest.Environment = "other"
	second, err := readGuidedApplicationContext(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotRevision == second.SnapshotRevision {
		t.Fatal("changed environment retained snapshot revision")
	}
	if first.Target != second.Target {
		t.Fatal("intent change replaced Target")
	}
}

func TestGuidedReadProjectsCapabilityFootprintAndAuthorityPlan(t *testing.T) {
	r := corelessResolvedFixture(t)
	r.Manifest = application.WithHTTPExposure(r.Manifest, "public", "api", 8080, "http")
	state, err := readGuidedApplicationContext(context.Background(), r)
	if err != nil {
		t.Fatalf("read actual exposure requirement: %v", err)
	}
	if len(state.Capabilities) == 0 || len(state.Footprint.Requirements) == 0 {
		t.Fatalf("missing projected capability or footprint: %+v", state)
	}
	snapshot, preferences, err := applicationLifecycleSnapshot(r, observeApplicationProviderFacts(context.Background(), r))
	if err != nil {
		t.Fatal(err)
	}
	authority, err := resolveApplicationProviderPlanFromSnapshot(r, snapshot, preferences)
	if err != nil {
		t.Fatal(err)
	}
	if authority.Impact != nil && authority.Impact.PlanID != "" && state.PlanRevision != authority.Impact.PlanID {
		t.Fatalf("consent PlanID not propagated: got %s, want %s", state.PlanRevision, authority.Impact.PlanID)
	}
	if len(state.Changes) != len(authority.Changes) {
		t.Fatalf("provider consequence count differs: %d / %d", len(state.Changes), len(authority.Changes))
	}
}

func TestGuidedRevisionsTrackInputs(t *testing.T) {
	a, err := guidedRevision(struct {
		Target  string
		Version int
	}{"test", 1})
	if err != nil || len(a) != 64 {
		t.Fatalf("revision %q %v", a, err)
	}
	same, err := guidedRevision(struct {
		Target  string
		Version int
	}{"test", 1})
	if err != nil || same != a {
		t.Fatalf("unstable revision: %v", err)
	}
	changed, err := guidedRevision(struct {
		Target  string
		Version int
	}{"test", 2})
	if err != nil || changed == a {
		t.Fatalf("unchanged revision: %v", err)
	}
}

func TestGuidedPublicShapeDoesNotExportRawSecretsOrManifests(t *testing.T) {
	state := GuidedReadContext{ApplicationID: "owned", Name: "app", Target: "test", SnapshotRevision: "rev"}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"manifest", "raw_intent", "credentials", "secret_value", "facts", "registry"} {
		if strings.Contains(string(encoded), "\""+forbidden+"\"") {
			t.Fatalf("public field %q leaked", forbidden)
		}
	}
}
