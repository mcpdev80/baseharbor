package runtimeexplorer

import (
	"context"
	"encoding/json"
	"testing"
)

func TestConnectorMetricsRequiresLiveCapabilityAndExactResource(t *testing.T) {
	b, transport := newRemoteBackend(t)
	ctx := context.Background()
	result, err := b.ContainerMetrics(ctx, "container-a")
	if err != nil || result.Available || len(transport.calls) != 0 {
		t.Fatal("unnegotiated metrics dispatched", result, err)
	}
	transport.metrics = true
	transport.result = json.RawMessage(`{"resource_id":"container-a","cpu_percent":"0.01%","memory_usage":"1MiB / 1GiB","network_io":"0B / 0B"}`)
	result, err = b.ContainerMetrics(ctx, "container-a")
	if err != nil || !result.Available || result.Sample == nil || result.Sample.ObservedAt.IsZero() || result.Sample.MemoryUsage != "1MiB / 1GiB" || transport.calls[0].Operation != "runtime.metrics" {
		t.Fatal(result, err)
	}
	for _, response := range []string{
		`{"resource_id":"foreign","cpu_percent":"1%"}`,
		`{"resource_id":"container-a","cpu_percent":"1%\u001b"}`,
		`null`,
	} {
		transport.result = json.RawMessage(response)
		if _, err := b.ContainerMetrics(ctx, "container-a"); err == nil {
			t.Fatal("invalid remote metric accepted", response)
		}
	}
	transport.result = json.RawMessage(`{"resource_id":"container-a"}`)
	if result, err := b.ContainerMetrics(ctx, "container-a"); err != nil || result.Available {
		t.Fatal("absent native samples invented", result, err)
	}
	transport.failed = true
	if _, err := b.ContainerMetrics(ctx, "container-a"); err == nil {
		t.Fatal("remote failure hidden")
	}
}

func TestMetricsRejectsForeignExplorerContextBeforeDispatch(t *testing.T) {
	b, transport := newRemoteBackend(t)
	transport.metrics = true
	s, err := NewService(b, "lab", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []ResourceRef{
		{Provider: "docker", Target: "foreign", Kind: KindContainer, ResourceID: "container-a"},
		{Provider: "podman", Target: "lab", Kind: KindContainer, ResourceID: "container-a"},
	} {
		if _, err := s.Metrics(context.Background(), ref); err == nil || len(transport.calls) != 0 {
			t.Fatal("foreign metric context dispatched", err)
		}
	}
}
