package machinehttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/runtime/terminal"
)

type TerminalExecutor interface {
	OpenTerminal(context.Context, machine.StreamRequest) (terminal.Session, error)
	RecordTerminalAudit(context.Context, machine.StreamDescriptor, string) error
}

func (h *Handler) handleTerminalOpen(w http.ResponseWriter, r *http.Request) {
	request, decision, streamCtx, err := h.authorizeStreamRequest(r, machine.StreamExec)
	if err != nil {
		writeMachineError(w, machineErrorStatus(err), err)
		return
	}
	if !request.TTY || request.Context.Target == "" || terminal.ValidateSize(request.Rows, request.Columns) != nil {
		writeMachineError(w, http.StatusBadRequest, machine.NewError(machine.ErrorValidationFailed, "Terminal requires tty and bounded rows/columns.", "Use explicit argv, resource, environment and a terminal size between 1 and 512.", false))
		return
	}
	executor, ok := h.executor.(TerminalExecutor)
	if !ok {
		writeMachineError(w, http.StatusNotImplemented, machine.NewError(machine.ErrorUnsupported, "Interactive terminal is unavailable.", "Negotiate the runtime terminal capability.", false))
		return
	}
	descriptor, err := newStreamDescriptor(request, decision.Actor)
	if err != nil {
		writeMachineError(w, http.StatusInternalServerError, machine.Classify(err))
		return
	}
	deadline := time.Now().Add(streamMaxRuntime)
	principal, _ := identity.FromContext(r.Context())
	if principal.ExpiresAt != nil && principal.ExpiresAt.Before(deadline) {
		deadline = *principal.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(context.WithoutCancel(streamCtx), deadline)
	ctx = machine.WithExecutionCorrelation(ctx, descriptor.StreamID)
	record := &terminalRecord{descriptor: descriptor, request: request, ctx: ctx, cancel: cancel}
	if err := h.terminals.reserve(record); err != nil {
		cancel()
		writeMachineError(w, http.StatusTooManyRequests, machine.NewError(machine.ErrorConflict, "Terminal capacity exhausted.", "Close an existing terminal before retrying.", true))
		return
	}
	if err := executor.RecordTerminalAudit(ctx, descriptor, "admitted"); err != nil {
		cancel()
		h.terminals.remove(descriptor.StreamID)
		writeMachineError(w, http.StatusInternalServerError, machine.NewError(machine.ErrorInternal, "Terminal audit could not be recorded.", "Restore the Core audit store before retrying.", true))
		return
	}
	session, err := executor.OpenTerminal(ctx, request)
	if err != nil {
		_ = executor.RecordTerminalAudit(context.WithoutCancel(ctx), descriptor, "failed")
		cancel()
		h.terminals.remove(descriptor.StreamID)
		writeMachineError(w, machineErrorStatus(err), err)
		return
	}
	record.mu.Lock()
	record.session = session
	record.mu.Unlock()
	record.startLifetime(h.terminals, func() { _ = executor.RecordTerminalAudit(context.WithoutCancel(ctx), descriptor, "closed") })
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, descriptor)
}

func (h *Handler) authorizedTerminal(w http.ResponseWriter, r *http.Request) *terminalRecord {
	principal, ok := identity.FromContext(r.Context())
	if !ok {
		writeMachineError(w, http.StatusUnauthorized, machine.NewError(machine.ErrorAuthenticationFailed, "Authenticated operator is required.", "Authenticate again.", false))
		return nil
	}
	record := h.terminals.get(r.PathValue("stream_id"))
	if record == nil || record.ctx.Err() != nil {
		writeMachineError(w, http.StatusNotFound, machine.NewError(machine.ErrorConflict, "Terminal is closed or unavailable.", "Open a new authorized terminal; replay is not supported.", false))
		return nil
	}
	actor := record.descriptor.Actor
	if actor.Mode != "authenticated" || actor.Subject != principal.Subject || actor.Issuer != principal.Issuer {
		writeMachineError(w, http.StatusForbidden, machine.NewError(machine.ErrorPolicyDenied, "Terminal belongs to another operator.", "Use a terminal created by this authenticated actor.", false))
		return nil
	}
	ctx := operatorauth.WithVerifiedPrincipal(r.Context(), principal)
	_, err := operatorauth.AuthorizeMachineOperation(ctx, operatorauth.AuthorizationRequest{Operation: machine.Operation{ID: "runtime.exec", Safety: machine.SafetyMutating, PolicyRequired: true, ContractVersion: machine.ContractVersion}, Context: record.request.Context})
	if err != nil {
		record.cancel()
		writeMachineError(w, machineErrorStatus(err), err)
		return nil
	}
	record.mu.Lock()
	ready := record.session != nil
	record.mu.Unlock()
	if !ready {
		writeMachineError(w, http.StatusConflict, machine.NewError(machine.ErrorConflict, "Terminal is starting.", "Wait for the creation response.", true))
		return nil
	}
	return record
}

func (h *Handler) handleTerminalInput(w http.ResponseWriter, r *http.Request) {
	record := h.authorizedTerminal(w, r)
	if record == nil {
		return
	}
	var input machine.TerminalInput
	if err := decodeRequest(r.Body, &input); err != nil {
		writeMachineError(w, http.StatusBadRequest, machine.NewError(machine.ErrorValidationFailed, "Invalid terminal input JSON.", "Send exactly one bounded terminal input object.", false))
		return
	}
	if input.ContractVersion != machine.StreamContractVersion || input.Sequence == 0 ||
		(input.Kind != "input" && input.Kind != "resize") || len(input.Data) > 16384 ||
		(input.Kind == "resize" && (len(input.Data) != 0 || terminal.ValidateSize(input.Rows, input.Columns) != nil)) ||
		(input.Kind == "input" && (len(input.Data) == 0 || input.Rows != 0 || input.Columns != 0)) {
		writeMachineError(w, http.StatusBadRequest, machine.NewError(machine.ErrorValidationFailed, "Invalid terminal input frame.", "Use a bounded input or resize frame with the next sequence.", false))
		return
	}
	record.mu.Lock()
	defer record.mu.Unlock()
	if input.Sequence != record.inputSequence+1 {
		writeMachineError(w, http.StatusConflict, machine.NewError(machine.ErrorConflict, "Terminal input sequence differs.", "Do not replay terminal input; reconnect by creating a new session.", false))
		return
	}
	// Consume the sequence before a side effect. Ambiguous writes are never retried.
	record.inputSequence = input.Sequence
	writeCtx, cancel := context.WithTimeout(record.ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(writeCtx, func() { _ = record.session.Close() })
	defer stop()
	var err error
	if input.Kind == "resize" {
		err = record.session.Resize(input.Rows, input.Columns)
	} else {
		var n int
		n, err = record.session.Write(input.Data)
		if n != len(input.Data) && err == nil {
			err = io.ErrShortWrite
		}
	}
	if err != nil {
		record.cancel()
		writeMachineError(w, http.StatusConflict, machine.NewError(machine.ErrorConflict, "Terminal input did not complete.", "Open a new session; the input must not be automatically replayed.", false))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleTerminalClose(w http.ResponseWriter, r *http.Request) {
	record := h.authorizedTerminal(w, r)
	if record == nil {
		return
	}
	record.cancel()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleTerminalEvents(w http.ResponseWriter, r *http.Request) {
	record := h.authorizedTerminal(w, r)
	if record == nil {
		return
	}
	if r.Header.Get("Last-Event-ID") != "" {
		writeMachineError(w, http.StatusConflict, machine.NewError(machine.ErrorConflict, "Terminal replay is unavailable.", "Create a new session instead of replaying terminal events.", false))
		return
	}
	record.mu.Lock()
	if record.attached {
		record.mu.Unlock()
		writeMachineError(w, http.StatusConflict, machine.NewError(machine.ErrorConflict, "Terminal output is already attached.", "Use the original connection or open a new session.", false))
		return
	}
	record.attached = true
	record.mu.Unlock()
	defer record.cancel()
	stop := context.AfterFunc(r.Context(), func() { record.cancel() })
	defer stop()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writer := streamDeadlineWriter{ResponseWriter: w, ctx: record.ctx}
	sequence := uint64(0)
	emit := func(kind string, data []byte, exit *int) error {
		sequence++
		event := machine.TerminalEvent{ContractVersion: machine.StreamContractVersion, StreamID: record.descriptor.StreamID, Sequence: sequence, Kind: kind, OccurredAt: time.Now().UTC(), Data: data, ExitCode: exit}
		wire, _ := json.Marshal(event)
		if _, err := fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", sequence, kind, wire); err != nil {
			return err
		}
		return http.NewResponseController(w).Flush()
	}
	if err := emit("terminal.ready", nil, nil); err != nil {
		return
	}
	// Read at most one chunk ahead. A slow client blocks bounded socket writes,
	// then cancellation closes the PTY; no unbounded replay queue exists.
	buffer := make([]byte, 16384)
	for {
		n, err := record.session.Read(buffer)
		if n > 0 {
			if writeErr := emit("terminal.output", buffer[:n], nil); writeErr != nil {
				return
			}
		}
		if err != nil {
			break
		}
	}
	exit, err := record.session.Wait(record.ctx)
	if err == nil {
		if auditErr := h.executor.(TerminalExecutor).RecordTerminalAudit(context.WithoutCancel(record.ctx), record.descriptor, "exited"); auditErr != nil {
			return
		}
		_ = emit("terminal.exit", nil, &exit)
	}
}
