package workstationenrollmenthttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// The invitation is created by the already authorized Core owner, not by an
// unauthenticated workstation. Only its one-use bearer crosses the OIDC edge.
func TestDelegatedEnrollmentUsesExactPreissuedGrant(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	scope := ws.Scope{TenantID: tenantA, Station: validStation}
	grant, err := e.Create(context.Background(), scope, 5*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	invite := enrollBody(t, validStation, tenantA, grant.Token, grant.Nonce)
	invoke := func(body map[string]any, secure bool) *httptest.ResponseRecorder {
		t.Helper()
		var payload bytes.Buffer
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
		origin := "https://127.0.0.1/api/v1/workstations/enroll"
		if !secure {
			origin = "http://127.0.0.1/api/v1/workstations/enroll"
		}
		req := httptest.NewRequest(http.MethodPost, origin, &payload)
		rec := httptest.NewRecorder()
		h.ServeDelegatedEnroll(rec, req)
		return rec
	}
	foreign := enrollBody(t, validStation, tenantB, grant.Token, grant.Nonce)
	if rec := invoke(foreign, true); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign tenant grant accepted: %d", rec.Code)
	}
	if rec := invoke(invite, false); rec.Code != http.StatusForbidden {
		t.Fatalf("plaintext enrollment accepted: %d", rec.Code)
	}
	if rec := invoke(invite, true); rec.Code != http.StatusOK {
		t.Fatalf("pre-authorized grant rejected: %d", rec.Code)
	}
	if rec := invoke(invite, true); rec.Code != http.StatusForbidden {
		t.Fatalf("spent grant was replayed: %d", rec.Code)
	}
	if len(e.registry) != 1 || e.registry[0] != scope {
		t.Fatalf("unexpected workstation registry scope: %+v", e.registry)
	}
}

func TestDelegatedEnrollmentCannotMintGrant(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	r := httptest.NewRequest(http.MethodPost, "https://127.0.0.1/api/v1/workstations/enroll",
		bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()
	h.ServeDelegatedEnroll(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("request without pre-issued grant accepted: %d", rec.Code)
	}
}
