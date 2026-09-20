package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/preflight"
)

func TestParseAppDoctorRepairArgs(t *testing.T) {
	nameArgs, fix, err := parseAppDoctorRepairArgs([]string{"mailflow", "--fix"})
	if err != nil {
		t.Fatal(err)
	}
	if !fix {
		t.Fatal("expected --fix")
	}
	if len(nameArgs) != 1 || nameArgs[0] != "mailflow" {
		t.Fatalf("nameArgs = %#v", nameArgs)
	}
}

func TestClassifyStructuredAppDoctorGeneratedSecretAutoFixable(t *testing.T) {
	result := appDoctorStructuredResult{
		Healthy: false,
		Checks: []preflight.Result{
			{Name: "required application secrets", OK: false, Detail: "missing application secrets"},
		},
	}
	result.RequiredSecrets = append(result.RequiredSecrets, struct {
		Name      string `json:"name"`
		Present   bool   `json:"present"`
		Usable    bool   `json:"usable"`
		Generated bool   `json:"generated"`
	}{
		Name: "SECRET_KEY", Present: false, Usable: false, Generated: true,
	})
	findings := classifyStructuredAppDoctor(result)
	if len(findings) != 1 || findings[0].Class != doctorAutoFixable {
		t.Fatalf("expected generated secret to be auto-fixable: %#v", findings)
	}
}

func TestClassifyStructuredAppDoctorPermissionFailureManual(t *testing.T) {
	result := appDoctorStructuredResult{
		Healthy: false,
		Checks: []preflight.Result{
			{Name: "manifest permissions", OK: false, Detail: "baseharbor.yaml is writable by others"},
		},
	}
	findings := classifyStructuredAppDoctor(result)
	if len(findings) != 1 || findings[0].Class != doctorManualAction {
		t.Fatalf("expected permission failure to require manual action: %#v", findings)
	}
}

func TestClassifyStructuredAppDoctorMailFlowRecoveryIsAutoFixable(t *testing.T) {
	result := appDoctorStructuredResult{
		Healthy: false,
		Checks: []preflight.Result{
			{Name: "managed runtime definition", OK: false, Detail: "application runtime definition differs from the BaseHarbor-managed definition"},
			{Name: "OpenBao application scope", OK: false, Detail: "inspect OpenBao status: service openbao is not running"},
			{Name: "application runtime broker", OK: false, Detail: "runtime broker readiness probe returned 503"},
			{Name: "required application secrets", OK: false, Detail: "inspect OpenBao status: service openbao is not running"},
			{Name: "workload security", OK: false, Detail: "services.api.environment.SECRET_KEY: required variable SECRET_KEY is missing a value: SECRET_KEY is required - managed by BaseHarbor/OpenBao"},
			{Name: "repository workload", OK: false, Detail: "resolve required workload secret SECRET_KEY: inspect OpenBao status: service openbao is not running"},
		},
	}
	findings := classifyStructuredAppDoctor(result)
	if len(findings) != 6 {
		t.Fatalf("len(findings) = %d, want 6: %#v", len(findings), findings)
	}
	if !allAppDoctorFindingsAutoFixable(findings) {
		t.Fatalf("expected MailFlow recovery findings to be auto-fixable: %#v", findings)
	}
}

func TestClassifyStructuredAppDoctorExternalSecretStillNeedsInput(t *testing.T) {
	result := appDoctorStructuredResult{
		Healthy: false,
		Checks: []preflight.Result{
			{Name: "required application secrets", OK: false, Detail: "missing application secrets"},
		},
	}
	result.RequiredSecrets = append(result.RequiredSecrets, struct {
		Name      string `json:"name"`
		Present   bool   `json:"present"`
		Usable    bool   `json:"usable"`
		Generated bool   `json:"generated"`
	}{
		Name: "SMTP_PASSWORD", Present: false, Usable: false, Generated: false,
	})
	findings := classifyStructuredAppDoctor(result)
	if len(findings) != 1 || findings[0].Class != doctorNeedsInput {
		t.Fatalf("expected external secret to require input: %#v", findings)
	}
}

func TestFindingsNeedControlPlaneRepair(t *testing.T) {
	findings := []appDoctorFinding{
		{Name: "managed runtime definition", Class: doctorAutoFixable},
		{Name: "OpenBao application scope", Class: doctorAutoFixable},
	}
	if !findingsNeedControlPlaneRepair(findings) {
		t.Fatal("expected OpenBao application scope failure to require control-plane recovery")
	}
	if !findingsContain(findings, "managed runtime definition") {
		t.Fatal("expected managed runtime definition finding")
	}
}
