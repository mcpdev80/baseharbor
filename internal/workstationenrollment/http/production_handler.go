package workstationenrollmenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	workstationlifecycle "github.com/mcpdev80/baseharbor/contracts/workstationlifecycle/v1"
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
	// Renew re-signs exactly one scope's workstation key for a fresh leaf. The
	// production path persists the fresh serial (RecordRegistryFact) so the
	// durable registry's current-serial owner follows the renewal; a caller that
	// never supplied a durable registry cannot renew and reports it unavailable.
	Renew(ctx context.Context, request ws.Request) (ws.Result, error)
	// Revoke adds the scope's currently issued serial to the issuer CRL.
	Revoke(ctx context.Context, scope ws.Scope) error
	// RevokedState reports whether the scope's currently issued leaf is revoked;
	// an unreadable status is reported as an error, never a safe "not revoked".
	RevokedState(ctx context.Context, scope ws.Scope) (bool, error)
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

// ServeHTTP dispatches the production bootstrap/enroll/lifecycle boundary. All
// paths stay behind the authenticated operator middleware; the caller owns
// registration behind the identity/tenant + TLS protection.
//
// Go's ServeMux does not strip the matched prefix from r.URL.Path, so a handler
// mounted at an absolute path (e.g. /api/v1/workstations/bootstrap) still has to
// recognise its own suffix. Dispatch by suffix so both the exact mounted request
// and the bare endpoint path reach their method; anything else fails closed.
func (h *ProductionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/bootstrap" || strings.HasSuffix(r.URL.Path, "/bootstrap"):
		h.ServeBootstrap(w, r)
	case r.URL.Path == "/enroll" || strings.HasSuffix(r.URL.Path, "/enroll"):
		h.ServeEnroll(w, r)
	case r.URL.Path == "/renew" || strings.HasSuffix(r.URL.Path, "/renew"):
		h.ServeRenew(w, r)
	case r.URL.Path == "/revoke" || strings.HasSuffix(r.URL.Path, "/revoke"):
		h.ServeRevoke(w, r)
	default:
		writeError(w, http.StatusNotFound, ErrBadRequest)
	}
}

// Router registers the production handler on the given authenticated operator
// mux. The caller owns routing behind the identity/tenant middleware.
func (h *ProductionHandler) Router(mux *http.ServeMux) {
	mux.HandleFunc("/bootstrap", h.ServeBootstrap)
	mux.HandleFunc("/enroll", h.ServeEnroll)
	mux.HandleFunc("/renew", h.ServeRenew)
	mux.HandleFunc("/revoke", h.ServeRevoke)
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
	// separate workstation SPIFFE identity, serial, and expiry. Persistence is
	// mandatory and fail-closed: if the registry fact cannot be written the
	// enrollment is rolled back with an error rather than returning a success
	// response for a fact that was never recorded.
	if rec, ok := h.Enrollment.(interface {
		RecordRegistryFact(ctx context.Context, scope ws.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time) error
	}); ok {
		if err := rec.RecordRegistryFact(r.Context(), scope, time.Now().UTC(), scope.Identity(), scope.Station, result.Serial, result.ExpiresAt); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		writeError(w, http.StatusInternalServerError, errors.New("production enrollment is missing the registry-fakt owner"))
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

// RenewRequest is the workstation-owned half of a device-lifecycle renewal. The
// workstation supplies its fresh CSR and the desired fresh-leaf TTL; the
// installation/scope come from the authenticated operator, never the request.
type RenewRequest struct {
	Station               string `json:"station"`
	CSRPEM                string `json:"csr_pem"`
	CertificateTTLSeconds int    `json:"certificate_ttl_seconds"`
}

// RevokeRequest is the workstation-owned half of a device-lifecycle revocation.
// The workstation supplies only its station label; the installation/scope come
// from the authenticated operator, never the request.
type RevokeRequest struct {
	Station string `json:"station"`
}

// ServeRenew handles POST /renew. It binds the scope from the authenticated
// operator, validates the record through the portable device-lifecycle contract,
// re-signs exactly this scope's key for a fresh leaf through the protected
// signing boundary, and persists the fresh serial to the durable registry so the
// registry's current-serial owner follows the renewal. A foreign scope, a
// revoked current identity, an unreadable revocation status or a malformed CSR
// all fail closed; a durable registry that cannot write the fresh serial rolls
// the renewal back with an error rather than reporting a success never recorded.
func (h *ProductionHandler) ServeRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrBadRequest)
		return
	}
	var req RenewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	scope, err := h.Resolve(r.Context(), req.Station)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	certificateTTL := time.Duration(req.CertificateTTLSeconds) * time.Second
	// Build the portable, recipient-bound lifecycle record from the authenticated
	// installation and the workstation-owned station, then delegate the renewal
	// to the contract. The recipient binds the record to the operator's SPIFFE
	// identity for the scope, so a foreign operator can never renew another
	// scope. This is the production entry point: the contract is imported and
	// exercised here, not only from internal-API tests.
	recipient := scope.Identity()
	record := workstationlifecycle.Record{
		Kind:           workstationlifecycle.RecordKindWorkstationLifecycle,
		Version:        workstationlifecycle.Version,
		InstallationID: scope.TenantID,
		Recipient:      recipient,
		Scope:          scope.Station,
	}
	life := &authorityLifecycle{h.Enrollment}
	result, err := workstationlifecycle.RenewDevice(r.Context(), life, record, []byte(req.CSRPEM), certificateTTL)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	// Persist the fresh serial to the durable registry so the current-serial owner
	// follows the renewal: a later revoke or re-renew reads the new serial, not
	// the enrolled one. Persistence is mandatory and fail-closed.
	if rec, ok := h.Enrollment.(interface {
		RecordRegistryFact(ctx context.Context, scope ws.Scope, issuedAt time.Time, recipient, scopeID, serial string, expiresAt time.Time) error
	}); ok {
		if err := rec.RecordRegistryFact(r.Context(), scope, time.Now().UTC(), recipient, scope.Identity(), result.Serial, result.ExpiresAt); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		writeError(w, http.StatusInternalServerError, errors.New("production lifecycle is missing the registry-fakt owner"))
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

// ServeDeviceRenew is reachable only behind the Core's verified mTLS device
// middleware. A device cannot renew another station's certificate or become an
// operator: its tenant and station must exactly match the registered identity.
func (h *ProductionHandler) ServeDeviceRenew(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrBadRequest)
		return
	}
	tenant, ok := tenancy.FromContext(r.Context())
	if !ok || tenant.TenantID == "" || !strings.HasPrefix(tenant.ExternalIdentityID, "workstation/") ||
		r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || h == nil || h.Enrollment == nil {
		writeError(w, http.StatusForbidden, ws.ErrDenied)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req RenewRequest
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	station := strings.TrimPrefix(tenant.ExternalIdentityID, "workstation/")
	scope := ws.Scope{TenantID: tenant.TenantID, Station: station}
	if req.Station != station || scope.Validate() != nil ||
		len(r.TLS.PeerCertificates[0].URIs) != 1 ||
		r.TLS.PeerCertificates[0].URIs[0].String() != scope.Identity() {
		writeError(w, http.StatusForbidden, ws.ErrDenied)
		return
	}
	request, err := ws.NewRenewRequest(scope, []byte(req.CSRPEM),
		time.Duration(req.CertificateTTLSeconds)*time.Second)
	if err != nil {
		writeError(w, http.StatusForbidden, ws.ErrDenied)
		return
	}
	result, err := h.Enrollment.Renew(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusForbidden, ws.ErrDenied)
		return
	}
	if err := h.Enrollment.RecordRegistryFact(r.Context(), scope, time.Now().UTC(),
		scope.Identity(), scope.Station, result.Serial, result.ExpiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("workstation renewal registry persistence failed"))
		return
	}
	writeJSON(w, http.StatusOK, EnrollResponse{
		InstallationID: scope.TenantID, Station: scope.Station,
		Serial: result.Serial, ExpiresAt: result.ExpiresAt,
		LeafPEM: string(result.Certificate), TrustPEM: string(result.Trust.PEM),
		At: time.Now().UTC(),
	})
}

// ServeRevoke handles POST /revoke. It binds the scope from the authenticated
// operator and revokes exactly this scope's currently issued leaf through the
// portable device-lifecycle contract. A foreign scope or a scope with no issued
// identity fails closed without touching any other identity.
func (h *ProductionHandler) ServeRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, ErrBadRequest)
		return
	}
	var req RevokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrBadRequest)
		return
	}
	scope, err := h.Resolve(r.Context(), req.Station)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	recipient := scope.Identity()
	record := workstationlifecycle.Record{
		Kind:           workstationlifecycle.RecordKindWorkstationLifecycle,
		Version:        workstationlifecycle.Version,
		InstallationID: scope.TenantID,
		Recipient:      recipient,
		Scope:          scope.Station,
	}
	if err := workstationlifecycle.RevokeDevice(r.Context(), &authorityLifecycle{h.Enrollment}, record); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"installation_id": scope.TenantID,
		"station":         scope.Station,
		"revoked":         true,
		"at":              time.Now().UTC(),
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

// authorityLifecycle adapts the production-enrollment seam to the portable
// device-lifecycle contract. The contract calls Renew/Revoke/RevokedState; the
// production seam already provides all three (Renew/Revoke through the authority,
// RecordRegistryFact to persist the renewed serial, RevokedState to read the
// current leaf's revocation flag). A nil seam fails closed so the boundary never
// silently accepts a lifecycle effect.
type authorityLifecycle struct {
	ProductionEnrollment
}

// Renew re-signs exactly this scope's key through the protected signing boundary.
func (l *authorityLifecycle) Renew(ctx context.Context, request ws.Request) (ws.Result, error) {
	if l.ProductionEnrollment == nil {
		return ws.Result{}, workstationlifecycle.ErrInvalidRecord
	}
	return l.ProductionEnrollment.Renew(ctx, request)
}

// Revoke adds this scope's currently issued serial to the issuer CRL.
func (l *authorityLifecycle) Revoke(ctx context.Context, scope ws.Scope) error {
	if l.ProductionEnrollment == nil {
		return workstationlifecycle.ErrInvalidRecord
	}
	return l.ProductionEnrollment.Revoke(ctx, scope)
}

// RevokedState reports whether this scope's currently issued leaf is revoked.
func (l *authorityLifecycle) RevokedState(ctx context.Context, scope ws.Scope) (bool, error) {
	if l.ProductionEnrollment == nil {
		return false, workstationlifecycle.ErrInvalidRecord
	}
	return l.ProductionEnrollment.(interface {
		RevokedState(ctx context.Context, scope ws.Scope) (bool, error)
	}).RevokedState(ctx, scope)
}
