package httpsecurity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

type verifierFunc func(context.Context, string) (*identity.Principal, error)

func (f verifierFunc) Verify(ctx context.Context, token string) (*identity.Principal, error) {
	return f(ctx, token)
}

type tenantResolverFunc func(context.Context, *identity.Principal) (*tenancy.Context, error)

func (f tenantResolverFunc) ResolveTenant(ctx context.Context, principal *identity.Principal) (*tenancy.Context, error) {
	return f(ctx, principal)
}

func TestMiddlewareFailsClosedAndPropagatesVerifiedContext(t *testing.T) {
	const token = "super-sensitive-bearer-token"
	principal := &identity.Principal{Issuer: "https://issuer.example", Subject: "subject-1"}
	tenant := &tenancy.Context{
		TenantID:           "11111111-1111-4111-8111-111111111111",
		ExternalIdentityID: "33333333-3333-4333-8333-333333333333",
		Roles:              []string{"editor"},
	}

	tests := []struct {
		name       string
		header     string
		verify     verifierFunc
		resolve    tenantResolverFunc
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "missing bearer",
			verify:     func(context.Context, string) (*identity.Principal, error) { return principal, nil },
			resolve:    func(context.Context, *identity.Principal) (*tenancy.Context, error) { return tenant, nil },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid token",
			header:     "Bearer " + token,
			verify:     func(context.Context, string) (*identity.Principal, error) { return nil, errors.New("invalid") },
			resolve:    func(context.Context, *identity.Principal) (*tenancy.Context, error) { return tenant, nil },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing principal fields",
			header:     "Bearer " + token,
			verify:     func(context.Context, string) (*identity.Principal, error) { return &identity.Principal{}, nil },
			resolve:    func(context.Context, *identity.Principal) (*tenancy.Context, error) { return tenant, nil },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "ambiguous tenant",
			header: "Bearer " + token,
			verify: func(context.Context, string) (*identity.Principal, error) { return principal, nil },
			resolve: func(context.Context, *identity.Principal) (*tenancy.Context, error) {
				return nil, tenancy.ErrAmbiguousTenant
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:   "invalid resolved tenant",
			header: "Bearer " + token,
			verify: func(context.Context, string) (*identity.Principal, error) { return principal, nil },
			resolve: func(context.Context, *identity.Principal) (*tenancy.Context, error) {
				return &tenancy.Context{TenantID: tenant.TenantID}, nil
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "verified request",
			header:     "Bearer " + token,
			verify:     func(_ context.Context, got string) (*identity.Principal, error) {
				if got != token {
					t.Fatalf("verifier token = %q, want supplied bearer token", got)
				}
				return principal, nil
			},
			resolve:    func(context.Context, *identity.Principal) (*tenancy.Context, error) { return tenant, nil },
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware, err := New(tt.verify, tt.resolve)
			if err != nil {
				t.Fatal(err)
			}
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				gotPrincipal, ok := identity.FromContext(r.Context())
				if !ok || gotPrincipal.Subject != principal.Subject {
					t.Fatal("verified principal missing from request context")
				}
				gotTenant, ok := tenancy.FromContext(r.Context())
				if !ok || gotTenant.TenantID != tenant.TenantID {
					t.Fatal("resolved tenant missing from request context")
				}
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/demo/secrets", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			recorder := httptest.NewRecorder()
			middleware.Protect(next).ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if called != tt.wantNext {
				t.Fatalf("next called = %v, want %v", called, tt.wantNext)
			}
			if strings.Contains(recorder.Body.String(), token) {
				t.Fatal("bearer token leaked into HTTP response")
			}
		})
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	validVerifier := verifierFunc(func(context.Context, string) (*identity.Principal, error) { return nil, nil })
	validResolver := tenantResolverFunc(func(context.Context, *identity.Principal) (*tenancy.Context, error) { return nil, nil })

	if _, err := New(nil, validResolver); err == nil {
		t.Fatal("missing verifier was accepted")
	}
	if _, err := New(validVerifier, nil); err == nil {
		t.Fatal("missing tenant resolver was accepted")
	}
}
