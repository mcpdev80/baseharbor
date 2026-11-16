package runtimeexplorer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtime/terminal"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

type connectorStreamTransport interface {
	OpenStream(context.Context, targetenrollment.Scope, targetsession.StreamOpen) (*targetsession.Stream, error)
}

func (b *ConnectorBackend) openStream(ctx context.Context, id, kind string, logs *targetsession.LogOptions, tty *targetsession.TerminalOptions) (*targetsession.Stream, error) {
	transport, ok := b.pool.(connectorStreamTransport)
	if !ok {
		return nil, machine.NewError(machine.ErrorCapabilityMissing, "Remote stream transport is unavailable.", "Connect the selected Target with its authenticated stream capability.", false)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	streamID := hex.EncodeToString(nonce[:])
	correlation := machine.ExecutionCorrelation(ctx)
	if correlation == "" {
		correlation = streamID
	}
	deadline := time.Now().UTC().Add(4 * time.Minute)
	if bound, ok := ctx.Deadline(); ok && bound.Before(deadline) {
		deadline = bound.UTC()
	}
	stream, err := transport.OpenStream(ctx, b.scope, targetsession.StreamOpen{ContractVersion: "baseharbor.target-access/v1", ProtocolVersion: "1", StreamID: streamID, CorrelationID: correlation, TargetID: b.scope.TargetID, ResourceID: id, Kind: kind, DeadlineAt: deadline, Logs: logs, Terminal: tty})
	if err != nil {
		return nil, machine.NewError(machine.ErrorRuntimeUnavailable, "Authenticated remote stream could not be opened.", "Reconcile the selected Target before opening a new stream; interactive processes are not replayed.", false)
	}
	return stream, nil
}

func (b *ConnectorBackend) followLogs(ctx context.Context, id string, since *time.Time, tail int) (io.ReadCloser, error) {
	if tail <= 0 {
		tail = 200
	}
	if tail > 10000 {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Remote log tail exceeds the bound.", "Request at most 10000 lines.", false)
	}
	options := &targetsession.LogOptions{Tail: tail, Follow: true}
	if since != nil {
		options.Since = since.UTC().Format(time.RFC3339Nano)
	}
	return b.openStream(ctx, id, "logs", options, nil)
}

func (b *ConnectorBackend) ContainerTerminal(ctx context.Context, id string, argv []string, rows, columns int) (terminal.Session, error) {
	if err := terminal.ValidateSize(rows, columns); err != nil {
		return nil, err
	}
	return b.openStream(ctx, id, "terminal", nil, &targetsession.TerminalOptions{Rows: rows, Cols: columns, Argv: append([]string(nil), argv...)})
}

func (b *ConnectorBackend) TerminalAvailable() bool {
	if _, ok := b.pool.(connectorStreamTransport); !ok {
		return false
	}
	capabilities, err := b.pool.LiveCapabilities(b.scope)
	if err != nil {
		return false
	}
	for _, c := range capabilities.Capabilities {
		if c.Name == "runtime.terminal" && c.Available {
			return true
		}
	}
	return false
}
