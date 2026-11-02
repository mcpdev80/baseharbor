package contracts_test

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/contracts"
)

const managementAccessPath = "managementaccess/v1/profile.schema.json"

func managementAccessResolve(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	raw, err := contracts.ReadSchema(contracts.Namespace + managementAccessPath)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := profileResolve(raw)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func managementAccessBaseline(t *testing.T, resolved *jsonschema.Resolved) map[string]any {
	t.Helper()
	descriptor := map[string]any{
		"version":       "baseharbor.management-access/v1",
		"kind":          "management-profile",
		"installation_id": "11111111-1111-4111-8111-111111111111",
		"trust": map[string]any{
			"ca_pem": "-----BEGIN CERTIFICATE-----\nPUBLIC-MANAGEMENT-FIXTURE",
			"sha256": "0000000000000000000000000000000000000000000000000000000000000000",
		},
		"recipient": "spiffe://baseharbor/platform/core/11111111-1111-4111-8111-111111111111/homelab",
		"scope":     "control-plane",
		"not_before": "2026-11-02T00:00:00Z",
		"not_after":  "2027-11-02T00:00:00Z",
		"import": map[string]any{
			"rights_granting":     false,
			"plugin_execution":    false,
			"credential_redirect": false,
		},
		"storage": map[string]any{
			"class":              "protected-durable",
			"durable":            true,
			"encrypted":          false,
			"disaster_recovery":  true,
		},
	}
	if err := resolved.Validate(descriptor); err != nil {
		t.Fatalf("schema-valid management-access/v1 descriptor rejected by the packaged schema: %v", err)
	}
	return cloneObject(descriptor)
}

func managementAccessFixtures(t *testing.T) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("v1", "profile.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Record  string          `json:"record"`
		Fixture map[string]any  `json:"fixture"`
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
	encrypted := cloneObject(base)
	exportCopy := cloneObject(base["export"].(map[string]any))
	exportCopy["encrypted"] = false
	encrypted["export"] = exportCopy
	if err := resolved.Validate(encrypted); err == nil {
		t.Fatal("schema accepted an unencrypted credential-export")
	}
	plain := cloneObject(base)
	delete(plain, "export")
	if err := resolved.Validate(plain); err == nil {
		t.Fatal("schema accepted credential-export without encryption descriptor")
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
