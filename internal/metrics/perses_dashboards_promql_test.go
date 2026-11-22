//go:build promqlparser

// This file is only compiled when the `promqlparser` build tag is set. It adds
// the real Prometheus PromQL parser witness that proves the emitted selector is
// syntactically valid exactly the way a running Prometheus instance enforces it
// at load time. The dependency is pulled in through the existing test setup
// (go.mod direct dependency on github.com/prometheus/prometheus); the untagged
// heuristic in perses_dashboards_test.go keeps the default build free of that
// dependency. The coordinator/CI runs the metrics tests with -tags promqlparser.

package metrics

import (
	"strings"
	"testing"

	parser "github.com/prometheus/prometheus/promql/parser"
)

// assertValidPromQLPROMETHEUS parses the Prometheus selector with the real
// Prometheus PromQL parser so a syntactically malformed query (a trailing bare
// matcher token, or a selector missing its closing brace) is rejected exactly
// the way a real Prometheus instance would reject it at load time. This is the
// parser-tagged witness; the default build uses the heuristic assertValidPromQL.
func assertValidPromQLPROMETHEUS(t *testing.T, key, query string) {
	t.Helper()
	expr, err := parser.ParseExpr(query)
	if err != nil {
		t.Fatalf("%s: emitted Prometheus selector is not valid PromQL (%v): %q", key, err, query)
	}
	if expr == nil {
		t.Fatalf("%s: emitted Prometheus selector parsed to nil expression: %q", key, query)
	}
	if strings.Contains(query, "baseharbor_application=\"$application\"") == false {
		t.Fatalf("%s: Prometheus selector missing baseharbor_application matcher: %q", key, query)
	}
	if strings.Contains(query, `baseharbor_environment="$env"}`) == false {
		t.Fatalf("%s: Prometheus selector must carry a closed baseharbor_environment matcher: %q", key, query)
	}
}

// TestPersesPrometheusParserRejectsMalformedSelectors is the negative witness
// under the `promqlparser` build tag: it parses the old trailing-token selector
// and the missing-brace selector with the real Prometheus PromQL parser and
// asserts that BOTH are rejected, proving the regression is covered by the real
// parser and not by a string-shape heuristic.
func TestPersesPrometheusParserRejectsMalformedSelectors(t *testing.T) {
	trailing := `up{job="baseharbor-applications",baseharbor_application="$application",baseharbor_environment="$env",up}`
	if _, err := parser.ParseExpr(trailing); err == nil {
		t.Fatalf("Prometheus parser must reject the trailing bare matcher token form: %q", trailing)
	}
	missingBrace := `up{job="baseharbor-applications",baseharbor_application="$application",baseharbor_environment="$env"`
	if _, err := parser.ParseExpr(missingBrace); err == nil {
		t.Fatalf("Prometheus parser must reject the selector missing its closing brace: %q", missingBrace)
	}
}
