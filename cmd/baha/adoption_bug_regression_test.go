package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func httpsAdoptionFixture(t *testing.T, ambiguous bool) string {
	t.Helper()
	root := t.TempDir()
	ports := "      - '8080:8080'\n"
	if ambiguous {
		ports += "      - '8443:8443'\n"
	}
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  demo-app:\n    build: .\n    labels:\n      io.baseharbor.workload.protocol: https\n    ports:\n"+ports)
	return root
}

func TestQuickAdoptionCarriesExposureAndRepeatedInitPreservesContract(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[jsonMode], func(t *testing.T) {
			configureTestTarget(t)
			root := httpsAdoptionFixture(t, false)
			t.Chdir(root)
			args := []string{"app", "init", "--quick", "--no-input"}
			if jsonMode {
				args = append(args, "--json")
			}
			var out, errOut bytes.Buffer
			if err := runWithIO(context.Background(), args, &out, &errOut); err != nil {
				t.Fatalf("initial adoption: %v %s", err, errOut.String())
			}
			original, err := os.ReadFile(application.RepositoryManifestName)
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := application.LoadManifestFile(application.RepositoryManifestName)
			if err != nil {
				t.Fatal(err)
			}
			if len(manifest.Exposures) != 1 || manifest.Exposures[0].Service != "demo-app" || manifest.Exposures[0].Protocol != "https" || manifest.Exposures[0].Port != 8080 {
				t.Fatalf("missing exposure: %#v", manifest.Exposures)
			}
			resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "preflight")
			if err != nil {
				t.Fatal(err)
			}
			if err := preflightRepositoryWorkload(resolved); err != nil {
				t.Fatalf("generated contract rejected: %v", err)
			}
			out.Reset()
			errOut.Reset()
			if err := runWithIO(context.Background(), args, &out, &errOut); err != nil {
				t.Fatalf("repeat: %v %s", err, errOut.String())
			}
			repeated, err := os.ReadFile(application.RepositoryManifestName)
			if err != nil || !bytes.Equal(original, repeated) {
				t.Fatal("repeated quick init changed contract")
			}
			if jsonMode {
				var result map[string]any
				if err := json.Unmarshal(out.Bytes(), &result); err != nil || result["created"] != false {
					t.Fatalf("repeat JSON: %s %v", out.String(), err)
				}
			} else if !strings.Contains(out.String(), "already exists") {
				t.Fatalf("repeat diagnostic: %s", out.String())
			}
			records, err := deployment.ListDeployments(resolved.Target.Name)
			if err != nil || len(records) != 0 {
				t.Fatal("quick adoption configured or started a deployment")
			}
		})
	}
}

func TestQuickAdoptionRejectsAmbiguousHTTPSPortsBeforeWriting(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(httpsAdoptionFixture(t, true))
	err := runWithIO(context.Background(), []string{"app", "init", "--quick", "--no-input"}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous port accepted: %v", err)
	}
	if _, err := os.Stat(application.RepositoryManifestName); !os.IsNotExist(err) {
		t.Fatal("failed adoption wrote manifest")
	}
}

func TestStandalonePreflightCLIAndMCPRejectMissingWorkloadExposure(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	manifest := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Workload: application.WorkloadConfig{Components: []string{"demo-app"}}}
	mustWriteWizardTestFile(t, application.RepositoryManifestName, manifest.YAML())
	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "preflight", "--json"}, &out, &errOut); err == nil {
		t.Fatal("CLI accepted missing exposure")
	}
	var cliResult applicationPreflightResult
	if err := json.Unmarshal(out.Bytes(), &cliResult); err != nil {
		t.Fatalf("CLI JSON: %s %v", out.String(), err)
	}
	assertFailedWorkloadCheck(t, cliResult)
	session := workspaceMutationClient(t)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app.preflight", Arguments: map[string]any{}})
	if err != nil || !result.IsError {
		t.Fatalf("MCP accepted missing exposure: %#v %v", result, err)
	}
	data, _ := json.Marshal(result.StructuredContent)
	var mcpResult applicationPreflightResult
	if err := json.Unmarshal(data, &mcpResult); err != nil {
		t.Fatal(err)
	}
	assertFailedWorkloadCheck(t, mcpResult)
	target, _ := effectiveTarget(context.Background())
	records, err := deployment.ListDeployments(target.Name)
	if err != nil || len(records) != 0 {
		t.Fatal("read-only preflight wrote deployment state")
	}
	// The developer path rejects this contract before trying to start a runtime.
	out.Reset()
	if err := runtimeUpCommandWithInputResolver(context.Background(), []string{"--yes"}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "before control-plane start") {
		t.Fatalf("up did not reject before mutation: %v %s", err, out.String())
	}
}

func assertFailedWorkloadCheck(t *testing.T, result applicationPreflightResult) {
	t.Helper()
	if result.Passed {
		t.Fatal("preflight passed")
	}
	for _, check := range result.Checks {
		if check.Name == "application workload" && !check.OK && strings.Contains(check.Detail, "exposure.http") {
			return
		}
	}
	t.Fatalf("missing workload finding: %#v", result)
}

func TestApplicationHelpAndCommandRegistryHaveUniqueChildren(t *testing.T) {
	var walk func(*cli.Command)
	walk = func(command *cli.Command) {
		seen := map[string]bool{}
		for _, child := range command.Children {
			for _, name := range append([]string{child.Name}, child.Aliases...) {
				if seen[name] {
					t.Fatalf("duplicate child %s in %s", name, command.Name)
				}
				seen[name] = true
			}
			walk(child)
		}
	}
	walk(rootCommand())
	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "--help"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "Create a new ecosystem-native application") != 1 {
		t.Fatalf("duplicate new help: %s", out.String())
	}
}
