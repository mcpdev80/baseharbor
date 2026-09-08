package httpsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mcpdev80/baseharbor/internal/apierror"
	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// TenantResolver converts one verified principal into exactly one tenant scope.
// Implementations must fail closed on missing or ambiguous memberships.
type TenantResolver interface {
	ResolveTenant(context.Context, *identity.Principal) (*tenancy.Context, error)
}

type Middleware struct {
	verifier auth.Verifier
	tenants  TenantResolver
}

func New(verifier auth.Verifier, tenants TenantResolver) (*Middleware, error) {
	if verifier == nil || tenants == nil {
		return nil, errors.New("HTTP security dependencies are required")
	}
	return &Middleware{verifier: verifier, tenants: tenants}, nil
}

// Protect authenticates the bearer token, resolves one tenant scope and only
// then calls the protected handler with both principal and tenancy context set.
func (m *Middleware) Protect(next http.Handler) http.Handler {
	if next == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, apierror.New(apierror.CodeInternal, "", 0))
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken, err := auth.BearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, apierror.New(apierror.CodeUnauthorized, "", 0))
			return
		}

		principal, err := m.verifier.Verify(r.Context(), rawToken)
		if err != nil || principal == nil || principal.Issuer == "" || principal.Subject == "" {
			writeError(w, apierror.New(apierror.CodeUnauthorized, "", 0))
			return
		}

		tenant, err := m.tenants.ResolveTenant(r.Context(), principal)
		if err != nil || tenant == nil || tenant.TenantID == "" || tenant.ExternalIdentityID == "" || len(tenant.Roles) == 0 {
			writeError(w, apierror.New(apierror.CodeForbidden, "", 0))
			return
		}

		ctx := identity.WithPrincipal(r.Context(), principal)
		ctx = tenancy.WithContext(ctx, tenant)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeError(w http.ResponseWriter, err *apierror.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status)
	_ = json.NewEncoder(w).Encode(err)
}
