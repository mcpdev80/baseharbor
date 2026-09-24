package serviceaccess

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectLifecycleManagedIssuerState(t *testing.T) {
	dir := t.TempDir()
	state := managedPKIState{
		Version:         2,
		Source:          PKIManagedLocal,
		IssuerReference: "openbao://baseharbor-pki/baseharbor-services",
		LifecycleOwner:  "issuer",
		RenewalMode:     "automatic-reconcile",
		ServerExpiresAt: time.Now().Add(20 * 24 * time.Hour).UTC(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := InspectLifecycle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != PKIManagedLocal || got.LifecycleOwner != "issuer" || got.RenewalMode != "automatic-reconcile" {
		t.Fatalf("unexpected lifecycle observation: %+v", got)
	}
	if got.Health != "warn" {
		t.Fatalf("health = %q, want warn", got.Health)
	}
}

func TestInspectLifecycleStaticBYOCState(t *testing.T) {
	dir := t.TempDir()
	state := staticPKIState{
		Version:         1,
		Source:          PKIBYOC,
		LifecycleOwner:  "operator",
		RenewalMode:     "replace-and-reconcile",
		ServerExpiresAt: time.Now().Add(5 * 24 * time.Hour).UTC(),
		Health:          "critical",
		Warning:         "certificate expires within 7 days; replace the configured certificate/key material and reconcile before expiry",
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "static-state.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := InspectLifecycle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != PKIBYOC || got.LifecycleOwner != "operator" || got.Health != "critical" {
		t.Fatalf("unexpected lifecycle observation: %+v", got)
	}
	if got.Warning == "" {
		t.Fatal("critical BYOC lifecycle did not expose an actionable warning")
	}
}

func TestInspectLifecycleRecomputesStaticExpiryHealth(t *testing.T) {
	dir := t.TempDir()
	state := staticPKIState{
		Version:         1,
		Source:          PKIBYOC,
		LifecycleOwner:  "operator",
		RenewalMode:     "replace-and-reconcile",
		ServerExpiresAt: time.Now().Add(5 * 24 * time.Hour).UTC(),
		Health:          "ok",
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "static-state.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := InspectLifecycle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != "critical" {
		t.Fatalf("health = %q, want critical", got.Health)
	}
}

func TestInspectLifecycleExternalIssuerState(t *testing.T) {
	dir := t.TempDir()
	state := managedPKIState{
		Version:         2,
		Source:          PKIExternal,
		IssuerReference: "enterprise://issuer",
		LifecycleOwner:  "issuer",
		RenewalMode:     "automatic-reconcile",
		ServerExpiresAt: time.Now().Add(45 * 24 * time.Hour).UTC(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := InspectLifecycle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != PKIExternal || got.IssuerReference != "enterprise://issuer" || got.Health != "ok" {
		t.Fatalf("unexpected external issuer lifecycle observation: %+v", got)
	}
}
