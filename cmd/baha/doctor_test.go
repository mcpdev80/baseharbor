package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/health"
)

func TestClassifyDoctorFindings(t *testing.T) {
	checks := []health.Check{
		{Name: "os", OK: true, Message: "linux/amd64"},
		{Name: "postgres", OK: false, Message: "connection failed on 127.0.0.1:5432"},
		{Name: "openbao", OK: false, Message: "initialized but sealed; run 'baha openbao unseal --recovery-file PATH'"},
		{Name: "container-runtime", OK: false, Message: "docker/podman daemon not reachable"},
	}

	findings := classifyDoctorFindings(checks)
	if len(findings) != 3 {
		t.Fatalf("len(findings) = %d, want 3", len(findings))
	}

	byName := make(map[string]doctorFinding, len(findings))
	for _, finding := range findings {
		byName[finding.Check.Name] = finding
	}
	if got := byName["postgres"].Class; got != doctorAutoFixable {
		t.Fatalf("postgres class = %q, want %q", got, doctorAutoFixable)
	}
	if got := byName["openbao"].Class; got != doctorNeedsInput {
		t.Fatalf("openbao class = %q, want %q", got, doctorNeedsInput)
	}
	if got := byName["container-runtime"].Class; got != doctorManualAction {
		t.Fatalf("container-runtime class = %q, want %q", got, doctorManualAction)
	}
}

func TestClassifyUnreachableOpenBaoAsAutoFixable(t *testing.T) {
	findings := classifyDoctorFindings([]health.Check{{
		Name:    "openbao",
		OK:      false,
		Message: "not reachable at 127.0.0.1:8200",
	}})
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Class != doctorAutoFixable {
		t.Fatalf("class = %q, want %q", findings[0].Class, doctorAutoFixable)
	}
}
