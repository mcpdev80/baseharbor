package machinehttp

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/sourcetransfer"
)

// All routes pass through Handler.ServeHTTP's existing TLS and Origin
// protections. Each request is independently authorized for an apply to its
// immutable application/target/environment, including upload chunks.
func (h *Handler) authorizeSourceTransfer(r *http.Request, m sourcetransfer.Manifest) error {
	principal, ok := identity.FromContext(r.Context())
	if !ok {
		return machine.NewError(machine.ErrorAuthenticationFailed, "Authenticated operator identity is required.", "Authenticate with the configured OIDC provider.", false)
	}
	operation, ok := machine.OperationByID("apply")
	if !ok {
		return errors.New("machine apply operation is unavailable")
	}
	ctx := operatorauth.WithVerifiedPrincipal(r.Context(), principal)
	decision, err := operatorauth.AuthorizeMachineOperation(ctx, operatorauth.AuthorizationRequest{
		Operation: operation,
		Context:   machine.OperationContext{Target: m.Target, Application: m.Application, Environment: m.Environment},
	})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return machine.NewError(machine.ErrorPolicyDenied, "Source publication is not authorized.", "Review operator policy for the selected Target and application.", false)
	}
	return nil
}
func sourceTransferError(w http.ResponseWriter, err error) {
	writeMachineError(w, machineErrorStatus(err), err)
}

func (h *Handler) handleSourceTransferBegin(w http.ResponseWriter, r *http.Request) {
	var m sourcetransfer.Manifest
	if err := decodeRequest(r.Body, &m); err != nil {
		sourceTransferError(w, machine.NewError(machine.ErrorValidationFailed, "Invalid source manifest.", "Send the canonical transfer manifest.", false))
		return
	}
	if err := m.Validate(); err != nil {
		sourceTransferError(w, machine.Wrap(machine.ErrorValidationFailed, err, "Fix the source snapshot manifest.", false))
		return
	}
	if err := h.authorizeSourceTransfer(r, m); err != nil {
		sourceTransferError(w, err)
		return
	}
	id, missing, err := h.sourceTransfers.Begin(m)
	if err != nil {
		sourceTransferError(w, machine.Wrap(machine.ErrorConflict, err, "Refresh the current source snapshot and retry.", false))
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		ID      string   `json:"id"`
		Missing []string `json:"missing"`
	}{id, missing})
}
func (h *Handler) handleSourceTransferCurrent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	m := sourcetransfer.Manifest{Target: q.Get("target"), Application: q.Get("application"), Environment: q.Get("environment")}
	if err := h.authorizeSourceTransfer(r, m); err != nil {
		sourceTransferError(w, err)
		return
	}
	current, _, err := h.sourceTransfers.Current(m.Target, m.Application, m.Environment)
	if errors.Is(err, os.ErrNotExist) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		sourceTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Digest string `json:"digest"`
	}{current.Digest})
}
func (h *Handler) sourceTransferFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" {
		sourceTransferError(w, machine.NewError(machine.ErrorValidationFailed, "Invalid transfer identifier.", "Begin a new source transfer.", false))
		return "", false
	}
	m, err := h.sourceTransfers.Pending(id)
	if err != nil {
		sourceTransferError(w, machine.Wrap(machine.ErrorNotFound, err, "Begin a new source transfer.", false))
		return "", false
	}
	if err := h.authorizeSourceTransfer(r, m); err != nil {
		sourceTransferError(w, err)
		return "", false
	}
	return id, true
}
func (h *Handler) handleSourceTransferPut(w http.ResponseWriter, r *http.Request) {
	id, ok := h.sourceTransferFromRequest(w, r)
	if !ok {
		return
	}
	var request struct {
		Path   string `json:"path"`
		Offset int64  `json:"offset"`
		Data   []byte `json:"data"`
	}
	if err := decodeRequest(r.Body, &request); err != nil {
		sourceTransferError(w, machine.NewError(machine.ErrorValidationFailed, "Invalid source chunk.", "Send one bounded, ordered chunk.", false))
		return
	}
	if err := h.sourceTransfers.Put(id, request.Path, request.Offset, request.Data); err != nil {
		sourceTransferError(w, machine.Wrap(machine.ErrorConflict, err, "Restart the source transfer if the upload sequence changed.", false))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Accepted bool `json:"accepted"`
	}{true})
}
func (h *Handler) handleSourceTransferCommit(w http.ResponseWriter, r *http.Request) {
	id, ok := h.sourceTransferFromRequest(w, r)
	if !ok {
		return
	}
	// A commit contains no payload: do not accept unverified client substitutions.
	if body, err := io.ReadAll(io.LimitReader(r.Body, 2)); err != nil || len(body) != 0 {
		sourceTransferError(w, machine.NewError(machine.ErrorValidationFailed, "Transfer commit must have an empty body.", "Retry with an empty request body.", false))
		return
	}
	manifest, _, err := h.sourceTransfers.Commit(id)
	if err != nil {
		sourceTransferError(w, machine.Wrap(machine.ErrorConflict, err, "Inspect the transfer and retry with a current base.", false))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Digest string `json:"digest"`
	}{manifest.Digest})
}