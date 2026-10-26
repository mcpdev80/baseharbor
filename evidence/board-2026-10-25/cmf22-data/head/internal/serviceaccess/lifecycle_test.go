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
	if got.Health != "ok" || got.Warning != "" {
		t.Fatalf("fresh automatically renewed certificate should be healthy: %+v", got)
	}
}

func TestAutomaticCertificateHealthUsesActionableRenewalWindow(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		remaining time.Duration
		want      string
	}{
		{"fresh thirty day certificate", 30*24*time.Hour - time.Second, "ok"},
		{"before renewal window", 7*24*time.Hour + time.Second, "ok"},
		{"renewal window reached", 7 * 24 * time.Hour, "critical"},
		{"renewal overdue", time.Hour, "critical"},
		{"expired", -time.Second, "critical"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			health, warning := lifecycleHealthAt(now.Add(tc.remaining), "automatic-reconcile", now)
			if health != tc.want || (health == "ok" && warning != "") || (health != "ok" && warning == "") {
				t.Fatalf("health=%s warning=%q", health, warning)
			}
		})
	}
	if health, warning := lifecycleHealthAt(time.Time{}, "automatic-reconcile", now); health != "unknown" || warning == "" {
		t.Fatal("missing expiry was treated as healthy")
	}
	if health, warning := lifecycleHealthAt(now.Add(20*24*time.Hour), "replace-and-reconcile", now); health != "warn" || warning == "" {
		t.Fatal("operator-owned replacement warning disappeared")
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
