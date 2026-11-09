package workstationenrollmenthttp

import (
	"encoding/json"
	"errors"
	"context"
	"io"
	"net/http"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// ServeDelegatedEnroll is the sole unauthenticated-to-OIDC entry point for a
// previously issued, one-use workstation invitation. The Core's durable grant
// store, not the caller's tenant/station fields, authorizes consumption:
// Authority.Enroll atomically matches both token digests and the exact scope.
// A foreign tenant, station, expired or spent grant is denied before signing.
//
// The only permitted effect is enrollment of a new workstation leaf. This
// handler MUST NOT be mounted on /bootstrap, machine APIs or operator routes.
func (h *ProductionHandler) ServeDelegatedEnroll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrBadRequest)
		return
	}
	if r.TLS == nil || h == nil || h.Enrollment == nil {
		writeError(w, http.StatusForbidden, ErrBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req EnrollRequest
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	} else if !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	scope := ws.Scope{TenantID: req.InstallationID, Station: req.Station}
	if scope.Validate() != nil || req.Token == "" || req.Nonce == "" {
		writeError(w, http.StatusForbidden, ws.ErrDenied)
		return
	}
	// The one-use bearer is the delegated authorization. It cannot mint
	// more grants, choose another station, or authorize runtime operations.
	request, err := ws.NewRequest(scope, req.Token, req.Nonce, []byte(req.CSRPEM), time.Hour)
	if err != nil {
		writeError(w, http.StatusForbidden, ws.ErrDenied)
		return
	}
	result, err := h.Enrollment.Enroll(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusForbidden, ws.ErrDenied)
		return
	}
	recorder, ok := h.Enrollment.(interface {
		RecordRegistryFact(context.Context, ws.Scope, time.Time, string, string, string, time.Time) error
	})
	if !ok || recorder.RecordRegistryFact(r.Context(), scope, time.Now().UTC(),
		scope.Identity(), scope.Station, result.Serial, result.ExpiresAt) != nil {
		writeError(w, http.StatusInternalServerError, errors.New("workstation registry persistence failed"))
		return
	}
	writeJSON(w, http.StatusOK, EnrollResponse{
		InstallationID: scope.TenantID,
		Station: scope.Station,
		Serial: result.Serial,
		ExpiresAt: result.ExpiresAt,
		LeafPEM: string(result.Certificate),
		TrustPEM: string(result.Trust.PEM),
		At: time.Now().UTC(),
	})
}
