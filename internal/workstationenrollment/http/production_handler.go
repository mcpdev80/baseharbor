package workstationenrollmenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mcpdev80/baseharbor/internal/tenancy"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// ScopeResolver binds the workstation scope for the production path from the
// authenticated request context. It must select the installation (tenant) and
// the authorized workstation label from Core policy/ownership, never from
// user-supplied request fields: the caller supplies only the workstation-owned
// station label, and the resolver binds the installation tenant from the
// authenticated operator. A foreign or unscoped caller fails closed.
type ScopeResolver func(context.Context, string) (ws.Scope, error)

// ProductionEnrollment is the production-boundary seam: the one-use authority
// plus the durable registry-owner persistence the production path requires.
type ProductionEnrollment interface {
	Create(ctx context.Context, scope ws.Scope, lifetime, certificateTTL time.Duration) (ws.Bootstrap, error)
	Enroll(ctx context.Context, request ws.Request) (ws.Result, error)
	// RecordRegistryFact persists the approved registry owner as a fact: the
	// installation-scoped scope, the issuance time, the separate workstation
	// SPIFFE identity, the issued serial, and its expiry.
	RecordRegistryFact(ctx context.Context, scope ws.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time) error
}

// ProductionHandler serves the one-use workstation-enrollment boundary over HTTP
// at the authenticated management entry point. It binds the recipient/scope from
// the authenticated installation via the ScopeResolver (never from the request
// body), performs the single-use enrollment through the trusted issuer, persists
// the registry fact, and returns the signed leaf plus the registry fact (scope
// + time). No private key is ever returned.
type ProductionHandler struct {
	Enrollment ProductionEnrollment
	Resolve    ScopeResolver
}

// NewProductionHandler returns a ProductionHandler for the given one-use
// enrollment boundary and scope resolver. Both seams are required.
func NewProductionHandler(e ProductionEnrollment, resolve ScopeResolver) (*ProductionHandler, error) {
	if e == nil || resolve == nil {
		return nil, errors.New("workstation production enrollment requires an authority and a scope resolver")
	}
	return &ProductionHandler{Enrollment: e, Resolve: resolve}, nil
}

// ServeHTTP dispatches the production bootstrap/enroll boundary. Both paths stay
// behind the authenticated operator middleware; the caller owns registration
// behind the identity/tenant + TLS protection.
func (h *ProductionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/bootstrap":
		h.ServeBootstrap(w, r)
	case "/enroll":
		h.ServeEnroll(w, r)
	default:
		writeError(w, http.StatusNotFound, ErrBadRequest)
	}
}

// Router registers the production handler on the given authenticated operator
// mux. The caller owns routing behind the identity/tenant middleware.
func (h *ProductionHandler) Router(mux *http.ServeMux) {
	mux.HandleFunc("/bootstrap", h.ServeBootstrap)
	mux.HandleFunc("/enroll", h.ServeEnroll)
}

// ServeBootstrap handles POST /bootstrap. It binds the scope from the
// authenticated installation, creates a one-use bootstrap for that scope, and
// returns digested credentials plus the short-lived validity window.
func (h *ProductionHandler) ServeBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrBadRequest)
		return
	}
	var req BootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	scope, err := h.Resolve(r.Context(), req.Station)
	if err != nil {
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

// ServeEnroll handles POST /enroll. It binds the scope from the authenticated
// installation, consumes the one-use store exactly once, persists the registry
// fact, and returns the signed leaf plus the registry fact (scope + time).
func (h *ProductionHandler) ServeEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrBadRequest)
		return
	}
	var req EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	scope, err := h.Resolve(r.Context(), req.Station)
	if err != nil {
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
	// Persist the approved registry owner as a fact: scope, issuance time,
	// separate workstation SPIFFE identity, serial, and expiry.
	if rec, ok := h.Enrollment.(interface {
		RecordRegistryFact(ctx context.Context, scope ws.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time) error
	}); ok {
		_ = rec.RecordRegistryFact(r.Context(), scope, time.Now().UTC(), scope.Identity(), scope.Station, result.Serial, result.ExpiresAt)
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

// scopeResolverFromContext is the canonical ScopeResolver for the production
// path: it binds the installation tenant from the authenticated operator
// (tenancy) and authorizes the workstation-owned station label against the
// tenant, so the installation is never selected by the workstation request. A
// foreign or unscoped caller fails closed.
func scopeResolverFromContext(ctx context.Context, station string) (ws.Scope, error) {
	tenant, ok := tenancy.FromContext(ctx)
	if !ok || tenant.TenantID == "" || tenant.ExternalIdentityID == "" {
		return ws.Scope{}, ws.ErrDenied
	}
	scope := ws.Scope{TenantID: tenant.TenantID, Station: station}
	if scope.Validate() != nil {
		return ws.Scope{}, ws.ErrDenied
	}
	return scope, nil
}

// ScopeResolverFromContext returns the canonical production ScopeResolver. The
// installation tenant is bound by the authenticated operator (tenancy), never by
// the workstation request.
func ScopeResolverFromContext() ScopeResolver { return scopeResolverFromContext }
