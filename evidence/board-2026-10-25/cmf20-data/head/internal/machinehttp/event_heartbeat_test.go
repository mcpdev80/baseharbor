package machinehttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestHTTPSExecutionHeartbeatFlushesIdleStreamWithoutInventingEvent(t *testing.T) {
	heartbeat := make(chan time.Time, 1)
	events := make(chan machine.MachineEvent)
	done := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		serveExecutionEvents(w, r, nil, events, heartbeat)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("idle event admission withheld headers: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("event admission differs")
	}
	heartbeat <- time.Now()
	comment := make([]byte, len(": keepalive\n\n"))
	if _, err := io.ReadFull(response.Body, comment); err != nil || string(comment) != ": keepalive\n\n" {
		t.Fatalf("idle heartbeat differs: %q %v", comment, err)
	}
	events <- machine.MachineEvent{Sequence: 7, Kind: machine.EventOperationSucceeded, State: machine.ExecutionSucceeded}
	data, err := io.ReadAll(response.Body)
	if err != nil || !strings.HasPrefix(string(data), "id: 7\nevent: operation.succeeded\n") || strings.Count(string(data), "data: ") != 1 {
		t.Fatalf("heartbeat changed authoritative sequence/result: %q %v", data, err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("terminal event did not close observation")
	}
}

type heartbeatRecorder struct {
	*httptest.ResponseRecorder
	written chan struct{}
}

func (w heartbeatRecorder) Write(data []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(data)
	close(w.written)
	return n, err
}

func TestExecutionHeartbeatStopsAtObservationDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	recorder := heartbeatRecorder{httptest.NewRecorder(), make(chan struct{})}
	events := make(chan machine.MachineEvent)
	heartbeat := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		serveExecutionEvents(recorder, httptest.NewRequest(http.MethodGet, "https://core.example", nil).WithContext(ctx), nil, events, heartbeat)
		close(done)
	}()
	heartbeat <- time.Now()
	<-recorder.written
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat kept cancelled observation alive")
	}
	if recorder.Body.String() != ": keepalive\n\n" {
		t.Fatalf("cancelled observation fabricated an event: %q", recorder.Body.String())
	}
}
