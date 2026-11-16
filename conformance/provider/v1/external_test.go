package provider_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	provider "github.com/mcpdev80/baseharbor/conformance/provider/v1"
)

func TestExternalModuleCanRunPublicProfileWithoutInternalImports(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "run", "-mod=mod", ".")
	command.Dir = filepath.Join("testdata", "external")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("external module failed: %v\n%s", err, output)
	}
	var report provider.Report
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("not a JSON report: %v\n%s", err, output)
	}
	if report.Status != provider.Pass || report.Profile != provider.ProfileID || report.Suite != "full" || len(report.Checks) < 20 {
		t.Fatalf("external report = %#v", report)
	}
}
