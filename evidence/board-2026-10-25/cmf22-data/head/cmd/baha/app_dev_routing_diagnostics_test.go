package main

import (
	"errors"
	"strings"
	"testing"
)

func TestDevelopmentGatewayUpstreamDiagnostic(t *testing.T) {
	for _, code := range []string{"502", "503", "504"} {
		upstream := errors.New("browser surface final response is HTTP " + code + " at /private/Caddyfile/state.json")
		diagnosed := diagnoseDevelopmentGatewayVerification(upstream)
		got := diagnosed.Error()
		for _, required := range []string{"HTTP " + code, "exposure.http.port", "selected Target", "baha doctor"} {
			if !strings.Contains(got, required) {
				t.Errorf("%s diagnostic missing %q: %s", code, required, got)
			}
		}
		for _, forbidden := range []string{"canonical route missing", "Caddyfile", "state.json", "/private/"} {
			if strings.Contains(got, forbidden) {
				t.Errorf("%s diagnostic exposed misleading/private details %q: %s", code, forbidden, got)
			}
		}
	}
}

func TestDevelopmentGatewayNonUpstreamErrorNotMisclassified(t *testing.T) {
	source := errors.New("canonical authority mismatch")
	diagnosed := diagnoseDevelopmentGatewayVerification(source)
	if !errors.Is(diagnosed, source) {
		t.Fatal("non-upstream gateway errors should retain typed cause")
	}
	if strings.Contains(diagnosed.Error(), "HTTP 502") {
		t.Fatal("non-upstream failure misclassified as 502")
	}
	if diagnoseDevelopmentGatewayVerification(nil) != nil {
		t.Fatal("nil gateway error should remain nil")
	}
}
