package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// renewRemoteWorkstationIdentity renews only the current mTLS workstation
// identity. The private key never leaves the workstation, and no OIDC token or
// invitation grant is needed. The Core admission boundary checks the current
// leaf against its durable registry and OpenBao CRL before renewing.
func renewRemoteWorkstationIdentity(ctx context.Context, client *http.Client, profile remoteConnectionProfile) error {
	if profile.Device == nil || client == nil {
		return errors.New("workstation renewal requires a verified device connection")
	}
	dir := profile.Device.IdentityDir
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("workstation renewal requires an owner-private identity directory")
	}
	lock := filepath.Join(dir, ".renew.lock")
	fd, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("another workstation renewal is active or renewal lock is unavailable: %w", err)
	}
	if err := fd.Close(); err != nil {
		_ = os.Remove(lock)
		return err
	}
	defer os.Remove(lock)

	certPath := filepath.Join(dir, "certificate.pem")
	keyPath := filepath.Join(dir, "private-key.pem")
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil || len(pair.Certificate) == 0 {
		return errors.New("workstation renewal requires a valid existing certificate and key")
	}
	old, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || !old.NotAfter.After(time.Now()) {
		return errors.New("workstation certificate already expired; reissue from the Core owner")
	}
	if time.Until(old.NotAfter) >= 15*time.Minute {
		return nil
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	block, remaining := pem.Decode(keyBytes)
	if block == nil || block.Type != "EC PRIVATE KEY" || len(bytes.TrimSpace(remaining)) != 0 {
		return errors.New("workstation private key format is invalid")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	if len(old.URIs) != 1 || old.URIs[0] == nil {
		return errors.New("workstation certificate identity is missing")
	}
	scopeURI := old.URIs[0]
	const prefix = "spiffe://baseharbor/workstations/"
	parts := strings.Split(strings.TrimPrefix(scopeURI.String(), prefix), "/")
	if !strings.HasPrefix(scopeURI.String(), prefix) || len(parts) != 2 {
		return errors.New("invalid workstation scope for renewal")
	}
	scope := ws.Scope{TenantID: parts[0], Station: parts[1]}
	if scope.Validate() != nil || scope.Identity() != scopeURI.String() {
		return errors.New("foreign workstation renewal identity")
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader,
		&x509.CertificateRequest{URIs: old.URIs}, key)
	if err != nil {
		return err
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	payload, err := json.Marshal(map[string]any{
		"station": scope.Station, "csr_pem": string(csrPEM), "certificate_ttl_seconds": 3600,
	})
	if err != nil {
		return err
	}

	// Establish the protected local write destination before the server
	// changes the authoritative serial. A failed local preflight never spends
	// the current leaf.
	staged, err := os.CreateTemp(dir, ".renew-cert-*")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	if err := staged.Chmod(0o600); err != nil {
		_ = staged.Close()
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(profile.Core.URL, "/")+"/api/v1/workstations/renew", bytes.NewReader(payload))
	if err != nil {
		_ = staged.Close()
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		_ = staged.Close()
		return fmt.Errorf("workstation mTLS renewal request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = staged.Close()
		return fmt.Errorf("Core rejected workstation renewal (HTTP %d)", resp.StatusCode)
	}
	var reply struct {
		InstallationID string    `json:"installation_id"`
		Station        string    `json:"station"`
		Serial         string    `json:"serial"`
		ExpiresAt      time.Time `json:"expires_at"`
		LeafPEM        string    `json:"leaf_pem"`
		TrustPEM       string    `json:"trust_pem"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 64<<10))
	dec.DisallowUnknownFields()
	// The service also includes its own issuance timestamp.
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		_ = staged.Close()
		return errors.New("invalid workstation renewal response")
	}
	for name := range raw {
		switch name {
		case "installation_id", "station", "serial", "expires_at", "leaf_pem", "trust_pem", "at":
		default:
			_ = staged.Close()
			return errors.New("unexpected workstation renewal response field")
		}
	}
	b, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(b, &reply) != nil {
		_ = staged.Close()
		return errors.New("invalid workstation renewal result")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		_ = staged.Close()
		return errors.New("trailing workstation renewal response data")
	}
	if reply.InstallationID != scope.TenantID || reply.Station != scope.Station ||
		!reply.ExpiresAt.After(time.Now().Add(15*time.Minute)) ||
		!bytes.Equal([]byte(reply.TrustPEM), []byte(profile.Trust.CAPEM)) {
		_ = staged.Close()
		return errors.New("workstation renewal returned an untrusted identity")
	}
	leafBlock, tail := pem.Decode([]byte(reply.LeafPEM))
	if leafBlock == nil || leafBlock.Type != "CERTIFICATE" || len(bytes.TrimSpace(tail)) != 0 {
		_ = staged.Close()
		return errors.New("malformed workstation renewal certificate")
	}
	leaf, err := x509.ParseCertificate(leafBlock.Bytes)
	if err != nil {
		_ = staged.Close()
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(profile.Trust.CAPEM)) {
		_ = staged.Close()
		return errors.New("workstation CA trust is unavailable")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: time.Now(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		_ = staged.Close()
		return fmt.Errorf("renewed workstation leaf not trusted: %w", err)
	}
	pubNew, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil {
		_ = staged.Close()
		return err
	}
	pubOwn, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil || !bytes.Equal(pubNew, pubOwn) || len(leaf.URIs) != 1 ||
		leaf.URIs[0] == nil || leaf.URIs[0].String() != scope.Identity() ||
		leaf.IsCA || leaf.SerialNumber.Cmp(old.SerialNumber) == 0 ||
		leaf.NotAfter.Sub(reply.ExpiresAt) > time.Second ||
		reply.ExpiresAt.Sub(leaf.NotAfter) > time.Second {
		_ = staged.Close()
		return errors.New("renewed workstation leaf does not match the pinned key and identity")
	}
	digest := sha256.Sum256([]byte(reply.TrustPEM))
	if fmt.Sprintf("%x", digest[:]) != profile.Trust.SHA256 {
		_ = staged.Close()
		return errors.New("renewed workstation trust pin differs from profile")
	}
	if _, err := staged.Write([]byte(reply.LeafPEM)); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Sync(); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := os.Rename(staged.Name(), certPath); err != nil {
		return fmt.Errorf("renewed Core leaf could not be activated: %w", err)
	}
	return nil
}
