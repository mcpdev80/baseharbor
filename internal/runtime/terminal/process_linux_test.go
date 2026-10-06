package terminal

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPTYInputResizeExitAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A test-only process exercises the PTY primitive, not a host-shell API.
	session, err := Start(ctx, exec.CommandContext(ctx, "/bin/sh", "-c", "read value; stty size; printf 'received:%s\\n' \"$value\"; exit 7"), 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Resize(30, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(session)
	if !strings.Contains(string(data), "30 100") || !strings.Contains(string(data), "received:hello") {
		t.Fatalf("PTY input/resize lost: %q", data)
	}
	exit, err := session.Wait(ctx)
	if err != nil || exit != 7 {
		t.Fatalf("exit = %d, %v", exit, err)
	}

	idleCtx, stop := context.WithCancel(context.Background())
	idle, err := Start(idleCtx, exec.CommandContext(idleCtx, "/bin/cat"), 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	blocked := make(chan struct{})
	go func() { _, _ = idle.Read(make([]byte, 10)); close(blocked) }()
	stop()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("cancel did not close idle PTY read")
	}
	if _, err := idle.Write([]byte("after cancellation")); err == nil {
		t.Fatal("cancelled PTY remained writable")
	}
}

func TestPTYRejectsUnboundedSizeAndCancelledAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Start(ctx, exec.Command("/bin/cat"), 24, 80); err == nil {
		t.Fatal("cancelled admission started a process")
	}
	if _, err := Start(context.Background(), exec.Command("/bin/cat"), 0, 80); err == nil {
		t.Fatal("invalid size admitted")
	}
}
