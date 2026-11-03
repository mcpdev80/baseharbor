package managementaccess_test

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
)

const managementAccessPath = "managementaccess/v1/profile.schema.json"

func managementAccessResolve(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	raw, err := contracts.ReadSchema(contracts.Namespace + managementAccessPath)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := profileResolveLocal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// profileResolveLocal resolves a raw JSON Schema offline. Defined here because
// this test package (contracts/managementaccess/v1) is separate from the
// contracts package test helpers in the parent package.
func profileResolveLocal(data []byte) (*jsonschema.Resolved, error) {
	var s jsonschema.Schema
	if err := s.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return s.Resolve(&jsonschema.ResolveOptions{Loader: func(uri *url.URL) (*jsonschema.Schema, error) {
		raw, err := contracts.ReadSchema(uri.String())
		if err != nil {
			return nil, err
		}
		var inner jsonschema.Schema
		if err := inner.UnmarshalJSON(raw); err != nil {
			return nil, err
		}
		return &inner, nil
	}})
}

// cloneObject returns a shallow copy of m. Defined here because this package is
// separate from the contracts package test helpers in the parent package.
func cloneObject(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func managementAccessBaseline(t *testing.T, resolved *jsonschema.Resolved) map[string]any {
	t.Helper()
	descriptor := map[string]any{
		"version":         "baseharbor.management-access/v1",
		"kind":            "management-profile",
		"installation_id": "11111111-1111-4111-8111-111111111111",
		"trust": map[string]any{
			"ca_pem": managementFixturesCA(t),
			"sha256": managementFixturesCAFA(t),
		},
		"recipient":  "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab",
		"scope":      "control-plane",
		"not_before": "2026-11-02T00:00:00Z",
		"not_after":  "2027-11-02T00:00:00Z",
		"import": map[string]any{
			"rights_granting":     false,
			"plugin_execution":    false,
			"credential_redirect": false,
		},
		"storage": map[string]any{
			"class":             "protected-durable",
			"durable":           true,
			"encrypted":         false,
			"disaster_recovery": true,
		},
	}
	if err := resolved.Validate(descriptor); err != nil {
		t.Fatalf("schema-valid management-access/v1 descriptor rejected by the packaged schema: %v", err)
	}
	return cloneObject(descriptor)
}

func managementAccessFixtures(t *testing.T) []map[string]any {
	t.Helper()
	data, err := os.ReadFile("profile.golden.json")
	if err != nil {
		// Tests are normally run from the package directory; fall back to the
		// canonical packaged path when invoked via ./contracts/... from elsewhere.
		data, err = os.ReadFile(filepath.Join("..", "..", "managementaccess", "v1", "profile.golden.json"))
	}
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Record  string         `json:"record"`
		Fixture map[string]any `json:"fixture"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("golden fixtures not valid JSON: %v", err)
	}
	out := make([]map[string]any, 0, len(fixtures))
	for _, f := range fixtures {
		out = append(out, f.Fixture)
	}
	return out
}

func TestManagementAccessGoldenFixturesValidate(t *testing.T) {
	resolved := managementAccessResolve(t)
	for _, fixture := range managementAccessFixtures(t) {
		if err := resolved.Validate(fixture); err != nil {
			t.Fatalf("%s golden fixture rejected: %v", fixture["kind"], err)
		}
	}
}

// TestManagementAccessGoldenFixturesPassTrust validates every positive golden
// through the same real semantic path the production path uses: it decodes the
// golden into a ManagementAccessProfile and runs ValidateTrust, which recomputes
// the CA fingerprint from the actual certificate and binds it to the stored
// fingerprint. A mismatched or fabricated fingerprint fails closed.
func TestManagementAccessGoldenFixturesPassTrust(t *testing.T) {
	for _, fixture := range managementAccessFixtures(t) {
		raw, err := json.Marshal(fixture)
		if err != nil {
			t.Fatalf("%s golden marshal: %v", fixture["kind"], err)
		}
		var profile managementaccess.ManagementAccessProfile
		if err := json.Unmarshal(raw, &profile); err != nil {
			t.Fatalf("%s golden unmarshal: %v", fixture["kind"], err)
		}
		if err := profile.ValidateTrust(); err != nil {
			t.Fatalf("%s golden fails ValidateTrust: %v", fixture["kind"], err)
		}
	}
}

// TestManagementAccessGoldenFixturesNegativeCoversMismatchedPin builds an
// in-memory golden that reuses the shared fixture CA but stores a wrong
// fingerprint and asserts it fails closed — the negative counterpart to the
// positive goldens in profile.golden.json.
func TestManagementAccessGoldenFixturesNegativeCoversMismatchedPin(t *testing.T) {
	ca := managementFixturesCA(t)
	profile := managementaccess.ManagementAccessProfile{
		Kind:    managementaccess.ManagementProfile,
		Version: "baseharbor.management-access/v1",
		Trust:   managementaccess.Trust{CAPEM: ca, SHA256: "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	if err := profile.ValidateTrust(); err == nil {
		t.Fatal("golden with mismatched fingerprint must fail closed")
	}
}

func TestManagementAccessSchemaRejectsMissingTopLevelRequired(t *testing.T) {
	resolved := managementAccessResolve(t)
	base := managementAccessBaseline(t, resolved)

	for _, field := range []string{"version", "kind", "installation_id", "trust", "recipient", "scope", "not_before", "not_after"} {
		mutated := cloneObject(base)
		delete(mutated, field)
		err := resolved.Validate(mutated)
		if err == nil {
			t.Fatalf("descriptor schema accepted a descriptor missing required top-level field %q", field)
		}
		if !strings.Contains(err.Error(), field) {
			t.Fatalf("missing top-level %q: error %q does not name the required constraint", field, err.Error())
		}
	}
}

func TestManagementAccessSchemaRejectsUnknownKind(t *testing.T) {
	resolved := managementAccessResolve(t)
	base := managementAccessBaseline(t, resolved)
	base["kind"] = "management-role"
	if err := resolved.Validate(base); err == nil {
		t.Fatal("schema accepted an unknown kind")
	}
}

func TestManagementAccessImportNeverGrantsRights(t *testing.T) {
	resolved := managementAccessResolve(t)
	base := managementAccessBaseline(t, resolved)
	for _, flag := range []string{"rights_granting", "plugin_execution", "credential_redirect"} {
		mutated := cloneObject(base)
		importCopy := cloneObject(base["import"].(map[string]any))
		importCopy[flag] = true
		mutated["import"] = importCopy
		if err := resolved.Validate(mutated); err == nil {
			t.Fatalf("schema accepted import with %s=true", flag)
		}
	}
}

func TestManagementAccessExportMustBeEncrypted(t *testing.T) {
	resolved := managementAccessResolve(t)
	base := managementAccessBaseline(t, resolved)
	base["kind"] = "credential-export"
	exportCopy := map[string]any{
		"algorithm": "aes-256-gcm",
		"encrypted": true,
		"nonce":     "E1f2g3h4i5j6k7l8m9n0o1p2q3r4s5t6u7v8w9x0y_z",
	}
	base["export"] = exportCopy
	// Golden credential-export carries an encryption descriptor and validates.
	if err := resolved.Validate(cloneObject(base)); err != nil {
		t.Fatalf("credential-export with encryption descriptor rejected: %v", err)
	}
	// Encryption obligation: an export payload with encrypted=false is rejected.
	exportCopy2 := cloneObject(base["export"].(map[string]any))
	exportCopy2["encrypted"] = false
	encrypted := cloneObject(base)
	encrypted["export"] = exportCopy2
	if err := resolved.Validate(encrypted); err == nil {
		t.Fatal("schema accepted an unencrypted credential-export")
	}
}

func TestManagementAccessProfileRejectsTransportRole(t *testing.T) {
	resolved := managementAccessResolve(t)
	base := managementAccessBaseline(t, resolved)
	base["kind"] = "management-profile"
	base["access"] = map[string]any{"client_id": "transport-client"}
	if err := resolved.Validate(base); err == nil {
		t.Fatal("management-profile must not carry connection-profile transport role fields")
	}
}

// Finding 1: kind-specific fail-closed enforcement via the Go contract path
// (schema keeps storage optional; the per-kind storage obligation lives in
// ValidateKindEnforcement, complementing the schema's import/encryption rules).
func TestManagementAccessKindStorageEnforcement(t *testing.T) {
	// device-enrollment plaintext durable fails closed.
	p := profileFor(t)
	p.Kind = managementaccess.DeviceEnrollment
	p.Storage = managementaccess.Storage{Class: managementaccess.ProtectedDurable, Durable: true, Encrypted: false, DisasterRecovery: false}
	if err := p.ValidateKindEnforcement(); err == nil {
		t.Fatal("plaintext durable device-enrollment must fail closed")
	}

	// management-profile plaintext durable fails closed.
	pm := profileFor(t)
	pm.Storage = managementaccess.Storage{Class: managementaccess.ProtectedDurable, Durable: true, Encrypted: false, DisasterRecovery: false}
	if err := pm.ValidateKindEnforcement(); err == nil {
		t.Fatal("plaintext durable management-profile must fail closed")
	}
}
