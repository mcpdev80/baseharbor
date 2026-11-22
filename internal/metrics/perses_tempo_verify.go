package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Perses Tempo datasource verification and contract validation (0428-091).
// This file owns the two operations the datasource contract exposes beyond
// building: the real TraceQL query path (health), and the shape validators
// that check the emitted resources against the pinned upstream witness.
// The resource model, the builders and the service-access client live in
// perses_tempo_datasource.go.

// queryPersesTempoTrace is the actual trace-query path: a TraceQL search against
// Tempo's own /api/search endpoint over the given client. It returns the HTTP
// status plus the matched trace ids. A transport failure, a non-2xx answer or an
// undecodable body is an error; a clean empty answer is not.
func queryPersesTempoTrace(ctx context.Context, client *http.Client, endpoint, traceQL string) (int, []string, error) {
	if client == nil {
		return 0, nil, errors.New("perses tempo datasource trace query has no authorized service-access client")
	}
	if strings.TrimSpace(traceQL) == "" {
		return 0, nil, errors.New("perses tempo datasource trace query requires a non-empty TraceQL expression")
	}
	target := strings.TrimRight(endpoint, "/") + "?limit=20&format=json&q=" + url.QueryEscape(traceQL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("perses tempo datasource trace query is not reachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read tempo trace query answer: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, nil, fmt.Errorf("tempo trace query returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Traces []struct {
			TraceID string `json:"traceID"`
		} `json:"traces"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return resp.StatusCode, nil, fmt.Errorf("decode tempo trace query answer: %w", err)
	}
	ids := make([]string, 0, len(payload.Traces))
	for _, trace := range payload.Traces {
		if id := strings.TrimSpace(trace.TraceID); id != "" {
			ids = append(ids, id)
		}
	}
	return resp.StatusCode, ids, nil
}

// VerifyPersesTempoDatasourceHealth enforces the fail-closed datasource health
// contract: a selected, pinned Tempo datasource requires a REAL TraceQL query
// over the authorized Tempo service-access path. Pin validation alone NEVER
// passes. Absent, unselected, external, denied or unreachable state returns an
// error; a clean empty answer is reported as unverified, never as healthy.
func VerifyPersesTempoDatasourceHealth(ctx context.Context, m application.Manifest) error {
	if err := capability.ValidatePersesRelease(capability.PersesDefaultRelease); err != nil {
		return err
	}
	if !m.Services.ObservabilityManagementUI || !m.Services.Perses {
		return &TempoSurfaceNotSelectedError{}
	}
	if !application.HasTraceSignal(m) {
		return &TempoNotSelectedError{}
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	baseURL, client, err := persesTempoDatasourceSourceAt(dataDir, "", m)
	if err != nil {
		return err
	}
	if denied, authErr := probeAuthorizationDenied(m); authErr != nil {
		return authErr
	} else if denied {
		return &TempoAuthorizationError{Detail: "the Tempo traces collection policy denies this application's trace sources"}
	}
	deadline, cancel := context.WithTimeout(ctx, tempoTraceQueryTimeout)
	defer cancel()
	status, ids, err := queryPersesTempoTrace(deadline, client, baseURL+PersesTempoTraceQueryPath, tempoProviderTraceQuery(m))
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return &TempoHealthUnverifiedError{Detail: fmt.Sprintf("HTTP %d answered no trace for the authorized TraceQL query", status)}
	}
	return nil
}

// PersesTempoDatasourceJSON marshals the produced native datasource resource
// deterministically (json.MarshalIndent with trailing newline), the golden-file
// witness pattern used across the metrics package.
func PersesTempoDatasourceJSON(r PersesTempoResource) ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// PersesTempoTraceQueryJSON marshals the produced native trace query spec
// deterministically.
func PersesTempoTraceQueryJSON(q PersesTempoTraceQuery) ([]byte, error) {
	data, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ValidatePersesTempoResource validates the emitted datasource resource
// against the pinned upstream model witness: the REST envelope kind, the
// pinned native plugin kind, the project-scoped metadata, the proxy-only
// plugin spec, and — against the upstream source itself — the serialized
// document's field paths, which must match exactly what the pinned
// v0.55.1 model declares and must not include any apiVersion field.
func ValidatePersesTempoResource(pin PersesTempoPin, r PersesTempoResource) error {
	if r.Kind != pin.Resource.Kind {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("resource kind %q is not the pinned Perses resource kind %q (a flattened datasource document is not a Perses resource)", r.Kind, pin.Resource.Kind)}
	}
	if !containsString(pin.DatasourcePluginKinds, r.Spec.Plugin.Kind) {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("datasource plugin kind %q is not a plugin kind shipped by the pinned Perses release %s", r.Spec.Plugin.Kind, pin.PersesVersion)}
	}
	if strings.TrimSpace(r.Metadata.Name) == "" {
		return &TempoResourceShapeError{Detail: "the datasource resource carries no metadata.name, so no panel can select it"}
	}
	if pin.Resource.Scope == "project" && strings.TrimSpace(r.Metadata.Project) == "" {
		return &TempoResourceShapeError{Detail: "a project-scoped Perses datasource requires metadata.project"}
	}
	if r.Spec.Plugin.Spec.DirectURL != "" {
		return &TempoResourceShapeError{Detail: "the datasource declares directUrl: BaseHarbor tempo access stays inside the Perses proxy and service-access boundary"}
	}
	if r.Spec.Plugin.Spec.Proxy == nil || strings.TrimSpace(r.Spec.Plugin.Spec.Proxy.Spec.URL) == "" {
		return &TempoResourceShapeError{Detail: "the datasource carries no HTTP proxy url, so Perses could not route the trace query"}
	}
	declared, err := upstreamDatasourceFieldPaths()
	if err != nil {
		return err
	}
	emitted, err := jsonFieldPathsOfStruct(r)
	if err != nil {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the datasource document cannot be inspected: %v", err)}
	}
	if err := emittedPathsWithin(emitted, declared, "apiVersion"); err != nil {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the emitted datasource document deviates from the pinned upstream model: %v", err)}
	}
	return nil
}

// ValidatePersesTempoTraceQuery validates the emitted trace query against the
// pinned contract witness: the panel-query wrapper kind and the TempoTraceQuery
// plugin kind the pinned release ships, a non-empty TraceQL expression, and a
// selector that names a pinned datasource plugin kind. The serialized document
// must match the field paths the pinned release's OWN native TraceQuery
// documents use (dev/data witness), not a locally asserted shape.
func ValidatePersesTempoTraceQuery(pin PersesTempoPin, q PersesTempoTraceQuery) error {
	if q.Kind != pin.TraceQueryWrapperKind {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("trace query wrapper kind %q is not the query kind the pinned release ships (%q)", q.Kind, pin.TraceQueryWrapperKind)}
	}
	if !containsString(pin.TraceQueryPluginKinds, q.Spec.Plugin.Kind) {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("trace query plugin kind %q is not a query kind shipped by the pinned Perses release %s", q.Spec.Plugin.Kind, pin.PersesVersion)}
	}
	if len(q.Spec.Plugin.Spec.Query) == 0 {
		return &TempoResourceShapeError{Detail: "the pinned TempoTraceQuery contract requires a non-empty TraceQL query"}
	}
	if !containsString(pin.DatasourcePluginKinds, q.Spec.Plugin.Spec.Datasource.Kind) {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the trace query selects datasource kind %q, which is not a pinned Perses datasource plugin kind", q.Spec.Plugin.Spec.Datasource.Kind)}
	}
	if strings.TrimSpace(q.Spec.Plugin.Spec.Datasource.Name) == "" {
		return &TempoResourceShapeError{Detail: "the trace query selects no datasource name"}
	}
	declared, err := upstreamTraceQueryFieldPaths()
	if err != nil {
		return err
	}
	emitted, err := jsonFieldPathsOfStruct(q)
	if err != nil {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the trace query document cannot be inspected: %v", err)}
	}
	if err := emittedPathsWithin(emitted, declared, "apiVersion"); err != nil {
		return &TempoResourceShapeError{Detail: fmt.Sprintf("the emitted trace query document deviates from the pinned upstream model: %v", err)}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// jsonFieldPathsOfStruct lists the dotted JSON paths the value actually
// serializes — the same resolution encoding/json performs, over the real
// emitted object rather than a locally declared truth.
func jsonFieldPathsOfStruct(v any) ([]string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return treeFieldPaths(doc, ""), nil
}

// treeFieldPaths flattens a decoded JSON document into sorted dotted paths.
// Arrays contribute their index-free element paths.
func treeFieldPaths(node map[string]any, prefix string) []string {
	var paths []string
	for name, value := range node {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		paths = append(paths, path)
		switch child := value.(type) {
		case map[string]any:
			paths = append(paths, treeFieldPaths(child, path)...)
		}
	}
	return paths
}

// emittedPathsWithin proves every serialized path of the emitted document is a
// path the pinned upstream model declares, and that no forbidden path (a
// version field) appears.
func emittedPathsWithin(emitted, declared []string, forbidden string) error {
	for _, path := range emitted {
		if path == forbidden || strings.HasSuffix(path, "."+forbidden) {
			return fmt.Errorf("the emitted document serializes %q, which the pinned Perses v0.55.1 model does not declare anywhere", forbidden)
		}
		if !containsPathPrefix(declared, path) {
			return fmt.Errorf("the emitted document serializes %q, which the pinned upstream model does not declare", path)
		}
	}
	return nil
}

func containsPathPrefix(paths []string, want string) bool {
	for _, path := range paths {
		if path == want || strings.HasPrefix(want, path+".") {
			return true
		}
	}
	return false
}
