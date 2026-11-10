package workstationenrollmenthttp_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func deviceRenewRequest(t *testing.T, station, certificateStation, contextStation string) *http.Request {
	t.Helper()
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(map[string]any{
		"station": station, "csr_pem": "not-a-valid-csr", "certificate_ttl_seconds": 3600,
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workstations/renew", &payload)
	uri, err := url.Parse("spiffe://baseharbor/workstations/" + tenantA + "/" + certificateStation)
	if err != nil {
		t.Fatal(err)
	}
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	ctx := tenancy.WithContext(req.Context(), &tenancy.Context{
		TenantID: tenantA, ExternalIdentityID: "workstation/" + contextStation,
		Roles: []string{"editor"},
	})
	return req.WithContext(ctx)
}

func TestDeviceRenewRefusesForeignStationAndCertificate(t *testing.T) {
	e := newMockEnrollment(t)
	h := handler(t, e)
	for _, tc := range []struct {
		name, station, certStation, contextStation string
	}{
		{"foreign requested station", "station-b", validStation, validStation},
		{"foreign certificate station", validStation, "station-b", validStation},
		{"foreign context station", validStation, validStation, "station-b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeDeviceRenew(rec, deviceRenewRequest(t, tc.station, tc.certStation, tc.contextStation))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("foreign workstation renewal admitted: HTTP %d", rec.Code)
			}
			if len(e.registry) != 0 {
				t.Fatal("foreign workstation renewal persisted registry state")
			}
		})
	}
}

func TestDeviceRenewRefusesOperatorContextAndMissingMTLS(t *testing.T) {
	h := handler(t, newMockEnrollment(t))
	req := deviceRenewRequest(t, validStation, validStation, validStation)
	req = req.WithContext(ctx(t, tenantA))
	rec := httptest.NewRecorder()
	h.ServeDeviceRenew(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator context was accepted as a workstation: HTTP %d", rec.Code)
	}
	req = deviceRenewRequest(t, validStation, validStation, validStation)
	req.TLS = nil
	rec = httptest.NewRecorder()
	h.ServeDeviceRenew(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing mTLS was accepted: HTTP %d", rec.Code)
	}
}
