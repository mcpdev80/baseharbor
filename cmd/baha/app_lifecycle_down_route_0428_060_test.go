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
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// 0428-060 r2: route-level regression for the shared down/stop consent seam.
//
// The round-1 review (evidence/cli-redesign-20261114/0428-060-review-1.md,
// finding 1) rejected tests that only call a helper: they stayed green when
// the production gate was removed. These tests instead run the REGISTERED
// commands (appDownCommand().Run / appStopCommand().Run — the same
// constructors commands.go puts into rootCommand()) in both directions:
//
//	(a) without --yes: the refusal message, the deployment record and the
//	    application store are byte-identical, the retirement beneath the
//	    command is entered UNCONFIRMED and refuses at its own gate, and NO
//	    provider command ever runs;
//	(b) with --yes: the seam opens — the route advances past it into the
//	    retirement and the provider-consequence consent gate (which --yes
//	    never grants), exactly like the native-cell regressions.
//
// Both directions are pinned for 'app down' AND 'app stop' because the route
// is shared — that is the content of the card.
//
// Hermeticity: the fixture declares NO managed services, so every managed-
// runtime mutation beneath the seam is a guarded no-op (the destroy and
// retirement primitives all check HasApplicationScopedRuntimeServices /
// len(m.Exposures) first), and the route needs no engine. The docker CLI is
// additionally replaced by a FAILING PATH stub: an unconfirmed route must not
// invoke it even once, and a confirmed route that invoked it would fail
// loudly instead of mutating anything.

// downRouteFixture builds an applied repository application (workload only,
// no managed services) and returns the record baseline for byte-identity
// comparison.
func downRouteFixture(t *testing.T) (resolvedApplication, []byte) {
	t.Helper()
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m = application.WithHTTPExposure(m, "demo-app", "demo-app", 8080, "https")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	files := application.RuntimeFilesFor(resolved.Store, m)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := application.RuntimeComposeYAMLForProject(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Compose, []byte(canonical), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, []byte("preserved=protected\n"), 0600); err != nil {
		t.Fatal(err)
	}
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	return resolved, deploymentRecordBytes(t, record)
}

// downRouteNoEngine isolates the PATH with a counting docker stub that fails
// loudly: an unconfirmed route must invoke it ZERO times, and any deeper
// provider attempt fails the test instead of reaching a daemon.
func downRouteNoEngine(t *testing.T) (string, string) {
	t.Helper()
	bin := t.TempDir()
	countPath := filepath.Join(t.TempDir(), "docker-stub-tape")
	// The stub records EVERY docker invocation on a tape and answers the
	// read-classification calls (inspect / ls) with "absent" (exit 1), so the
	// engine-free route behaves like a host with nothing running. Any MUTATION
	// (rm, stop, kill, down, destroy, create, up, run, start) exits 97 loudly
	// so a route can never mutate anything silently.
	script := "#!/bin/sh\nargs=\"$*\"\necho \"$args\" >>" + countPath + "\ncase \"$args\" in\n  *\\ rm\\ *|*\\ stop\\ *|*kill*|*\\ down*|*destroy*|*create*|*\\ up\\ *|*\\ run\\ *|*\\ start\\ *|*restart*)\n    echo \"0428-060 route stub: unexpected docker mutation: $args\" >&2\n    exit 97\n    ;;\nesac\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return bin, countPath
}

// downRouteEngineReadsOnly isolates the PATH with a docker stub that serves
// the engine/compose version reads, renders an empty config and reports every
// project resource as ABSENT, while every mutation call fails loudly
// (exit 97). The confirmed route therefore runs its real
// preflight/retirement logic against an empty host and completes without a
// daemon — proving the retirement runs while making mutation impossible.
func downRouteEngineReadsOnly(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	const script = "#!/bin/sh\n" +
		"args=\"$*\"\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in rm|stop|kill|restart|start|up|down|create|run|build|pull|prune) mutation=1 ;; esac\n" +
		"done\n" +
		"if [ -n \"${mutation:-}\" ]; then\n" +
		"  echo \"0428-060 route stub: unexpected docker mutation: $args\" >&2\n" +
		"  exit 97\n" +
		"fi\n" +
		"if [ \"${1:-}\" = context ] && [ \"${2:-}\" = show ]; then\n" +
		"  echo default\n" +
		"  exit 0\n" +
		"fi\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in\n" +
		"    info)\n" +
		"      printf '{\"ID\":\"0428060stubdaemon\",\"SecurityOptions\":[\"name=rootless\"],\"ServerVersion\":\"27.0.0\"}\n'\n" +
		"      exit 0\n" +
		"      ;;\n" +
		"    version)\n" +
		"      printf 'Client: stub\\nServer: stub\\n'\n" +
		"      exit 0\n" +
		"      ;;\n" +
		"    compose) composeseen=1 ;;\n" +
		"    config)\n" +
		"      if [ -n \"${composeseen:-}\" ]; then\n" +
		"        printf 'name: stub\\nservices: {}\\n'\n" +
		"        exit 0\n" +
		"      fi\n" +
		"      ;;\n" +
		"    ls) exit 0 ;;\n" +
		"    ps) exit 0 ;;\n" +
		"    inspect) exit 1 ;;\n" +
		"  esac\n" +
		"done\n" +
		"if [ -n \"${composeseen:-}\" ]; then\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

// downRouteMutationCalls counts the docker invocations on the tape that are
// actual runtime mutations (not the read/inspect calls the route uses to
// classify state). The route must produce ZERO of these without consent.
func downRouteMutationCalls(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var mutations []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if downRouteIsReadInvocation(line) {
			continue
		}
		mutations = append(mutations, line)
	}
	return mutations
}

func downRouteIsReadInvocation(line string) bool {
	for _, token := range []string{" rm ", " stop ", "kill", " down", "destroy", "create", " up ", " run ", " start ", "restart"} {
		if strings.Contains(line, token) {
			return false
		}
	}
	return true
}

// deploymentRecordBytes canonicalizes a record for byte-identity comparison.
func deploymentRecordBytes(t *testing.T, record deployment.DeploymentRecord) []byte {
	t.Helper()
	comparable := record
	comparable.Observed.VerifiedAt = time.Time{}
	data, err := json.Marshal(comparable)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func currentDeploymentRecordBytes(t *testing.T, resolved resolvedApplication) []byte {
	t.Helper()
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatalf("read deployment record: %v", err)
	}
	return deploymentRecordBytes(t, record)
}

// runRegisteredRoute runs the command instance the CLI registers for the
// verb — appDownCommand / appStopCommand constructors, the exact route
// production dispatches — and reports output and error.
func runRegisteredRoute(t *testing.T, verb string, ctx context.Context, args []string) (string, error) {
	t.Helper()
	command := appDownCommand(application.DefaultStore())
	if verb == "stop" {
		command = appStopCommand(application.DefaultStore())
	}
	if command.Name != verb {
		t.Fatalf("command builder produced %q for verb %q", command.Name, verb)
	}
	var out, errOut bytes.Buffer
	err := command.Run(ctx, args, &out, &errOut)
	return out.String(), err
}

func TestDownRouteWithoutConsentMakesNoChange(t *testing.T) {
	for _, verb := range []string{"down", "stop"} {
		t.Run(verb, func(t *testing.T) {
			resolved, recordBefore := downRouteFixture(t)
			_, stubCount := downRouteNoEngine(t)
			storeBefore := dirBytes(t, resolved.Store.Root)

			ctx := machineNoninteractiveContext(context.Background())
			output, err := runRegisteredRoute(t, verb, ctx, nil)
			if err != nil {
				t.Fatalf("unconfirmed 'app %s' must refuse cleanly, got: %v\n%s", verb, err, output)
			}
			if !strings.Contains(output, "No changes were made") {
				t.Fatalf("refusal message missing on 'app %s':\n%s", verb, output)
			}
			// The refusal must come BEFORE the retirement: the retirement's
			// own plan banner may never appear on an unconfirmed invocation.
			if strings.Contains(output, "Runtime retirement removes selected owned") {
				t.Fatalf("unconfirmed 'app %s' entered the retirement beneath the seam:\n%s", verb, output)
			}
			if calls := downRouteMutationCalls(t, stubCount); len(calls) != 0 {
				t.Fatalf("unconfirmed 'app %s' invoked engine mutations without consent: %v", verb, calls)
			}
			if data := currentDeploymentRecordBytes(t, resolved); !bytes.Equal(data, recordBefore) {
				t.Fatalf("unconfirmed 'app %s' changed the deployment record:\nbefore: %s\nafter:  %s", verb, recordBefore, data)
			}
			if storeAfter := dirBytes(t, resolved.Store.Root); !bytes.Equal(storeAfter, storeBefore) {
				t.Fatalf("unconfirmed 'app %s' changed the application store", verb)
			}

			// Same route, unknown option: the usage contract (exit 2) holds
			// even without --yes — a mistyped invocation is an error, never a
			// silent successful refusal.
			command := appDownCommand(application.DefaultStore())
			if verb == "stop" {
				command = appStopCommand(application.DefaultStore())
			}
			var bogusOut bytes.Buffer
			if berr := command.Run(ctx, []string{"--bogus"}, &bogusOut, &bogusOut); berr == nil || cli.ExitCode(berr) != 2 {
				t.Fatalf("unconfirmed 'app %s --bogus' must be a usage error (exit 2), got %v: %s", verb, berr, bogusOut.String())
			}
			if strings.Contains(bogusOut.String(), "No changes were made") {
				t.Fatalf("'app %s --bogus' masqueraded as a successful refusal: %s", verb, bogusOut.String())
			}
		})
	}
}

func TestDownRouteWithConsentRunsRetirement(t *testing.T) {
	for _, verb := range []string{"down", "stop"} {
		t.Run(verb, func(t *testing.T) {
			resolved, _ := downRouteFixture(t)
			downRouteEngineReadsOnly(t)
			// The retirement runs with its REAL consequence plan (the
			// package's own stop-consent pattern from the native-cell
			// helpers): --yes alone never grants the two acknowledgement
			// stages; the plan-scoped consent carries them. The CLI entry
			// supplies the invocation-local operator principal exactly as
			// production dispatch does.
			stopCtx := context.WithValue(machineNoninteractiveContext(context.Background()), applicationDownModeKey{}, true)
			plan, err := buildApplicationStopPlan(stopCtx, resolved)
			if err != nil {
				t.Fatalf("stop plan for consent probe: %v", err)
			}
			ctx := withProviderChangeConsent(machineNoninteractiveContext(context.Background()), &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true})
			command := appDownCommand(application.DefaultStore())
			if verb == "stop" {
				command = appStopCommand(application.DefaultStore())
			}
			var out bytes.Buffer
			perr := command.Run(ctx, []string{"--yes"}, &out, &out)
			if perr != nil {
				t.Fatalf("confirmed 'app %s' must run the retirement to completion: %v\n%s", verb, perr, out.String())
			}
			// The seam opened and the retirement beneath it ran: the record
			// ends in the verified retirement state.
			after := currentDeploymentRecordBytes(t, resolved)
			if !bytes.Contains(after, []byte("runtime-retired")) {
				t.Fatalf("confirmed 'app %s' never reached the verified retirement state: %s\n%s", verb, after, out.String())
			}
			// Persistent-data guard: the retirement is stop-not-delete, the
			// store's runtime.env must still exist.
			env := filepath.Join(resolved.Store.Root, resolved.Manifest.Name, "runtime", "runtime.env")
			if data, err := os.ReadFile(env); err != nil || !strings.Contains(string(data), "preserved=protected") {
				t.Fatalf("confirmed 'app %s' destroyed preserved data (%s): %v", verb, env, err)
			}
		})
	}
}

// TestDownExecutionRetirementRefusesUnconfirmed pins the retirement function
// beneath the command: entered the way the registered route enters it
// WITHOUT --yes (confirmed=false), it refuses at its own gate and mutates
// nothing — no engine call, no pending state, no record change. This is the
// live path, not a dead code path.
func TestDownExecutionRetirementRefusesUnconfirmed(t *testing.T) {
	resolved, recordBefore := downRouteFixture(t)
	_, stubCount := downRouteNoEngine(t)
	ctx := machineNoninteractiveContext(context.Background())
	ctx = context.WithValue(ctx, applicationStopModeKey{}, true)
	ctx = context.WithValue(ctx, applicationDownModeKey{}, true)
	execution, err := newApplicationDestroyExecution(ctx, resolved.Store, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("destroy execution for probe: %v", err)
	}
	if execution.confirmed {
		t.Fatal("the destroy execution beneath the unconfirmed route reported confirmation")
	}
	var out bytes.Buffer
	if err := execution.retireRuntimePreservingData(ctx, &out); err != nil {
		t.Fatalf("unconfirmed retirement must refuse cleanly, got: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "No changes were made") {
		t.Fatalf("retirement refusal message missing:\n%s", out.String())
	}
	if calls := downRouteMutationCalls(t, stubCount); len(calls) != 0 {
		t.Fatalf("unconfirmed retirement invoked engine mutations: %v", calls)
	}
	if data := currentDeploymentRecordBytes(t, resolved); !bytes.Equal(data, recordBefore) {
		t.Fatalf("unconfirmed retirement changed the deployment record:\nbefore: %s\nafter:  %s", recordBefore, data)
	}
}

// dirBytes serializes the (small) state tree for byte-identity comparison.
func dirBytes(t *testing.T, root string) []byte {
	t.Helper()
	var buf bytes.Buffer
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			buf.WriteString("d:" + rel + "\n")
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		buf.WriteString("f:" + rel + "=")
		buf.Write(data)
		buf.WriteByte('\n')
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("inspect state tree %s: %v", root, err)
	}
	return buf.Bytes()
}
