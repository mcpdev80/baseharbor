package runtimeresourceapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

type Authorizer interface {
	AuthorizeRuntimeOperation(app, capability, operation string) error
}

type Handler struct {
	app        string
	operations *runtimeoperation.Manager
	authorizer Authorizer
	executor   runtimeoperation.Executor
	mux        *http.ServeMux
}

func New(app string, operations *runtimeoperation.Manager, authorizer Authorizer, executor runtimeoperation.Executor) (*Handler, error) {
	app = strings.TrimSpace(app)
	if app == "" || operations == nil || authorizer == nil || executor == nil {
		return nil, errors.New("runtime resource API dependencies are required")
	}
	h := &Handler{app: app, operations: operations, authorizer: authorizer, executor: executor, mux: http.NewServeMux()}
	h.mux.HandleFunc("POST /runtime/v1/resources", h.create)
	h.mux.HandleFunc("GET /runtime/v1/resources/{resourceId}", h.get)
	h.mux.HandleFunc("DELETE /runtime/v1/resources/{resourceId}", h.delete)
	h.mux.HandleFunc("GET /runtime/v1/resources/{resourceId}/binding", h.binding)
	h.mux.HandleFunc("GET /runtime/v1/operations/{operationId}", h.operation)
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeProblem(w, http.StatusBadRequest, "missing idempotency key", "Idempotency-Key is required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Capability string         `json:"capability"`
		Name       string         `json:"name"`
		Parameters map[string]any `json:"parameters,omitempty"`
	}
	if err := decoder.Decode(&request); err != nil || ensureEOF(decoder) != nil {
		writeProblem(w, http.StatusBadRequest, "invalid resource request", "request body is invalid")
		return
	}
	request.Capability = strings.TrimSpace(request.Capability)
	request.Name = strings.TrimSpace(request.Name)
	if request.Capability == "" || request.Name == "" {
		writeProblem(w, http.StatusBadRequest, "invalid resource request", "capability and name are required")
		return
	}
	if err := h.authorizer.AuthorizeRuntimeOperation(h.app, request.Capability, "runtime.create"); err != nil {
		writeProblem(w, http.StatusForbidden, "runtime operation not allowed", "the application is not authorized for this capability operation")
		return
	}

	op, replay, err := h.operations.Submit(r.Context(), runtimeoperation.Request{
		Application:    h.app,
		Capability:     request.Capability,
		Operation:      "runtime.create",
		ResourceName:   request.Name,
		IdempotencyKey: idempotencyKey,
		Parameters:     request.Parameters,
	})
	if err != nil {
		if strings.Contains(err.Error(), "unsupported capability operation") {
			writeProblem(w, http.StatusUnprocessableEntity, "unsupported capability operation", err.Error())
			return
		}
		writeProblem(w, http.StatusInternalServerError, "runtime operation failed", "the runtime operation could not be accepted")
		return
	}
	status := http.StatusAccepted
	if replay {
		status = http.StatusOK
	}
	writeJSON(w, status, operationResponse(op))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	request, ok := h.resourceRequest(w, r, "runtime.get")
	if !ok {
		return
	}
	result, err := h.executor.Execute(r.Context(), runtimeoperation.Request{
		Application:  h.app,
		Capability:   request.Capability,
		Operation:    "runtime.get",
		ResourceName: request.ResourceName,
	})
	if err != nil {
		writeProblem(w, http.StatusNotFound, "runtime resource not found", "resource does not exist or is not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         result.ResourceID,
		"capability": request.Capability,
		"name":       request.ResourceName,
		"state":      "ready",
	})
}

func (h *Handler) binding(w http.ResponseWriter, r *http.Request) {
	request, ok := h.resourceRequest(w, r, "runtime.get")
	if !ok {
		return
	}
	result, err := h.executor.Execute(r.Context(), runtimeoperation.Request{
		Application:  h.app,
		Capability:   request.Capability,
		Operation:    "runtime.get",
		ResourceName: request.ResourceName,
	})
	if err != nil || result.Binding == nil {
		writeProblem(w, http.StatusNotFound, "runtime resource binding not found", "binding does not exist or is not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource_id": result.ResourceID,
		"capability":  request.Capability,
		"binding":     result.Binding,
	})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	request, ok := h.resourceRequest(w, r, "runtime.delete")
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeProblem(w, http.StatusBadRequest, "missing idempotency key", "Idempotency-Key is required")
		return
	}
	op, replay, err := h.operations.Submit(r.Context(), runtimeoperation.Request{
		Application:    h.app,
		Capability:     request.Capability,
		Operation:      "runtime.delete",
		ResourceName:   request.ResourceName,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		if strings.Contains(err.Error(), "unsupported capability operation") {
			writeProblem(w, http.StatusUnprocessableEntity, "unsupported capability operation", err.Error())
			return
		}
		writeProblem(w, http.StatusInternalServerError, "runtime operation failed", "the delete operation could not be accepted")
		return
	}
	status := http.StatusAccepted
	if replay {
		status = http.StatusOK
	}
	writeJSON(w, status, operationResponse(op))
}

func (h *Handler) resourceRequest(w http.ResponseWriter, r *http.Request, operation string) (runtimeoperation.Request, bool) {
	id := strings.TrimSpace(r.PathValue("resourceId"))
	if id == "" {
		writeProblem(w, http.StatusBadRequest, "invalid resource id", "resource id is required")
		return runtimeoperation.Request{}, false
	}
	request, err := h.operations.FindResource(id)
	if errors.Is(err, runtimeoperation.ErrNotFound) || request.Application != h.app {
		writeProblem(w, http.StatusNotFound, "runtime resource not found", "resource does not exist")
		return runtimeoperation.Request{}, false
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "runtime resource lookup failed", "resource state could not be loaded")
		return runtimeoperation.Request{}, false
	}
	if err := h.authorizer.AuthorizeRuntimeOperation(h.app, request.Capability, operation); err != nil {
		writeProblem(w, http.StatusForbidden, "runtime operation not allowed", "the application is not authorized for this capability operation")
		return runtimeoperation.Request{}, false
	}
	return request, true
}

func (h *Handler) operation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("operationId"))
	if id == "" {
		writeProblem(w, http.StatusBadRequest, "invalid operation id", "operation id is required")
		return
	}
	op, err := h.operations.Get(id)
	if errors.Is(err, runtimeoperation.ErrNotFound) || op.Request.Application != h.app {
		writeProblem(w, http.StatusNotFound, "operation not found", "operation does not exist")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "runtime operation failed", "operation state could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, operationResponse(op))
}

func operationResponse(op runtimeoperation.Operation) map[string]any {
	response := map[string]any{
		"id":    op.ID,
		"state": op.State,
	}
	if op.Result.ResourceID != "" {
		response["resource_id"] = op.Result.ResourceID
	}
	if op.Error != "" {
		response["error"] = map[string]any{
			"type": "about:blank", "title": "runtime operation failed",
			"status": http.StatusInternalServerError, "detail": op.Error,
		}
	}
	return response
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	writeJSON(w, status, map[string]any{
		"type": "about:blank", "title": title, "status": status, "detail": detail,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
