package runtimeexplorer

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
)

func (b *ConnectorBackend) ContainerMetrics(ctx context.Context, id string) (MetricsHandle, error) {
	if err := ctx.Err(); err != nil {
		return MetricsHandle{}, err
	}
	capabilities, err := b.pool.LiveCapabilities(b.scope)
	if err != nil {
		return MetricsHandle{}, err
	}
	available := false
	for _, capability := range capabilities.Capabilities {
		if capability.Name == "runtime.metrics" {
			available = capability.Available
		}
	}
	if !available {
		return MetricsHandle{Available: false}, nil
	}
	var sample MetricsSample
	if err := b.invoke(ctx, "runtime.metrics", map[string]string{"resource_id": id}, &sample); err != nil {
		return MetricsHandle{}, err
	}
	if sample.ResourceID != id {
		return MetricsHandle{}, errors.New("remote metrics resource identity differs")
	}
	for _, value := range []string{sample.CPUPercent, sample.MemoryUsage, sample.NetworkIO} {
		if len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return MetricsHandle{}, errors.New("invalid remote native metrics value")
		}
	}
	if sample.CPUPercent == "" && sample.MemoryUsage == "" && sample.NetworkIO == "" {
		return MetricsHandle{Available: false}, nil
	}
	sample.ObservedAt = time.Now().UTC()
	return MetricsHandle{Available: true, Sample: &sample}, nil
}
