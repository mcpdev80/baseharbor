package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The hunt-916 defect I1 regression: the `app edit` partial-result path in
// app_edit_apply.go carried an `if true`-guarded DIAG_EXEC fmt.Printf. The guard
// never blocked, so every incomplete execution result leaked a full
// ProviderChangeResult dump into the process os.Stdout — bypassing the `out`
// writer and corrupting --json/machine output streams. There is no DIAG_EXEC
// environment check anywhere in the repository, so on the unfixed tree the leak
// was unconditional: the environment signal had zero influence on it. The fix
// removes the diagnostic execution path entirely.
//
// The behavior assertion runs in a re-executed child because the leak targets
// the process stdout at descriptor level: the child dup2s fd 1 onto an
// os.Pipe (so bytes written through os.Stdout, the testing framework's own
// banner writer, or a direct syscall all land in the capture), invokes the
// production entry appEditApplyConfirmed, then restores fd 1 and emits exactly
// one JSON report line on the real stdout: the capture plus canonical digests
// of the returned result and presentation.
//
// The parent compares the two DIAG_EXEC modes (unset vs set):
//   - the capture must hold nothing but the bounded framework banner, and in
//     particular no DIAG_EXEC text (this is the leak assertion), and
//   - the result and presentation digests must be byte-identical across the
//     modes, with the partial classification intact (this is the
//     "DIAG_EXEC cannot alter production behavior" contract).
func TestAppEditApplyPartialHasNoDiagnosticStdoutInAnyDiagExecMode(t *testing.T) {
	var unsetBaseline string
	for _, mode := range []string{"unset", "set"} {
		t.Run(mode, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=^TestAppEditDiagExecStdoutChild$", "-test.v=false")
			child.Env = append(os.Environ(),
				"BAHA_118_DIAG_EXEC_STDOUT_CHILD=1",
				"BAHA_118_DIAG_EXEC_MODE="+mode,
			)
			var stderr bytes.Buffer
			child.Stderr = &stderr
			out, err := child.Output()
			if err != nil {
				t.Fatalf("child %s failed: %v\nstderr:\n%s", mode, err, stderr.String())
			}
			lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
			var report diagExecStdoutChildReport
			found := false
			for _, line := range lines {
				trimmed := bytes.TrimSpace(line)
				if !bytes.HasPrefix(trimmed, []byte(`{"process_stdout"`)) {
					continue
				}
				if decodeErr := json.Unmarshal(trimmed, &report); decodeErr != nil {
					t.Fatalf("child %s report line not decodable: %v\nline:\n%s", mode, decodeErr, trimmed)
				}
				found = true
				break
			}
			if !found {
				t.Fatalf("child %s emitted no report line:\n%s", mode, out)
			}
			// The capture may hold the framework's own banner bytes (fd 1 was
			// real when testing.Main wrote them before the child body's
			// redirect, so normally nothing lands here); a DIAG_EXEC dump or
			// any unexpected bulk content is the leak and fails.
			if strings.Contains(report.ProcessStdout, "DIAG_EXEC") {
				t.Fatalf("child %s: the DIAG_EXEC diagnostic reached the process stdout (%d bytes): %q",
					mode, len(report.ProcessStdout), report.ProcessStdout)
			}
			if len(report.ProcessStdout) > diagExecCaptureBoundBytes {
				t.Fatalf("child %s: %d unexpected bytes on the process stdout (capture bound %d): %q",
					mode, len(report.ProcessStdout), diagExecCaptureBoundBytes, report.ProcessStdout)
			}
			// The fd-1 capture legitimately holds the testing framework's own
			// PASS banner and `ok` summary, which testing.Main writes through
			// its own fd reference after the redirect. They are contentless
			// for the contract: no app data, no DIAG_EXEC text, bounded size.
			if !diagExecOnlyFrameworkText(report.ProcessStdout) {
				t.Fatalf("child %s: capture carries content beyond framework status text: %q", mode, report.ProcessStdout)
			}
			if report.GotError != "partial" || report.Status != "partial" || report.Outcomes != 2 {
				t.Fatalf("child %s: partial path regressed: err=%q status=%q outcomes=%d",
					mode, report.GotError, report.Status, report.Outcomes)
			}
			if report.ResultDigest == "" || report.PresentationDigest == "" {
				t.Fatalf("child %s: result or presentation was empty: %+v", mode, report)
			}
			if mode == "unset" {
				unsetBaseline = report.ResultDigest + "\x00" + report.PresentationDigest
			} else {
				if unsetBaseline == "" {
					t.Fatal(`"set" child ran without the "unset" baseline`)
				}
				baseline := strings.Split(unsetBaseline, "\x00")
				if len(baseline) != 2 || baseline[0] != report.ResultDigest || baseline[1] != report.PresentationDigest {
					t.Fatalf("DIAG_EXEC=on altered production result content:\ngot          result=%q\n             presentation=%q\nwant (unset) result=%q\n             presentation=%q",
						report.ResultDigest, report.PresentationDigest, baseline[0], baseline[1])
				}
			}
		})
	}
}

// TestAppEditDiagExecStdoutChild is the re-exec child body: it is a no-op in
// normal runs (selected only by BAHA_118_DIAG_EXEC_STDOUT_CHILD=1). The parent
// started the child with its os.Stdout (fd 1) pointing at a pipe, so the child
// inherits a real pipe as stdout: writing the report closes that pipe and the
// parent's reader goroutine sees EOF exactly when the body is done.
func TestAppEditDiagExecStdoutChild(t *testing.T) {
	if os.Getenv("BAHA_118_DIAG_EXEC_STDOUT_CHILD") != "1" {
		t.Skip("re-exec child body: only runs when explicitly re-executed")
	}
	mode := os.Getenv("BAHA_118_DIAG_EXEC_MODE")
	if mode != "unset" && mode != "set" {
		t.Fatalf("unknown diag-exec mode %q", mode)
	}
	if mode == "set" {
		if err := os.Setenv("DIAG_EXEC", "on"); err != nil {
			t.Fatal(err)
		}
		defer os.Unsetenv("DIAG_EXEC")
	} else if _, present := os.LookupEnv("DIAG_EXEC"); present {
		// The unset mode must be genuinely unset, so a DIAG_EXEC value
		// inherited from the parent environment cannot change what is measured.
		if err := os.Unsetenv("DIAG_EXEC"); err != nil {
			t.Fatal(err)
		}
		defer os.Setenv("DIAG_EXEC", "")
	}

	captureReader, captureWriter, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	// Swap the fd behind os.Stdout (fd 1) onto the capture pipe. The Go
	// runtime's own status writer keeps a reference to the ORIGINAL stdout
	// file (testlog's *os.File), so the framework's final PASS banner
	// bypasses the pipe and reaches the parent report channel; the report
	// line itself is written to the saved duplicate below. Every byte written
	// through os.Stdout during the window — including a fmt.Printf on the
	// inherited file — lands in the capture.
	savedFd, _, fcntlErrno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(os.Stdout.Fd()), uintptr(syscall.F_DUPFD_CLOEXEC), 0)
	if fcntlErrno != 0 {
		t.Fatal(fcntlErrno)
	}
	if err := syscall.Dup2(int(captureWriter.Fd()), int(os.Stdout.Fd())); err != nil {
		t.Fatal(err)
	}
	captureDone := make(chan string, 1)
	go func() {
		var collected bytes.Buffer
		_, _ = collected.ReadFrom(captureReader)
		captureDone <- collected.String()
	}()

	fixture, fixtureErr := runAppEditDiagExecStdoutChild(t)

	// Restore fd 1 to the saved original stdout, then close the pipe write
	// end: the reader goroutine finishes with everything the production code
	// path wrote to stdout during the window.
	if err := syscall.Dup2(int(savedFd), int(os.Stdout.Fd())); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Close(int(savedFd)); err != nil {
		t.Error(err)
	}
	if closeErr := captureWriter.Close(); closeErr != nil {
		t.Error(closeErr)
	}
	var captured string
	select {
	case captured = <-captureDone:
	case <-time.After(5 * time.Second):
		t.Error("stdout capture drain timed out")
	}
	if closeErr := captureReader.Close(); closeErr != nil {
		t.Error(closeErr)
	}

	report := diagExecStdoutChildReport{
		ProcessStdout:      captured,
		GotError:           fixture.GotError,
		Status:             fixture.Status,
		Outcomes:           fixture.Outcomes,
		ResultDigest:       fixture.ResultDigest,
		PresentationDigest: fixture.PresentationDigest,
	}
	if fixtureErr != nil {
		report.FixtureError = fixtureErr.Error()
	}
	encoded, marshalErr := json.Marshal(report)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	// The report goes to the real stdout. The parent selects this child with
	// its -test.run filter, so testing.Main prints no PASS banner into the
	// stream; the child's entire fd-1 output is the capture plus this line.
	_, _ = os.Stdout.Write(append(encoded, '\n'))
}

type diagExecStdoutChildReport struct {
	ProcessStdout      string `json:"process_stdout"`
	GotError           string `json:"got_error"`
	FixtureError       string `json:"fixture_error,omitempty"`
	Status             string `json:"status"`
	Outcomes           int    `json:"outcomes"`
	ResultDigest       string `json:"result_digest"`
	PresentationDigest string `json:"presentation_digest"`
}

// The capture bound: legitimate bytes on the child's process stdout can only
// come from the framework banner machinery (PASS line plus `ok` summary); the
// app edit fixture itself writes only to its io.Discard `out` writer.
const diagExecCaptureBoundBytes = 512

// diagExecOnlyFrameworkText accepts an empty capture or one that holds nothing
// but go test status output: PASS/FAIL/SKIP banners, `=== RUN` markers and the
// `ok`/`FAIL` package summary. Any app payload (result dumps, prompts, JSON)
// fails the predicate.
func diagExecOnlyFrameworkText(captured string) bool {
	for _, line := range strings.Split(captured, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "PASS"),
			strings.HasPrefix(trimmed, "FAIL"),
			strings.HasPrefix(trimmed, "SKIP"),
			strings.HasPrefix(trimmed, "=== RUN"),
			strings.HasPrefix(trimmed, "ok "),
			strings.HasPrefix(trimmed, "--- PASS"),
			strings.HasPrefix(trimmed, "--- FAIL"):
			continue
		default:
			return false
		}
	}
	return true
}

// runAppEditDiagExecStdoutChild drives the exact production seam that carried
// the leak: appEditApplyConfirmed with an incomplete (partial) execution
// result. The decision comes from a real controller pipeline
// (newAppWizardController + Step next/yes); the execution authority returns an
// applied receipt for the added cache while the planned SQL stop stays pending,
// so result.Status == "partial" — the condition the leak printed under.
func runAppEditDiagExecStdoutChild(t *testing.T) (diagExecStdoutChildReport, error) {
	t.Helper()
	var zero diagExecStdoutChildReport
	before := []capability.Requirement{{Kind: capability.SQL, Name: "primary"}}
	loaded := appEditTestLoaded(t, "dev", before, nil)
	plan := application.ProviderImpactPlan{Version: application.ProviderImpactVersion, Target: "dev", Revision: "r1", PlanID: "diag-exec-partial", ConfirmationsRequired: 1,
		Impacts: []application.ProviderImpact{
			{InstanceID: "provider-cache-cache", Operation: "add"},
			{InstanceID: "provider-sql-primary", Operation: "stop"},
		}}

	input := appWizardTestInput(t)
	input.Manifest = mustAppEditManifest(t, loaded)
	input.Snapshot.Target = "dev"
	input.Selection = appCapabilitySelectionInput{Current: []capability.Requirement{
		{Kind: capability.SQL, Name: "primary"}, {Kind: capability.KeyValue, Name: "cache"},
	}}
	input.Refresh = func(context.Context, appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
		facts := &GuidedReadContext{ApplicationID: loaded.ApplicationID, Name: "edit-safe", Environment: "dev", Target: "dev", Complete: true, SnapshotRevision: "snapshot-1"}
		facts.Footprint.Target = "dev"
		facts.Footprint.Complete = true
		return facts, &plan, nil
	}
	controller, err := newAppWizardController(context.Background(), input)
	if err != nil {
		return zero, err
	}
	controller, err = controller.Step(context.Background(), appWizardNext)
	if err != nil {
		return zero, err
	}
	controller, err = controller.Step(context.Background(), appWizardYes)
	if err != nil {
		return zero, err
	}

	executed := 0
	authorities := appEditTestAuthorities("dev", loaded.ApplicationID, before, nil, []application.ProviderImpactPlan{plan}, &executed)
	authorities.Execute = func(context.Context, appEditDesired, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		executed++
		// Applied receipt for the added cache; the planned SQL stop never
		// reported -> its outcome stays pending and the result is partial.
		return []application.ProviderExecutionOutcome{{
			InstanceID: "provider-cache-cache", Operation: "add", Status: "applied", Verified: true, Evidence: "child execution receipt",
		}}, nil
	}
	result, callErr := appEditApplyConfirmed(context.Background(), controller, loaded, application.ProviderChangeConsent{PlanID: plan.PlanID, First: true}, authorities)
	if executed != 1 || !result.Changed {
		return zero, errorStringf("execution seam broken: err=%v changed=%t calls=%d", callErr, result.Changed, executed)
	}
	gotError := ""
	if callErr != nil {
		var partial *appMachineError
		if !errors.As(callErr, &partial) || partial.Code != "partial" {
			return zero, errorStringf("partial path error = %v, want appMachineError partial", callErr)
		}
		gotError = partial.Code
	}
	encoded, err := json.Marshal(result.Result)
	if err != nil {
		return zero, err
	}
	return diagExecStdoutChildReport{
		GotError:           gotError,
		Status:             result.Result.Status,
		Outcomes:           len(result.Result.Outcomes),
		ResultDigest:       digestJSON(encoded),
		PresentationDigest: strings.Clone(result.Presentation),
	}, nil
}

func errorStringf(format string, args ...any) error {
	return errors.New(fmt.Sprintf(format, args...))
}

// digestJSON re-marshals the decoded result so key order is canonical for a
// content comparison.
func digestJSON(raw []byte) string {
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return "undecodable"
	}
	normalized, err := json.Marshal(generic)
	if err != nil {
		return "undecodable"
	}
	return strings.Clone(string(normalized))
}
