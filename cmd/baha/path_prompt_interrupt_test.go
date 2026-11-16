//go:build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestRawPathPromptCtrlCRestoresTerminal(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()

	fd := int(slave.Fd())
	original, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}

	type promptResult struct {
		path string
		err  error
	}
	done := make(chan promptResult, 1)
	var output bytes.Buffer
	go func() {
		path, promptErr := promptPathWithCompletionBase(bufio.NewReader(slave), &output, "Project location", slave, "")
		done <- promptResult{path: path, err: promptErr}
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		current, readErr := unix.IoctlGetTermios(fd, unix.TCGETS)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if current.Lflag&unix.ISIG == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("prompt never entered raw mode")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := master.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.path != "" || !errors.Is(result.err, context.Canceled) {
			t.Fatalf("Ctrl-C result = %q, %v", result.path, result.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Ctrl-C did not cancel raw path prompt")
	}
	if !strings.Contains(output.String(), "Interrupted. No changes were made.") {
		t.Fatalf("cancellation lacks next-step notice: %q", output.String())
	}
	restored, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Lflag != original.Lflag {
		t.Fatalf("terminal local flags not restored: before=%#x after=%#x", original.Lflag, restored.Lflag)
	}
}
