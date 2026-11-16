package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
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
