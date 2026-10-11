package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// Run after the fresh no-Core gate, in a separate process on the same isolated
// host. This explicitly installs management Core, then reuses that authority.
func TestApplicationCapabilityLifecycleExistingCore(t *testing.T) {
	if os.Getenv("BASEHARBOR_CORELESS_ACCEPTANCE") != "1" {
		t.Skip("requires isolated exact-SHA runtime acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 25*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := configureTestTarget(t)
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	t.Chdir(root)
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, runtime, target)
	var out bytes.Buffer
	first, err := installCore(ctx, strings.NewReader(""), &out, runtimeUpOptions{Yes: true, ControlPlaneOnly: true, MachineRole: coreinstallation.Development, RecoveryFile: filepath.Join(t.TempDir(), "recovery.json")})
	if err != nil || !first.Ready {
		logCoreBootstrapFailure(t, runtime, target.Name, target.RuntimeProvider)
		t.Fatalf("explicit management install: %v\n%s", err, out.String())
	}
	t.Logf("explicit Core installation_id=%s target=%s", first.ID, target.Name)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cleanupCancel()
		if err := runtimeDestroy(machineNoninteractiveContext(cleanupCtx), []string{"--yes"}, &bytes.Buffer{}); err != nil {
			t.Errorf("isolated Core cleanup: %v", err)
		}
	})
	parent := t
	var sqlFirst resolvedApplication
	for _, kind := range []string{"sql", "sql-second", "secrets", "identity", "https"} {
		if !t.Run(kind, func(t *testing.T) {
			appRoot := parent.TempDir()
			t.Chdir(appRoot)
			m := application.WithWorkloadComponents(application.New("capability-"+kind, "dev", false, false, false), "api")
			switch kind {
			case "sql", "sql-second":
				m.Services.SQL = true
			case "secrets":
				m.Services.Secrets = true
			case "identity":
				m = application.WithIdentity(m)
			case "https":
				m = application.WithHTTPExposure(m, "web", "api", 8080, "https")
			}
			if err := os.WriteFile("baseharbor.yaml", []byte(m.YAML()), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
				t.Fatal(err)
			}
			parent.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
				defer cleanupCancel()
				previous, _ := os.Getwd()
				if err := os.Chdir(appRoot); err != nil {
					parent.Error(err)
					return
				}
				defer os.Chdir(previous)
				if err := runWithIO(machineNoninteractiveContext(cleanupCtx), []string{"app", "destroy", "--yes"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
					parent.Errorf("isolated %s cleanup: %v", kind, err)
				}
			})
			if kind == "https" {
				out.Reset()
				if err := runWithIO(ctx, []string{"app", "init", "--input", "hostname=capability-https.baha.localhost", "--input", "tls_mode=local", "--yes"}, &out, &out); err != nil {
					t.Fatalf("HTTPS inputs: %v\n%s", err, out.String())
				}
			}
			for _, command := range [][]string{{"app", "apply", "--skip-memory-preflight"}, {"app", "status", "--json"}, {"app", "doctor", "--json"}} {
				out.Reset()
				if err := runWithIO(ctx, command, &out, &out); err != nil {
					t.Fatalf("%v: %v\n%s", command, err, out.String())
				}
			}
			resolved, err := resolveApplication(ctx, application.Store{}, nil, "status")
			if err != nil {
				t.Fatal(err)
			}
			status, _, err := collectResolvedApplicationStatus(ctx, resolved)
			if err != nil || !status.Ready {
				t.Fatalf("capability verification: %+v %v", status, err)
			}
			coreRoot, err := targetRuntimeStateRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			current, err := coreinstallation.Load(coreRoot)
			if err != nil || current.ID != first.ID {
				t.Fatalf("existing Core was replaced: %v", err)
			}
			if kind == "sql" {
				sqlFirst = resolved
			}
			if kind == "sql-second" {
				store := capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}
				registry, err := store.Load()
				if err != nil {
					t.Fatal(err)
				}
				firstID, secondID := "", ""
				for _, binding := range registry.Bindings {
					if binding.Resource.Kind == capability.SQL {
						if binding.ApplicationID == sqlFirst.Manifest.ApplicationID {
							firstID = binding.ProviderInstanceID
						}
						if binding.ApplicationID == m.ApplicationID {
							secondID = binding.ProviderInstanceID
						}
					}
				}
				if firstID == "" || firstID != secondID {
					t.Fatalf("two apps did not reuse native Target binding: %s %s", firstID, secondID)
				}
				reduced := sqlFirst.Manifest
				reduced.Services.SQL = false
				if err := os.WriteFile(sqlFirst.ManifestPath, []byte(reduced.YAML()), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(sqlFirst.RepositoryRoot); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				if err := runWithIO(ctx, []string{"app", "apply", "--skip-memory-preflight"}, &out, &out); err != nil {
					t.Fatalf("capability removal: %v\n%s", err, out.String())
				}
				if err := os.Chdir(appRoot); err != nil {
					t.Fatal(err)
				}
				status, _, err = collectResolvedApplicationStatus(ctx, resolved)
				if err != nil || !status.Ready {
					t.Fatalf("capability removal broke another app: %+v %v", status, err)
				}
				registry, err = store.Load()
				if err != nil {
					t.Fatal(err)
				}
				surviving := false
				for _, binding := range registry.Bindings {
					if binding.ApplicationID == m.ApplicationID && binding.ProviderInstanceID == secondID {
						surviving = true
					}
				}
				if !surviving {
					t.Fatal("capability removal erased other consumer binding")
				}
				t.Logf("two apps reused %s; removal retained other app live", secondID)
				// Reattach the retained scope through the existing lifecycle so
				// explicit app destroy can reclaim this isolated test's resources.
				// Capability removal itself deliberately does not perform GC.
				if err := os.WriteFile(sqlFirst.ManifestPath, []byte(sqlFirst.Manifest.YAML()), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(sqlFirst.RepositoryRoot); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				if err := runWithIO(ctx, []string{"app", "apply", "--skip-memory-preflight"}, &out, &out); err != nil {
					t.Fatalf("reattach retained test scope: %v\n%s", err, out.String())
				}
				if err := os.Chdir(appRoot); err != nil {
					t.Fatal(err)
				}
			}
		}) {
			return
		}
	}
}
