package targetsession

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// Stream exclusively owns a session and retires it on every outcome. A pipe
// applies backpressure without buffering unbounded peer output. Interactive
// processes are never resumed or transparently moved to another connection.
type Stream struct {
	session       *Session
	open          StreamOpen
	ctx           context.Context
	cancel        context.CancelFunc
	reader        *io.PipeReader
	writer        *io.PipeWriter
	done          chan struct{}
	once          sync.Once
	inputMu       sync.Mutex
	inputSequence uint64
	exitCode      int
	err           error
}

func (s *Session) OpenStream(ctx context.Context, open StreamOpen) (*Stream, error) {
	if s == nil || ctx.Err() != nil || s.ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	data, err := json.Marshal(open)
	now := time.Now().UTC()
	if err != nil || contracts.ValidateTargetAccessRecord("stream_open", data) != nil || open.TargetID != s.node.TargetID ||
		!open.DeadlineAt.After(now) || open.DeadlineAt.After(now.Add(5*time.Minute)) {
		return nil, contracts.ErrTargetAccessWire
	}
	capability := "runtime.logs.read"
	if open.Kind == "terminal" {
		capability = "runtime.terminal"
	}
	available := false
	for _, c := range s.capabilities.Capabilities {
		if c.Name == capability && c.Available {
			available = true
		}
	}
	if !available {
		return nil, ErrUnavailable
	}
	select {
	case s.busy <- struct{}{}:
	default:
		return nil, ErrUnavailable
	}
	// The slot is intentionally never reused; the Connector ends its control
	// reader when handing over this connection to the stream protocol.
	check, stop := context.WithTimeout(ctx, 2*time.Second)
	err = targetenrollment.AdmitTLSNode(check, s.conn.ConnectionState(), s.node.Scope(), s.registry)
	stop()
	if err != nil {
		_ = s.Close()
		return nil, ErrUnavailable
	}
	lifetime, cancel := context.WithDeadline(ctx, open.DeadlineAt)
	reader, writer := io.Pipe()
	stream := &Stream{session: s, open: open, ctx: lifetime, cancel: cancel, reader: reader, writer: writer, done: make(chan struct{}), exitCode: -1}
	stopSession := context.AfterFunc(s.ctx, func() { _ = stream.Close() })
	stopCaller := context.AfterFunc(lifetime, func() { _ = stream.Close() })
	deadline, _ := lifetime.Deadline()
	err = s.conn.SetReadDeadline(deadline)
	if err == nil {
		writeDeadline := now.Add(5 * time.Second)
		if deadline.Before(writeDeadline) {
			writeDeadline = deadline
		}
		err = s.conn.SetWriteDeadline(writeDeadline)
	}
	if err == nil {
		err = writeRecord(s.conn, "stream_open", open)
	}
	var ready streamEvent
	if err == nil {
		err = readRecord(s.conn, "stream_event", &ready)
	}
	if err != nil || !stream.matches(ready, 1) || ready.Type != "ready" || lifetime.Err() != nil {
		stopSession()
		stopCaller()
		_ = stream.Close()
		return nil, ErrUnavailable
	}
	go func() {
		defer close(stream.done)
		stream.receive()
		stopSession()
		stopCaller()
		_ = stream.session.Close()
		stream.cancel()
	}()
	return stream, nil
}

func (s *Stream) matches(event streamEvent, sequence uint64) bool {
	return event.StreamID == s.open.StreamID && event.CorrelationID == s.open.CorrelationID && event.Sequence == sequence
}

func (s *Stream) receive() {
	for sequence := uint64(2); ; sequence++ {
		var event streamEvent
		if readRecord(s.session.conn, "stream_event", &event) != nil || !s.matches(event, sequence) || s.ctx.Err() != nil {
			s.err = ErrUnavailable
			_ = s.writer.CloseWithError(s.err)
			return
		}
		switch event.Type {
		case "data":
			if _, err := s.writer.Write(event.Data); err != nil {
				s.err = ErrUnavailable
				return
			}
		case "exit":
			if s.open.Kind != "terminal" || event.ExitCode == nil {
				s.err = ErrUnavailable
				break
			}
			s.exitCode = *event.ExitCode
			_ = s.writer.Close()
			return
		case "end":
			if s.open.Kind != "logs" {
				s.err = ErrUnavailable
				break
			}
			_ = s.writer.Close()
			return
		default:
			s.err = ErrUnavailable
		}
		if s.err != nil {
			_ = s.writer.CloseWithError(s.err)
			return
		}
	}
}

func (s *Stream) Read(data []byte) (int, error) { return s.reader.Read(data) }

func (s *Stream) input(kind string, data []byte, rows, cols int) error {
	if s.open.Kind != "terminal" || s.ctx.Err() != nil || s.session.ctx.Err() != nil {
		return ErrUnavailable
	}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.inputSequence++
	event := streamEvent{ContractVersion: contractVersion, ProtocolVersion: protocolVersion, StreamID: s.open.StreamID, CorrelationID: s.open.CorrelationID, Sequence: s.inputSequence, ObservedAt: time.Now().UTC(), Type: kind, Data: data, Rows: rows, Cols: cols}
	deadline := time.Now().Add(5 * time.Second)
	if bound, _ := s.ctx.Deadline(); bound.Before(deadline) {
		deadline = bound
	}
	err := s.session.conn.SetWriteDeadline(deadline)
	if err == nil {
		err = writeRecord(s.session.conn, "stream_event", event)
	}
	if err != nil {
		_ = s.Close()
		return ErrUnavailable
	}
	return nil
}

func (s *Stream) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		size := len(data)
		if size > contracts.TargetAccessMaxStreamDataBytes {
			size = contracts.TargetAccessMaxStreamDataBytes
		}
		if err := s.input("data", data[:size], 0, 0); err != nil {
			return written, err
		}
		written += size
		data = data[size:]
	}
	return written, nil
}

func (s *Stream) Resize(rows, cols int) error {
	if rows < 1 || rows > 512 || cols < 1 || cols > 512 {
		return contracts.ErrTargetAccessWire
	}
	return s.input("resize", nil, rows, cols)
}

func (s *Stream) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.done:
		return s.exitCode, s.err
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

func (s *Stream) Close() error {
	s.once.Do(func() {
		s.cancel()
		_ = s.reader.CloseWithError(ErrUnavailable)
		_ = s.writer.CloseWithError(ErrUnavailable)
		_ = s.session.Close()
	})
	return nil
}

func (p *Pool) OpenStream(ctx context.Context, scope targetenrollment.Scope, open StreamOpen) (*Stream, error) {
	if p == nil || scope.Validate() != nil || open.TargetID != scope.TargetID {
		return nil, ErrUnavailable
	}
	p.mu.Lock()
	var selected *Session
	for _, session := range p.sessions[scope] {
		if session.ctx.Err() == nil && len(session.busy) == 0 {
			selected = session
			break
		}
	}
	p.mu.Unlock()
	if selected == nil {
		return nil, ErrUnavailable
	}
	return selected.OpenStream(ctx, open)
}
