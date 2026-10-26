package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestDeclaredExposureWithRemovedGatewayRoutesDoesNotBlameManifest(t *testing.T) {
	target := configureTestTarget(t)
	m := application.New("stopped", "dev", false, false, false)
	m.Exposures = []application.HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "http"}}
	c := applicationStatusCollection{manifest: m, resolved: resolvedApplication{Target: target}}
	c.collectCanonicalDevelopmentCheck(context.Background())
	if len(c.result.Checks) != 1 {
		t.Fatalf("missing stopped-route finding: %+v", c.result)
	}
	check := c.result.Checks[0]
	if check.OK || !strings.Contains(check.Detail, "stopped") || !strings.Contains(check.Detail, "baha up") || strings.Contains(check.Detail, "defines no canonical route") || strings.Contains(check.Detail, "declare the required") {
		t.Fatalf("misdiagnosed declared exposure: %+v", check)
	}
}
