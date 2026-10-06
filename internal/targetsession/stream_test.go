package targetsession

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func streamNow(kind string) StreamOpen {
	open := StreamOpen{ContractVersion: contractVersion, ProtocolVersion: protocolVersion, StreamID: "stream-a", CorrelationID: "core-stream-a", TargetID: "lab", ResourceID: "owned-a", Kind: kind, DeadlineAt: time.Now().UTC().Add(time.Minute)}
	if kind == "terminal" {
		open.Terminal = &TerminalOptions{Rows: 24, Cols: 80, Argv: []string{"/bin/sh"}}
	} else {
		open.Logs = &LogOptions{Tail: 10, Follow: true}
	}
	return open
}

func eventFor(open StreamOpen, kind string, sequence uint64) streamEvent {
	return streamEvent{ContractVersion: contractVersion, ProtocolVersion: protocolVersion, StreamID: open.StreamID, CorrelationID: open.CorrelationID, Sequence: sequence, ObservedAt: time.Now().UTC(), Type: kind}
}

func TestRemoteFollowLogsEndsCleanlyWithoutTerminalInput(t *testing.T) {
	s, peer, _ := newSessionPair(t, false)
	s.capabilities.Capabilities = append(s.capabilities.Capabilities, Capability{Name: "runtime.logs.read", Available: true})
	go func() {
		var open StreamOpen
		if readRecord(peer, "stream_open", &open) != nil {
			return
		}
		if writeRecord(peer, "stream_event", eventFor(open, "ready", 1)) != nil {
			return
		}
		data := eventFor(open, "data", 2)
		data.Data = []byte("line\n")
		if writeRecord(peer, "stream_event", data) != nil {
			return
		}
		_ = writeRecord(peer, "stream_event", eventFor(open, "end", 3))
	}()
	stream, err := s.OpenStream(context.Background(), streamNow("logs"))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if _, err := stream.Write([]byte("unexpected input")); !errors.Is(err, ErrUnavailable) {
		t.Fatal("logs accepted terminal input", err)
	}
	data, err := io.ReadAll(stream)
	if err != nil || string(data) != "line\n" {
		t.Fatal(string(data), err)
	}
	if _, err := stream.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteTerminalPreservesInputResizeOutputExitAndRetiresSession(t *testing.T) {
	s, peer, _ := newSessionPair(t, false)
	s.capabilities.Capabilities = append(s.capabilities.Capabilities, Capability{Name: "runtime.terminal", Available: true})
	peerDone := make(chan error, 1)
	go func() {
		var open StreamOpen
		if err := readRecord(peer, "stream_open", &open); err != nil {
			peerDone <- err
			return
		}
		if err := writeRecord(peer, "stream_event", eventFor(open, "ready", 1)); err != nil {
			peerDone <- err
			return
		}
		for sequence := uint64(1); sequence <= 2; sequence++ {
			var input streamEvent
			if err := readRecord(peer, "stream_event", &input); err != nil {
				peerDone <- err
				return
			}
			if input.Sequence != sequence || input.CorrelationID != open.CorrelationID || input.StreamID != open.StreamID ||
				(sequence == 1 && (input.Type != "data" || string(input.Data) != "echo hello\n")) || (sequence == 2 && (input.Type != "resize" || input.Rows != 40 || input.Cols != 100)) {
				peerDone <- errors.New("invalid scoped terminal input")
				return
			}
		}
		output := eventFor(open, "data", 2)
		output.Data = []byte("hello\n")
		if err := writeRecord(peer, "stream_event", output); err != nil {
			peerDone <- err
			return
		}
		exit := eventFor(open, "exit", 3)
		code := 7
		exit.ExitCode = &code
		peerDone <- writeRecord(peer, "stream_event", exit)
	}()
	stream, err := s.OpenStream(context.Background(), streamNow("terminal"))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if n, err := stream.Write([]byte("echo hello\n")); err != nil || n != 11 {
		t.Fatal(n, err)
	}
	if err := stream.Resize(40, 100); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(stream)
	if err != nil || string(output) != "hello\n" {
		t.Fatal(string(output), err)
	}
	code, err := stream.Wait(context.Background())
	if err != nil || code != 7 || s.ctx.Err() == nil {
		t.Fatal(code, err)
	}
	if err := <-peerDone; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dispatch(context.Background(), requestNow("after-terminal")); !errors.Is(err, ErrUnavailable) {
		t.Fatal("stream session reused", err)
	}
}

func TestRemoteLogStreamRejectsForeignScopeSequenceAndEarlyDisconnect(t *testing.T) {
	for _, mode := range []string{"scope", "sequence", "disconnect", "terminal-exit"} {
		t.Run(mode, func(t *testing.T) {
			s, peer, _ := newSessionPair(t, false)
			s.capabilities.Capabilities = append(s.capabilities.Capabilities, Capability{Name: "runtime.logs.read", Available: true})
			go func() {
				var open StreamOpen
				if readRecord(peer, "stream_open", &open) != nil {
					return
				}
				if writeRecord(peer, "stream_event", eventFor(open, "ready", 1)) != nil {
					return
				}
				event := eventFor(open, "data", 2)
				event.Data = []byte("hello")
				switch mode {
				case "scope":
					event.CorrelationID = "foreign"
				case "sequence":
					event.Sequence = 3
				case "disconnect":
					_ = peer.NetConn().Close()
					return
				case "terminal-exit":
					event = eventFor(open, "exit", 2)
					code := 0
					event.ExitCode = &code
				}
				_ = writeRecord(peer, "stream_event", event)
			}()
			stream, err := s.OpenStream(context.Background(), streamNow("logs"))
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			data, err := io.ReadAll(stream)
			if len(data) != 0 || !errors.Is(err, ErrUnavailable) {
				t.Fatal("invalid stream accepted", string(data), err)
			}
			if _, err := stream.Wait(context.Background()); !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
		})
	}
}

func TestSlowRemoteLogReaderIsCancelledWithoutBufferedOutputLeak(t *testing.T) {
	s, peer, _ := newSessionPair(t, false)
	s.capabilities.Capabilities = append(s.capabilities.Capabilities, Capability{Name: "runtime.logs.read", Available: true})
	go func() {
		var open StreamOpen
		if readRecord(peer, "stream_open", &open) != nil {
			return
		}
		if writeRecord(peer, "stream_event", eventFor(open, "ready", 1)) != nil {
			return
		}
		event := eventFor(open, "data", 2)
		event.Data = []byte("backpressure")
		_ = writeRecord(peer, "stream_event", event)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := s.OpenStream(ctx, streamNow("logs"))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if _, err := stream.Wait(wait); !errors.Is(err, ErrUnavailable) {
		t.Fatal("backpressure held connection", err)
	}
	if s.ctx.Err() == nil {
		t.Fatal("cancelled stream remained live")
	}
}

func TestUnavailableRemoteStreamCannotDowngradeToControlOrAnotherTarget(t *testing.T) {
	s, _, r := newSessionPair(t, false)
	if _, err := s.OpenStream(context.Background(), streamNow("terminal")); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	s.capabilities.Capabilities = append(s.capabilities.Capabilities, Capability{Name: "runtime.terminal", Available: true})
	open := streamNow("terminal")
	open.TargetID = "foreign"
	if _, err := s.OpenStream(context.Background(), open); err == nil {
		t.Fatal("foreign stream written")
	}
	r.denied.Store(true)
	if _, err := s.OpenStream(context.Background(), streamNow("terminal")); !errors.Is(err, ErrUnavailable) || s.ctx.Err() == nil {
		t.Fatal("revoked stream admitted", err)
	}
}
