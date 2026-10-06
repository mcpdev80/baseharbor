package openbao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/httpsecurity"
)

type nativeConnectorOIDC struct {
	issuer   string
	client   *http.Client
	security *httpsecurity.Middleware
}

// The native fixture obtains real Keycloak access tokens. Password grant is
// confined to these disposable test users; this is not a product login flow.
func newNativeConnectorOIDC(t *testing.T, ctx context.Context, fixture *nativeConnectorFixture, certificate tls.Certificate, roots *x509.CertPool) *nativeConnectorOIDC {
	t.Helper()
	image := os.Getenv("BASEHARBOR_CONNECTOR_IDENTITY_IMAGE")
	if !strings.HasPrefix(image, "quay.io/keycloak/keycloak@sha256:") {
		t.Fatal("immutable reference Keycloak image required")
	}
	upstream := &httputil.ReverseProxy{}
	issuerServer := httptest.NewUnstartedServer(upstream)
	issuerServer.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}
	issuerServer.StartTLS()
	t.Cleanup(issuerServer.Close)
	_, port, err := net.SplitHostPort(issuerServer.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	origin := "https://core.test:" + port
	issuer := origin + "/realms/native-connector"
	realmRoot := filepath.Join(fixture.dir, "realm")
	if err := os.Mkdir(realmRoot, 0755); err != nil {
		t.Fatal(err)
	}
	clients := []map[string]any{}
	for _, client := range []string{"native-core", "foreign-client", "expired-client"} {
		audience := "native-core"
		if client == "foreign-client" {
			audience = "foreign-audience"
		}
		attributes := map[string]string{}
		if client == "expired-client" {
			attributes["access.token.lifespan"] = "1"
		}
		clients = append(clients, map[string]any{"clientId": client, "enabled": true, "publicClient": true, "directAccessGrantsEnabled": true, "attributes": attributes,
			"protocolMappers": []map[string]any{{"name": "Core audience", "protocol": "openid-connect", "protocolMapper": "oidc-audience-mapper", "config": map[string]string{"included.client.audience": audience, "access.token.claim": "true", "id.token.claim": "false"}}}})
	}
	users := []map[string]any{}
	for i, user := range []string{"editor", "viewer"} {
		users = append(users, map[string]any{"id": fmt.Sprintf("66666666-6666-4666-8666-%012d", i+1), "username": user, "enabled": true, "emailVerified": true, "firstName": "Native", "lastName": user, "email": user + "@fixture.invalid", "credentials": []map[string]any{{"type": "password", "value": "isolated-native-operator-only", "temporary": false}}})
	}
	data, err := json.Marshal(map[string]any{"realm": "native-connector", "enabled": true, "sslRequired": "none", "accessTokenLifespan": 60, "clients": clients, "users": users})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realmRoot, "realm.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		data, err := exec.CommandContext(ctx, fixture.engine, args...).CombinedOutput()
		if err != nil {
			t.Fatal("native Identity fixture command failed", err)
		}
		return strings.TrimSpace(string(data))
	}
	id := run("run", "--detach", "--name", fixture.name+"-identity", "--label", "baseharbor.enrollment-qualification=true", "--user", "1000:1000", "--publish", "127.0.0.1::8080", "--volume", realmRoot+":/opt/keycloak/data/import:ro", image, "start-dev", "--hostname", origin, "--proxy-headers", "xforwarded", "--import-realm")
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := exec.CommandContext(cleanup, fixture.engine, "rm", "-f", id).Run(); err != nil {
			t.Error("native Identity fixture cleanup failed", err)
		}
	})
	var ports []struct {
		HostPort string `json:"HostPort"`
	}
	if err := json.Unmarshal([]byte(run("inspect", "--format", `{{json (index .NetworkSettings.Ports "8080/tcp")}}`, id)), &ports); err != nil || len(ports) != 1 {
		t.Fatal("native Identity port mapping invalid", err)
	}
	target, err := url.Parse("http://127.0.0.1:" + ports[0].HostPort)
	if err != nil {
		t.Fatal(err)
	}
	upstream.Director = func(r *http.Request) {
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.Host = "core.test:" + port
		r.Header.Set("X-Forwarded-Host", r.Host)
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-Port", port)
	}
	upstream.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "Identity starting", http.StatusServiceUnavailable)
	}
	client := nativeConnectorHTTPSClient(roots)
	t.Cleanup(client.CloseIdleConnections)
	deadline := time.Now().Add(90 * time.Second)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := client.Do(request)
		ready := false
		if err == nil {
			ready = reply.StatusCode == http.StatusOK
			_ = reply.Body.Close()
		}
		if ready {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			t.Fatal("native Keycloak discovery did not converge")
		}
		time.Sleep(100 * time.Millisecond)
	}
	verifier, err := auth.NewOIDCVerifier(oidc.ClientContext(ctx, client), auth.Config{Issuer: issuer, Audiences: []string{"native-core"}})
	if err != nil {
		t.Fatal("native OIDC discovery failed", err)
	}
	pool, err := pgxpool.New(ctx, os.Getenv("BASEHARBOR_TEST_OPENBAO_STORAGE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for i, role := range []string{"editor", "viewer"} {
		identityID := fmt.Sprintf("77777777-7777-4777-8777-%012d", i+1)
		subject := fmt.Sprintf("66666666-6666-4666-8666-%012d", i+1)
		if _, err := pool.Exec(ctx, "INSERT INTO external_identities(id,issuer,subject) VALUES($1,$2,$3)", identityID, issuer, subject); err != nil {
			t.Fatal("native operator identity seed failed", err)
		}
		if err := database.WithTenantTx(ctx, pool, "11111111-1111-4111-8111-111111111111", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "INSERT INTO memberships(id,tenant_id,external_identity_id,role) VALUES($1,'11111111-1111-4111-8111-111111111111',$2,$3)", fmt.Sprintf("88888888-8888-4888-8888-%012d", i+1), identityID, role)
			return err
		}); err != nil {
			t.Fatal("native operator membership seed failed", err)
		}
	}
	security, err := httpsecurity.New(verifier, database.NewIdentityTenantResolver(pool))
	if err != nil {
		t.Fatal(err)
	}
	return &nativeConnectorOIDC{issuer: issuer, client: client, security: security}
}

func (o *nativeConnectorOIDC) qualifyGrantDenials(t *testing.T, ctx context.Context, client *http.Client, endpoint string) {
	t.Helper()
	for _, check := range []struct {
		user, client, target string
		status               int
	}{
		{"", "", "native-pki", http.StatusUnauthorized},
		{"viewer", "native-core", "native-pki", http.StatusForbidden},
		{"editor", "foreign-client", "native-pki", http.StatusUnauthorized},
		{"editor", "native-core", "foreign-target", http.StatusForbidden},
		{"editor", "expired-client", "native-pki", http.StatusUnauthorized},
	} {
		token := ""
		if check.user != "" {
			token = o.token(t, ctx, check.user, check.client)
		}
		if check.client == "expired-client" {
			parts := strings.Split(token, ".")
			if len(parts) != 3 {
				t.Fatal("real token shape invalid")
			}
			data, err := base64.RawURLEncoding.DecodeString(parts[1])
			if err != nil {
				t.Fatal("real token claims invalid")
			}
			var claims struct {
				Exp int64 `json:"exp"`
			}
			if json.Unmarshal(data, &claims) != nil || claims.Exp == 0 {
				t.Fatal("real token expiration missing")
			}
			wait := time.Until(time.Unix(claims.Exp, 0).Add(100 * time.Millisecond))
			if wait > 3*time.Second {
				t.Fatal("isolated expiry client lifetime differs")
			}
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					t.Fatal("native expiry observation cancelled")
				}
			}
		}
		status, body := nativeOperatorGrantRequest(t, ctx, client, endpoint, token, check.target)
		if status != check.status || (token != "" && bytes.Contains(body, []byte(token))) {
			t.Fatal("protected enrollment denial differs or leaked bearer", status, check.status)
		}
	}
}

func nativeConnectorHTTPSClient(roots *x509.CertPool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "core.test"}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if host == "core.test" {
			address = net.JoinHostPort("127.0.0.1", port)
		}
		return dialer.DialContext(ctx, network, address)
	}}}
}

func (o *nativeConnectorOIDC) token(t *testing.T, ctx context.Context, user, clientID string) string {
	t.Helper()
	form := url.Values{"grant_type": {"password"}, "client_id": {clientID}, "username": {user}, "password": {"isolated-native-operator-only"}, "scope": {"openid"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, o.issuer+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reply, err := o.client.Do(request)
	if err != nil {
		t.Fatal("native operator token request failed", err)
	}
	defer reply.Body.Close()
	var response struct {
		AccessToken string `json:"access_token"`
	}
	if reply.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(reply.Body, 64<<10)).Decode(&response) != nil || response.AccessToken == "" {
		t.Fatal("real Keycloak operator token missing", reply.StatusCode)
	}
	return response.AccessToken
}

func nativeOperatorGrantRequest(t *testing.T, ctx context.Context, client *http.Client, endpoint, token, target string) (int, []byte) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"target_id": target, "node_id": "node-live", "environment": "dev", "lifetime_seconds": 60, "certificate_ttl_seconds": 3600})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	reply, err := client.Do(request)
	if err != nil {
		t.Fatal("protected operator grant request failed", err)
	}
	defer reply.Body.Close()
	body, err := io.ReadAll(io.LimitReader(reply.Body, 64<<10))
	if err != nil {
		t.Fatal("protected operator grant response failed", err)
	}
	return reply.StatusCode, body
}
