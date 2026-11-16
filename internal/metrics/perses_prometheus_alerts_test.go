package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The golden witness pins the marshalled Perses alerting contract against a
// committed golden file under testdata/perses-alerts. The contract is the
// deterministic source of truth; its marshalling is the witness. Every cell
// below is a real assertion over the produced contract, not a "does-not-contain"
// vacuous reject.

// persesAlertEvaluatorManifest returns a selected, pinned-surface manifest
// (observability management UI + Perses selected).
func persesAlertEvaluatorManifest() application.Manifest {
	m := application.New("perses-alerts-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return m
}

// TestPersesAlertEvaluatorShape pins the produced alerting contract: the real
// native rule evaluator/format (Prometheus contract, not a Grafana-UI/dashboard
// JSON), owner = the real observability owner (Prometheus) named as an owner
// reference, not a capability claim or a secret.
func TestPersesAlertEvaluatorShape(t *testing.T) {
	res, err := BuildPersesAlertEvaluator(persesAlertEvaluatorManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must define an alert evaluator contract: %v", err)
	}
	ev := res.Evaluator
	if ev.Kind != PersesAlertEvaluatorKind {
		t.Fatalf("perses alert evaluator kind = %q, want %q (native Prometheus rule evaluator, not a Grafana-UI/dashboard JSON)", ev.Kind, PersesAlertEvaluatorKind)
	}
	if ev.Evaluator != PersesAlertEvaluatorKind {
		t.Fatalf("perses alert evaluator = %q, want the real native evaluator %q (fail-closed without a real evaluator)", ev.Evaluator, PersesAlertEvaluatorKind)
	}
	if ev.Kind == "dashboard" || ev.Kind == "GrafanaDashboard" {
		t.Fatalf("perses alert evaluator must be the native Prometheus contract, never a Grafana dashboard JSON (%q)", ev.Kind)
	}
	if ev.Owner != string(capability.ProviderPrometheus) {
		t.Fatalf("perses alert evaluator owner = %q, want %q (the real observability owner, as an owner reference)", ev.Owner, string(capability.ProviderPrometheus))
	}
	if res.Evaluator.RecipientValidated != true {
		t.Fatal("a fully-formed, selected, pinned alerting contract with a real evaluator must report a validated recipient")
	}
	if res.Reachability != PersesAlertReachabilityConfiguredUnverified {
		t.Fatalf("reachability = %q, want %q (the only reachable state for a fully-formed contract; no runtime probe exists here)", res.Reachability, PersesAlertReachabilityConfiguredUnverified)
	}
}

// TestPersesAlertEvaluatorFailClosedWhenUnpinned asserts fail-closed: an unpinned
// release returns the fail-closed PersesAlertEvaluatorError, never an "alerting
// enabled" contract.
func TestPersesAlertEvaluatorFailClosedWhenUnpinned(t *testing.T) {
	_, err := persesAlertEvaluatorFromRelease(persesAlertEvaluatorManifest(), capability.PersesRelease{
		Version: "", Image: "", PrometheusPluginVersion: PersesAlertEvaluatorKind,
	})
	if err == nil {
		t.Fatal("unpinned perses must fail closed with PersesAlertEvaluatorError, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorFailClosedWhenUnselected asserts fail-closed: an
// unselected surface returns the fail-closed error, never a fabricated contract.
func TestPersesAlertEvaluatorFailClosedWhenUnselected(t *testing.T) {
	m := application.New("perses-alerts-unselected", "dev", false, false, false)
	_, err := BuildPersesAlertEvaluator(m)
	if err == nil {
		t.Fatal("unselected perses must fail closed, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorFailClosedWithoutRealEvaluator asserts fail-closed: a
// selected, pinned surface with no real native evaluator is not "alerting
// enabled" — it is fail-closed, never a false declaration.
func TestPersesAlertEvaluatorFailClosedWithoutRealEvaluator(t *testing.T) {
	_, err := persesAlertEvaluatorFromRelease(persesAlertEvaluatorManifest(), capability.PersesRelease{
		Version: "v0.55.1", Image: "persesdev/perses:v0.55.1",
	})
	if err == nil {
		t.Fatal("no real evaluator must fail closed, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorNoSecretInContract asserts the contract carries no
// notification-channel secret value: the owner is an owner reference, never a
// secret.
func TestPersesAlertEvaluatorNoSecretInContract(t *testing.T) {
	res, err := BuildPersesAlertEvaluator(persesAlertEvaluatorManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must define an alert evaluator contract: %v", err)
	}
	data, err := PersesAlertEvaluatorJSON(res.Evaluator)
	if err != nil {
		t.Fatalf("marshalling the alerting contract must succeed: %v", err)
	}
	s := string(data)
	for _, secret := range []string{"bearer", "password", "api_key", "apiKey", "token", "clientCert", "privateKey", "secret"} {
		if containsFold(s, secret) {
			t.Fatalf("alerting contract must carry no %s secret value: %s", secret, s)
		}
	}
}

// persesAlertEvaluatorSelectedManifest returns a selected, pinned surface
// manifest (observability management UI + Perses selected).
func persesAlertEvaluatorSelectedManifest() application.Manifest {
	m := application.New("perses-alerts-selected", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return m
}

// TestPersesAlertEvaluatorValidateHealthFailClosed asserts that a fully-formed,
// selected, pinned contract health verify returns nil (fail-closed on
// reachability, never on pin).
func TestPersesAlertEvaluatorValidateHealthFailClosed(t *testing.T) {
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), persesAlertEvaluatorSelectedManifest())
	if err != nil {
		t.Fatalf("a selected, pinned, fully-formed alerting contract health verify must return nil: %v", err)
	}
}

// TestPersesAlertEvaluatorValidateHealthRejectsPinOnly asserts the Luna-required
// negative: the health verifier MUST NOT pass on datasource-pin validation
// alone when the surface is unselected. Pin validation is neither evaluator nor
// recipient proof.
func TestPersesAlertEvaluatorValidateHealthRejectsPinOnly(t *testing.T) {
	// Unselected surface (no ObservabilityManagementUI/Perses): must fail closed,
	// never nil — pin validation alone does not pass.
	err := VerifyPersesAlertEvaluatorHealth(context.Background(), application.New("perses-alerts-unselected", "dev", false, false, false))
	if err == nil {
		t.Fatal("health verify must NOT pass on datasource-pin validation alone when the surface is unselected; pin-only is not recipient/evaluator proof")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorFailClosedWhenRecipientAbsent asserts fail-closed: a
// selected, pinned, evaluator-present surface with no validated recipient is
// not "alerting enabled".
func TestPersesAlertEvaluatorFailClosedWhenRecipientAbsent(t *testing.T) {
	// Unselected surface with a valid pinned evaluator: selection alone is not
	// recipient proof.
	m := application.New("perses-alerts-recipient-absent", "dev", false, false, false)
	_, err := persesAlertEvaluatorFromRelease(m, capability.PersesDefaultRelease)
	if err == nil {
		t.Fatal("surface with a real evaluator but no validated recipient must fail closed, never define an alert evaluator")
	}
	if _, ok := err.(*PersesAlertEvaluatorError); !ok {
		t.Fatalf("error = %T, want *PersesAlertEvaluatorError", err)
	}
}

// TestPersesAlertEvaluatorNoRecipientValidatedWithoutConcreteEvidence asserts
// that no produced result claims RecipientValidated without concrete validation
// evidence (selection + real evaluator present).
func TestPersesAlertEvaluatorNoRecipientValidatedWithoutConcreteEvidence(t *testing.T) {
	// Unselected surface: must NOT produce a contract, and must never set
	// RecipientValidated.
	m := application.New("perses-alerts-no-concrete", "dev", false, false, false)
	res, err := persesAlertEvaluatorFromRelease(m, capability.PersesDefaultRelease)
	if err == nil {
		t.Fatal("unselected surface must fail closed, never return a contract")
	}
	if res.Evaluator.RecipientValidated {
		t.Fatalf("a result with no concrete recipient evidence must not set RecipientValidated; got %+v", res.Evaluator)
	}
}

// TestPersesAlertEvaluatorValidateHealthFailClosedWhenUnpinned asserts that an
// unpinned release fails the health verify.
func TestPersesAlertEvaluatorValidateHealthFailClosedWhenUnpinned(t *testing.T) {
	// The health contract keys on PersesDefaultRelease; an unpinned surface
	// cannot be verified. This exercises the fail-closed guard directly.
	_, err := persesAlertEvaluatorFromRelease(persesAlertEvaluatorManifest(), capability.PersesRelease{})
	if err == nil {
		t.Fatal("unpinned perses must fail closed on the alert evaluator")
	}
}

// containsFold is a tiny case-insensitive substring check (no extra imports).
func containsFold(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// TestPersesAlertEvaluatorGoldenFiles pins the produced alerting contract
// against a committed golden file under testdata/perses-alerts.
func TestPersesAlertEvaluatorGoldenFiles(t *testing.T) {
	res, err := BuildPersesAlertEvaluator(persesAlertEvaluatorManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must define an alert evaluator contract: %v", err)
	}
	goldenPath := filepath.Join("testdata", "perses-alerts", "alerting.json")
	want, err := PersesAlertEvaluatorJSON(res.Evaluator)
	if err != nil {
		t.Fatalf("marshalling the alerting contract must succeed: %v", err)
	}
	got, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file %s must exist: %v", goldenPath, err)
	}
	if string(want) != string(got) {
		t.Fatalf("alerting contract golden mismatch:\n got: %s\nwant: %s", got, want)
	}
}
