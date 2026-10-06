package terminal

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPTYTransportsEarlyInputBytesWithoutLineDiscipline(t *testing.T) {
	payload := []byte("echo terminal-ok\r\x03\x7f\x1b[A")
	if os.Getenv("BASEHARBOR_TEST_PTY_BYTE_TRANSPORT") == "1" {
		data := make([]byte, len(payload))
		if _, err := io.ReadFull(os.Stdin, data); err != nil {
			os.Exit(2)
		}
		fmt.Printf("received:%s", hex.EncodeToString(data))
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPTYTransportsEarlyInputBytesWithoutLineDiscipline$")
	cmd.Env = append(os.Environ(), "BASEHARBOR_TEST_PTY_BYTE_TRANSPORT=1")
	session, err := Start(ctx, cmd, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if n, err := session.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("input write = %d, %v", n, err)
	}
	data, _ := io.ReadAll(session)
	if !strings.Contains(string(data), "received:"+hex.EncodeToString(payload)) {
		t.Fatalf("PTY changed or withheld early input bytes: %q", data)
	}
}

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
	defer idle.Close()
	blocked := make(chan struct{})
	go func() { _, _ = idle.Read(make([]byte, 10)); close(blocked) }()
	stop()
	if _, err := idle.Write([]byte("immediately after cancellation")); err == nil {
		t.Fatal("cancelled admission still accepted terminal input")
	}
	if err := idle.Resize(30, 100); err == nil {
		t.Fatal("cancelled admission still accepted terminal resize")
	}
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
