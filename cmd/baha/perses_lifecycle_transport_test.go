package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestPersesLifecycleTransportActionMatrix is the transport/action matrix the
// CODEX correction round demands for enable, disable and remove: the direct
// human CLI changes the repository manifest WITHOUT executing the shared apply
// lifecycle and reports the deferred action, while machine transports
// (MCP/A2A) execute the shared apply lifecycle. The rejected candidate had
// this gate inverted. Cells run the production command seam
// (runPersesLifecycleCommand) under the context the transport really stamps
// ("cli" per main.go, "stdio" per the MCP server); the shared apply lifecycle
// is observed and bounded at the production seam variable
// persesApplyLifecycleFunc (cmd/baha/perses_lifecycle.go), and the runtime
// boundary is a recording fake (a live engine call fails the cell — the
// mission forbids containers on this host).
// persesMatrixDecodeResult decodes the public result object from a --json run;
// the direct CLI legitimately prints the actionable deferred-apply notice line
// before the JSON document, so decoding starts at the first JSON object byte.
func persesMatrixDecodeResult(t *testing.T, out []byte) persesLifecycleResult {
	t.Helper()
	index := bytes.IndexByte(out, '{')
	if index < 0 {
		t.Fatalf("the --json surface must contain the public result object: %s", out)
	}
	var result persesLifecycleResult
	if err := json.Unmarshal(bytes.TrimSpace(out[index:]), &result); err != nil {
		t.Fatalf("the --json surface must be the public result: %v\n%s", err, out)
	}
	return result
}

// persesMatrixContext stamps the invocation transport the way the production
// CLI entry does: main.go builds every direct run with the operator-auth
// enforcement boundary and tags direct CLI runs "cli" (the MCP stdio server
// tags "stdio"; HTTP/remote machine paths leave the tag empty).
func persesMatrixContext(ctx context.Context, transport string) context.Context {
	return machineNoninteractiveContext(withLocalInvocation(operatorauth.WithEnforcement(ctx), transport))
}

func TestPersesLifecycleTransportActionMatrix(t *testing.T) {
	actions := []struct {
		name      string
		action    application.PersesLifecycleAction
		set       bool
		startFrom bool
	}{
		{name: "enable", action: application.PersesLifecycleEnable, set: true, startFrom: false},
		{name: "disable", action: application.PersesLifecycleDisable, set: false, startFrom: true},
		{name: "remove", action: application.PersesLifecycleRemove, set: false, startFrom: true},
	}
	transports := []struct {
		name      string
		transport string
		deferred  bool
	}{
		{name: "direct-cli", transport: "cli", deferred: true},
		{name: "mcp-stdio", transport: "stdio", deferred: false},
		{name: "machine-transport-empty", transport: "", deferred: false},
	}
	for _, action := range actions {
		for _, transport := range transports {
			t.Run(action.name+"-"+transport.name, func(t *testing.T) {
				configureTestTarget(t)
				fixture := persesMatrixRepository(t, action.startFrom)
				applyCalls := installPersesMatrixApplySeam(t)
				ctx := persesMatrixContext(t.Context(), transport.transport)
				var out bytes.Buffer
				err := fixture.persesCommand(t, action.name).Run(ctx, []string{"--json"}, &out, &bytes.Buffer{})
				if err != nil {
					t.Fatalf("baha app perses %s (%s): %v\n%s", action.name, transport.name, err, out.String())
				}
				result := persesMatrixDecodeResult(t, out.Bytes())
				if result.Action != string(action.action) || result.Enabled != action.set || !result.RequiresReconcile || !result.BackendPreserved {
					t.Fatalf("perses %s (%s) result surface: %+v", action.name, transport.name, result)
				}
				persesMatrixAssertManifest(t, fixture, action.set)
				// The task contract registers the typed operation each cell
				// authorizes under; the machine transports converge through it.
				operation, registered := machine.OperationByID(persesLifecycleOperationID(action.action))
				if !registered || operation.ID != persesLifecycleOperationID(action.action) {
					t.Fatalf("perses %s must authorize under the registered typed operation: %+v ok=%v", action.name, operation, registered)
				}
				calls := *applyCalls
				if transport.deferred {
					// STOP CRITERION: the direct CLI must defer; a changed
					// manifest with an executed apply means it did not.
					if calls != 0 {
						t.Fatalf("STOP CRITERION: the direct human CLI ran the shared apply lifecycle %d time(s) for perses %s; it must defer it", calls, action.name)
					}
					// STOP CRITERION: deferring requires the actionable hint.
					if result.DeferredAction == "" || !strings.Contains(result.DeferredAction, "baha app apply") {
						t.Fatalf("STOP CRITERION: the direct CLI deferred the apply but the result carries no deferred-action hint: %+v", result)
					}
					return
				}
				// STOP CRITERION: a machine transport must never report success
				// after changing only the manifest.
				if calls == 0 {
					t.Fatalf("STOP CRITERION: machine transport %s reported success after changing only the manifest; the shared apply lifecycle never ran", transport.name)
				}
				if result.DeferredAction != "" {
					t.Fatalf("machine transports converge and never defer: %+v", result)
				}
			})
		}
	}
	// No-op cell: unchanged selections do not invoke apply in any transport.
	for _, transport := range transports {
		t.Run("enable-noop-"+transport.name, func(t *testing.T) {
			configureTestTarget(t)
			fixture := persesMatrixRepository(t, true)
			applyCalls := installPersesMatrixApplySeam(t)
			ctx := persesMatrixContext(t.Context(), transport.transport)
			var out bytes.Buffer
			if err := fixture.persesCommand(t, "enable").Run(ctx, []string{"--json"}, &out, &bytes.Buffer{}); err != nil {
				t.Fatalf("baha app perses enable (%s) on the already-selected surface: %v", transport.name, err)
			}
			result := persesMatrixDecodeResult(t, out.Bytes())
			if *applyCalls != 0 {
				t.Fatalf("the unchanged selection must not invoke apply (%s): %d calls", transport.name, *applyCalls)
			}
			if result.RequiresReconcile || result.DeferredAction != "" || !result.Enabled {
				t.Fatalf("the unchanged selection must keep the no-op behavior (%s): %+v", transport.name, result)
			}
			persesMatrixAssertManifest(t, fixture, true)
		})
	}
}

// TestPersesLifecycleHumanNoticeForEveryDeferredAction proves the notice half
// of the matrix: the direct human CLI prints the actionable deferred-apply
// notice for EVERY changed action — enable, disable and remove (the rejected
// candidate printed it for enable only) — and the human face of the same seam
// prints "No change" instead of the notice when nothing changed.
func TestPersesLifecycleHumanNoticeForEveryDeferredAction(t *testing.T) {
	for _, tc := range []struct {
		command   string
		startFrom bool
	}{
		{command: "enable", startFrom: false},
		{command: "disable", startFrom: true},
		{command: "remove", startFrom: true},
	} {
		t.Run(tc.command, func(t *testing.T) {
			configureTestTarget(t)
			fixture := persesMatrixRepository(t, tc.startFrom)
			applyCalls := installPersesMatrixApplySeam(t)
			ctx := persesMatrixContext(t.Context(), "cli")
			var out, errOut bytes.Buffer
			if err := fixture.persesCommand(t, tc.command).Run(ctx, nil, &out, &errOut); err != nil {
				t.Fatalf("baha app perses %s: %v\n%s", tc.command, err, out.String())
			}
			if *applyCalls != 0 {
				t.Fatalf("the direct CLI must defer the shared apply lifecycle, but it ran %d time(s)", *applyCalls)
			}
			if !strings.Contains(out.String(), "baha app apply") {
				t.Fatalf("STOP CRITERION: the direct CLI deferred convergence for %s but printed no actionable deferred-action notice: %q", tc.command, out.String())
			}
			if strings.Contains(out.String(), "No change") {
				t.Fatalf("a changed manifest must not report a no-op: %q", out.String())
			}
			persesMatrixAssertManifest(t, fixture, tc.command == "enable")
		})
	}
	t.Run("noop-keeps-no-change-notice", func(t *testing.T) {
		configureTestTarget(t)
		fixture := persesMatrixRepository(t, true)
		applyCalls := installPersesMatrixApplySeam(t)
		ctx := persesMatrixContext(t.Context(), "cli")
		var out, errOut bytes.Buffer
		if err := fixture.persesCommand(t, "enable").Run(ctx, nil, &out, &errOut); err != nil {
			t.Fatalf("baha app perses enable on the already-selected surface: %v", err)
		}
		if *applyCalls != 0 {
			t.Fatalf("the unchanged selection must not invoke apply: %d calls", *applyCalls)
		}
		if !strings.Contains(out.String(), "No change") {
			t.Fatalf("the unchanged selection keeps the no-change notice: %q", out.String())
		}
	})
}

// TestPersesLifecycleMCPSharedSeamRunsApplyLifecycle drives the typed MCP
// operations through the production server registration (newMCPServer + an
// in-memory client, the shared CLI/MCP lifecycle-test pattern) and proves the
// machine transport executes the shared apply lifecycle for every action and
// reports it in the structured result, with CLI-JSON parity on the shared
// fields.
func TestPersesLifecycleMCPSharedSeamRunsApplyLifecycle(t *testing.T) {
	configureTestTarget(t)
	fixture := persesMatrixRepository(t, false)
	applyCalls := installPersesMatrixApplySeam(t)
	var enableMCPResult persesLifecycleResult
	server := newMCPServer(application.DefaultStore())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "perses-transport-matrix", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	for _, tc := range []struct {
		tool   string
		action string
		set    bool
		from   bool
	}{
		{tool: "baseharbor.app.perses.enable", action: "enable", set: true, from: false},
		{tool: "baseharbor.app.perses.disable", action: "disable", set: false, from: true},
		{tool: "baseharbor.app.perses.remove", action: "remove", set: false, from: true},
	} {
		t.Run(tc.action, func(t *testing.T) {
			persesMatrixSetSelected(t, fixture, tc.from)
			before := *applyCalls
			result, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      tc.tool,
				Arguments: map[string]any{"approval": true},
			})
			if callErr != nil || (result != nil && result.IsError) {
				t.Fatalf("MCP %s: %v %+v", tc.tool, callErr, result)
			}
			if *applyCalls == before {
				t.Fatalf("STOP CRITERION: MCP %s reported success after changing only the manifest; the shared apply lifecycle never ran", tc.tool)
			}
			encoded, marshalErr := json.Marshal(result.StructuredContent)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var mcpResult persesLifecycleResult
			if decodeErr := json.Unmarshal(encoded, &mcpResult); decodeErr != nil {
				t.Fatalf("MCP %s structured content is not the public result: %v\n%s", tc.tool, decodeErr, encoded)
			}
			if mcpResult.Action != tc.action || mcpResult.Enabled != tc.set || mcpResult.DeferredAction != "" {
				t.Fatalf("MCP %s structured result: %+v", tc.tool, mcpResult)
			}
			if tc.action == "enable" {
				enableMCPResult = mcpResult
			}
			persesMatrixAssertManifest(t, fixture, tc.set)
		})
	}
	// CLI JSON parity on the shared fields (the deferred-action hint is the
	// transport-specific human addition): the enable cell's MCP structured
	// result is the machine face for the comparison.
	var cliOut bytes.Buffer
	if err := fixture.persesCommand(t, "enable").Run(persesMatrixContext(t.Context(), "cli"), []string{"--json"}, &cliOut, &bytes.Buffer{}); err != nil {
		t.Fatalf("CLI perses enable for parity: %v\n%s", err, cliOut.String())
	}
	cliResult := persesMatrixDecodeResult(t, cliOut.Bytes())
	if cliResult.DeferredAction == "" {
		t.Fatal("the deferred CLI result must carry the deferred action before the parity comparison")
	}
	cliCopy := cliResult
	cliCopy.DeferredAction = ""
	machineCopy := enableMCPResult
	machineCopy.DeferredAction = ""
	if !reflect.DeepEqual(cliCopy, machineCopy) {
		t.Fatalf("CLI JSON and the MCP structured result disagree: cli=%+v mcp=%+v", cliCopy, machineCopy)
	}
}

// TestPersesLifecycleGatePredicate pins the transport gate itself with its
// file:line (cmd/baha/perses_lifecycle.go): the seam names the transport that
// defers, every machine transport converges.
func TestPersesLifecycleGatePredicate(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(productPackageDirForPersesTest(t), "perses_lifecycle.go"))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(source), "\n") {
		if strings.Contains(line, "return cliInvocationTransport(ctx) ==") {
			_ = i
			if strings.Contains(line, `"cli"`) && !strings.Contains(line, "!=") {
				return
			}
			t.Fatalf("the convergence gate must defer ONLY the direct human CLI; found: %q (perses_lifecycle.go:%d)", line, i+1)
		}
	}
	t.Fatal("the perses convergence gate predicate disappeared from cmd/baha/perses_lifecycle.go")
}

// persesMatrixFixture is one repository-owned application the matrix mutates.
type persesMatrixFixture struct {
	root     string
	manifest string
}

// persesMatrixRepository authors an initialized application repository whose
// observability management-ui surface optionally realizes Perses.
func persesMatrixRepository(t *testing.T, persesSelected bool) persesMatrixFixture {
	t.Helper()
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	root := t.TempDir()
	m := application.New("perses-matrix", "dev", false, false, false)
	m = application.WithWorkloadComponents(m, "api")
	m = application.WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m.Services.ObservabilityManagementUI = true
	m.Services.Perses = persesSelected
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: alpine:3.22\n    command: [sleep, infinity]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, application.RepositoryManifestName)
	if err := writeInitializedAuthoredManifest(t, manifestPath, m, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return persesMatrixFixture{root: root, manifest: manifestPath}
}

// persesCommand resolves one production perses subcommand through the real
// command tree (the same wiring the CLI dispatches).
func (f persesMatrixFixture) persesCommand(t *testing.T, name string) *cli.Command {
	t.Helper()
	for _, child := range persesLifecycleCommand(application.DefaultStore()).Children {
		if child.Name == name {
			return child
		}
	}
	t.Fatalf("perses %s command missing", name)
	return nil
}

func persesMatrixSetSelected(t *testing.T, f persesMatrixFixture, perses bool) {
	t.Helper()
	manifest, err := application.LoadManifestFile(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Services.Perses = perses
	if err := os.WriteFile(f.manifest, []byte(manifest.AuthoredYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func persesMatrixAssertManifest(t *testing.T, f persesMatrixFixture, want bool) {
	t.Helper()
	manifest, err := application.LoadManifestFile(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Services.Perses != want {
		t.Fatalf("the repository manifest must reflect services.perses=%v, found %v", want, manifest.Services.Perses)
	}
}

// installPersesMatrixApplySeam replaces the Perses apply-lifecycle seam for the
// duration of the cell, counting invocations and restoring the production
// binding on cleanup. The replacement still proves the machine transport
// reached the shared lifecycle; running the real lifecycle here would reach
// the container engine on this coordinator host, which the mission forbids
// (docker/podman tests run only on the named SSH hosts).
func installPersesMatrixApplySeam(t *testing.T) *int {
	t.Helper()
	calls := 0
	previous := persesApplyLifecycleFunc
	persesApplyLifecycleFunc = func(context.Context, application.Store, []string, io.Writer, io.Writer) error {
		calls++
		return nil
	}
	t.Cleanup(func() { persesApplyLifecycleFunc = previous })
	return &calls
}

func productPackageDirForPersesTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
