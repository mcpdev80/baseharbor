package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func TestStatusHumanOutputHidesRuntimeInternalsByDefault(t *testing.T) {
	result := application.StatusResult{
		Application: "mailflow",
		Environment: "production",
		State:       "running",
		Ready:       false,
		Checks: []application.StatusCheck{
			{
				Name:   "runtime-broker",
				OK:     false,
				Detail: "application runtime broker mTLS readiness probe failed: compose exec -T broker curl --fail https://baseharbor-runtime:8443/readyz: 503",
			},
			{
				Name:   "workload",
				OK:     false,
				Detail: "resolve required workload secret SECRET_KEY: inspect OpenBao status: compose exec -T openbao sh -c bao status: service openbao is not running",
			},
		},
	}

	var out bytes.Buffer
	renderApplicationStatus(context.Background(), &out, &out, result)
	got := out.String()

	for _, forbidden := range []string{"compose exec", "curl --fail", "bao status"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("normal status leaked low-level diagnostic %q:\n%s", forbidden, got)
		}
	}
	for _, wanted := range []string{
		"runtime broker is not ready",
		"required secrets unavailable because OpenBao is not running",
		"DEGRADED",
	} {
		if !strings.Contains(got, wanted) {
			t.Fatalf("normal status missing %q:\n%s", wanted, got)
		}
	}
}

func TestStatusVerboseOutputKeepsRuntimeDiagnostics(t *testing.T) {
	result := application.StatusResult{
		Application: "mailflow",
		Environment: "production",
		State:       "running",
		Ready:       false,
		Checks: []application.StatusCheck{
			{
				Name:   "runtime-broker",
				OK:     false,
				Detail: "compose exec -T broker curl --fail https://baseharbor-runtime:8443/readyz: 503",
			},
		},
	}

	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{Verbose: true})
	var out bytes.Buffer
	renderApplicationStatus(ctx, &out, &out, result)
	if !strings.Contains(out.String(), "compose exec -T broker") {
		t.Fatalf("verbose status hid diagnostic detail:\n%s", out.String())
	}
}

func TestStatusTLSRendersBeforeFinalState(t *testing.T) {
	result := application.StatusResult{
		Application: "mailflow",
		Environment: "production",
		State:       "running",
		Ready:       false,
	}
	tlsStatus := applicationTLSStatus{
		State: repositoryInitState{
			TLSMode:  "existing",
			Hostname: "mailflow.mcp-dev.de",
		},
		Installed: &x509.Certificate{
			Subject:  pkix.Name{CommonName: "*.mcp-dev.de"},
			NotAfter: time.Now().Add(120 * 24 * time.Hour),
		},
		Source: &x509.Certificate{
			Subject:  pkix.Name{CommonName: "*.mcp-dev.de"},
			NotAfter: time.Now().Add(120 * 24 * time.Hour),
		},
	}

	var out bytes.Buffer
	renderApplicationStatusWithExtra(context.Background(), &out, &out, result, func(term *cli.Terminal) {
		term.Section("TLS")
		renderApplicationTLSStatus(term, tlsStatus)
	})
	got := out.String()
	tlsIndex := strings.Index(got, "\nTLS\n")
	finalIndex := strings.LastIndex(got, "\nDEGRADED\n")
	if tlsIndex < 0 || finalIndex < 0 || tlsIndex > finalIndex {
		t.Fatalf("TLS must render before final state:\n%s", got)
	}
}
