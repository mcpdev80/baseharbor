package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/stableid"
)

func snapshotBugState(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if filepath.Base(path) == "AGENTS.md" {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			value := fmt.Sprintf("%v", info.Mode())
			if !d.IsDir() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				value += string(data)
			}
			result[path] = value
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func bugApplicationRepository(t *testing.T) (application.Manifest, string) {
	t.Helper()
	repo := t.TempDir()
	m := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "demo", Environment: "dev", Services: application.Services{SQL: true}, Workload: application.WorkloadConfig{Components: []string{"app"}}}
	if err := os.WriteFile(filepath.Join(repo, application.RepositoryManifestName), []byte(m.YAML()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte("services:\n  app:\n    image: docker.io/library/alpine:3.23\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return m, repo
}

func TestAgentsOnlyLeavesEntireStateAndApplicationListUnchanged(t *testing.T) {
	for _, mode := range []string{"new", "manifest", "initialized", "registered"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			target := configureTestTarget(t)
			m, repo := bugApplicationRepository(t)
			if mode == "new" {
				if err := os.Remove(filepath.Join(repo, application.RepositoryManifestName)); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(repo)
			previous := appInitInput
			appInitInput = strings.NewReader("")
			t.Cleanup(func() { appInitInput = previous })
			if mode == "registered" {
				registerTestDeployment(t, target, m, repo, filepath.Join(repo, application.RepositoryManifestName))
			}
			var out bytes.Buffer
			if mode == "initialized" {
				if err := runWithIO(context.Background(), []string{"--no-input", "app", "init", "--yes"}, &out, &out); err != nil {
					t.Fatalf("normal init: %v: %s", err, out.String())
				}
			}
			roots := []string{repo, os.Getenv("XDG_CONFIG_HOME"), os.Getenv("XDG_DATA_HOME"), os.Getenv("BASEHARBOR_STATE_DIR")}
			var beforeList bytes.Buffer
			if err := runWithIO(context.Background(), []string{"app", "list", "--json"}, &beforeList, &beforeList); err != nil {
				t.Fatal(err)
			}
			before := snapshotBugState(t, roots...)
			original := "# Existing instructions\r\n\r\n\r\n"
			if err := os.WriteFile("AGENTS.md", []byte(original), 0640); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				out.Reset()
				if err := runWithIO(context.Background(), []string{"--no-input", "app", "init", "--agents", "--json"}, &out, &out); err != nil {
					t.Fatalf("agents: %v: %s", err, out.String())
				}
				var result struct {
					Path    string
					Changed bool
				}
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Changed != (i == 0) || result.Path != filepath.Join(repo, "AGENTS.md") {
					t.Fatalf("wrong documentation result %+v", result)
				}
				if after := snapshotBugState(t, roots...); !reflect.DeepEqual(before, after) {
					t.Fatalf("documentation changed persistent state: before=%v after=%v", before, after)
				}
				var afterList bytes.Buffer
				if err := runWithIO(context.Background(), []string{"app", "list", "--json"}, &afterList, &afterList); err != nil {
					t.Fatal(err)
				}
				if beforeList.String() != afterList.String() {
					t.Fatalf("application list changed: %s / %s", beforeList.String(), afterList.String())
				}
			}
			data, err := os.ReadFile("AGENTS.md")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(data), original) || strings.Count(string(data), baseHarborAgentsStart) != 1 || strings.Count(string(data), baseHarborAgentsEnd) != 1 {
				t.Fatalf("outside content or markers changed: %s", data)
			}
		})
	}
}

func TestAgentsOnlyRejectsLifecycleArgumentsBeforeAnyWrites(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	for _, extra := range [][]string{{"--yes"}, {"--input", "hostname=test"}, {"--quick"}, {"demo"}} {
		var out bytes.Buffer
		err := runWithIO(context.Background(), append([]string{"app", "init", "--agents"}, extra...), &out, &out)
		if cli.ExitCode(err) != 2 {
			t.Fatalf("expected input rejection: %v", err)
		}
		if _, err := os.Stat("AGENTS.md"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("rejected operation wrote AGENTS.md")
		}
	}
}

func TestDestroyGatewayAbsenceAndIncompleteState(t *testing.T) {
	for _, mode := range []string{"absent", "other-owner", "owned-route", "corrupt", "missing-registry", "runtime-orphan", "inventory-error"} {
		t.Run(mode, func(t *testing.T) {
			target := configureTestTarget(t)
			files, err := devgateway.FilesFor(target.Name)
			if err != nil {
				t.Fatal(err)
			}
			runtime := &inventoryTestRuntime{resources: map[string][]bhruntime.ProjectResource{}}
			switch mode {
			case "other-owner", "owned-route", "corrupt":
				if err := os.MkdirAll(files.Dir, 0700); err != nil {
					t.Fatal(err)
				}
				data := `{"version":1,"routes":[{"owner":"app/other/dev","key":"other","host":"other.test","upstream":"http://other:80","network":"foreign"}]}`
				if mode == "owned-route" {
					data = strings.ReplaceAll(data, "app/other/dev", "app/demo/dev")
				}
				if mode == "corrupt" {
					data = "invalid"
				}
				if err := os.WriteFile(files.State, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-registry":
				if err := os.MkdirAll(files.Dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(files.Compose, []byte("retained"), 0600); err != nil {
					t.Fatal(err)
				}
			case "runtime-orphan":
				runtime.resources[files.Project] = []bhruntime.ProjectResource{{Kind: "container", Name: "orphan"}}
			case "inventory-error":
				runtime.fail = true
			}
			before := snapshotBugState(t, files.Dir)
			present, err := devgateway.OwnerRoutesPresent(context.Background(), runtime, target.Name, "app/demo/dev")
			wantsError := mode == "corrupt" || mode == "missing-registry" || mode == "runtime-orphan" || mode == "inventory-error"
			if (err != nil) != wantsError || present != (mode == "owned-route") {
				t.Fatalf("present=%v err=%v", present, err)
			}
			if !reflect.DeepEqual(before, snapshotBugState(t, files.Dir)) || len(runtime.deleted) > 0 {
				t.Fatal("read-only preflight mutated gateway")
			}
		})
	}
}

func TestDestroyWithoutCoreRemovesRecordAndIsIdempotent(t *testing.T) {
	target := configureTestTarget(t)
	m, repo := bugApplicationRepository(t)
	record := registerTestDeployment(t, target, m, repo, filepath.Join(repo, application.RepositoryManifestName))
	resolved, err := resolvedRepositoryApplication(target, mustTargetRoot(t, target), application.RepositoryEnvironmentSelection{Manifest: m, ManifestPath: filepath.Join(repo, application.RepositoryManifestName), RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	runtime := &inventoryTestRuntime{resources: map[string][]bhruntime.ProjectResource{}}
	execution := applicationDestroyExecution{resolved: resolved, manifest: m, compose: runtime, term: cli.NewTerminal(context.Background(), &out, &out), out: &out}
	for i := 0; i < 2; i++ {
		if err := execution.cleanupDevelopmentCanonicalRoutes(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := execution.cleanupProviderState(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := execution.removeApplicationState(); err != nil {
			t.Fatal(err)
		}
		if _, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment); err != nil || found {
			t.Fatalf("record retained %v %v %+v", found, err, record)
		}
	}
	if !strings.Contains(out.String(), "NOT DEPLOYED") {
		t.Fatal(out.String())
	}
}
func mustTargetRoot(t *testing.T, target deployment.ResolvedTarget) string {
	t.Helper()
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDoctorNotDeployedVersusIncomplete(t *testing.T) {
	for _, artifact := range []string{"", "compose.yaml", "runtime.env", "topology.json", "installation.json", "pki"} {
		t.Run("artifact="+artifact, func(t *testing.T) {
			target := configureTestTarget(t)
			root, err := targetRuntimeStateRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			if artifact != "" {
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, artifact), []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			checks := collectControlPlaneDoctorChecks(context.Background())
			if controlPlaneNotDeployed(checks) != (artifact == "") {
				t.Fatalf("wrong state %+v", checks)
			}
			for _, check := range checks {
				if strings.Contains(strings.ToLower(check.Name), "availability") || strings.Contains(check.Message, "HA") {
					t.Fatalf("absence reported as HA failure: %+v", checks)
				}
			}
			report, err := inspectControlPlaneDoctor(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if report.Ready {
				t.Fatalf("not-deployed/partial Core is ready: %+v", report)
			}
		})
	}
}

func TestContextualCLIErrorHintsAndContracts(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	err := createDuplicateTargetForBugTest(t)
	uuidErr := stableid.ValidateUUIDv4("application", "123")
	for _, tc := range []struct {
		name string
		err  error
		code int
		next string
	}{
		{"duplicate", err, 1, "baha target list"}, {"uuid", uuidErr, 1, "lowercase UUIDv4"}, {"unknown failure", errors.New("unclassified failure"), 1, ""},
		{"runtime", machine.NewError(machine.ErrorRuntimeUnavailable, "runtime unavailable", "Run baha doctor to inspect the runtime.", true), 1, "baha doctor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out.Reset()
			formatCLIError(&out, tc.err)
			if cli.ExitCode(tc.err) != tc.code {
				t.Fatal("exit code changed")
			}
			if tc.next != "" && !strings.Contains(out.String(), tc.next) {
				t.Fatal(out.String())
			}
			if tc.name != "runtime" && strings.Contains(out.String(), "baha doctor") {
				t.Fatal(out.String())
			}
			if tc.next == "" && (strings.Contains(out.String(), "Next:") || machine.Classify(classifyMachineCLIError(tc.err)).Next != "") {
				t.Fatal(out.String())
			}
			classified := classifyMachineCLIError(tc.err)
			data, err := json.Marshal(machine.ResultError(classified))
			if err != nil || !json.Valid(data) {
				t.Fatalf("invalid machine envelope: %s %v", data, err)
			}
			if tc.name == "duplicate" || tc.name == "uuid" {
				typed := machine.Classify(classified)
				if typed.Code != machine.ErrorInternal || !strings.Contains(typed.Next, tc.next) {
					t.Fatalf("machine contract changed: %+v", typed)
				}
			}
		})
	}
	// Exercise UUID validation through the real repository command as well.
	m, repo := bugApplicationRepository(t)
	if err := os.WriteFile(filepath.Join(repo, application.RepositoryManifestName), []byte(strings.ReplaceAll(m.YAML(), m.ApplicationID, "123")), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	out.Reset()
	commandErr := runWithIO(context.Background(), []string{"--no-input", "app", "init"}, &out, &out)
	formatCLIError(&out, commandErr)
	if cli.ExitCode(commandErr) != 1 || !strings.Contains(out.String(), "lowercase UUIDv4") || strings.Contains(out.String(), "baha doctor") {
		t.Fatalf("UUID CLI: %v: %s", commandErr, out.String())
	}
	t.Chdir(t.TempDir())

	for _, args := range [][]string{{"down", "--yes"}, {"target", "delete", "demo", "--yes"}, {"app", "down", "--yes"}} {
		out.Reset()
		err := runWithIO(context.Background(), args, &out, &out)
		formatCLIError(&out, err)
		if cli.ExitCode(err) != 2 || !strings.Contains(out.String(), "--yes") || strings.Contains(out.String(), "Did you mean") || strings.Contains(out.String(), "baha doctor") {
			t.Fatalf("%v: %v %s", args, err, out.String())
		}
	}
}
func createDuplicateTargetForBugTest(t *testing.T) error {
	t.Helper()
	_, err := createTargetDefinition(context.Background(), machineTargetCreateInput{Name: "docker-dev", RuntimeProvider: "docker", Access: "local-docker", Reference: "local"})
	if err == nil {
		t.Fatal("duplicate accepted")
	}
	return err
}
