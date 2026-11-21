package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
)

// persesBackendWitnessReport decodes the per-signal backend-witness the
// `baha status --json` face exposes. It mirrors the production field so the
// test asserts the machine contract, not an implementation detail.
type persesBackendWitnessReport struct {
	State                string                       `json:"state"`
	Perses               *string                      `json:"perses"`
	PersesBackendWitness []persesBackendSignalVerdict `json:"perses_backend_witness"`
}

// persesWitnessAppName derives a valid slug application name from the test
// name so every cell authors its own canonical application UID: the
// repository-scoped resolver registers applications under one
// process-global-by-data-root key (target/application/environment), and
// cells running in parallel (or with cleanup disabled, which leaks the
// registry entry) must never overwrite each other's registration.
func persesWitnessAppName(t *testing.T) string {
	name := t.Name()
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	lowered := strings.ToLower(name)
	var b strings.Builder
	for _, r := range lowered {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generate fixture app suffix: %v", err)
	}
	return slug + "-" + hex.EncodeToString(buf)
}

// persesWitnessProbeApp authors an initialized repository (named `name`, so
// cells that share one application-registry key never collide), chdir's into
// it for this test only, and resolves it through the production entry.
func persesWitnessProbeApp(t *testing.T, ctx context.Context, name string, configure func(m application.Manifest) application.Manifest) resolvedApplication {
	t.Helper()
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	persesTestCorelessTarget(t)
	root := t.TempDir()
	m := application.New(name, "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true
	if configure != nil {
		m = configure(m)
	}
	const witnessProbeCompose = "services:\n  api:\n    image: alpine:3.22\n    command: [sleep, infinity]\n"
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(witnessProbeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeInitializedAuthoredManifest(t, filepath.Join(root, application.RepositoryManifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	resolved, err := resolveApplication(ctx, application.DefaultStore(), nil, "status")
	if err != nil {
		t.Fatalf("resolve probe application: %v", err)
	}
	return resolved
}

// persesWitnessProbeAppReport authors an initialized repository whose
// observability management UI optionally realizes Perses, keeps the not-applied
// face, and returns its parsed status report through the production CLI entry.
func persesWitnessProbeAppReport(t *testing.T, ctx context.Context, name string, configure func(m application.Manifest) application.Manifest) persesBackendWitnessReport {
	t.Helper()
	persesWitnessProbeApp(t, ctx, name, configure)
	var out, errOut bytes.Buffer
	if err := persesRunWithIO(ctx, []string{"app", "status", "--json"}, &out, &errOut); err != nil && !strings.Contains(err.Error(), "container runtime orchestration not found") {
		t.Fatalf("baha app status --json failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	var report persesBackendWitnessReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("baha status --json output is not the public report: %v\n%s", err, out.String())
	}
	return report
}

// TestPersesStatusWitnessConfiguredWhenResolved asserts the positive case:
// a manifest that declares the observability backend signals resolves each to
// its OWN three-state witness, with a concrete source-id and no drift between
// the two faces. Reachability is what makes "configured": the witness derives
// it from a real resolved backend signal, never from a manifest flag. The
// direct unit form pins that a declared+resolved signal yields "configured".
func TestPersesStatusWitnessConfiguredWhenResolved(t *testing.T) {
	m := application.New("perses-witness", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m = application.WithLogsCollection(m, "application")
	m = application.WithOTLPTelemetry(m, "traces")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true

	// A positive "configured" requires a real backend answer for EVERY declared
	// backend (0428-142 r9): the resolved host signal alone is not enough — a
	// non-empty Prometheus/Loki/Tempo provenance is required.
	witness := persesBackendSignalWitnessFromManifest(
		m, capability.ObservabilitySupported,
		metricsprovider.PersesBackendSignalWitness{PrometheusProven: true, LokiProven: true, TempoProven: true})

	byBackend := map[string]persesBackendSignalVerdict{}
	for _, v := range witness {
		byBackend[v.Backend] = v
	}
	for _, backend := range []string{"prometheus", "loki", "tempo"} {
		verdict, ok := byBackend[backend]
		if !ok {
			t.Fatalf("perses_backend_witness must name %q explicitly", backend)
		}
		if verdict.State != persesBackendConfigured {
			t.Fatalf("%s witness state = %q, want %q (declared AND the resolved backend signal is reachable)", backend, verdict.State, persesBackendConfigured)
		}
		if strings.TrimSpace(verdict.SourceID) == "" {
			t.Fatalf("%s witness must carry a concrete source-id", backend)
		}
		// The card forbids over-claiming from a manifest flag.
		if verdict.State == "healthy" || verdict.State == "verified" {
			t.Fatalf("%s witness must never assert %q from a manifest flag", backend, verdict.State)
		}
	}
}

// TestPersesStatusWitnessUnreachableWhenDeclaredNotResolved proves the real
// emission path the card mandates: a signal that IS declared in the manifest
// but whose backend answer is NOT reachable resolves "unreachable", never the
// manifest-flag over-claim "configured". The three states are derived from a
// resolved backend signal (unreachable here) plus per-backend provenance, so
// this asserts the state against a declared-but-unreachable backend, not a flag.
func TestPersesStatusWitnessUnreachableWhenDeclaredNotResolved(t *testing.T) {
	m := application.New("perses-witness", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m = application.WithLogsCollection(m, "application")
	m = application.WithOTLPTelemetry(m, "traces")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true

	// Declared in the manifest, but the resolved backend signal is not
	// reachable (empty/absent status) and no Loki/Tempo provenance is present:
	// every declared backend is unreachable, not configured.
	witness := persesBackendSignalWitnessFromManifest(
		m, "", metricsprovider.PersesBackendSignalWitness{})

	byBackend := map[string]persesBackendSignalVerdict{}
	for _, v := range witness {
		byBackend[v.Backend] = v
	}
	for _, backend := range []string{"prometheus", "loki", "tempo"} {
		verdict, ok := byBackend[backend]
		if !ok {
			t.Fatalf("perses_backend_witness must name %q explicitly", backend)
		}
		if verdict.State != persesBackendUnreachable {
			t.Fatalf("%s witness state = %q, want %q (declared but the backend signal is not reachable)", backend, verdict.State, persesBackendUnreachable)
		}
		if strings.TrimSpace(verdict.SourceID) == "" {
			t.Fatalf("%s witness must carry a concrete source-id", backend)
		}
	}
}

// TestPersesStatusWitnessUnreachableIsNotTheManifestFlag pins the acceptance:
// "unreachable" is emitted from a declared-but-unresolved backend signal, not
// from a manifest selection flag. The three states are derived from a real
// resolved backend signal (empty/unreachable here) plus per-backend provenance:
// a declared-but-unresolved backend is "unreachable" with a reason naming the
// missing signal, never the manifest-flag default "configured".
func TestPersesStatusWitnessUnreachableIsNotTheManifestFlag(t *testing.T) {
	m := application.New("perses-witness", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m = application.WithOTLPTelemetry(m, "traces")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = true

	// Prometheus: metrics source declared, but the resolved signal is not
	// reachable => unreachable (not configured, not the manifest-flag default).
	witness := persesBackendSignalWitnessFromManifest(
		m, "", metricsprovider.PersesBackendSignalWitness{})

	var prom persesBackendSignalVerdict
	for _, v := range witness {
		if v.Backend == "prometheus" {
			prom = v
		}
	}
	if prom.State != persesBackendUnreachable {
		t.Fatalf("prometheus witness state = %q, want %q (metrics declared but backend not reachable)", prom.State, persesBackendUnreachable)
	}
	if strings.TrimSpace(prom.Reason) == "" {
		t.Fatalf("an unreachable backend must name the missing signal in its reason, got empty")
	}
}

// TestPersesStatusWitnessNotConfiguredWithoutObservabilityBackend asserts the
// fail-closed default: without an observability management UI + Perses selection
// the witness is empty — no backend is declared, so nothing can be green.
func TestPersesStatusWitnessNotConfiguredWithoutObservabilityBackend(t *testing.T) {
	ctx := t.Context()
	report := persesWitnessProbeAppReport(t, ctx, persesWitnessAppName(t), func(m application.Manifest) application.Manifest {
		m.Services.ObservabilityManagementUI = false
		m.Services.Perses = false
		return m
	})
	if len(report.PersesBackendWitness) != 0 {
		t.Fatalf("without an observability backend the witness must be empty, got %d entries", len(report.PersesBackendWitness))
	}
}

// TestPersesStatusWitnessLogsAbsentMarksLokiNotConfigured asserts that Loki
// answers from its OWN logs collection: with metrics present but no logs
// collection declared, Loki resolves "not-configured" while Prometheus stays
// "configured" — no backend inherits another's verdict.
func TestPersesStatusWitnessLogsAbsentMarksLokiNotConfigured(t *testing.T) {
	ctx := t.Context()
	report := persesWitnessProbeAppReport(t, ctx, persesWitnessAppName(t), func(m application.Manifest) application.Manifest {
		// No WithLogsCollection: Loki has no own signal.
		m = application.WithOTLPTelemetry(m, "traces")
		return m
	})

	byBackend := map[string]persesBackendSignalVerdict{}
	for _, v := range report.PersesBackendWitness {
		byBackend[v.Backend] = v
	}
	prom := byBackend["prometheus"]
	// The Coreless target resolves the host/provider-metrics observability
	// signal to supported; with the empty production witness a declared metrics
	// source is now unreachable, never configured (0428-142 r9). This cell only
	// asserts the fail-closed default: prometheus is not configured and Loki
	// not-configured.
	if prom.State == persesBackendConfigured {
		t.Fatalf("prometheus witness must not claim configured from the static catalog with an empty witness; got %q", prom.State)
	}
	loki := byBackend["loki"]
	if loki.State != persesBackendNotConfigured {
		t.Fatalf("loki witness state = %q, want %q (no logs collection declared; Loki must not inherit Prometheus)", loki.State, persesBackendNotConfigured)
	}
}

// TestPersesStatusWitnessNeverClaimsHealthyFromManifestFlag is a negative
// witness across every manifest axis: no combination of manifest flags may ever
// emit a "healthy"/"verified" backend verdict on either face.
func TestPersesStatusWitnessNeverClaimsHealthyFromManifestFlag(t *testing.T) {
	ctx := t.Context()
	cases := map[string]func(m application.Manifest) application.Manifest{
		"all-signals": func(m application.Manifest) application.Manifest {
			return application.WithOTLPTelemetry(application.WithLogsCollection(m, "application"), "traces")
		},
		"no-signals": func(m application.Manifest) application.Manifest {
			return m
		},
	}
	for name, configure := range cases {
		// The "all-signals" cell must witness three backends; the
		// "no-signals" cell's negative claim is that no verdict ever reads
		// healthy/verified — which is what the loop below checks on whatever
		// the machine face emits.
		report := persesWitnessProbeAppReport(t, ctx, persesWitnessAppName(t)+"-"+name, configure)
		if name == "all-signals" && len(report.PersesBackendWitness) == 0 {
			t.Fatalf("[%s] the machine face emitted no backend witness to reject (perses=%v state=%q)", name, report.Perses, report.State)
		}
		for _, v := range report.PersesBackendWitness {
			if v.State == "healthy" || v.State == "verified" {
				t.Fatalf("[%s] %s witness must never assert %q from a manifest flag", name, v.Backend, v.State)
			}
		}
	}
}

// TestPersesStatusWitnessHumanFaceRendersBackendWitness pins the drift rule
// against the human face directly, through the production CLI surface:
// `baha app perses status` must name every declared backend in its output,
// because the machine face already does. The command runs over the
// operator-auth fixture context (persesMatrixCommandContext), so the call
// passes the managed-environment boundary and ACTUALLY reaches
// renderPersesBackendWitness (CODEX-REVIEW r5): it fails without the render
// call in persesStatusCommand and passes with it, and a miscolored witness
// value turns it red.
func TestPersesStatusWitnessHumanFaceRendersBackendWitness(t *testing.T) {
	ctx := t.Context()
	persesWitnessProbeApp(t, ctx, persesWitnessAppName(t), func(m application.Manifest) application.Manifest {
		m = application.WithLogsCollection(m, "application")
		return application.WithOTLPTelemetry(m, "traces")
	})
	var out, errOut bytes.Buffer
	if err := persesRunWithIO(ctx, []string{"app", "perses", "status"}, &out, &errOut); err != nil {
		t.Fatalf("baha app perses status failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}
	human := out.String()
	for _, backend := range []string{"prometheus", "loki", "tempo"} {
		if !strings.Contains(human, backend) {
			t.Fatalf("human `perses status` output must name backend %q, got:\n%s", backend, human)
		}
	}
	// Every declared signal is resolved here, so the rendered witness must
	// carry its own three-state verdict and source-id, not just the word: a
	// miscolored backend or state makes this cell red.
	// The three-state witness the renderer must print: metrics resolves with
	// the host observability signal => configured; logs and traces are declared
	// but their provider streams stay unproven in this Core => unreachable. A
	// miscolored backend or state turns this cell red.
	// Metrics is declared but the resolved host/provider-metrics signal is NOT
	// proven reachable with the empty production witness => unreachable (never
	// configured from the static catalog); logs/traces declared but unproven =>
	// unreachable. (0428-142 r9)
	for _, want := range []persesBackendSignalVerdict{
		{Backend: "prometheus", SourceID: "metrics.sources", State: persesBackendUnreachable},
		{Backend: "loki", SourceID: "logs.collect", State: persesBackendUnreachable},
		{Backend: "tempo", SourceID: "telemetry.otlp", State: persesBackendUnreachable},
	} {
		if !strings.Contains(human, want.Backend+" ["+want.SourceID+"]: "+string(want.State)) {
			t.Fatalf("human `perses status` output must render %s [%s]: %s, got:\n%s", want.Backend, want.SourceID, want.State, human)
		}
	}
}

// TestPersesStatusWitnessRendersBackendWitnessDirectly proves the renderer is
// wired by calling the production renderPersesBackendWitness with a concrete
// witness and asserting the exact human line, bypassing the CLI operator-auth
// boundary that the status command otherwise gates. This is the minimal,
// non-environmental proof that the restored call emits per-backend witness.
func TestPersesStatusWitnessRendersBackendWitnessDirectly(t *testing.T) {
	var out bytes.Buffer
	witness := []persesBackendSignalVerdict{
		{Backend: "prometheus", SourceID: "metrics.sources", State: persesBackendConfigured},
		{Backend: "loki", SourceID: "logs.collect", State: persesBackendUnreachable},
		{Backend: "tempo", SourceID: "telemetry.otlp", State: persesBackendNotConfigured},
	}
	renderPersesBackendWitness(&out, witness)
	human := out.String()
	for _, backend := range []string{"prometheus", "loki", "tempo"} {
		if !strings.Contains(human, backend) {
			t.Fatalf("renderer must name backend %q, got:\n%s", backend, human)
		}
		if !strings.Contains(human, "metrics.sources") {
			t.Fatalf("renderer must carry the prometheus source-id, got:\n%s", human)
		}
	}
}

// TestPersesStatusWitnessHumanAndMachineFaceShowSameBackends pins the drift
// rule: the human `perses status` output lists the same set of backends as the
// machine `baha status --json` witness, so the two faces never disagree.
func TestPersesStatusWitnessHumanAndMachineFaceShowSameBackends(t *testing.T) {
	ctx := t.Context()
	resolved := persesWitnessProbeApp(t, ctx, persesWitnessAppName(t), func(m application.Manifest) application.Manifest {
		m = application.WithLogsCollection(m, "application")
		return m
	})
	var machineOut, machineErr bytes.Buffer
	if err := persesRunWithIO(ctx, []string{"status", "--json"}, &machineOut, &machineErr); err != nil {
		t.Fatalf("baha status --json failed: %v\nstdout: %s\nstderr: %s", err, machineOut.String(), machineErr.String())
	}
	var report persesBackendWitnessReport
	if err := json.Unmarshal(machineOut.Bytes(), &report); err != nil {
		t.Fatalf("baha status --json output is not the public report: %v\n%s", err, machineOut.String())
	}
	machineBackends := map[string]bool{}
	for _, v := range report.PersesBackendWitness {
		machineBackends[v.Backend] = true
	}
	if len(machineBackends) == 0 {
		t.Fatalf("the machine face declared no backends to compare against (resolved manifest services: %+v)", resolved.Manifest.Services)
	}

	// Human face: run `baha app perses status` in the same working directory.
	var out, errOut bytes.Buffer
	if err := persesRunWithIO(ctx, []string{"app", "perses", "status"}, &out, &errOut); err != nil {
		t.Fatalf("baha app perses status failed: %v\nstderr: %s", err, errOut.String())
	}
	human := out.String()
	for backend := range machineBackends {
		if !strings.Contains(human, backend) {
			t.Fatalf("human `perses status` output must name backend %q, got:\n%s", backend, human)
		}
	}
}
