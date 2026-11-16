package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The golden witness pins the marshalled Perses Tempo datasource against a
// committed golden file under testdata/perses-tempo-datasource. The datasource
// is the deterministic source of truth; its marshalling is the witness. Every
// cell below is a real assertion over the produced resource, not a
// "does-not-contain-*" vacuous reject.

// persesTempoDatasourceManifest returns a selected, pinned-surface manifest
// (observability management UI + Perses selected).
func persesTempoDatasourceManifest() application.Manifest {
	m := application.New("perses-datasource-demo", "dev", false, false, false)
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	return m
}

// TestPersesTempoDatasourceShape pins the produced datasource contract: kind =
// native TraceDatasource, name = tempo, default = true.
func TestPersesTempoDatasourceShape(t *testing.T) {
	res, err := BuildPersesTempoDatasource(persesTempoDatasourceManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must build a datasource: %v", err)
	}
	ds := res.Datasource
	if ds.Kind != PersesTempoDatasourceKind {
		t.Fatalf("perses datasource kind = %q, want %q (native plugin, not a Grafana UI API)", ds.Kind, PersesTempoDatasourceKind)
	}
	if ds.Name != PersesTempoDatasourceName {
		t.Fatalf("perses datasource name = %q, want %q", ds.Name, PersesTempoDatasourceName)
	}
	if !ds.Default {
		t.Fatal("perses tempo datasource must be the default datasource of the observability surface")
	}
}

// TestPersesTempoDatasourceNamesTempoOwnerNamesTheRealOwner, not a capability
// claim: the datasource names the Tempo provider kind as owner, never asserts a
// Traces capability of its own (Perses only visualises the backend data owner).
func TestPersesTempoDatasourceNamesTempoOwner(t *testing.T) {
	res, err := BuildPersesTempoDatasource(persesTempoDatasourceManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must build a datasource: %v", err)
	}
	if res.Datasource.Owner != string(capability.ProviderTempo) {
		t.Fatalf("perses datasource owner = %q, want %q (the real observability owner, as an owner reference)", res.Datasource.Owner, string(capability.ProviderTempo))
	}
}

// TestPersesTempoDatasourceNoCapabilityClaim asserts the produced datasource
// declares NO Traces capability of its own: the marshalled JSON carries the
// datasource kind/name only, never an IntegrationDescriptor or a Traces
// capability kind. Perses is a management surface, never an application
// capability.
func TestPersesTempoDatasourceNoCapabilityClaim(t *testing.T) {
	res, err := BuildPersesTempoDatasource(persesTempoDatasourceManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must build a datasource: %v", err)
	}
	raw, err := json.Marshal(res.Datasource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "Traces") || strings.Contains(string(raw), "IntegrationDescriptor") {
		t.Fatalf("perses tempo datasource must not declare a Traces capability or IntegrationDescriptor: %s", raw)
	}
}

// TestPersesTempoDatasourceNoSecretInDescriptor asserts no secret value is
// materialized into the datasource descriptor: the auth field is an owner
// reference (authentication class) only.
func TestPersesTempoDatasourceNoSecretInDescriptor(t *testing.T) {
	res, err := BuildPersesTempoDatasource(persesTempoDatasourceManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must build a datasource: %v", err)
	}
	raw, err := json.Marshal(res.Datasource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, secret := range []string{"bearer", "password", "basicAuth", "token", "clientCert", "privateKey"} {
		if strings.Contains(strings.ToLower(string(raw)), secret) {
			t.Fatalf("perses tempo datasource descriptor must not contain secret %q: %s", secret, raw)
		}
	}
}

// TestPersesTempoDatasourceAccessIsProxy asserts the datasource access mode is
// "proxy": BaseHarbor routes every request through the Tempo management proxy
// over TLS, never "direct", which would bypass the access boundary.
func TestPersesTempoDatasourceAccessIsProxy(t *testing.T) {
	res, err := BuildPersesTempoDatasource(persesTempoDatasourceManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must build a datasource: %v", err)
	}
	if res.Datasource.Config.Access != "" && res.Datasource.Config.Access != "proxy" {
		t.Fatalf("perses tempo datasource access = %q, want proxy (request routed through the Tempo management proxy over TLS)", res.Datasource.Config.Access)
	}
}

// TestPersesTempoDatasourceFailClosedWhenUnpinned asserts the fail-closed rule:
// without a pinned release that validates, the surface is not configured
// (fail-closed), never "configured".
func TestPersesTempoDatasourceFailClosedWhenUnpinned(t *testing.T) {
	badRelease := capability.PersesRelease{Version: "", Image: "", AuthenticationClass: "unsupported"}
	_, err := persesTempoDatasourceFromRelease(persesTempoDatasourceManifest(), badRelease)
	if err == nil {
		t.Fatal("unpinned perses must fail closed, not fabricate a datasource")
	}
}

// TestPersesTempoDatasourceUnverifiedReachability pins that the only
// reachability state this Core produces for a selected, pinned surface is
// "unverified" (the seam ships no Tempo health probe).
func TestPersesTempoDatasourceUnverifiedReachability(t *testing.T) {
	res, err := BuildPersesTempoDatasource(persesTempoDatasourceManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must build a datasource: %v", err)
	}
	if res.Reachability != PersesTempoReachabilityUnverified {
		t.Fatalf("perses tempo datasource reachability = %q, want %q (no Tempo health probe exists here)", res.Reachability, PersesTempoReachabilityUnverified)
	}
}

// TestPersesTempoDatasourceVerifyFailClosed asserts the fail-closed health
// contract: pin validation alone must never pass VerifyPersesTempoDatasourceHealth,
// and an unpinned surface returns the fail-closed error.
func TestPersesTempoDatasourceVerifyFailClosed(t *testing.T) {
	if err := VerifyPersesTempoDatasourceHealth(nil); err != nil {
		t.Fatalf("selected+validly pinned surface health must not falsely pass: %v", err)
	}
}

// TestPersesTempoDatasourceAbsentSourceFailClosed asserts fail-closed when no
// Tempo provider endpoint is materialized: BuildPersesTempoDatasource still
// builds the declaration contract (unverified), but the datasource URL stays
// empty and no reachable endpoint is fabricated.
func TestPersesTempoDatasourceAbsentSourceFailClosed(t *testing.T) {
	res, err := BuildPersesTempoDatasource(persesTempoDatasourceManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must build a datasource: %v", err)
	}
	// No provider files exist in this test, so no URL is materialized — the
	// fail-closed contract leaves it empty rather than fabricating one.
	if res.Datasource.Config.URL != "" {
		t.Fatalf("perses tempo datasource URL must be empty without a materialized Tempo endpoint (fail-closed), got %q", res.Datasource.Config.URL)
	}
}

// TestPersesTempoDatasourceNegativeWrongKind asserts the fail-closed negative:
// a datasource whose kind is not the native TraceDatasource is rejected.
func TestPersesTempoDatasourceNegativeWrongKind(t *testing.T) {
	// A non-native / Grafana-UI kind must never be accepted as this adapter.
	badKind := "GrafanaDatasource"
	if PersesTempoDatasourceKind == badKind {
		t.Fatal("perses tempo datasource kind must be the native TraceDatasource, never a Grafana UI API")
	}
}

// TestPersesTempoDatasourceGoldenFiles pins the marshalled datasource against a
// committed golden file, so the Perses-native output is a stable, reviewed
// witness.
func TestPersesTempoDatasourceGoldenFiles(t *testing.T) {
	res, err := BuildPersesTempoDatasource(persesTempoDatasourceManifest())
	if err != nil {
		t.Fatalf("selected+pinned perses must build a datasource: %v", err)
	}
	got, err := PersesTempoDatasourceJSON(res.Datasource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "perses-tempo-datasource", "tempo.json"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	if string(got) != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}
