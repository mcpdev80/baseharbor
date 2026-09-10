package main

import "testing"

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

func TestClassifyAppDoctorOutputAutoFixableRuntime(t *testing.T) {
	findings := classifyAppDoctorOutput("[FAIL] postgres running: no postgres instance is running\n[FAIL] repository workload: workload stopped\n")
	if len(findings) != 2 {
		t.Fatalf("len(findings) = %d, want 2", len(findings))
	}
	if !allAppDoctorFindingsAutoFixable(findings) {
		t.Fatalf("expected all runtime findings to be auto-fixable: %#v", findings)
	}
}

func TestClassifyAppDoctorOutputExternalSecretRequiresInput(t *testing.T) {
	output := "[FAIL] required application secrets: missing application secrets: SMTP_PASSWORD\n" +
		"REQUIRED SECRET\tSTATUS\tACTION\n" +
		"SMTP_PASSWORD\tmissing - user input required\tbaha app secret set SMTP_PASSWORD --stdin\n"
	findings := classifyAppDoctorOutput(output)
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Class != doctorNeedsInput {
		t.Fatalf("class = %q, want %q", findings[0].Class, doctorNeedsInput)
	}
}

func TestClassifyAppDoctorOutputGeneratedSecretAutoFixable(t *testing.T) {
	output := "[FAIL] required application secrets: missing application secrets: SECRET_KEY\n" +
		"REQUIRED SECRET\tSTATUS\tACTION\n" +
		"SECRET_KEY\tmissing - will be generated automatically\tbaha app apply\n"
	findings := classifyAppDoctorOutput(output)
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Class != doctorAutoFixable {
		t.Fatalf("class = %q, want %q", findings[0].Class, doctorAutoFixable)
	}
}

func TestClassifyAppDoctorOutputPermissionFailureManual(t *testing.T) {
	findings := classifyAppDoctorOutput("[FAIL] manifest permissions: baseharbor.yaml is writable by others\n")
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Class != doctorManualAction {
		t.Fatalf("class = %q, want %q", findings[0].Class, doctorManualAction)
	}
}
