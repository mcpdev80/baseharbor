package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/runtime"
)

func ExportProviderInteractionTrace(ctx context.Context, m application.Manifest, source observability.SignalSource) (string, error) {
	dataDir, err := runtime.DataDir("")
	if err != nil {
		return "", err
	}
	return ExportProviderInteractionTraceAt(ctx, m, source, dataDir, "")
}

// ExportNativeReceiverTraceAt produces the verification span for a native OTLP
// trace source whose provider names the OTLP receiver that must accept it. The
// source itself carries no endpoint: the span travels over exactly the
// application's materialized telemetry binding, so the managed collector is
// exercised through its published HTTPS receiver and a registered external
// source through its authorized endpoint. A provider that ingests no spans
// fails visibly here instead of being probed against an unrelated service.
func ExportNativeReceiverTraceAt(ctx context.Context, m application.Manifest, source observability.SignalSource, files application.RuntimeFiles, dataDir, namespace string) (string, error) {
	if source.Kind != observability.SignalTraces || source.Protocol != "otlp" {
		return "", fmt.Errorf("native OTLP trace source %q is not an otlp trace", source.ID)
	}
	switch source.Provider {
	case capability.ProviderOTelCollector, capability.ProviderExternalOTLP:
	default:
		return "", fmt.Errorf("native OTLP trace source %q has provider %q that receives no OTLP spans", source.ID, source.Provider)
	}
	binding, found, err := application.ExistingRuntimeOTLPBinding(m, files)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("native OTLP trace source %q has no materialized telemetry binding", source.ID)
	}
	if binding.Provider != source.Provider {
		return "", fmt.Errorf("native OTLP trace source %q is registered for provider %q but the application exports to %q", source.ID, source.Provider, binding.Provider)
	}
	// The source declares no endpoint of its own: it names the receiver the
	// application's spans already reach. So the verification span travels over
	// exactly that materialized binding, using the transport, TLS material and
	// authorization the deployment projection already holds. Rebuilding a
	// transport here would prove a delivery path the workload never uses.
	if err := VerifyApplicationAt(ctx, m, files, dataDir, namespace); err != nil {
		return "", fmt.Errorf("export native OTLP trace source %s: %w", source.ID, err)
	}
	return ProbeTraceIDHex, nil
}

func ExportProviderInteractionTraceAt(ctx context.Context, m application.Manifest, source observability.SignalSource, dataDir, namespace string) (string, error) {
	if source.Kind != observability.SignalTraces || source.Protocol != "interaction" {
		return "", fmt.Errorf("provider interaction trace source %q is not an interaction trace", source.ID)
	}
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return "", err
	}
	endpoint, err := providerEndpoint(files)
	if err != nil {
		return "", err
	}
	client, err := managedOTLPHTTPClient(m.Environment, files)
	if err != nil {
		return "", err
	}
	payload, traceID := providerInteractionTracePayload(m, source)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/v1/traces", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("export provider interaction trace %s: %w", source.ID, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("export provider interaction trace %s: endpoint returned HTTP %d", source.ID, resp.StatusCode)
	}
	return traceID, nil
}
