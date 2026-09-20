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
	"github.com/mcpdev80/baseharbor/internal/preflight"
)

func TestDoctorHumanOutputHidesRuntimeInternalsByDefault(t *testing.T) {
	results := []preflight.Result{
		{Name: "OpenBao application scope", OK: false, Detail: "compose exec -T openbao sh -c bao status -format=json: service openbao is not running"},
		{Name: "application runtime broker", OK: false, Detail: "compose exec -T broker curl --fail https://baseharbor-runtime:8443/readyz: 503"},
		{Name: "required application secrets", OK: false, Detail: "inspect OpenBao status: compose exec -T openbao sh -c bao status"},
		{Name: "repository workload", OK: false, Detail: "resolve required workload secret SECRET_KEY: inspect OpenBao status: compose exec -T openbao sh -c bao status"},
	}
	var out bytes.Buffer
	renderApplicationDoctor(
		context.Background(),
		&out,
		&out,
		application.Manifest{Name: "mailflow", Environment: "production"},
		results,
		repositoryWorkloadStatus{},
		nil,
		application.WorkloadSecurityReport{},
		false,
		nil,
		nil,
	)
	got := out.String()
	for _, forbidden := range []string{"compose exec", "curl --fail", "bao status"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("normal doctor leaked %q:\n%s", forbidden, got)
		}
	}
	for _, wanted := range []string{
		"OpenBao application scope unavailable",
		"runtime broker is not ready",
		"required secrets could not be verified because OpenBao is unavailable",
		"workload cannot resolve required secrets because OpenBao is unavailable",
	} {
		if !strings.Contains(got, wanted) {
			t.Fatalf("normal doctor missing %q:\n%s", wanted, got)
		}
	}
	if strings.Contains(got, "Workload services") {
		t.Fatalf("empty workload services section rendered:\n%s", got)
	}
}

func TestDoctorVerboseKeepsRuntimeDiagnostics(t *testing.T) {
	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{Verbose: true})
	results := []preflight.Result{
		{Name: "application runtime broker", OK: false, Detail: "compose exec -T broker curl --fail https://baseharbor-runtime:8443/readyz: 503"},
	}
	var out bytes.Buffer
	renderApplicationDoctor(
		ctx,
		&out,
		&out,
		application.Manifest{Name: "mailflow", Environment: "production"},
		results,
		repositoryWorkloadStatus{},
		nil,
		application.WorkloadSecurityReport{},
		false,
		nil,
		nil,
	)
	if !strings.Contains(out.String(), "compose exec -T broker") {
		t.Fatalf("verbose doctor hid diagnostic detail:\n%s", out.String())
	}
}

func TestDoctorTLSRendersBeforeFinalState(t *testing.T) {
	status := &applicationTLSStatus{
		State: repositoryInitState{TLSMode: "existing", Hostname: "mailflow.mcp-dev.de"},
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
	renderApplicationDoctor(
		context.Background(),
		&out,
		&out,
		application.Manifest{Name: "mailflow", Environment: "production"},
		[]preflight.Result{{Name: "manifest", OK: false, Detail: "example failure"}},
		repositoryWorkloadStatus{},
		nil,
		application.WorkloadSecurityReport{},
		false,
		status,
		nil,
	)
	got := out.String()
	tlsIndex := strings.Index(got, "\nTLS\n")
	finalIndex := strings.LastIndex(got, "\nDEGRADED")
	if tlsIndex < 0 || finalIndex < 0 || tlsIndex > finalIndex {
		t.Fatalf("TLS must render before final state:\n%s", got)
	}
}
