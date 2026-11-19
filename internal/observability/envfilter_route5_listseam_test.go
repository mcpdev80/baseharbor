package observability

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// envfilter_route5_listseam_test.go — caller-visible seam regression for card
// 0428-121, correction round r5. TestRoute2AnchorRuleTable pins only the helper
// projectTokenFromID; it never exercises the seam the caller actually sees.
//
// These cases call the real observability.List/ListLogs/ListMetrics entry points
// and assert on the RETURNED source set (length + IDs), not on a helper. They
// cover exactly the two colliding anchor cases: the same project
// bh-local-demo-prod-west under OwnerApplication 'demo' (anchor resolves
// 'prod-west', not 'west' -> excluded) and OwnerApplication 'demo-prod' (anchor
// resolves 'west' -> included), plus case (c) my-app dev/prod and a mutation
// control that pins the exclusion to structure, not chance.
//
// Each colliding case is proven for TWO signal kinds, each with its own
// provider-class fixture (Class=SourceApplicationProvider) so the environment
// resolves over projectTokenFromID (the anchor rule), not over the legacy
// metrics-ID path — the path that decides them, as proven by the
// projectTokenFromID line under each fixture:
//   - logs fixture: Target=RuntimeTarget(collisionProject,'loki'); asserted on
//     List/ListLogs.
//   - metrics fixture: Target=RuntimeTarget(collisionProject,'prometheus');
//     asserted on List/ListMetrics (with includeApplicationProviders=true).
//
// For every fixture a positive control asserts that the same source with its
// matching environment IS visible, so the exclusion is shown to come from the
// environment rule, not from the Kind-mirror filter (source.Kind != kind is
// applied before the environment guard, so a wrong Kind would always look
// excluded).

const collisionProject = "bh-local-demo-prod-west"

// logsFixture registers a provider-class logs source for one OwnerApplication on
// the colliding project; the anchor resolves 'prod-west' for owner 'demo' and
// 'west' for owner 'demo-prod'. It returns the source ID and the anchor env.
// The ID is a three-part "<provider>:<project>:<service>" fixture (the form
// internal/application/runtime_observability.go:242 builds) so the environment
// resolves from the project segment.
func logsFixture(t *testing.T, ownerApplication string) (id string, anchorEnv string) {
	t.Helper()
	id = "loki:" + collisionProject + ":loki"
	proofEnv := ""
	if env, ok := projectTokenFromID(SignalSource{OwnerApplication: ownerApplication, ID: id, Target: RuntimeTarget(collisionProject, "loki")}); !ok || env == "" {
		t.Fatalf("%s logs fixture proof: projectTokenFromID(env)=%q ok=%v, want non-empty (prod-west for 'demo', west for 'demo-prod')", ownerApplication, env, ok)
	} else {
		proofEnv = env
	}
	if err := UpdateSignal(SignalSource{
		ID:               id,
		Kind:             SignalLogs,
		Provider:         capability.ProviderLoki,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: ownerApplication,
		Network:          "net-bh-local-demo-prod-west",
		Target:           RuntimeTarget(collisionProject, "loki"),
		Protocol:         "syslog-rfc5424",
		Path:             "/logs",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}
	return id, proofEnv
}

// metricsFixture registers a provider-class metrics source for one OwnerApplication
// on the colliding project; the anchor resolves 'prod-west' for owner 'demo' and
// 'west' for owner 'demo-prod'. It returns the source ID and the anchor env.
// The ID is a three-part "<provider>:<project>:<service>" fixture so the
// environment resolves from the project segment.
func metricsFixture(t *testing.T, ownerApplication string) (id string, anchorEnv string) {
	t.Helper()
	id = "prometheus:" + collisionProject + ":prometheus"
	proofEnv := ""
	if env, ok := projectTokenFromID(SignalSource{OwnerApplication: ownerApplication, ID: id, Target: RuntimeTarget(collisionProject, "prometheus")}); !ok || env == "" {
		t.Fatalf("%s metrics fixture proof: projectTokenFromID(env)=%q ok=%v, want non-empty (prod-west for 'demo', west for 'demo-prod')", ownerApplication, env, ok)
	} else {
		proofEnv = env
	}
	if err := UpdateSignal(SignalSource{
		ID:               id,
		Kind:             SignalMetrics,
		Provider:         capability.ProviderPrometheus,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: ownerApplication,
		Network:          "net-bh-local-demo-prod-west",
		Target:           RuntimeTarget(collisionProject, "prometheus"),
		Protocol:         "openmetrics",
		Path:             "/metrics",
		Verification:     capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}
	return id, proofEnv
}

// Case (a) logs: OwnerApplication 'demo', project bh-local-demo-prod-west,
// requestedEnvironment 'west' => the anchor resolves 'prod-west', which is not
// 'west', so the logs source must be excluded (fail-closed).
func TestRoute5LogsCaseA_DemoWestCallerExcludes(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	id, anchorEnv := logsFixture(t, "demo")
	if anchorEnv != "prod-west" {
		t.Fatalf("case (a) logs proof: anchor env=%q, want prod-west", anchorEnv)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}
	logs, err := ListLogs(placement, []string{"demo"}, true, false, "west")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatalf("case (a) logs: 'demo' + %s with requested env 'west' must return zero app sources, got %d: %v", collisionProject, len(logs), ids(logs))
	}
	for _, s := range logs {
		if s.ID == id {
			t.Fatalf("case (a) logs: ListLogs must not include the 'demo' prod-west logs provider with id %q", id)
		}
	}

	listLogs, err := List(SignalLogs, placement, []string{"demo"}, false, true, false, "west")
	if err != nil {
		t.Fatal(err)
	}
	if len(listLogs) != 0 {
		t.Fatalf("case (a) logs: List must exclude the 'demo' prod-west logs provider for a west caller, got %d: %v", len(listLogs), ids(listLogs))
	}

	// positive control: requested env 'prod-west' sees exactly this source.
	good, err := ListLogs(placement, []string{"demo"}, true, false, "prod-west")
	if err != nil {
		t.Fatal(err)
	}
	if len(good) != 1 || good[0].ID != id {
		t.Fatalf("case (a) logs positive-control: requested env 'prod-west' must see the demo prod-west logs provider, got %d: %v", len(good), ids(good))
	}
}

// Case (b) logs: OwnerApplication 'demo-prod', SAME project bh-local-demo-prod-west,
// requestedEnvironment 'west' => the anchor resolves 'west', so the logs source
// IS visible.
func TestRoute5LogsCaseB_DemoProdWestCallerIncludes(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	id, anchorEnv := logsFixture(t, "demo-prod")
	if anchorEnv != "west" {
		t.Fatalf("case (b) logs proof: anchor env=%q, want west", anchorEnv)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}
	logs, err := ListLogs(placement, []string{"demo-prod"}, true, false, "west")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].ID != id {
		t.Fatalf("case (b) logs: 'demo-prod' + %s with requested env 'west' must return exactly the one source, got %d: %v", collisionProject, len(logs), ids(logs))
	}

	listLogs, err := List(SignalLogs, placement, []string{"demo-prod"}, false, true, false, "west")
	if err != nil {
		t.Fatal(err)
	}
	if len(listLogs) != 1 || listLogs[0].ID != id {
		t.Fatalf("case (b) logs: List must return exactly the one source, got %d: %v", len(listLogs), ids(listLogs))
	}

	// positive control: a non-matching env 'dev' excludes it.
	miss, err := ListLogs(placement, []string{"demo-prod"}, true, false, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(miss) != 0 {
		t.Fatalf("case (b) logs negative-control: requested env 'dev' must exclude the demo-prod west logs provider, got %d", len(miss))
	}
}

// Case (a) metrics: OwnerApplication 'demo', project bh-local-demo-prod-west,
// requestedEnvironment 'west' => the anchor resolves 'prod-west', which is not
// 'west', so the metrics source must be excluded (fail-closed).
func TestRoute5MetricsCaseA_DemoWestCallerExcludes(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	id, anchorEnv := metricsFixture(t, "demo")
	if anchorEnv != "prod-west" {
		t.Fatalf("case (a) metrics proof: anchor env=%q, want prod-west", anchorEnv)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}
	metrics, err := ListMetrics(placement, []string{"demo"}, true, false, "west")
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 0 {
		t.Fatalf("case (a) metrics: 'demo' + %s with requested env 'west' must return zero app sources, got %d", collisionProject, len(metrics))
	}

	listMetrics, err := List(SignalMetrics, placement, []string{"demo"}, false, true, false, "west")
	if err != nil {
		t.Fatal(err)
	}
	if len(listMetrics) != 0 {
		t.Fatalf("case (a) metrics: List must exclude the 'demo' prod-west metrics provider for a west caller, got %d", len(listMetrics))
	}

	// positive control: requested env 'prod-west' sees exactly this source.
	good, err := ListMetrics(placement, []string{"demo"}, true, false, "prod-west")
	if err != nil {
		t.Fatal(err)
	}
	if len(good) != 1 || good[0].ID != id {
		t.Fatalf("case (a) metrics positive-control: requested env 'prod-west' must see the demo prod-west metrics provider, got %d: id=%q", len(good), id)
	}
}

// Case (b) metrics: OwnerApplication 'demo-prod', SAME project bh-local-demo-prod-west,
// requestedEnvironment 'west' => the anchor resolves 'west', so the metrics
// source IS visible.
func TestRoute5MetricsCaseB_DemoProdWestCallerIncludes(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	id, anchorEnv := metricsFixture(t, "demo-prod")
	if anchorEnv != "west" {
		t.Fatalf("case (b) metrics proof: anchor env=%q, want west", anchorEnv)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}
	metrics, err := ListMetrics(placement, []string{"demo-prod"}, true, false, "west")
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 || metrics[0].ID != id {
		t.Fatalf("case (b) metrics: 'demo-prod' + %s with requested env 'west' must return exactly the one source, got %d: id=%q", collisionProject, len(metrics), id)
	}

	listMetrics, err := List(SignalMetrics, placement, []string{"demo-prod"}, false, true, false, "west")
	if err != nil {
		t.Fatal(err)
	}
	if len(listMetrics) != 1 || listMetrics[0].ID != id {
		t.Fatalf("case (b) metrics: List must return exactly the one source, got %d: id=%q", len(listMetrics), id)
	}

	// positive control: a non-matching env 'dev' excludes it.
	miss, err := ListMetrics(placement, []string{"demo-prod"}, true, false, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(miss) != 0 {
		t.Fatalf("case (b) metrics negative-control: requested env 'dev' must exclude the demo-prod west metrics provider, got %d", len(miss))
	}
}

// Case (c): OwnerApplication 'my-app', generated namespaced project
// 'bh-prod-my-app-dev', requestedEnvironment 'dev' includes, requestedEnvironment
// 'prod' excludes. Asserted on the Logs seam so the exclusion is proven at the
// caller-visible surface.
func TestRoute5ListSeamCaseC_MyAppDevIncludesProdExcludes(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	// Canonical generator project name (internal/runtime/project_names.go:31):
	// 'bh-<target>-<app>-<env>'. One single value drives ID and Target.
	project := bhruntime.ApplicationProjectName("prod", "my-app", "dev")

	// (i) a real application-provider ID is three-segmented:
	// "<provider>:<project>:<service>" (internal/application/runtime_observability.go:242
	// builds id := fmt.Sprintf("%s:%s:%s", provider, project, service)).
	idWithService := "loki:" + project + ":loki"
	if err := UpdateSignal(SignalSource{
		ID:               idWithService,
		Kind:             SignalLogs,
		Provider:         capability.ProviderLoki,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "my-app",
		Network:          "baseharbor-my-app-dev-logs",
		Target:           RuntimeTarget(project, "loki"),
		Protocol:         "syslog-rfc5424",
	}); err != nil {
		t.Fatal(err)
	}

	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}

	devLogs, err := ListLogs(placement, []string{"my-app"}, true, false, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(devLogs) != 1 || devLogs[0].ID != idWithService {
		t.Fatalf("case (c): dev caller must see the my-app dev provider, got %d: %v", len(devLogs), ids(devLogs))
	}

	prodLogs, err := ListLogs(placement, []string{"my-app"}, true, false, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(prodLogs) != 0 {
		t.Fatalf("case (c): prod caller must NOT see the my-app dev provider, got %d: %v", len(prodLogs), ids(prodLogs))
	}

	// (ii) three anchor paths all resolve 'dev' for owner 'my-app', each isolated
	// so one cannot hide another:
	//   (a) three-part ID "<provider>:<project>:<service>" (internal/application/
	//       runtime_observability.go:242 builds id := fmt.Sprintf("%s:%s:%s",
	//       provider, project, service)) — the ID segment carries 'dev'.
	//   (b) runtime:// "<project>/<service>" (registry.go:263 RuntimeTarget) —
	//       the only source of the environment here is the target; the ID is a
	//       bare provider token with no project so the target path is forced.
	//   (c) ID-only (Target = "") — the gap run 44506393063 exposed: with Target
	//       given the target fallback can hide a broken ID grammar; with Target
	//       = "" the ID grammar is the only thing left and must carry 'dev'.
	for _, tc := range []struct {
		name, id, target string
	}{
		{"three-part", "loki:" + project + ":loki", RuntimeTarget(project, "loki")},
		{"runtime-target-only", "loki", RuntimeTarget(project, "loki")},
		{"id-only", "loki:" + project + ":loki", ""},
	} {
		if env, ok := projectTokenFromID(SignalSource{OwnerApplication: "my-app", ID: tc.id, Target: tc.target}); !ok || env != "dev" {
			t.Fatalf("case (c) colon proof [%s]: projectTokenFromID(env)=%q ok=%v, want env=dev ok=true", tc.name, env, ok)
		}
	}

	// a bare provider token with no project carries no anchor: nothing resolves.
	if _, ok := projectTokenFromID(SignalSource{OwnerApplication: "my-app", ID: "loki"}); ok {
		t.Fatalf("case (c) colon proof: bare provider token 'loki' resolved, want ok=false")
	}
	// an empty project segment fails closed.
	if _, ok := projectTokenFromID(SignalSource{OwnerApplication: "my-app", ID: "loki::loki"}); ok {
		t.Fatalf("case (c) colon proof: empty project segment 'loki::loki' resolved, want ok=false")
	}
}

// Regression (r5): the service dimension of a three-part ID is inert — the
// environment resolves from the PROJECT segment alone, so two different services
// on the same project yield the same environment, and the exclusion is driven by
// the requested environment, not by which service the ID names.
func TestRoute5ServiceDimensionInert(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	project := bhruntime.ApplicationProjectName("prod", "my-app", "dev")
	placement := capability.ProviderPlacement{Scope: capability.ScopeApplication}

	lokiID := "loki:" + project + ":loki"
	grafanaID := "loki:" + project + ":grafana"

	// Same project, two different services. Both must resolve 'dev' from their
	// ID alone (Target = ""), proving the service segment does not move the
	// environment.
	for _, id := range []string{lokiID, grafanaID} {
		if env, ok := projectTokenFromID(SignalSource{OwnerApplication: "my-app", ID: id, Target: ""}); !ok || env != "dev" {
			t.Fatalf("service-dimension: projectTokenFromID(%q) env=%q ok=%v, want env=dev ok=true", id, env, ok)
		}
	}

	// Register both at the seam; the dev caller sees exactly one of them per ID.
	if err := UpdateSignal(SignalSource{
		ID: lokiID, Kind: SignalLogs, Provider: capability.ProviderLoki,
		Class: SourceApplicationProvider, Scope: capability.ScopeApplication,
		OwnerApplication: "my-app", Network: "baseharbor-my-app-dev-logs",
		Target: RuntimeTarget(project, "loki"), Protocol: "syslog-rfc5424",
		Verification: capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSignal(SignalSource{
		ID: grafanaID, Kind: SignalLogs, Provider: capability.ProviderTempo,
		Class: SourceApplicationProvider, Scope: capability.ScopeApplication,
		OwnerApplication: "my-app", Network: "baseharbor-my-app-dev-traces",
		Target: RuntimeTarget(project, "tempo"), Protocol: "otlp",
		Path: "/traces", Verification: capability.ObservabilityVerifyBackend,
	}); err != nil {
		t.Fatal(err)
	}

	devLogs, err := ListLogs(placement, []string{"my-app"}, true, false, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(devLogs) != 2 {
		t.Fatalf("service-dimension: dev caller must see both services on the same project, got %d: %v", len(devLogs), ids(devLogs))
	}
	for _, s := range devLogs {
		if s.ID != lokiID && s.ID != grafanaID {
			t.Fatalf("service-dimension: unexpected source id %q", s.ID)
		}
	}

	prodLogs, err := ListLogs(placement, []string{"my-app"}, true, false, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(prodLogs) != 0 {
		t.Fatalf("service-dimension: prod caller must see neither dev-scoped service, got %d: %v", len(prodLogs), ids(prodLogs))
	}
}

// Mutation control (r5): the same physical source, wired WITH its anchor, is
// visible; the same source with a DESTROYED project (no '-<app>-' anchor
// remains) is invisible. If the exclusion in cases (a)-(c) were accidental
// (a fixed index, a constant, a stray slice boundary), destroying the project
// name would not change the outcome. The flip from visible to invisible proves
// the exclusion is structural.
func TestRoute5MutationAnchorDestruction(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())

	// Canonical generator project name; the anchor is '-my-app-' in the project.
	project := bhruntime.ApplicationProjectName("prod", "my-app", "dev")
	if err := UpdateSignal(SignalSource{
		ID:               "loki:" + project + ":loki",
		Kind:             SignalLogs,
		Provider:         capability.ProviderLoki,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "my-app",
		Network:          "baseharbor-my-app-dev-logs",
		Target:           RuntimeTarget(project, "loki"),
		Protocol:         "syslog-rfc5424",
	}); err != nil {
		t.Fatal(err)
	}

	withAnchor, err := ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeApplication},
		[]string{"my-app"}, true, false, "dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(withAnchor) != 1 {
		t.Fatalf("mutation control: source WITH anchor must be visible to the dev caller, got %d", len(withAnchor))
	}

	// Remove the visible source, then register the same logical source with a
	// destroyed project (ID and target both lack '-my-app-'). Anchor resolution
	// now fails closed -> invisible.
	if err := Remove("loki:" + project + ":loki"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSignal(SignalSource{
		ID:               "loki:bh-prod-my-logs:loki",
		Kind:             SignalLogs,
		Provider:         capability.ProviderLoki,
		Class:            SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: "my-app",
		Network:          "net-bh-prod-my-app-logs",
		Target:           RuntimeTarget("bh-prod-my-app", "loki"),
		Protocol:         "syslog-rfc5424",
	}); err != nil {
		t.Fatal(err)
	}

	afterDestruction, err := ListLogs(
		capability.ProviderPlacement{Scope: capability.ScopeApplication},
		[]string{"my-app"}, true, false, "dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterDestruction) != 0 {
		t.Fatalf("mutation control: source with destroyed project must be invisible, got %d", len(afterDestruction))
	}
}

func ids(sources []SignalSource) []string {
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, s.ID)
	}
	return out
}
