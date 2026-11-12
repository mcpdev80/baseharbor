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
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

const remoteConnectionProfileVersion = "baseharbor.connection-profile/v1"
const managementInstallationVersion = "baseharbor.management-installation/v1"

// A connection profile is a transport selection, never an authority itself.
// The server proves the configured installation and tenant after OIDC.
type remoteConnectionProfile struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Core    struct {
		InstallationID string `json:"installation_id"`
		URL            string `json:"url"`
	} `json:"core"`
	Trust struct {
		CAPEM  string `json:"ca_pem"`
		SHA256 string `json:"sha256"`
	} `json:"trust"`
	Targets []struct {
		Name            string `json:"name"`
		CoreTarget      string `json:"core_target,omitempty"`
		TenantID        string `json:"tenant_id"`
		RuntimeProvider string `json:"runtime_provider"`
		Scope           string `json:"scope,omitempty"`
	} `json:"targets"`
	Device *struct {
		IdentityDir string `json:"identity_dir"`
	} `json:"device,omitempty"`
	Access *struct {
		Issuer   string   `json:"issuer"`
		ClientID string   `json:"client_id"`
		Scopes   []string `json:"scopes"`
	} `json:"access,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Explicit opt-in only; no remote connection is inferred from a name, a
// missing local installation or a cached last-known connection status.
type verifiedRemoteCore struct {
	trustPEM     []byte
	installation string
	client       *http.Client
	origin       string
	token        string
	target       string
	coreTarget   string
	tenant       string
}

func verifyRemoteManagementConnection(ctx context.Context, target, runtime, expectedTenant string) error {
	_, err := dialVerifiedRemoteCore(ctx, target, runtime, expectedTenant)
	return err
}

func dialVerifiedRemoteCore(ctx context.Context, target, runtime, expectedTenant string) (*verifiedRemoteCore, error) {
	path := strings.TrimSpace(os.Getenv("BASEHARBOR_CONNECTION_PROFILE"))
	if path == "" {
		root, err := deployment.DataRoot()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(root, "remote-connections", target, "profile.json")
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remote management connection profile not configured (BASEHARBOR_CONNECTION_PROFILE): %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("open remote connection profile: %w", err)
	}
	defer file.Close()
	var profile remoteConnectionProfile
	decoder := json.NewDecoder(io.LimitReader(file, 128*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode remote connection profile: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("remote connection profile contains trailing data")
	}
	if profile.Version != remoteConnectionProfileVersion || profile.Name == "" || profile.ExpiresAt != nil && !profile.ExpiresAt.After(time.Now()) {
		return nil, errors.New("unsupported or expired remote connection profile")
	}
	origin, err := url.Parse(profile.Core.URL)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil ||
		origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" && origin.Path != "/" || origin.Opaque != "" {
		return nil, errors.New("remote Core URL must be an HTTPS origin")
	}
	if profile.Core.InstallationID == "" || profile.Trust.CAPEM == "" {
		return nil, errors.New("remote Core identity and pinned CA are required")
	}
	digest := sha256.Sum256([]byte(profile.Trust.CAPEM))
	if hex.EncodeToString(digest[:]) != profile.Trust.SHA256 {
		return nil, errors.New("remote Core CA pin mismatch")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(profile.Trust.CAPEM)) {
		return nil, errors.New("remote Core CA cannot be verified")
	}
	tenant := ""
	coreTarget := ""
	for _, item := range profile.Targets {
		if item.Name != target {
			continue
		}
		if item.RuntimeProvider != runtime || item.TenantID == "" || tenant != "" {
			return nil, errors.New("remote target is ambiguous or mismatched in connection profile")
		}
		tenant = item.TenantID
		coreTarget = strings.TrimSpace(item.CoreTarget)
		if coreTarget == "" {
			coreTarget = item.Name // v1 profiles created before workstation aliases
		}
		if err := deployment.ValidateTargetName(coreTarget); err != nil {
			return nil, errors.New("remote Core Target selector in connection profile is invalid")
		}
	}
	if expectedTenant == "" || tenant != expectedTenant {
		return nil, errors.New("remote target tenant does not match the configured target authority")
	}
	if tenant == "" {
		return nil, errors.New("remote target is not declared by the connection profile")
	}

	var deviceCert *tls.Certificate
	var bearerToken string
	var expectedOperatorIssuer string
	if profile.Device != nil {
		// A verified enrolled workstation uses only its private key and
		// bounded mTLS identity. It must never silently fall back to OIDC or
		// inherit another Target's token.
		keyFile := filepath.Join(profile.Device.IdentityDir, "private-key.pem")
		leafFile := filepath.Join(profile.Device.IdentityDir, "certificate.pem")
		pair, err := tls.LoadX509KeyPair(leafFile, keyFile)
		if err != nil {
			return nil, errors.New("verified workstation certificate is unavailable")
		}
		deviceCert = &pair
	} else {
		session, err := operatorauth.LoadSession(target, "dev")
		if err != nil || !session.ValidAt(time.Now()) {
			return nil, errors.New("remote Core requires a valid operator OIDC session for the selected target")
		}
		if profile.Access != nil && (profile.Access.Issuer != session.Issuer || profile.Access.ClientID != session.ClientID) {
			return nil, errors.New("remote OIDC issuer/client differs from the pinned connection")
		}
		bearerToken = session.IDToken
		expectedOperatorIssuer = session.Issuer
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: func() []tls.Certificate {
			if deviceCert != nil {
				return []tls.Certificate{*deviceCert}
			}
			return nil
		}()}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if profile.Device != nil {
		if err := renewRemoteWorkstationIdentity(ctx, client, profile); err != nil {
			return nil, fmt.Errorf("refresh bound workstation identity: %w", err)
		}
		// Re-read the protected leaf after a possible renewal. Every later
		// request in this invocation must present the registry-current serial.
		fresh, err := tls.LoadX509KeyPair(
			filepath.Join(profile.Device.IdentityDir, "certificate.pem"),
			filepath.Join(profile.Device.IdentityDir, "private-key.pem"))
		if err != nil {
			return nil, errors.New("renewed workstation certificate could not be loaded")
		}
		client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{fresh}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(profile.Core.URL, "/")+"/api/v1/management/installation", nil)
	if err != nil {
		return nil, err
	}
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("verify live remote Core: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote Core authentication/identity verification failed (HTTP %d)", response.StatusCode)
	}
	var identity struct {
		Version        string `json:"version"`
		InstallationID string `json:"installation_id"`
		IdentityIssuer string `json:"identity_issuer"`
		OperatorIssuer string `json:"operator_issuer"`
		TenantID       string `json:"tenant_id"`
	}
	decoded := json.NewDecoder(io.LimitReader(response.Body, 16*1024))
	decoded.DisallowUnknownFields()
	if decoded.Decode(&identity) != nil || decoded.Decode(new(any)) != io.EOF ||
		identity.Version != managementInstallationVersion ||
		identity.InstallationID != profile.Core.InstallationID ||
		identity.IdentityIssuer == "" || (expectedOperatorIssuer != "" && identity.OperatorIssuer != expectedOperatorIssuer) ||
		identity.TenantID != tenant {
		return nil, errors.New("remote Core installation, OIDC issuer or tenant does not match pinned connection")
	}
	return &verifiedRemoteCore{trustPEM: []byte(profile.Trust.CAPEM), installation: profile.Core.InstallationID, client: client, origin: strings.TrimRight(profile.Core.URL, "/"), token: bearerToken, target: target, coreTarget: coreTarget, tenant: tenant}, nil
}
