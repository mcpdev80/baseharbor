package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func runFinalBugRuntimeRegression(t *testing.T, ctx context.Context, target deployment.ResolvedTarget, opts runtimeUpOptions) {
	t.Helper()
	initialRecovery, _, err := resolveTargetRecoveryFile(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	oldRecovery, err := os.ReadFile(initialRecovery)
	if err != nil {
		t.Fatal(err)
	}
	for _, managed := range []bool{false, true} {
		t.Run(fmt.Sprintf("managed-%t", managed), func(t *testing.T) {
			repo := t.TempDir()
			t.Chdir(repo)
			m := application.WithWorkloadComponents(application.New(fmt.Sprintf("final-bugs-%t", managed), "dev", managed, managed, false), "app")
			if err := os.WriteFile(application.RepositoryManifestName, []byte(m.YAML()), 0600); err != nil {
				t.Fatal(err)
			}
			compose := "services:\n  app:\n    image: docker.io/library/alpine:3.23\n    command: [sh, -ec, 'echo application-started; sleep 600']\n"
			var occupied net.Listener
			if managed {
				occupied, err = net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer occupied.Close()
				port := occupied.Addr().(*net.TCPAddr).Port
				compose += fmt.Sprintf("    ports: ['127.0.0.1:%d:8080']\n", port)
				compose += "    environment:\n      DATABASE_URL: ${DATABASE_URL}\n      DATABASE_CA_FILE: ${DATABASE_CA_FILE}\n      REDIS_URL: ${REDIS_URL}\n      REDIS_CA_FILE: ${REDIS_CA_FILE}\n    healthcheck:\n      test: [CMD, 'true']\n      interval: 1s\n      timeout: 2s\n      retries: 15\n"
			}
			if err := os.WriteFile("compose.yaml", []byte(compose), 0600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				var b bytes.Buffer
				if err := runWithIO(context.Background(), []string{"app", "destroy", "--yes"}, &b, &b); err != nil {
					t.Errorf("application cleanup: %v", err)
				}
			})
			var output runtimeAcceptanceOutput
			command := func(args ...string) {
				t.Helper()
				output.Reset()
				if err := runWithIO(ctx, args, &output, &output); err != nil {
					t.Fatalf("CLI %v failed: %v", args, err)
				}
			}
			command("--plain", "up", "--yes")
			if strings.Contains(output.String(), "application and requested infrastructure verified") != managed {
				t.Fatalf("up reported false readiness: %s", output.String())
			}
			if managed && !strings.Contains(output.String(), "[UPDATED] workload-port") {
				t.Fatal("actual port remapping hidden")
			}
			r, err := resolveApplication(ctx, application.DefaultStore(), nil, "status")
			if err != nil {
				t.Fatal(err)
			}
			verify := func() {
				t.Helper()
				status, err := collectApplicationStatus(ctx, r.Store, nil)
				if err != nil || status.Ready != managed {
					t.Fatalf("readiness mismatch: ready=%t err=%v", status.Ready, err)
				}
				rec, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
				if err != nil || !found || rec.Observed.Ready != managed || rec.Observed.State != observedApplicationState(status) {
					t.Fatalf("deployment/live observation mismatch: %+v found=%t err=%v", rec.Observed, found, err)
				}
				// Changing XDG data/config must not change isolated installation identity.
				t.Setenv("XDG_DATA_HOME", t.TempDir())
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				rec2, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
				if err != nil || !found || rec2.Identity != rec.Identity {
					t.Fatal("XDG split brain", err)
				}
			}
			verify()
			command("down")
			rec, found, err := deployment.FindDeployment(target.Name, m.ApplicationID, m.Environment)
			if err != nil || !found || rec.Observed.Ready || rec.Observed.State != "stopped" {
				t.Fatal("down observation stale", err)
			}
			command("--plain", "up", "--yes")
			verify()
			command("--plain", "up", "--yes")
			verify()
			command("app", "logs")
			if strings.Contains(output.String(), "variable is not set") || !strings.Contains(output.String(), "application-started") {
				t.Fatal("logs lost environment or application output")
			}
			if managed {
				files, err := application.ExistingRuntimeFiles(r.Store, r.Manifest)
				if err != nil {
					t.Fatal(err)
				}
				env, err := application.RuntimeEnvironment(files)
				if err != nil {
					t.Fatal(err)
				}
				for key, value := range env {
					if len(value) > 8 && (strings.Contains(key, "PASSWORD") || strings.Contains(key, "SECRET")) && strings.Contains(output.String(), value) {
						t.Fatal("logs exposed a managed credential")
					}
				}
			}
			command("app", "destroy", "--yes")
			t.Logf("native lifecycle accepted: managed=%t, observed ready=%t; resume/noop, XDG isolation, logs and owned cleanup", managed, managed)
		})
	}
	t.Chdir(t.TempDir())
	var output runtimeAcceptanceOutput
	if err := runtimeDestroy(ctx, []string{"--yes"}, &output); err != nil {
		t.Fatalf("Core destroy: %v", err)
	}
	opts.RecoveryFile = ""
	next, err := installCore(ctx, strings.NewReader(""), &output, opts)
	if err != nil || !next.Ready {
		t.Fatalf("Core rebootstrap: ready=%t err=%v", next.Ready, err)
	}
	newRecovery, _, err := resolveTargetRecoveryFile(ctx, "")
	if err != nil || newRecovery == initialRecovery {
		t.Fatal("rebootstrap reused recovery material", err)
	}
	preserved, err := os.ReadFile(initialRecovery)
	if err != nil || sha256.Sum256(preserved) != sha256.Sum256(oldRecovery) {
		t.Fatal("rebootstrap modified preserved recovery material")
	}
	state, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(state, newRecovery); err != nil || (!strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		t.Fatal("recovery lies inside destructible installation state")
	}
	t.Log("native destroy/rebootstrap accepted; previous recovery bytes preserved; fresh private recovery outside installation state")
}
