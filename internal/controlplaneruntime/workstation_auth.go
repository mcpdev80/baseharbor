package controlplaneruntime

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// workstationRevocationReader resolves serial status using the actual
// installation-owned OpenBao issuer. A missing or unreadable CRL denies access.
type workstationRevocationReader interface {
	WorkstationCertificateRevoked(context.Context, string) (bool, error)
}

// workstationDeviceMiddleware admits only a certificate that was signed by
// the managed Core, is current in its *durable* tenant-scoped workstation
// registry and not revoked by the issuer. An invitation is not authentication:
// the minted leaf is checked on every subsequent request.
//
// It never changes OIDC authentication; it is a parallel, explicit device
// identity for the bounded management/machine API routes only.
func workstationDeviceMiddleware(pool *pgxpool.Pool, caFile string, revocation workstationRevocationReader, next http.Handler) (http.Handler, error) {
	if pool == nil || revocation == nil || next == nil || strings.TrimSpace(caFile) == "" {
		return nil, errors.New("workstation device admission requires Core DB, managed CA, revocation authority and API")
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("workstation CA file is not a usable certificate trust anchor")
	}
	registry := database.NewWorkstationEnrollmentStore(pool)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler is called only for explicit device-certificate requests.
		// It cannot be used to forge the Bearer session of a human operator.
		w.Header().Set("Cache-Control", "no-store")
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "verified workstation certificate required", http.StatusUnauthorized)
			return
		}
		path := r.URL.Path
		if !(path == "/api/v1/management/installation" && r.Method == http.MethodGet) &&
			!(path == "/api/v1/workstations/renew" && r.Method == http.MethodPost) &&
			!strings.HasPrefix(path, "/api/v1/machine/") {
			http.Error(w, "workstation identity cannot access this operator endpoint", http.StatusForbidden)
			return
		}
		leaf := r.TLS.PeerCertificates[0]
		if leaf.IsCA || len(leaf.URIs) != 1 || leaf.URIs[0] == nil {
			http.Error(w, "invalid workstation certificate", http.StatusUnauthorized)
			return
		}
		if _, err := leaf.Verify(x509.VerifyOptions{
			Roots: roots, CurrentTime: time.Now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}); err != nil {
			http.Error(w, "workstation certificate trust rejected", http.StatusUnauthorized)
			return
		}
		uri := leaf.URIs[0]
		const prefix = "spiffe://baseharbor/workstations/"
		if uri.Scheme != "spiffe" || uri.Host != "baseharbor" || !strings.HasPrefix(uri.String(), prefix) ||
			uri.RawQuery != "" || uri.Fragment != "" {
			http.Error(w, "foreign workstation identity", http.StatusUnauthorized)
			return
		}
		parts := strings.Split(strings.TrimPrefix(uri.String(), prefix), "/")
		if len(parts) != 2 {
			http.Error(w, "invalid workstation scope", http.StatusUnauthorized)
			return
		}
		scope := ws.Scope{TenantID: parts[0], Station: parts[1]}
		if scope.Validate() != nil || scope.Identity() != uri.String() {
			http.Error(w, "unowned workstation scope", http.StatusUnauthorized)
			return
		}
		_, recipient, scopeID, registeredSerial, expiry, ok, err := registry.ReadRegistryFact(r.Context(), scope)
		if err != nil || !ok || recipient != scope.Identity() || scopeID != scope.Station ||
			!expiry.After(time.Now()) || !equalHexSerial(registeredSerial, leaf.SerialNumber.Text(16)) {
			http.Error(w, "workstation identity not current in Core registry", http.StatusUnauthorized)
			return
		}
		revoked, err := revocation.WorkstationCertificateRevoked(r.Context(), registeredSerial)
		if err != nil || revoked {
			http.Error(w, "workstation issuer revocation status rejected", http.StatusUnauthorized)
			return
		}
		// A workstation invite delegates developer-level operations, not the
		// initial-owner/admin role. Authorization still runs for each semantic
		// machine operation via the existing Core policy engine.
		now := time.Now()
		exp := leaf.NotAfter
		if !exp.After(now) {
			http.Error(w, "workstation certificate expired", http.StatusUnauthorized)
			return
		}
		principal := &identity.Principal{
			Issuer:    prefix + scope.TenantID,
			Subject:   scope.Station,
			Assurance: "verified-mutual-tls",
			Methods:   []string{"workstation-mtls"},
			ExpiresAt: &exp,
			// This is deliberately not OIDC. The certificate/registry/CRL
			// checks above, not an unverified claim, authorize this device.
			IssuerVerified: false,
		}
		tenant := &tenancy.Context{
			TenantID:           scope.TenantID,
			ExternalIdentityID: "workstation/" + scope.Station,
			Roles:              []string{"editor"},
		}
		ctx := identity.WithPrincipal(r.Context(), principal)
		ctx = tenancy.WithContext(ctx, tenant)
		next.ServeHTTP(w, r.WithContext(ctx))
	}), nil
}

func equalHexSerial(a, b string) bool {
	normalize := func(s string) string {
		return strings.TrimLeft(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), ":", ""), "0")
	}
	return normalize(a) != "" && normalize(a) == normalize(b)
}
