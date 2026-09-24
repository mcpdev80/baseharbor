package runtimeobservability

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	otelEndpointEnv   = "OTEL_EXPORTER_OTLP_ENDPOINT"
	otelProtocolEnv   = "OTEL_EXPORTER_OTLP_PROTOCOL"
	otelCAEnv         = "OTEL_EXPORTER_OTLP_CERTIFICATE"
	otelClientCertEnv = "OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE"
	otelClientKeyEnv  = "OTEL_EXPORTER_OTLP_CLIENT_KEY"
	otelHeadersEnv    = "OTEL_EXPORTER_OTLP_HEADERS"
)

type Config struct {
	Component   string
	Application string
	Environment string
	LogWriter   io.Writer
}

type Observer struct {
	component   string
	application string
	environment string
	logWriter   io.Writer
	logMu       sync.Mutex

	requests      atomic.Uint64
	errors        atomic.Uint64
	durationNanos atomic.Uint64
	traceDropped  atomic.Uint64

	traceEndpoint string
	traceHeaders  http.Header
	traceClient   *http.Client
	traceQueue    chan traceEvent
}

type traceEvent struct {
	Method     string
	Path       string
	StatusCode int
	Start      time.Time
	End        time.Time
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func NewFromEnvironment(config Config) (*Observer, error) {
	component := strings.TrimSpace(config.Component)
	if component == "" {
		return nil, errors.New("runtime observability component is required")
	}
	writer := config.LogWriter
	if writer == nil {
		writer = os.Stdout
	}
	observer := &Observer{
		component:   component,
		application: strings.TrimSpace(config.Application),
		environment: strings.TrimSpace(config.Environment),
		logWriter:   writer,
	}

	endpoint := strings.TrimSpace(os.Getenv(otelEndpointEnv))
	if endpoint == "" {
		return observer, nil
	}
	protocol := strings.TrimSpace(os.Getenv(otelProtocolEnv))
	if protocol != "" && protocol != "http/protobuf" {
		return nil, fmt.Errorf("runtime observability requires OTLP http/protobuf, got %q", protocol)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("runtime observability OTLP endpoint must be an absolute http/https URL")
	}
	client, err := otlpHTTPClient(parsed)
	if err != nil {
		return nil, err
	}
	headers, err := parseOTLPHeaders(os.Getenv(otelHeadersEnv))
	if err != nil {
		return nil, err
	}
	observer.traceEndpoint = strings.TrimRight(endpoint, "/") + "/v1/traces"
	observer.traceHeaders = headers
	observer.traceClient = client
	observer.traceQueue = make(chan traceEvent, 256)
	go observer.runTraceExporter()
	return observer, nil
}

func (o *Observer) Wrap(next http.Handler) http.Handler {
	if o == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isInternalProbe(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		recorder := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		end := time.Now()
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		o.requests.Add(1)
		if status >= http.StatusInternalServerError {
			o.errors.Add(1)
		}
		o.durationNanos.Add(uint64(end.Sub(start)))

		event := traceEvent{
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: status,
			Start:      start,
			End:        end,
		}
		o.writeRequestLog(event)
		if o.traceQueue != nil {
			select {
			case o.traceQueue <- event:
			default:
				o.traceDropped.Add(1)
			}
		}
	})
}

func (o *Observer) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/openmetrics-text; version=1.0.0; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		component := strconv.Quote(o.component)
		_, _ = fmt.Fprintf(w, "# TYPE baseharbor_runtime_http_requests_total counter\nbaseharbor_runtime_http_requests_total{component=%s} %d\n", component, o.requests.Load())
		_, _ = fmt.Fprintf(w, "# TYPE baseharbor_runtime_http_errors_total counter\nbaseharbor_runtime_http_errors_total{component=%s} %d\n", component, o.errors.Load())
		_, _ = fmt.Fprintf(w, "# TYPE baseharbor_runtime_http_request_duration_seconds counter\nbaseharbor_runtime_http_request_duration_seconds{component=%s} %.9f\n", component, float64(o.durationNanos.Load())/float64(time.Second))
		_, _ = fmt.Fprintf(w, "# TYPE baseharbor_runtime_otlp_dropped_spans_total counter\nbaseharbor_runtime_otlp_dropped_spans_total{component=%s} %d\n# EOF\n", component, o.traceDropped.Load())
	})
}

func (o *Observer) writeRequestLog(event traceEvent) {
	record := map[string]any{
		"timestamp":   event.End.UTC().Format(time.RFC3339Nano),
		"level":       "info",
		"component":   o.component,
		"event":       "http_request",
		"method":      event.Method,
		"path":        event.Path,
		"status":      event.StatusCode,
		"duration_ms": float64(event.End.Sub(event.Start)) / float64(time.Millisecond),
	}
	if o.application != "" {
		record["application"] = o.application
	}
	if o.environment != "" {
		record["environment"] = o.environment
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	o.logMu.Lock()
	defer o.logMu.Unlock()
	_, _ = o.logWriter.Write(append(data, '\n'))
}

func (o *Observer) runTraceExporter() {
	for event := range o.traceQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := o.exportTrace(ctx, event)
		cancel()
		if err != nil {
			o.traceDropped.Add(1)
			o.writeExportError(err)
		}
	}
}

func (o *Observer) exportTrace(ctx context.Context, event traceEvent) error {
	payload, err := tracePayload(o.component, o.application, o.environment, event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.traceEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	for key, values := range o.traceHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := o.traceClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("OTLP endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (o *Observer) writeExportError(err error) {
	record := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"level":     "warn",
		"component": o.component,
		"event":     "otlp_export_failed",
		"error":     err.Error(),
	}
	data, marshalErr := json.Marshal(record)
	if marshalErr != nil {
		return
	}
	o.logMu.Lock()
	defer o.logMu.Unlock()
	_, _ = o.logWriter.Write(append(data, '\n'))
}

func isInternalProbe(path string) bool {
	switch path {
	case "/metrics", "/healthz", "/readyz":
		return true
	default:
		return false
	}
}

func otlpHTTPClient(endpoint *url.URL) (*http.Client, error) {
	transport := &http.Transport{}
	if endpoint.Scheme == "https" {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		if caPath := strings.TrimSpace(os.Getenv(otelCAEnv)); caPath != "" {
			caPEM, err := os.ReadFile(caPath)
			if err != nil {
				return nil, fmt.Errorf("read runtime OTLP trust bundle: %w", err)
			}
			roots := x509.NewCertPool()
			if !roots.AppendCertsFromPEM(caPEM) {
				return nil, errors.New("runtime OTLP trust bundle is invalid")
			}
			tlsConfig.RootCAs = roots
		}
		certPath := strings.TrimSpace(os.Getenv(otelClientCertEnv))
		keyPath := strings.TrimSpace(os.Getenv(otelClientKeyEnv))
		if (certPath == "") != (keyPath == "") {
			return nil, errors.New("runtime OTLP client certificate and key must be configured together")
		}
		if certPath != "" {
			cert, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err != nil {
				return nil, fmt.Errorf("load runtime OTLP client identity: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}, nil
}

func parseOTLPHeaders(raw string) (http.Header, error) {
	headers := http.Header{}
	for _, item := range strings.Split(strings.TrimSpace(raw), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, errors.New("runtime OTLP headers are invalid")
		}
		decoded, err := url.QueryUnescape(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("decode runtime OTLP header %q: %w", key, err)
		}
		headers.Add(strings.TrimSpace(key), decoded)
	}
	return headers, nil
}

func tracePayload(component, application, environment string, event traceEvent) ([]byte, error) {
	traceID := make([]byte, 16)
	spanID := make([]byte, 8)
	if _, err := rand.Read(traceID); err != nil {
		return nil, err
	}
	if _, err := rand.Read(spanID); err != nil {
		return nil, err
	}

	span := appendBytes(nil, 1, traceID)
	span = appendBytes(span, 2, spanID)
	span = appendString(span, 5, "baseharbor.runtime.request")
	span = appendVarint(span, 6, 2)
	span = appendFixed64(span, 7, uint64(event.Start.UnixNano()))
	span = appendFixed64(span, 8, uint64(event.End.UnixNano()))
	span = appendMessage(span, 9, keyValue("http.request.method", event.Method))
	span = appendMessage(span, 9, keyValue("url.path", event.Path))
	span = appendMessage(span, 9, keyValueInt("http.response.status_code", int64(event.StatusCode)))
	if event.StatusCode >= http.StatusInternalServerError {
		status := appendVarint(nil, 3, 2)
		span = appendMessage(span, 15, status)
	}

	scope := appendString(nil, 1, "baseharbor.runtime")
	scopeSpans := appendMessage(nil, 1, scope)
	scopeSpans = appendMessage(scopeSpans, 2, span)

	resource := []byte{}
	resource = appendMessage(resource, 1, keyValue("service.name", component))
	resource = appendMessage(resource, 1, keyValue("baseharbor.component", component))
	if application != "" {
		resource = appendMessage(resource, 1, keyValue("service.namespace", application))
		resource = appendMessage(resource, 1, keyValue("baseharbor.application", application))
	}
	if environment != "" {
		resource = appendMessage(resource, 1, keyValue("deployment.environment.name", environment))
	}

	resourceSpans := appendMessage(nil, 1, resource)
	resourceSpans = appendMessage(resourceSpans, 2, scopeSpans)
	return appendMessage(nil, 1, resourceSpans), nil
}

func keyValue(key, value string) []byte {
	anyValue := appendString(nil, 1, value)
	msg := appendString(nil, 1, key)
	return appendMessage(msg, 2, anyValue)
}

func keyValueInt(key string, value int64) []byte {
	anyValue := appendVarint(nil, 3, uint64(value))
	msg := appendString(nil, 1, key)
	return appendMessage(msg, 2, anyValue)
}

func appendTag(dst []byte, field int, wire byte) []byte {
	return binary.AppendUvarint(dst, uint64(field<<3)|uint64(wire))
}

func appendMessage(dst []byte, field int, msg []byte) []byte {
	dst = appendTag(dst, field, 2)
	dst = binary.AppendUvarint(dst, uint64(len(msg)))
	return append(dst, msg...)
}

func appendBytes(dst []byte, field int, value []byte) []byte {
	return appendMessage(dst, field, value)
}

func appendString(dst []byte, field int, value string) []byte {
	return appendMessage(dst, field, []byte(value))
}

func appendVarint(dst []byte, field int, value uint64) []byte {
	dst = appendTag(dst, field, 0)
	return binary.AppendUvarint(dst, value)
}

func appendFixed64(dst []byte, field int, value uint64) []byte {
	dst = appendTag(dst, field, 1)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	return append(dst, buf[:]...)
}
