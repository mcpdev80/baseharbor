package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRuntimeBrokerMetricsTargetIsNamespacedByApplication(t *testing.T) {
	first := application.New("full-demo", "dev", false, false, false)
	second := application.New("small-demo", "dev", false, false, false)
	firstTarget := runtimeBrokerMetricsTarget(first)
	if firstTarget != application.MetricsTargetAlias(first, "runtime-broker-provider")+":8443" {
		t.Fatalf("runtime broker metrics target = %q", firstTarget)
	}
	if firstTarget == runtimeBrokerMetricsTarget(second) {
		t.Fatalf("distinct application brokers share metrics target %q", firstTarget)
	}
}
