package machinehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

const (
	maxRequestBytes     = 1 << 20
	executionMaxRuntime = 30 * time.Minute
	streamMaxRuntime    = 5 * time.Minute
)

type ProgressReporter func(machine.OperationProgress)

type Executor interface {
	SupportedOperationIDs() []string
	Execute(context.Context, machine.Operation, machine.OperationContext, json.RawMessage, ProgressReporter) (json.RawMessage, error)
}

type ExecuteRequest struct {
	OperationID string                   `json:"operation_id"`
	Context     machine.OperationContext `json:"context"`
	Input       json.RawMessage          `json:"input,omitempty"`
}

type Handler struct {
	operations        []machine.Operation
	operationByID     map[string]machine.Operation
	executor          Executor
	logStreamExecutor LogStreamExecutor
	executions        *executionStore
	mux               *http.ServeMux
	terminals         *terminalStore
}

func New(executor Executor) (*Handler, error) {
	if executor == nil {
		return nil, errors.New("machine HTTP executor is required")
	}
	h := &Handler{
		operations:    make([]machine.Operation, 0),
		operationByID: make(map[string]machine.Operation),
		executor:      executor,
		executions:    newExecutionStore(),
		mux:           http.NewServeMux(),
		terminals:     newTerminalStore(),
	}
	for _, id := range executor.SupportedOperationIDs() {
		operation, exists := machine.OperationByID(id)
		if _, duplicate := h.operationByID[id]; !exists || duplicate {
			return nil, errors.New("invalid machine HTTP operation support registry")
		}
		h.operationByID[id] = operation
	}
	for _, operation := range machine.Operations() {
		if _, supported := h.operationByID[operation.ID]; supported {
			h.operations = append(h.operations, operation)
		}
	}
	if logStreamExecutor, ok := executor.(LogStreamExecutor); ok {
		h.logStreamExecutor = logStreamExecutor
	}
	bindings := machine.MachineHTTPBindings()
	for key, handler := range map[string]http.HandlerFunc{
		"discovery": h.handleDiscovery, "execute": h.handleExecute, "execution": h.handleExecution, "execution_events": h.handleEvents,
		"logs": h.handleLogStream, "exec": h.handleExecStream, "terminal_open": h.handleTerminalOpen, "terminal_events": h.handleTerminalEvents,
		"terminal_input": h.handleTerminalInput, "terminal_close": h.handleTerminalClose,
	} {
		binding := bindings[key]
		h.mux.HandleFunc(binding.Method+" "+binding.Href, handler)
	}
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		writeMachineError(w, http.StatusUpgradeRequired, machine.NewError(
			machine.ErrorAuthenticationFailed,
			"BaseHarbor machine API requires HTTPS.",
			"Use the authenticated TLS management endpoint.",
			false,
		))
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "https" || parsed.Host != r.Host || parsed.User != nil ||
			parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			writeMachineError(w, http.StatusForbidden, machine.NewError(machine.ErrorPolicyDenied,
				"The browser origin does not match the protected Core destination.",
				"Use the configured same-origin HTTPS Console/Core endpoint.", false))
			return
		}
	}
	principal, authenticated := identity.FromContext(r.Context())
	if authenticated && principal.ExpiresAt != nil && !principal.ExpiresAt.After(time.Now()) {
		writeMachineError(w, http.StatusUnauthorized, machine.NewError(machine.ErrorAuthenticationFailed,
			"The authenticated operator session has expired.", "Authenticate again before opening a new request.", false))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/machine/streams/") || strings.HasSuffix(r.URL.Path, "/events") {
		deadline := time.Now().Add(streamMaxRuntime)
		if authenticated && principal.ExpiresAt != nil && principal.ExpiresAt.Before(deadline) {
			deadline = *principal.ExpiresAt
		}
		ctx, cancel := context.WithDeadline(r.Context(), deadline)
		defer cancel()
		r = r.WithContext(ctx)
	}
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if _, ok := identity.FromContext(r.Context()); !ok {
		writeMachineError(w, http.StatusUnauthorized, machine.NewError(machine.ErrorAuthenticationFailed, "Authenticated operator identity is required.", "Authenticate with the configured OIDC provider.", false))
		return
	}
	discovery := machine.MachineDiscovery()
	discovery.Operations = h.operations
	discovery.HTTP = machine.MachineHTTPBindings()
	if _, ok := h.executor.(TerminalExecutor); ok {
		discovery.Capabilities = append(discovery.Capabilities, "streams.terminal")
	}
	if h.logStreamExecutor != nil {
		discovery.Capabilities = append(discovery.Capabilities, "streams.logs")
	}
	writeJSON(w, http.StatusOK, discovery)
}

func (h *Handler) handleExecute(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.FromContext(r.Context())
	if !ok {
		writeMachineError(w, http.StatusUnauthorized, machine.NewError(machine.ErrorAuthenticationFailed, "Authenticated operator identity is required.", "Authenticate with the configured OIDC provider.", false))
		return
	}

	var request ExecuteRequest
	if err := decodeRequest(r.Body, &request); err != nil {
		writeMachineError(w, http.StatusBadRequest, machine.Wrap(machine.ErrorValidationFailed, err, "Send a valid machine execution request.", false))
		return
	}
	request.OperationID = strings.TrimSpace(request.OperationID)
	request.Context.Environment = strings.ToLower(strings.TrimSpace(request.Context.Environment))
	if request.OperationID == "" || request.Context.Environment == "" {
		writeMachineError(w, http.StatusBadRequest, machine.NewError(machine.ErrorValidationFailed, "operation_id and context.environment are required.", "Provide an explicit semantic operation and environment.", false))
		return
	}
	operation, exists := h.operationByID[request.OperationID]
	if !exists {
		writeMachineError(w, http.StatusNotFound, machine.NewError(machine.ErrorUnsupported, "Unknown machine operation.", "Use GET /api/v1/machine/discovery to negotiate supported operations.", false))
		return
	}

	ctx := operatorauth.WithVerifiedPrincipal(r.Context(), principal)
	decision, err := operatorauth.AuthorizeMachineOperation(ctx, operatorauth.AuthorizationRequest{
		Operation: operation,
		Context:   request.Context,
	})
	if err != nil || !decision.Allowed {
		if err == nil {
			err = machine.NewError(machine.ErrorPolicyDenied, "Machine operation is not authorized.", "Review operator identity and effective policy.", false)
		}
		writeMachineError(w, machineErrorStatus(err), err)
		return
	}

	ctx = evidence.WithActor(ctx, "http", decision.Actor.Subject)
	execution, err := h.executions.create(operation.ID, decision.Actor, request.Context)
	if err != nil {
		writeMachineError(w, http.StatusInternalServerError, machine.Wrap(machine.ErrorInternal, err, "Retry the request.", true))
		return
	}
	w.Header().Set("Location", "/api/v1/machine/executions/"+execution.ExecutionID)
	writeJSON(w, http.StatusAccepted, execution)

	input := append(json.RawMessage(nil), request.Input...)
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	go h.runExecution(context.WithoutCancel(ctx), execution.ExecutionID, operation, request.Context, input)
}

func (h *Handler) runExecution(parent context.Context, executionID string, operation machine.Operation, operationContext machine.OperationContext, input json.RawMessage) {
	parent = machine.WithExecutionCorrelation(parent, executionID)
	ctx, cancel := context.WithTimeout(parent, executionMaxRuntime)
	defer cancel()
	if err := h.executions.start(executionID); err != nil {
		return
	}
	result, err := h.executor.Execute(ctx, operation, operationContext, input, func(progress machine.OperationProgress) {
		_ = h.executions.progress(executionID, progress)
	})
	if err != nil {
		_ = h.executions.fail(executionID, err)
		return
	}
	if len(result) == 0 {
		result = json.RawMessage("{}")
	}
	_ = h.executions.succeed(executionID, result)
}

func (h *Handler) handleExecution(w http.ResponseWriter, r *http.Request) {
	if _, ok := identity.FromContext(r.Context()); !ok {
		writeMachineError(w, http.StatusUnauthorized, machine.NewError(machine.ErrorAuthenticationFailed, "Authenticated operator identity is required.", "Authenticate with the configured OIDC provider.", false))
		return
	}
	id := strings.TrimSpace(r.PathValue("execution_id"))
	execution, ok := h.executions.get(id)
	if !ok {
		writeMachineError(w, http.StatusNotFound, machine.NewError(machine.ErrorSourceMissing, "Execution was not found.", "Refresh execution state or start a new operation.", false))
		return
	}
	if err := authorizeExecutionReader(r.Context(), execution); err != nil {
		writeMachineError(w, http.StatusForbidden, err)
		return
	}
	writeJSON(w, http.StatusOK, execution)
}

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := identity.FromContext(r.Context()); !ok {
		writeMachineError(w, http.StatusUnauthorized, machine.NewError(machine.ErrorAuthenticationFailed, "Authenticated operator identity is required.", "Authenticate with the configured OIDC provider.", false))
		return
	}
	id := strings.TrimSpace(r.PathValue("execution_id"))
	execution, exists := h.executions.get(id)
	if !exists {
		writeMachineError(w, http.StatusNotFound, machine.NewError(machine.ErrorSourceMissing, "Execution was not found.", "Refresh execution state or start a new operation.", false))
		return
	}
	if err := authorizeExecutionReader(r.Context(), execution); err != nil {
		writeMachineError(w, http.StatusForbidden, err)
		return
	}
	history, events, cancel, ok := h.executions.subscribe(id)
	if !ok {
		writeMachineError(w, http.StatusNotFound, machine.NewError(machine.ErrorSourceMissing, "Execution was not found.", "Refresh execution state or start a new operation.", false))
		return
	}
	defer cancel()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeMachineError(w, http.StatusInternalServerError, machine.NewError(machine.ErrorUnsupported, "Streaming is not supported by this HTTP server.", "Use a server that supports streaming responses.", false))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	writer := streamDeadlineWriter{ResponseWriter: w, ctx: r.Context()}
	for _, event := range history {
		if err := writeSSEEvent(writer, event); err != nil {
			return
		}
		flusher.Flush()
		if terminalState(event.State) {
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-events:
			if !open {
				return
			}
			if err := writeSSEEvent(writer, event); err != nil {
				return
			}
			flusher.Flush()
			if terminalState(event.State) {
				return
			}
		}
	}
}

func authorizeExecutionReader(ctx context.Context, execution machine.Execution) error {
	principal, ok := identity.FromContext(ctx)
	if !ok {
		return machine.NewError(machine.ErrorAuthenticationFailed, "Authenticated operator identity is required.", "Authenticate with the configured OIDC provider.", false)
	}
	if execution.Actor.Mode == "authenticated" &&
		(strings.TrimSpace(execution.Actor.Subject) != strings.TrimSpace(principal.Subject) ||
			strings.TrimSpace(execution.Actor.Issuer) != strings.TrimSpace(principal.Issuer)) {
		return &machine.Error{
			Code:      machine.ErrorPolicyDenied,
			CauseCode: "execution_actor_mismatch",
			Message:   "Execution metadata belongs to a different authenticated operator.",
			Resource:  execution.ExecutionID,
			Next:      "Use the execution created by the current authenticated operator.",
		}
	}
	return nil
}

func terminalState(state machine.ExecutionState) bool {
	switch state {
	case machine.ExecutionSucceeded, machine.ExecutionFailed, machine.ExecutionCancelled:
		return true
	default:
		return false
	}
}

func writeSSEEvent(w io.Writer, event machine.MachineEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "id: "+strconv.FormatUint(event.Sequence, 10)+"\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "event: "+string(event.Kind)+"\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "data: "+string(data)+"\n\n"); err != nil {
		return err
	}
	return nil
}

func machineErrorStatus(err error) int {
	classified := machine.Classify(err)
	switch classified.Code {
	case machine.ErrorAuthenticationFailed:
		return http.StatusUnauthorized
	case machine.ErrorPolicyDenied:
		return http.StatusForbidden
	case machine.ErrorValidationFailed, machine.ErrorApprovalRequired:
		return http.StatusBadRequest
	case machine.ErrorConflict, machine.ErrorOwnershipAmbiguous:
		return http.StatusConflict
	case machine.ErrorSourceMissing:
		return http.StatusNotFound
	case machine.ErrorUnsupported:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

func writeMachineError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, machine.ResultError(err))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
