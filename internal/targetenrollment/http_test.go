package targetenrollment

import (
	"bytes"
	"context"
	"io"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func enrollmentHTTPFixture(t *testing.T) (*HTTPHandler, *memoryStore) {
	t.Helper()
	store := newStore()
	authority, err := New(store, serviceissuer.New(t))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTP(authority, func(_ context.Context, target, node, environment string) (Scope, error) {
		if target != testScope().TargetID || node != testScope().NodeID || environment != "dev" {
			return Scope{}, ErrDenied
		}
		return testScope(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, store
}

func grantHTTPRequest(role string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "https://core.example"+AuthorizationPath,
		strings.NewReader(`{"target_id":"lab","node_id":"node-a","environment":"dev","lifetime_seconds":60,"certificate_ttl_seconds":3600}`))
	expires := time.Now().Add(time.Minute)
	ctx := identity.WithPrincipal(r.Context(), &identity.Principal{Issuer: "https://issuer.example", Subject: "operator-a", ExpiresAt: &expires})
	ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: testScope().TenantID, ExternalIdentityID: "identity-a", Roles: []string{role}})
	return r.WithContext(ctx)
}

func enrollHTTPRequest(t *testing.T, bootstrap Bootstrap) *http.Request {
	t.Helper()
	request := requestFor(t, testScope(), bootstrap)
	data, err := json.Marshal(enrollmentInput{ContractVersion: enrollmentVersion, TenantID: request.Scope.TenantID,
		NodeID: request.Scope.NodeID, TargetID: request.Scope.TargetID, Runtime: request.Scope.Runtime,
		CSRPEM: string(request.CSRPEM), Nonce: request.Nonce})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "https://core.example"+EnrollmentPath, bytes.NewReader(data))
	r.Header.Set("Authorization", "Bearer "+bootstrap.Token)
	return r
}

func TestEnrollmentHTTPGrantRequiresCoreTenantPermission(t *testing.T) {
	for _, role := range []string{"viewer", "unknown", ""} {
		handler, store := enrollmentHTTPFixture(t)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, grantHTTPRequest(role))
		if recorder.Code != http.StatusForbidden || len(store.grants) != 0 {
			t.Fatal("unprivileged tenant membership created a bootstrap grant", role, recorder.Code)
		}
	}
	handler, store := enrollmentHTTPFixture(t)
	request := grantHTTPRequest("editor")
	request = request.WithContext(tenancy.WithContext(request.Context(), &tenancy.Context{
		TenantID: "00000000-0000-0000-0000-000000000002", ExternalIdentityID: "identity-a", Roles: []string{"editor"},
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || len(store.grants) != 0 {
		t.Fatal("Core scope resolver result escaped the authenticated tenant")
	}
}

func TestEnrollmentHTTPCanonicalOneUseExchange(t *testing.T) {
	handler, _ := enrollmentHTTPFixture(t)
	grant := httptest.NewRecorder()
	handler.ServeHTTP(grant, grantHTTPRequest("editor"))
	if grant.Code != http.StatusCreated || grant.Header().Get("Cache-Control") != "no-store" ||
		contracts.ValidateTargetAccessRecord("bootstrap_authorization", grant.Body.Bytes()) != nil {
		t.Fatal("bootstrap credential projection is not canonical or protected", grant.Code)
	}
	var bootstrap Bootstrap
	if err := json.Unmarshal(grant.Body.Bytes(), &bootstrap); err != nil {
		t.Fatal(err)
	}
	request := enrollHTTPRequest(t, bootstrap)
	body, err := ioReadBody(request)
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			r := httptest.NewRequest(http.MethodPost, "https://core.example"+EnrollmentPath, bytes.NewReader(body))
			r.Header.Set("Authorization", "Bearer "+bootstrap.Token)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, r)
			if recorder.Code == http.StatusOK {
				successes.Add(1)
				if contracts.ValidateTargetAccessRecord("enrollment_response", recorder.Body.Bytes()) != nil ||
					strings.Contains(recorder.Body.String(), "PRIVATE KEY") || recorder.Header().Get("Cache-Control") != "no-store" {
					t.Error("enrollment response lost canonical shape or key locality")
				}
			} else if recorder.Code != http.StatusForbidden || strings.Contains(recorder.Body.String(), bootstrap.Token) {
				t.Error("denial exposed a credential or has an unstable status")
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("HTTP enrollment admitted more than one scoped CSR", successes.Load())
	}
}

func TestEnrollmentHTTPRejectsPlaintextOriginAndAmbiguousCredentials(t *testing.T) {
	for _, variant := range []string{"plaintext", "origin", "headers", "duplicate", "foreign-tenant", "unknown-field"} {
		t.Run(variant, func(t *testing.T) {
			handler, store := enrollmentHTTPFixture(t)
			bootstrap, err := handler.authority.Create(context.Background(), testScope(), time.Minute, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			r := enrollHTTPRequest(t, bootstrap)
			switch variant {
			case "plaintext":
				r.TLS = nil
			case "origin":
				r.Header.Set("Origin", "https://foreign.example")
			case "headers":
				r.Header.Add("Authorization", "Bearer "+bootstrap.Token)
			case "duplicate", "unknown-field", "foreign-tenant":
				body, err := ioReadBody(r)
				if err != nil {
					t.Fatal(err)
				}
				if variant == "foreign-tenant" {
					body = bytes.ReplaceAll(body, []byte(testScope().TenantID), []byte("00000000-0000-0000-0000-000000000002"))
				} else if variant == "duplicate" {
					body = append(body[:len(body)-1], []byte(`,"nonce":"`+bootstrap.Nonce+`"}`)...)
				} else {
					body = append(body[:len(body)-1], []byte(`,"credential-secret":"sensitive-value"}`)...)
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, r)
			if recorder.Code < 400 || len(store.consumed) != 0 || strings.Contains(recorder.Body.String(), "sensitive-value") ||
				strings.Contains(recorder.Body.String(), bootstrap.Token) {
				t.Fatal("invalid enrollment admitted, consumed authorization or leaked credentials", recorder.Code)
			}
		})
	}
}

func ioReadBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(r.Body)
}

