package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Delay a peer's stdin half-close until complete requests have flushed replies.
// Return the SDK IO connection unchanged: its private negotiated-protocol hooks,
// framing, dispatch and batch-admission rules must remain authoritative.
type drainingMCPTransport struct {
	reader  io.ReadCloser
	writer  io.WriteCloser
	timeout time.Duration
}

func (t *drainingMCPTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	drain := &mcpEOFDrain{ctx: ctx, timeout: t.timeout, pending: make(map[jsonrpc.ID]int), changed: make(chan struct{}), closed: make(chan struct{})}
	return (&mcp.IOTransport{Reader: &mcpDrainReader{source: t.reader, reader: bufio.NewReader(t.reader), drain: drain}, Writer: &mcpDrainWriter{source: t.writer, drain: drain}}).Connect(ctx)
}

type mcpEOFDrain struct {
	ctx     context.Context
	timeout time.Duration
	mu      sync.Mutex
	pending map[jsonrpc.ID]int
	changed chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func mcpFrameMessages(raw []byte) []jsonrpc.Message {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		rows = []json.RawMessage{raw}
	}
	var messages []jsonrpc.Message
	for _, row := range rows {
		if msg, err := jsonrpc.DecodeMessage(row); err == nil {
			messages = append(messages, msg)
		}
	}
	return messages
}

func (d *mcpEOFDrain) incoming(raw []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, msg := range mcpFrameMessages(raw) {
		if req, ok := msg.(*jsonrpc.Request); ok && req.IsCall() {
			d.pending[req.ID]++
		}
	}
}

func (d *mcpEOFDrain) outgoing(raw []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, msg := range mcpFrameMessages(raw) {
		if reply, ok := msg.(*jsonrpc.Response); ok {
			if count := d.pending[reply.ID]; count > 1 {
				d.pending[reply.ID]--
			} else {
				delete(d.pending, reply.ID)
			}
		}
	}
	close(d.changed)
	d.changed = make(chan struct{})
}

func (d *mcpEOFDrain) wait() error {
	timer := time.NewTimer(d.timeout)
	defer timer.Stop()
	for {
		d.mu.Lock()
		empty, changed := len(d.pending) == 0, d.changed
		d.mu.Unlock()
		if empty {
			return io.EOF
		}
		select {
		case <-changed:
		case <-d.ctx.Done():
			return d.ctx.Err()
		case <-d.closed:
			return mcp.ErrConnectionClosed
		case <-timer.C:
			return fmt.Errorf("MCP stdin closed before pending replies completed; keep stdin open for long-running requests")
		}
	}
}
func (d *mcpEOFDrain) close() { d.once.Do(func() { close(d.closed) }) }

type mcpDrainReader struct {
	source io.ReadCloser
	reader *bufio.Reader
	drain  *mcpEOFDrain
	buffer []byte
	eof    bool
}

func (r *mcpDrainReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.buffer) == 0 {
		if r.eof {
			return 0, r.drain.wait()
		}
		for {
			fragment, err := r.reader.ReadSlice('\n')
			if len(r.buffer)+len(fragment) > mcp.DefaultMaxLineLength {
				return 0, errors.New("MCP stdin frame exceeds the SDK maximum line length")
			}
			r.buffer = append(r.buffer, fragment...)
			if err == bufio.ErrBufferFull {
				continue
			}
			if err != nil && err != io.EOF {
				return 0, err
			}
			r.eof = err == io.EOF
			if len(r.buffer) == 0 {
				return 0, r.drain.wait()
			}
			// Observation only; the SDK still validates and executes the original bytes.
			r.drain.incoming(r.buffer)
			break
		}
	}
	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}
func (r *mcpDrainReader) Close() error { r.drain.close(); return r.source.Close() }

type mcpDrainWriter struct {
	source io.WriteCloser
	drain  *mcpEOFDrain
}

// SDK ioConn serializes writes and sends one complete encoded frame per call.
func (w *mcpDrainWriter) Write(p []byte) (int, error) {
	n, err := w.source.Write(p)
	if err == nil && n != len(p) {
		return n, io.ErrShortWrite
	}
	if err == nil && n == len(p) {
		w.drain.outgoing(p)
	}
	return n, err
}
func (w *mcpDrainWriter) Close() error { w.drain.close(); return w.source.Close() }

type mcpStdoutWriter struct{ io.Writer }

func (mcpStdoutWriter) Close() error { return nil }
