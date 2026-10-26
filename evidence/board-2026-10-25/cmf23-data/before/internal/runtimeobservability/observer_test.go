package runtimeobservability

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestObserverEmitsMetricsAndStructuredRequestLog(t *testing.T) {
	var logs bytes.Buffer
	observer, err := NewFromEnvironment(Config{
		Component:   "runtime-broker",
		Application: "demo",
		Environment: "dev",
		LogWriter:   &logs,
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := observer.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "https://runtime.test/runtime/v1/query?secret=ignored", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if traceparent := rec.Header().Get("traceparent"); !strings.HasPrefix(traceparent, "00-") || len(traceparent) != 55 {
		t.Fatalf("traceparent = %q", traceparent)
	}

	records := decodeLogRecords(t, logs.Bytes())
	if len(records) != 2 {
		t.Fatalf("log records = %#v", records)
	}
	startup := records[0]
	if startup["component"] != "runtime-broker" || startup["application"] != "demo" || startup["environment"] != "dev" || startup["event"] != "started" {
		t.Fatalf("startup log attribution = %#v", startup)
	}
	record := records[1]
	if record["component"] != "runtime-broker" || record["application"] != "demo" || record["environment"] != "dev" {
		t.Fatalf("request log attribution = %#v", record)
	}
	if record["path"] != "/runtime/v1/query" {
		t.Fatalf("logged path = %#v", record["path"])
	}
	if strings.Contains(logs.String(), "secret") {
		t.Fatalf("structured request log leaked query string: %s", logs.String())
	}

	metrics := httptest.NewRecorder()
	observer.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, want := range []string{
		"application/openmetrics-text",
		"baseharbor_runtime_http_requests_total{component=\"runtime-broker\"} 1",
		"baseharbor_runtime_http_errors_total{component=\"runtime-broker\"} 0",
		"# EOF",
	} {
		if !strings.Contains(metrics.Body.String(), want) && !strings.Contains(metrics.Header().Get("Content-Type"), want) {
			t.Fatalf("metrics response missing %q: headers=%v body=%s", want, metrics.Header(), metrics.Body.String())
		}
	}
}

func TestObserverSkipsHealthAndMetricsSelfInstrumentation(t *testing.T) {
	observer, err := NewFromEnvironment(Config{Component: "runtime-executor"})
	if err != nil {
		t.Fatal(err)
	}
	handler := observer.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, path := range []string{"/metrics", "/healthz", "/readyz"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	metrics := httptest.NewRecorder()
	observer.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), "baseharbor_runtime_http_requests_total{component=\"runtime-executor\"} 0") {
		t.Fatalf("internal probes affected request metrics: %s", metrics.Body.String())
	}
}

func TestObserverExportsOTLPHTTPProtobuf(t *testing.T) {
	payloads := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		if r.Header.Get("Content-Type") != "application/x-protobuf" {
			http.Error(w, "unexpected content type", http.StatusUnsupportedMediaType)
			return
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		payloads <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv(otelEndpointEnv, server.URL)
	t.Setenv(otelProtocolEnv, "http/protobuf")

	observer, err := NewFromEnvironment(Config{
		Component:   "runtime-executor",
		Application: "demo",
		Environment: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := observer.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/internal/v1/execute", nil))

	select {
	case payload := <-payloads:
		for _, want := range []string{"baseharbor.runtime.request", "runtime-executor", "demo", "test", "/internal/v1/execute"} {
			if !bytes.Contains(payload, []byte(want)) {
				t.Fatalf("OTLP payload missing %q", want)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OTLP trace was not exported")
	}
}

func decodeLogRecords(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode log record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}
