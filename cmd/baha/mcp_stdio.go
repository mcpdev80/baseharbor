package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The SDK closes a session as soon as its reader reaches EOF, cancelling
// handlers even when complete requests were decoded before the peer half-close.
// Keep the SDK's framing and dispatch, but let those requests flush their replies.
type drainingMCPTransport struct {
	inner   mcp.Transport
	timeout time.Duration
}

func (t *drainingMCPTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	c, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &drainingMCPConnection{Connection: c, timeout: t.timeout, pending: make(map[jsonrpc.ID]int), changed: make(chan struct{}), closed: make(chan struct{})}, nil
}

type drainingMCPConnection struct {
	mcp.Connection
	timeout time.Duration
	mu      sync.Mutex
	pending map[jsonrpc.ID]int
	changed chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (c *drainingMCPConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	msg, err := c.Connection.Read(ctx)
	if err == nil {
		if req, ok := msg.(*jsonrpc.Request); ok && req.IsCall() {
			c.mu.Lock()
			c.pending[req.ID]++
			c.mu.Unlock()
		}
		return msg, nil
	}
	if !errors.Is(err, io.EOF) {
		return nil, err
	}
	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	for {
		c.mu.Lock()
		empty, changed := len(c.pending) == 0, c.changed
		c.mu.Unlock()
		if empty {
			return nil, io.EOF
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.closed:
			return nil, mcp.ErrConnectionClosed
		case <-timer.C:
			return nil, fmt.Errorf("MCP stdin closed before pending replies completed; keep stdin open for long-running requests")
		}
	}
}

func (c *drainingMCPConnection) Write(ctx context.Context, msg jsonrpc.Message) error {
	if err := c.Connection.Write(ctx, msg); err != nil {
		return err
	}
	if reply, ok := msg.(*jsonrpc.Response); ok {
		c.mu.Lock()
		if count := c.pending[reply.ID]; count > 1 {
			c.pending[reply.ID]--
		} else {
			delete(c.pending, reply.ID)
		}
		close(c.changed)
		c.changed = make(chan struct{})
		c.mu.Unlock()
	}
	return nil
}

func (c *drainingMCPConnection) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Connection.Close()
}
