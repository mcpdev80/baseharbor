package targetenrollment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/authorization"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

const (
	AuthorizationPath = "/api/v1/connectors/authorizations"
	EnrollmentPath    = "/api/v1/connectors/enroll"
	enrollmentVersion = "baseharbor.target-access-enrollment/v1"
)

// ScopeResolver must use Core's Target registry, tenant ownership and effective
// policy. User-supplied runtime and tenant fields are deliberately not accepted.
type ScopeResolver func(context.Context, string, string, string) (Scope, error)

type HTTPHandler struct {
	authority *Authority
	resolve   ScopeResolver
}

func NewHTTP(authority *Authority, resolve ScopeResolver) (*HTTPHandler, error) {
	if authority == nil || resolve == nil {
		return nil, errors.New("enrollment HTTP requires a persistent authority and Core scope resolver")
	}
	return &HTTPHandler{authority: authority, resolve: resolve}, nil
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.TLS == nil {
		enrollmentHTTPError(w, http.StatusUpgradeRequired, machine.ErrorAuthenticationFailed)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host != r.Host || u.User != nil ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
			enrollmentHTTPError(w, http.StatusForbidden, machine.ErrorPolicyDenied)
			return
		}
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		enrollmentHTTPError(w, http.StatusMethodNotAllowed, machine.ErrorUnsupported)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		_ = http.NewResponseController(w).SetReadDeadline(deadline)
		_ = http.NewResponseController(w).SetWriteDeadline(deadline)
	}
	switch r.URL.Path {
	case AuthorizationPath:
		h.authorize(w, r)
	case EnrollmentPath:
		h.enroll(w, r)
	default:
		enrollmentHTTPError(w, http.StatusNotFound, machine.ErrorNotFound)
	}
}

type authorizationInput struct {
	TargetID              string `json:"target_id"`
	NodeID                string `json:"node_id"`
	Environment           string `json:"environment"`
	LifetimeSeconds       int64  `json:"lifetime_seconds"`
	CertificateTTLSeconds int64  `json:"certificate_ttl_seconds"`
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request) {
	principal, authenticated := identity.FromContext(r.Context())
	tenant, scoped := tenancy.FromContext(r.Context())
	if !authenticated || (principal.ExpiresAt != nil && !principal.ExpiresAt.After(time.Now())) {
		enrollmentHTTPError(w, http.StatusUnauthorized, machine.ErrorAuthenticationFailed)
		return
	}
	if !scoped || tenant.TenantID == "" || tenant.ExternalIdentityID == "" ||
		!authorization.NewService().Allowed(tenant.Roles, authorization.PermCreate) {
		enrollmentHTTPError(w, http.StatusForbidden, machine.ErrorPolicyDenied)
		return
	}
	if principal.ExpiresAt != nil {
		ctx, cancel := context.WithDeadline(r.Context(), *principal.ExpiresAt)
		defer cancel()
		r = r.WithContext(ctx)
	}
	data, err := enrollmentBody(r.Body, 8192)
	if err != nil {
		enrollmentHTTPError(w, http.StatusBadRequest, machine.ErrorValidationFailed)
		return
	}
	var input authorizationInput
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.TargetID == "" || input.NodeID == "" || input.Environment == "" ||
		input.LifetimeSeconds < 1 || input.LifetimeSeconds > 600 || input.CertificateTTLSeconds < 1 || input.CertificateTTLSeconds > 86400 {
		enrollmentHTTPError(w, http.StatusBadRequest, machine.ErrorValidationFailed)
		return
	}
	scope, err := h.resolve(r.Context(), input.TargetID, input.NodeID, input.Environment)
	if err != nil || scope.Validate() != nil || scope.TenantID != tenant.TenantID ||
		scope.TargetID != input.TargetID || scope.NodeID != input.NodeID {
		enrollmentHTTPError(w, http.StatusForbidden, machine.ErrorPolicyDenied)
		return
	}
	bootstrap, err := h.authority.Create(r.Context(), scope, time.Duration(input.LifetimeSeconds)*time.Second, time.Duration(input.CertificateTTLSeconds)*time.Second)
	if err != nil {
		enrollmentHTTPError(w, http.StatusConflict, machine.ErrorConflict)
		return
	}
	enrollmentHTTPJSON(w, http.StatusCreated, bootstrap)
}

type enrollmentInput struct {
	ContractVersion string `json:"contract_version"`
	TenantID        string `json:"tenant_id"`
	NodeID          string `json:"node_id"`
	TargetID        string `json:"target_id"`
	Runtime         string `json:"runtime"`
	CSRPEM          string `json:"csr_pem"`
	Nonce           string `json:"nonce"`
}

func (h *HTTPHandler) enroll(w http.ResponseWriter, r *http.Request) {
	headers := r.Header.Values("Authorization")
	if len(headers) != 1 {
		enrollmentHTTPError(w, http.StatusUnauthorized, machine.ErrorAuthenticationFailed)
		return
	}
	parts := strings.Split(headers[0], " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !validCredential(parts[1]) {
		enrollmentHTTPError(w, http.StatusUnauthorized, machine.ErrorAuthenticationFailed)
		return
	}
	data, err := enrollmentBody(r.Body, 128<<10)
	if err != nil || contracts.ValidateTargetAccessRecord("enrollment_request", data) != nil {
		enrollmentHTTPError(w, http.StatusBadRequest, machine.ErrorValidationFailed)
		return
	}
	var input enrollmentInput
	if json.Unmarshal(data, &input) != nil {
		enrollmentHTTPError(w, http.StatusBadRequest, machine.ErrorValidationFailed)
		return
	}
	scope := Scope{TenantID: input.TenantID, TargetID: input.TargetID, NodeID: input.NodeID, Runtime: input.Runtime}
	result, err := h.authority.Enroll(r.Context(), Request{Scope: scope, Token: parts[1], Nonce: input.Nonce, CSRPEM: []byte(input.CSRPEM)})
	if err != nil {
		enrollmentHTTPError(w, http.StatusForbidden, machine.ErrorAuthenticationFailed)
		return
	}
	response := struct {
		ContractVersion string            `json:"contract_version"`
		Node            map[string]string `json:"node"`
		CertificatePEM  string            `json:"certificate_pem"`
		TrustBundlePEM  string            `json:"trust_bundle_pem"`
		Nonce           string            `json:"nonce"`
		NotAfter        time.Time         `json:"not_after"`
	}{
		ContractVersion: enrollmentVersion,
		Node:            map[string]string{"tenant_id": scope.TenantID, "node_id": scope.NodeID, "target_id": scope.TargetID, "runtime": scope.Runtime, "identity": scope.Identity()},
		CertificatePEM:  string(result.Certificate.Certificate),
		TrustBundlePEM:  string(result.Trust.PEM), Nonce: input.Nonce, NotAfter: result.Certificate.ExpiresAt.UTC(),
	}
	encoded, err := json.Marshal(response)
	if err != nil || contracts.ValidateTargetAccessRecord("enrollment_response", encoded) != nil {
		enrollmentHTTPError(w, http.StatusInternalServerError, machine.ErrorInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func enrollmentBody(body io.Reader, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil || machine.ValidateJSONObject(data, limit) != nil {
		return nil, machine.ErrJSONObject
	}
	return data, nil
}

func enrollmentHTTPError(w http.ResponseWriter, status int, code machine.ErrorCode) {
	enrollmentHTTPJSON(w, status, machine.ResultError(machine.NewError(code,
		"Connector enrollment request was not admitted.", "Review Core authorization, scope and credential expiry.", false)))
}

func enrollmentHTTPJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
