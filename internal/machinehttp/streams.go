package machinehttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

type LogStreamExecutor interface {
	OpenLogStream(context.Context, machine.StreamRequest) (io.ReadCloser, error)
}

type ExecStreamExecutor interface {
	OpenExecStream(context.Context, machine.StreamRequest) (io.ReadCloser, error)
}

func (h *Handler) handleLogStream(w http.ResponseWriter, r *http.Request) {
	request, decision, streamCtx, err := h.authorizeStreamRequest(r, machine.StreamLogs)
	if err != nil {
		writeMachineError(w, machineErrorStatus(err), err)
		return
	}
	if h.logStreamExecutor == nil {
		writeMachineError(w, http.StatusNotImplemented, machine.NewError(
			machine.ErrorUnsupported,
			"Runtime log streaming is not implemented by the active Runtime Explorer provider.",
			"Negotiate runtime capabilities before opening a log stream.",
			false,
		))
		return
	}
	stream, err := h.logStreamExecutor.OpenLogStream(streamCtx, request)
	if err != nil {
		writeMachineError(w, machineErrorStatus(err), err)
		return
	}
	defer stream.Close()
	stopClose := context.AfterFunc(r.Context(), func() { _ = stream.Close() })
	defer stopClose()

	descriptor, err := newStreamDescriptor(request, decision.Actor)
	if err != nil {
		writeMachineError(w, http.StatusInternalServerError, machine.Wrap(machine.ErrorInternal, err, "Retry the stream request.", true))
		return
	}
	writeStreamHeaders(w, descriptor)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(streamDeadlineWriter{ResponseWriter: w, ctx: r.Context()}, stream)
}

func (h *Handler) handleExecStream(w http.ResponseWriter, r *http.Request) {
	request, decision, streamCtx, err := h.authorizeStreamRequest(r, machine.StreamExec)
	if err != nil {
		writeMachineError(w, machineErrorStatus(err), err)
		return
	}
	if request.TTY {
		writeMachineError(w, http.StatusNotImplemented, machine.NewError(
			machine.ErrorUnsupported,
			"Interactive TTY exec is not implemented by the active Runtime Explorer provider.",
			"Use a non-interactive bounded exec command or a provider that advertises interactive terminal support.",
			false,
		))
		return
	}
	executor, ok := h.executor.(ExecStreamExecutor)
	if !ok {
		writeMachineError(w, http.StatusNotImplemented, machine.NewError(
			machine.ErrorUnsupported,
			"Runtime exec streaming is not implemented by the active Runtime Explorer provider.",
			"Negotiate runtime exec capability; BaseHarbor does not fall back to a host shell.",
			false,
		))
		return
	}
	stream, err := executor.OpenExecStream(streamCtx, request)
	if err != nil {
		writeMachineError(w, machineErrorStatus(err), err)
		return
	}
	defer stream.Close()
	stopClose := context.AfterFunc(r.Context(), func() { _ = stream.Close() })
	defer stopClose()

	descriptor, err := newStreamDescriptor(request, decision.Actor)
	if err != nil {
		writeMachineError(w, http.StatusInternalServerError, machine.Wrap(machine.ErrorInternal, err, "Retry the stream request.", true))
		return
	}
	writeStreamHeaders(w, descriptor)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(streamDeadlineWriter{ResponseWriter: w, ctx: r.Context()}, stream)
}

type streamDeadlineWriter struct {
	http.ResponseWriter
	ctx context.Context
}

func (w streamDeadlineWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	deadline := time.Now().Add(30 * time.Second)
	if limit, ok := w.ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(deadline)
	return w.ResponseWriter.Write(data)
}

func (h *Handler) authorizeStreamRequest(r *http.Request, kind machine.StreamKind) (machine.StreamRequest, operatorauth.AuthorizationDecision, context.Context, error) {
	if r.TLS == nil {
		return machine.StreamRequest{}, operatorauth.AuthorizationDecision{}, nil, machine.NewError(
			machine.ErrorAuthenticationFailed,
			"BaseHarbor machine streams require HTTPS.",
			"Use the authenticated TLS management endpoint.",
			false,
		)
	}
	principal, ok := identity.FromContext(r.Context())
	if !ok {
		return machine.StreamRequest{}, operatorauth.AuthorizationDecision{}, nil, machine.NewError(
			machine.ErrorAuthenticationFailed,
			"Authenticated operator identity is required.",
			"Authenticate with the configured OIDC provider.",
			false,
		)
	}

	var request machine.StreamRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return machine.StreamRequest{}, operatorauth.AuthorizationDecision{}, nil, machine.Wrap(machine.ErrorValidationFailed, err, "Send a valid stream request.", false)
	}
	request.Kind = kind
	if request.ContractVersion == "" {
		request.ContractVersion = machine.StreamContractVersion
	}
	request.Context.Environment = strings.ToLower(strings.TrimSpace(request.Context.Environment))
	if request.Context.Environment == "" {
		return machine.StreamRequest{}, operatorauth.AuthorizationDecision{}, nil, machine.NewError(machine.ErrorValidationFailed, "context.environment is required.", "Provide the explicit BaseHarbor environment for stream authorization.", false)
	}
	if err := request.Validate(); err != nil {
		return machine.StreamRequest{}, operatorauth.AuthorizationDecision{}, nil, err
	}

	ctx := operatorauth.WithVerifiedPrincipal(r.Context(), principal)
	operation := machine.Operation{
		ID:              "runtime." + string(kind),
		Description:     "Protected runtime " + string(kind) + " stream.",
		Safety:          machine.SafetyReadOnly,
		PolicyRequired:  true,
		ContractVersion: machine.ContractVersion,
	}
	if kind == machine.StreamExec {
		operation.Safety = machine.SafetyMutating
	}
	decision, err := operatorauth.AuthorizeMachineOperation(ctx, operatorauth.AuthorizationRequest{
		Operation: operation,
		Context:   request.Context,
	})
	if err != nil {
		return machine.StreamRequest{}, decision, nil, err
	}
	return request, decision, ctx, nil
}

func newStreamDescriptor(request machine.StreamRequest, actor machine.ActorRef) (machine.StreamDescriptor, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return machine.StreamDescriptor{}, err
	}
	return machine.StreamDescriptor{
		ContractVersion: machine.StreamContractVersion,
		StreamID:        "stream_" + hex.EncodeToString(raw[:]),
		Kind:            request.Kind,
		Actor:           actor,
		Context:         request.Context,
		ResourceKind:    request.ResourceKind,
		ResourceID:      request.ResourceID,
		CreatedAt:       time.Now().UTC(),
	}, nil
}

func writeStreamHeaders(w http.ResponseWriter, descriptor machine.StreamDescriptor) {
	w.Header().Set("X-BaseHarbor-Stream-ID", descriptor.StreamID)
	w.Header().Set("X-BaseHarbor-Stream-Kind", string(descriptor.Kind))
	w.Header().Set("X-BaseHarbor-Resource-Kind", descriptor.ResourceKind)
	w.Header().Set("X-BaseHarbor-Resource-ID", descriptor.ResourceID)
	w.Header().Set("X-BaseHarbor-Actor-Subject", descriptor.Actor.Subject)
	w.Header().Set("X-BaseHarbor-Target", descriptor.Context.Target)
}
