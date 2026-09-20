package connectivityrelay

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRunForwardsTCPAndReportsTargetReadiness(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	targetCtx, cancelTarget := context.WithCancel(context.Background())
	defer cancelTarget()
	go func() {
		for {
			conn, err := target.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
			select {
			case <-targetCtx.Done():
				return
			default:
			}
		}
	}()

	listenAddr := reserveTCPAddress(t)
	healthAddr := reserveTCPAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			ListenAddr: listenAddr,
			TargetAddr: target.Addr().String(),
			HealthAddr: healthAddr,
		})
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://" + healthAddr + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("relay readiness did not become healthy: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", listenAddr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	payload := []byte("baseharbor-connectivity")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("relay payload=%q want=%q", got, payload)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("relay shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not stop after context cancellation")
	}
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}
