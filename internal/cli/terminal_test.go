package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTerminalPlainResultUsesStableColumnsWithoutANSI(t *testing.T) {
	var out bytes.Buffer
	term := NewTerminal(context.Background(), &out, &out)
	term.Result("DELETED", "application", "demo permanently deleted")
	got := out.String()
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("plain output contains ANSI: %q", got)
	}
	if !strings.Contains(got, "DELETED") || !strings.Contains(got, "application") || !strings.Contains(got, "demo permanently deleted") {
		t.Fatalf("unexpected result row: %q", got)
	}
}

func TestTerminalNoColorEnvironmentDisablesColor(t *testing.T) {
	old := os.Getenv("NO_COLOR")
	t.Cleanup(func() { _ = os.Setenv("NO_COLOR", old) })
	_ = os.Setenv("NO_COLOR", "1")
	var out bytes.Buffer
	term := NewTerminal(context.Background(), &out, &out)
	term.Result("READY", "PostgreSQL", "authenticated")
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("NO_COLOR output contains ANSI: %q", out.String())
	}
}

func TestTerminalQuietSuppressesNonFailureRows(t *testing.T) {
	ctx := WithOutputOptions(context.Background(), OutputOptions{Quiet: true})
	var out bytes.Buffer
	term := NewTerminal(ctx, &out, &out)
	term.Result("READY", "PostgreSQL", "ready")
	term.Result("FAILED", "workload", "not ready")
	got := out.String()
	if strings.Contains(got, "PostgreSQL") {
		t.Fatalf("quiet output contains non-failure row: %q", got)
	}
	if !strings.Contains(got, "FAILED") {
		t.Fatalf("quiet output hid failure: %q", got)
	}
}

func TestTerminalActivityDoesNotRenderForFastOperation(t *testing.T) {
	var out bytes.Buffer
	term := NewTerminal(context.Background(), &out, &out)
	if err := term.Activity(context.Background(), "Fast operation", func(w io.Writer) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("fast operation should not flash progress: %q", out.String())
	}
}

func TestTerminalActivityPlainFallbackIsLineOriented(t *testing.T) {
	ctx := WithOutputOptions(context.Background(), OutputOptions{ReducedMotion: true})
	var out bytes.Buffer
	term := NewTerminal(ctx, &out, &out)
	err := term.Activity(context.Background(), "Waiting for readiness", func(w io.Writer) error {
		time.Sleep(400 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "\r") || strings.Contains(got, "\x1b[") {
		t.Fatalf("plain activity contains terminal control sequences: %q", got)
	}
	for _, wanted := range []string{"[START] Waiting for readiness", "[OK] Waiting for readiness - done"} {
		if !strings.Contains(got, wanted) {
			t.Fatalf("plain activity missing %q: %q", wanted, got)
		}
	}
}

func TestTerminalQuietPreservesFailureDiagnostics(t *testing.T) {
	ctx := WithOutputOptions(context.Background(), OutputOptions{Quiet: true})
	var out bytes.Buffer
	term := NewTerminal(ctx, &out, &out)
	err := term.Activity(context.Background(), "Failing operation", func(w io.Writer) error {
		_, _ = io.WriteString(w, "useful failure detail\n")
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected activity failure")
	}
	if !strings.Contains(out.String(), "useful failure detail") {
		t.Fatalf("quiet mode hid failure diagnostics: %q", out.String())
	}
}

func TestTerminalResultWrapsLongDetailsAtTerminalWidth(t *testing.T) {
	t.Setenv("COLUMNS", "60")
	var out bytes.Buffer
	term := NewTerminal(context.Background(), &out, &out)
	term.Result("FAILED", "workload", "this is a deliberately long diagnostic detail that should wrap below the detail column instead of drifting off screen")
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output, got %q", out.String())
	}
	if !strings.HasPrefix(lines[1], strings.Repeat(" ", 37)) {
		t.Fatalf("continuation is not aligned under detail column: %q", lines[1])
	}
}

func TestPlainModeDisablesInteractiveRendering(t *testing.T) {
	ctx := WithOutputOptions(context.Background(), OutputOptions{Plain: true})
	var out bytes.Buffer
	term := NewTerminal(ctx, &out, &out)
	if term.TTY() {
		t.Fatal("plain mode must not behave like an interactive TTY")
	}
	term.Result("READY", "application", "ready")
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("plain mode emitted ANSI: %q", out.String())
	}
}
