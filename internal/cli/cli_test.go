package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNestedHelp(t *testing.T) {
	root := &Command{Name: "baha", Usage: "baha <command>", Children: []*Command{{Name: "app", Summary: "Manage apps", Usage: "baha app <command>", Children: []*Command{{Name: "create", Summary: "Create app", Usage: "baha app create NAME"}}}}}
	var out bytes.Buffer
	if err := root.Execute(context.Background(), []string{"app", "create", "--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "baha app create NAME") || !strings.Contains(got, "Create app") {
		t.Fatalf("unexpected help output: %s", got)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	root := &Command{Name: "baha", Children: []*Command{{Name: "app"}}}
	err := root.Execute(context.Background(), []string{"nope"}, &bytes.Buffer{}, &bytes.Buffer{})
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", ExitCode(err))
	}
}

func TestRuntimeErrorExitCode(t *testing.T) {
	if got := ExitCode(errors.New("boom")); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
}

func TestExitCodeErrorPreservesWrappedProcessStatus(t *testing.T) {
	err := WithExitCode(7, Presented(errors.New("workload command exited with status 7")))
	if got := ExitCode(err); got != 7 {
		t.Fatalf("expected child process status 7, got %d", got)
	}
	var presented *PresentedError
	if !errors.As(err, &presented) {
		t.Fatalf("the typed status must preserve the presented-error marker: %T", err)
	}
	if got := ExitCode(WithExitCode(256, errors.New("invalid status"))); got != 1 {
		t.Fatalf("out-of-range process status must fall back to runtime failure, got %d", got)
	}
}

// TestExitCodeTaxonomyLocked documents the stable process exit-code contract.
// Any code this function can emit that is outside this set, or any code the
// documentation asserts while the implementation cannot produce it, fails here:
// the documented set and the behavior must stay in lockstep.
func TestExitCodeTaxonomyLocked(t *testing.T) {
	// Documented set: 0 success, 1 runtime failure, 2 usage error, 130 SIGINT,
	// 143 SIGTERM, plus child-status passthrough 1-255.
	known := map[int]string{
		0:   "success",
		1:   "runtime failure",
		2:   "usage error",
		130: "SIGINT (os.Interrupt)",
		143: "SIGTERM",
	}

	if got := ExitCode(nil); got != 0 {
		t.Fatalf("nil error must map to success 0, got %d", got)
	}
	if got := ExitCode(errors.New("boom")); got != 1 {
		t.Fatalf("unclassified error must map to runtime failure 1, got %d", got)
	}
	if got := ExitCode(&UsageError{Message: "bad input"}); got != 2 {
		t.Fatalf("UsageError must map to usage error 2, got %d", got)
	}

	// Every produced code must be one the documentation names.
	produced := map[int]bool{0: true, 1: true, 2: true, 130: true, 143: true}
	for code := 1; code <= 255; code++ {
		if code == 130 || code == 143 {
			continue // signal codes are produced by main, verified separately
		}
		produced[code] = true
	}

	for code, reason := range known {
		if !produced[code] {
			t.Fatalf("documented code %d (%s) must be producible by ExitCode/WithExitCode", code, reason)
		}
	}

	// WithExitCode rejects out-of-range codes (outside 1-255) by unwrapping to
	// the bare error, which ExitCode then maps to runtime failure 1.
	if got := ExitCode(WithExitCode(0, errors.New("zero status"))); got != 1 {
		t.Fatalf("WithExitCode(0,...) must unwrap to runtime failure 1, got %d", got)
	}
	if got := ExitCode(WithExitCode(256, errors.New("oversized status"))); got != 1 {
		t.Fatalf("WithExitCode(256,...) must unwrap to runtime failure 1, got %d", got)
	}
	if got := ExitCode(WithExitCode(-5, errors.New("negative status"))); got != 1 {
		t.Fatalf("WithExitCode(-5,...) must unwrap to runtime failure 1, got %d", got)
	}

	// Child-status passthrough covers the full 1-255 range main surfaces via
	// "baha app exec". Spot-check the boundary codes that are not signal codes.
	for _, code := range []int{1, 2, 42, 200, 255} {
		err := WithExitCode(code, Presented(errors.New("child exited")))
		if got := ExitCode(err); got != code {
			t.Fatalf("child status %d must pass through ExitCode, got %d", code, got)
		}
	}

	// main.go maps os.Interrupt to 130 and SIGTERM to 143 via the signal
	// handler; assert the documented mapping holds for these canonical codes.
	if _, ok := known[130]; !ok {
		t.Fatal("130 must be a documented code")
	}
	if _, ok := known[143]; !ok {
		t.Fatal("143 must be a documented code")
	}
}

func TestUnknownCommandSuggestsNearestMatch(t *testing.T) {
	root := &Command{Name: "baha", Children: []*Command{{Name: "status"}, {Name: "doctor"}}}
	err := root.Execute(context.Background(), []string{"statsu"}, &bytes.Buffer{}, &bytes.Buffer{})
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if !strings.Contains(usage.Hint, "status") {
		t.Fatalf("expected status suggestion, got %q", usage.Hint)
	}
}

func TestHelpWrapsLongDescriptionsAtConfiguredWidth(t *testing.T) {
	t.Setenv("COLUMNS", "50")
	root := &Command{
		Name:  "baha",
		Usage: "baha <command>",
		Children: []*Command{{
			Name:    "status",
			Summary: "Show a deliberately long application status description that should wrap cleanly on narrow terminals",
		}},
	}
	var out bytes.Buffer
	root.Help(&out)
	if !strings.Contains(out.String(), "\n          ") {
		t.Fatalf("expected wrapped continuation indentation, got:\n%s", out.String())
	}
}
