package main

import (
	"strings"
	"testing"
)

func TestCLICoreVersionMismatch(t *testing.T) {
	tests := []struct {
		name   string
		report controlPlaneDoctorReport
		want   bool
	}{
		{name: "equal release", report: controlPlaneDoctorReport{CLIVersion: "v0.4.27", CoreVersion: "0.4.27"}},
		{name: "different release", report: controlPlaneDoctorReport{CLIVersion: "0.4.28", CoreVersion: "0.4.27"}, want: true},
		{name: "different development commit", report: controlPlaneDoctorReport{CLIVersion: "dev", CoreVersion: "dev", CLICommit: "abc", CoreCommit: "def"}, want: true},
		{name: "unknown commit does not invent mismatch", report: controlPlaneDoctorReport{CLIVersion: "dev", CoreVersion: "dev", CLICommit: "none", CoreCommit: "def"}},
		{name: "missing remote version is not compared", report: controlPlaneDoctorReport{CLIVersion: "0.4.27"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cliCoreVersionMismatch(tt.report); got != tt.want {
				t.Fatalf("cliCoreVersionMismatch() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRemoteCoreVersionMismatchFailsDoctorAndOffersCoreUpdate(t *testing.T) {
	report := controlPlaneDoctorReport{
		Ready: true, CLIVersion: "0.4.28", CoreVersion: "0.4.27",
		Checks: []publicControlPlaneCheck{{Name: "runtime", Ready: true}},
	}
	applyRemoteCoreVersionCheck(&report)
	if report.Ready {
		t.Fatal("version mismatch must fail doctor readiness")
	}
	if report.UpdateCommand != "baha update --version 0.4.28 --yes (on the Core host)" {
		t.Fatalf("update command = %q", report.UpdateCommand)
	}
	if len(report.Checks) != 2 || report.Checks[1].Name != "BaseHarbor version" || report.Checks[1].Ready {
		t.Fatalf("version mismatch check missing or not failed: %#v", report.Checks)
	}
	if !strings.Contains(report.Checks[1].Message, "0.4.28") || !strings.Contains(report.Checks[1].Message, "0.4.27") || !strings.Contains(report.Checks[1].Message, report.UpdateCommand) {
		t.Fatalf("version failure message is not actionable: %q", report.Checks[1].Message)
	}
}

func TestRemoteCoreWithoutVersionFailsDoctor(t *testing.T) {
	report := controlPlaneDoctorReport{Ready: true, CLIVersion: "0.4.28"}
	applyRemoteCoreVersionCheck(&report)
	if report.Ready || len(report.Checks) != 1 || report.Checks[0].Ready {
		t.Fatalf("missing Core build version must fail the doctor check: %#v", report)
	}
	if !strings.Contains(report.Checks[0].Message, "compatibility cannot be verified") || !strings.Contains(report.UpdateCommand, "--version 0.4.28") {
		t.Fatalf("missing-version finding lacks an actionable update path: %#v", report)
	}
}

func TestDevelopmentVersionMismatchDoesNotOfferUnpublishedReleaseUpdate(t *testing.T) {
	report := controlPlaneDoctorReport{Ready: true, CLIVersion: "dev", CLICommit: "abc", CoreVersion: "dev", CoreCommit: "def"}
	applyRemoteCoreVersionCheck(&report)
	if report.Ready || report.UpdateCommand != "install the matching development build on the Core host (no published release command exists)" {
		t.Fatalf("development mismatch should fail clearly without inventing a release command: %#v", report)
	}
}
