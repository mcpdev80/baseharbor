package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// RuntimeSpan describes one BaseHarbor-owned runtime HTTP operation.
// It intentionally contains metadata only; request/response bodies, tokens,
// credentials and other secret-bearing values never enter telemetry.
type RuntimeSpan struct {
	ServiceName string
	Application string
	Environment string
	Name        string
	Method      string
	Path        string
	StatusCode  int
	StartedAt   time.Time
	EndedAt     time.Time
}

// RuntimeSpanPayload renders a standard OTLP ExportTraceServiceRequest protobuf
// payload without adding a second telemetry model or SDK dependency.
func RuntimeSpanPayload(span RuntimeSpan) ([]byte, string) {
	started := span.StartedAt
	if started.IsZero() {
		started = time.Now()
	}
	ended := span.EndedAt
	if ended.IsZero() || ended.Before(started) {
		ended = started
	}

	identity := strings.Join([]string{
		span.ServiceName,
		span.Application,
		span.Environment,
		span.Name,
		span.Method,
		span.Path,
		strconv.FormatInt(started.UnixNano(), 10),
	}, "\x00")
	traceHash := sha256.Sum256([]byte("runtime-trace\x00" + identity))
	spanHash := sha256.Sum256([]byte("runtime-span\x00" + identity))
	traceID := append([]byte(nil), traceHash[:16]...)
	spanID := append([]byte(nil), spanHash[:8]...)

	name := strings.TrimSpace(span.Name)
	if name == "" {
		name = "baseharbor.runtime.http"
	}
	record := appendBytes(nil, 1, traceID)
	record = appendBytes(record, 2, spanID)
	record = appendString(record, 5, name)
	record = appendFixed64(record, 7, uint64(started.UnixNano()))
	record = appendFixed64(record, 8, uint64(ended.UnixNano()))
	if span.Method != "" {
		record = appendMessage(record, 9, keyValue("http.request.method", span.Method))
	}
	if span.Path != "" {
		record = appendMessage(record, 9, keyValue("url.path", span.Path))
	}
	if span.StatusCode > 0 {
		record = appendMessage(record, 9, keyValue("http.response.status_code", strconv.Itoa(span.StatusCode)))
	}
	if span.Application != "" {
		record = appendMessage(record, 9, keyValue("baseharbor.application", span.Application))
	}
	if span.Environment != "" {
		record = appendMessage(record, 9, keyValue("deployment.environment.name", span.Environment))
	}

	scopeSpans := appendMessage(nil, 2, record)
	resource := []byte{}
	serviceName := strings.TrimSpace(span.ServiceName)
	if serviceName == "" {
		serviceName = "baseharbor-runtime"
	}
	resource = appendMessage(resource, 1, keyValue("service.name", serviceName))
	resource = appendMessage(resource, 1, keyValue("service.namespace", "baseharbor"))
	if span.Application != "" {
		resource = appendMessage(resource, 1, keyValue("baseharbor.application", span.Application))
	}
	if span.Environment != "" {
		resource = appendMessage(resource, 1, keyValue("deployment.environment.name", span.Environment))
	}
	resourceSpans := appendMessage(nil, 1, resource)
	resourceSpans = appendMessage(resourceSpans, 2, scopeSpans)
	return appendMessage(nil, 1, resourceSpans), hex.EncodeToString(traceID)
}
