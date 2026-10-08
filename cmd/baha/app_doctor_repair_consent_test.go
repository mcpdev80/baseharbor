package main

import (
	"bytes"
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"strings"
	"testing"
)

func TestAppDoctorRejectsMutationInJSONMode(t *testing.T) {
	var out bytes.Buffer
	err := executeApplicationRepairLifecycle(context.Background(), application.DefaultStore(), []string{"--json", "--fix", "--yes"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("JSON repair not refused: %v", err)
	}
}
func TestAppDoctorHelpRequiresConsent(t *testing.T) {
	cmd := appDoctorRepairCommand(application.DefaultStore())
	if !strings.Contains(cmd.Usage, "--fix --yes") {
		t.Fatalf("consent missing in app doctor usage: %s", cmd.Usage)
	}
}

func TestAppDoctorYesRequiresFix(t *testing.T) {
	var out bytes.Buffer
	err := executeApplicationRepairLifecycle(context.Background(), application.DefaultStore(), []string{"--yes"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "requires --fix") {
		t.Fatalf("expected typed consent-only error: %v", err)
	}
}
