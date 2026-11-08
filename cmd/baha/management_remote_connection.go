package main

import (
    "context"
    "crypto/sha256"
    "crypto/tls"
    "crypto/x509"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "strings"
    "time"

    "github.com/mcpdev80/baseharbor/internal/operatorauth"
)

const remoteConnectionProfileVersion = "baseharbor.connection-profile/v1"
const managementInstallationVersion = "baseharbor.management-installation/v1"

// A connection profile is a transport selection, never an authority itself.
// The server proves the configured installation and tenant after OIDC.
type remoteConnectionProfile struct {
    Version string `json:"version"`
    Name string `json:"name"`
    Core struct {
        InstallationID string `json:"installation_id"`
        URL string `json:"url"`
    } `json:"core"`
    Trust struct {
        CAPEM string `json:"ca_pem"`
        SHA256 string `json:"sha256"`
    } `json:"trust"`
    Targets []struct {
        Name string `json:"name"`
        TenantID string `json:"tenant_id"`
        RuntimeProvider string `json:"runtime_provider"`
        Scope string `json:"scope,omitempty"`
    } `json:"targets"`
    Access *struct {
        Issuer string `json:"issuer"`
        ClientID string `json:"client_id"`
        Scopes []string `json:"scopes"`
    } `json:"access,omitempty"`
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Explicit opt-in only; no remote connection is inferred from a name, a
// missing local installation or a cached last-known connection status.
func verifyRemoteManagementConnection(ctx context.Context, target, runtime string) error {
    path := strings.TrimSpace(os.Getenv("BASEHARBOR_CONNECTION_PROFILE"))
    if path == "" {
        return errors.New("remote management connection profile not configured (BASEHARBOR_CONNECTION_PROFILE)")
    }
    file, err := os.Open(path)
    if err != nil { return fmt.Errorf("open remote connection profile: %w", err) }
    defer file.Close()
    var profile remoteConnectionProfile
    decoder := json.NewDecoder(io.LimitReader(file, 128*1024))
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(&profile); err != nil { return fmt.Errorf("decode remote connection profile: %w", err) }
    if err := decoder.Decode(new(any)); err != io.EOF {
        return errors.New("remote connection profile contains trailing data")
    }
    if profile.Version != remoteConnectionProfileVersion || profile.Name == "" || profile.ExpiresAt != nil && !profile.ExpiresAt.After(time.Now()) {
        return errors.New("unsupported or expired remote connection profile")
    }
    origin, err := url.Parse(profile.Core.URL)
    if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil ||
        origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" && origin.Path != "/" || origin.Opaque != "" {
        return errors.New("remote Core URL must be an HTTPS origin")
    }
    if profile.Core.InstallationID == "" || profile.Trust.CAPEM == "" {
        return errors.New("remote Core identity and pinned CA are required")
    }
    digest := sha256.Sum256([]byte(profile.Trust.CAPEM))
    if hex.EncodeToString(digest[:]) != profile.Trust.SHA256 {
        return errors.New("remote Core CA pin mismatch")
    }
    roots := x509.NewCertPool()
    if !roots.AppendCertsFromPEM([]byte(profile.Trust.CAPEM)) {
        return errors.New("remote Core CA cannot be verified")
    }
    tenant := ""
    for _, item := range profile.Targets {
        if item.Name != target { continue }
        if item.RuntimeProvider != runtime || item.TenantID == "" || tenant != "" {
            return errors.New("remote target is ambiguous or mismatched in connection profile")
        }
        tenant = item.TenantID
    }
    if tenant == "" { return errors.New("remote target is not declared by the connection profile") }

    // The login is intentionally scoped to the requested Target/environment.
    // No token is substituted from another Target, tenant or local Core.
    session, err := operatorauth.LoadSession(target, "dev")
    if err != nil || !session.ValidAt(time.Now()) {
        return errors.New("remote Core requires a valid operator OIDC session for the selected target")
    }
    if profile.Access != nil && (profile.Access.Issuer != session.Issuer || profile.Access.ClientID != session.ClientID) {
        return errors.New("remote OIDC issuer/client differs from the pinned connection")
    }
    client := &http.Client{
        Timeout: 15 * time.Second,
        Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}},
        CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
    }
    request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(profile.Core.URL, "/")+"/api/v1/management/installation", nil)
    if err != nil { return err }
    request.Header.Set("Authorization", "Bearer "+session.IDToken)
    response, err := client.Do(request)
    if err != nil { return fmt.Errorf("verify live remote Core: %w", err) }
    defer response.Body.Close()
    if response.StatusCode != http.StatusOK {
        return fmt.Errorf("remote Core authentication/identity verification failed (HTTP %d)", response.StatusCode)
    }
    var identity struct {
        Version string `json:"version"`
        InstallationID string `json:"installation_id"`
        IdentityIssuer string `json:"identity_issuer"`
        OperatorIssuer string `json:"operator_issuer"`
        TenantID string `json:"tenant_id"`
    }
    decoded := json.NewDecoder(io.LimitReader(response.Body, 16*1024))
    decoded.DisallowUnknownFields()
    if decoded.Decode(&identity) != nil || decoded.Decode(new(any)) != io.EOF ||
        identity.Version != managementInstallationVersion ||
        identity.InstallationID != profile.Core.InstallationID ||
        identity.IdentityIssuer == "" || identity.OperatorIssuer != session.Issuer ||
        identity.TenantID != tenant {
        return errors.New("remote Core installation, OIDC issuer or tenant does not match pinned connection")
    }
    return nil
}
