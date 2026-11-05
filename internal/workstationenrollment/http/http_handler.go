package workstationenrollmenthttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// Handler serves the one-use workstation-enrollment boundary over HTTP at the
// management boundary. It binds the recipient/scope from the request, performs
// the single-use enrollment through the trusted issuer, and returns the signed
// leaf plus the registry fact (scope + time). No private key is ever returned.
type Handler struct {
	Enrollment Enrollment
}

// NewHandler returns a Handler for the given one-use enrollment boundary.
func NewHandler(e Enrollment) *Handler {
	return &Handler{Enrollment: e}
}

// ErrBadRequest signals a malformed request.
var ErrBadRequest = errors.New("invalid workstation enrollment request")

// ServeBootstrap handles POST /bootstrap. It creates a one-use bootstrap
// authorization for the workstation's installation/scope and returns digested
// credentials plus the short-lived validity window.
func (h *Handler) ServeBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req BootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	scope := ws.Scope{TenantID: req.InstallationID, Station: req.Station}
	if err := scope.Validate(); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	lifetime := 30 * time.Second
	certTTL := time.Duration(req.CertificateTTLSeconds) * time.Second
	bootstrap, err := h.Enrollment.Create(r.Context(), scope, lifetime, certTTL)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	writeJSON(w, http.StatusOK, BootstrapResponse{
		Token:                 bootstrap.Token,
		Nonce:                 bootstrap.Nonce,
		BootstrapID:           bootstrap.Token + "|" + bootstrap.Nonce,
		ExpiresAt:             bootstrap.ExpiresAt,
		CertificateTTLSeconds: int(bootstrap.CertificateTTL.Seconds()),
	})
}

// ServeEnroll handles POST /enroll. It consumes the one-use store exactly once
// and returns the signed leaf plus the registry fact (scope + time).
func (h *Handler) ServeEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	scope := ws.Scope{TenantID: req.InstallationID, Station: req.Station}
	if err := scope.Validate(); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	request, err := ws.NewRequest(scope, req.Token, req.Nonce, []byte(req.CSRPEM), time.Minute)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	result, err := h.Enrollment.Enroll(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	writeJSON(w, http.StatusOK, EnrollResponse{
		InstallationID: scope.TenantID,
		Station:        scope.Station,
		Serial:         result.Serial,
		ExpiresAt:      result.ExpiresAt,
		LeafPEM:        string(result.Certificate),
		TrustPEM:       string(result.Trust.PEM),
		At:             time.Now().UTC(),
	})
}

// Router registers the handler on the given mux. Shared register files are not
// touched; the caller owns routing.
func (h *Handler) Router(mux *http.ServeMux) {
	mux.HandleFunc("/bootstrap", h.ServeBootstrap)
	mux.HandleFunc("/enroll", h.ServeEnroll)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
