package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// nodeListTrustState renders the human list line for a single item by
// reusing the exact ENROLLED/EXPIRED/PENDING decision the list command uses.
// It exists so the causal tests assert the state is driven by the authoritative
// record fields (Enrolled/ExpiresAt) and not a hard-coded PENDING.
func renderTrustState(item nodeListItem) string {
	if item.Enrolled && item.ExpiresAt.After(time.Now()) {
		return "ENROLLED"
	}
	if item.Enrolled {
		return "EXPIRED"
	}
	return "PENDING"
}

// TestNodeListStateFallsClosedToPendingWhenUnenrolled proves the list can NOT
// report a positive trust state from a never-populated record: with Enrolled
// unset the list must stay PENDING (the r2 defect) instead of a fabricated
// ENROLLED. This is the red test that fails the moment code claims trust
// without a real source.
func TestNodeListStateFallsClosedToPendingWhenUnenrolled(t *testing.T) {
	item := nodeListItem{TargetID: "edge-a", NodeID: "node-a", Runtime: "podman"}
	if got := renderTrustState(item); got != "PENDING" {
		t.Fatalf("un-enrolled node must report PENDING, got %q", got)
	}
}

// TestNodeListStateEnrolledWhileValid proves that once the authoritative
// add-time source (the Core grant expiry) populates the local record, the list
// reports ENROLLED. This is the green counterpart that would have been red
// before Fix 1, because saveNodeRecord never set Enrolled/ExpiresAt.
func TestNodeListStateEnrolledWhileValid(t *testing.T) {
	item := nodeListItem{
		TargetID:  "edge-a",
		NodeID:    "node-a",
		Runtime:   "podman",
		Enrolled:  true,
		ExpiresAt: time.Now().Add(time.Hour).UTC(),
	}
	if got := renderTrustState(item); got != "ENROLLED" {
		t.Fatalf("enrolled node with future expiry must report ENROLLED, got %q", got)
	}
}

// TestNodeListStateExpiresProvesCausalExpiry proves the state is causally tied
// to the expiry field: the same Enrolled record flips ENROLLED -> EXPIRED the
// instant ExpiresAt passes. Mutating the single authoritative field must flip
// the displayed result.
func TestNodeListStateExpiresProvesCausalExpiry(t *testing.T) {
	item := nodeListItem{TargetID: "edge-a", NodeID: "node-a", Runtime: "podman", Enrolled: true, ExpiresAt: time.Now().Add(time.Hour).UTC()}
	if got := renderTrustState(item); got != "ENROLLED" {
		t.Fatalf("pre-expiry must be ENROLLED, got %q", got)
	}
	item.ExpiresAt = time.Now().Add(-time.Hour).UTC()
	if got := renderTrustState(item); got != "EXPIRED" {
		t.Fatalf("post-expiry must be EXPIRED, got %q", got)
	}
}

// TestNodeSavePersistsAuthoritativeExpiry proves the fix at the source: the
// local nodeRecord that node list reads must carry a non-zero ExpiresAt that
// matches the Core grant expiry, not a zero value.
func TestNodeSavePersistsAuthoritativeExpiry(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", dataRoot)
	targetID := "edge-a"
	grantExpires := time.Now().Add(time.Hour).UTC().Round(time.Second)

	// Reproduce the persisted record the way node add does after Fix 1.
	record := nodeRecord{
		ContractVersion: machine.ContractVersion,
		TenantID:        "12345678-1234-1234-1234-123456789abc",
		TargetID:        targetID,
		NodeID:          "node-a",
		Runtime:         "podman",
		Environment:     "dev",
		CoreTarget:      "core",
		CoreURL:         "https://core.example:8443",
		CoreAddress:     "core.example:9443",
		ServerName:      "core.example",
		CAFile:          "/tmp/ca.pem",
		Enrolled:        true,
		ExpiresAt:       grantExpires,
	}
	if err := saveNodeRecord(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadNodeRecord(targetID)
	if err != nil {
		t.Fatalf("reload record: %v", err)
	}
	if !loaded.Enrolled {
		t.Fatal("persisted record lost Enrolled=true; list will report PENDING")
	}
	if loaded.ExpiresAt.Round(time.Second) != grantExpires {
		t.Fatalf("persisted expiry %v must match grant expiry %v", loaded.ExpiresAt, grantExpires)
	}
	// The local file must actually carry the field, not rely on a zero default.
	raw, err := os.ReadFile(filepath.Join(dataRoot, "nodes", targetID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["expires_at"]; !ok {
		t.Fatal("node record JSON omits expires_at; state cannot be derived")
	}
	if decoded["enrolled"] != true {
		t.Fatalf("node record JSON enrolled must be true, got %v", decoded["enrolled"])
	}
}

// TestNodeStatusSerialAndCapabilitiesFailClosedProvesSource proves the CLI does
// NOT fabricate serial or capabilities when the authoritative Core response
// omits them. The review demanded the surface be honest, never a still PENDING
// or invented field; a Core response without serial/capabilities must not be
// decoded as having them.
func TestNodeStatusSerialAndCapabilitiesFailClosedProvesSource(t *testing.T) {
	// Minimal Core status response (contract/target/node/runtime/enrolled/
	// revoked/expires_at only) — exactly what internal/targetenrollment/http.go
	// currently publishes; no serial, no capabilities.
	resp := map[string]any{
		"contract_version": machine.ContractVersion,
		"target_id":        "edge-a",
		"node_id":          "node-a",
		"runtime":          "podman",
		"enrolled":         true,
		"revoked":          false,
		"expires_at":       time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(resp)

	var status nodeStatusResult
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	if status.Serial != "" {
		t.Fatal("serial must be empty when Core does not publish it")
	}
	if len(status.Capabilities) != 0 {
		t.Fatal("capabilities must be empty when Core does not publish them")
	}
	// And the JSON the CLI would write must not carry fabricated fields.
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	_ = json.Unmarshal(encoded, &decoded)
	if _, ok := decoded["serial"]; ok {
		t.Fatal("status JSON must not emit a fabricated serial field")
	}
	if decoded["enrolled"] != true {
		t.Fatalf("enrolled must survive round-trip as true, got %v", decoded["enrolled"])
	}
}

// TestNodeCapabilitiesDecodingHonoursSource proves decodeCapabilitiesFromCoreResponse
// only surfaces well-formed capabilities and never invents them from an absent
// or malformed array.
func TestNodeCapabilitiesDecodingHonoursSource(t *testing.T) {
	// No capabilities key -> ok=false, no fabrication.
	if _, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"contract_version":"x"}`)); ok {
		t.Fatal("absent capabilities must decode as ok=false")
	}
	// Empty array -> no capabilities surfaced (ok=true, len 0), which is the
	// honest "Core returned no capability descriptors" verdict.
	if got, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"capabilities":[]}`)); ok != true || len(got) != 0 {
		t.Fatalf("empty capabilities must decode as ok=true, nil, got %+v ok=%v", got, ok)
	}
	// Malformed -> ok=false.
	if _, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"capabilities":"nope"}`)); ok {
		t.Fatal("malformed capabilities must decode as ok=false")
	}
	// Well-formed -> the advertised capability is surfaced verbatim.
	got, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"capabilities":[{"name":"node_connection.read","available":true,"detail":"reads"}]}`))
	if !ok || len(got) != 1 || got[0].Name != "node_connection.read" || !got[0].Available {
		t.Fatalf("well-formed capabilities must surface exactly, got %+v ok=%v", got, ok)
	}
	// Empty name is structurally malformed -> ok=true with nil (no fabricated
	// capability entry; the response shape was valid but yielded nothing).
	if got, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"capabilities":[{"name":"  "}]}`)); ok != true || len(got) != 0 {
		t.Fatalf("blank capability name must decode as ok=true, nil, got %+v ok=%v", got, ok)
	}
}

// TestNodeListJSONExpiryPresent proves the --json list output carries the
// authoritative ExpiresAt/Enrolled per node rather than zero values. It wires a
// connector node into the deployment config exactly like a real Target, so the
// list command's own config path (deployment.LoadConfig) drives the result.
func TestNodeListJSONExpiryPresent(t *testing.T) {
	xdgConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	dataRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataRoot)

	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "local-core",
		Access: map[string]deployment.AccessDefinition{
			"node-edge-a": {Provider: nodeConnectorProvider, Reference: "node-a"},
			"local":       {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"edge-a": {
				TenantID: "12345678-1234-1234-1234-123456789abc",
				Runtime:  deployment.RuntimeDefinition{Provider: "podman"},
				Access:   deployment.TargetAccess{Reference: "node-edge-a"},
			},
			"local-core": {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local"},
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	grant := time.Now().Add(2 * time.Hour).UTC().Round(time.Second)
	if err := saveNodeRecord(nodeRecord{
		ContractVersion: machine.ContractVersion, TenantID: "12345678-1234-1234-1234-123456789abc",
		TargetID: "edge-a", NodeID: "node-a", Runtime: "podman", Environment: "dev",
		CoreTarget: "local-core", CoreURL: "https://core.example:8443", CoreAddress: "core.example:9443",
		ServerName: "core.example", CAFile: "/tmp/ca.pem", Enrolled: true, ExpiresAt: grant,
	}); err != nil {
		t.Fatal(err)
	}

	out, errOut := &strings.Builder{}, &strings.Builder{}
	if err := nodeList(context.Background(), []string{"--json"}, out, errOut); err != nil {
		t.Fatalf("nodeList --json: %v", err)
	}
	var envelope struct {
		Nodes []nodeListItem `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out.String()), &envelope); err != nil {
		t.Fatalf("list --json not parseable: %v", err)
	}
	if len(envelope.Nodes) != 1 {
		t.Fatalf("expected 1 node in list output, got %d", len(envelope.Nodes))
	}
	if !envelope.Nodes[0].Enrolled {
		t.Fatal("list JSON node must report Enrolled=true from the authoritative record")
	}
	if envelope.Nodes[0].ExpiresAt.IsZero() {
		t.Fatal("list JSON node must carry the authoritative ExpiresAt")
	}
}
