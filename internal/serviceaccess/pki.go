package serviceaccess

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const managedCertificateRenewalWindow = 7 * 24 * time.Hour

type TLSMaterial struct {
	Source            PKISource `json:"source"`
	CA                string    `json:"ca"`
	ServerCertificate string    `json:"server_certificate"`
	ServerKey         string    `json:"server_key"`
	ClientCertificate string    `json:"client_certificate,omitempty"`
	ClientKey         string    `json:"client_key,omitempty"`
	ServerName        string    `json:"server_name"`
}

type staticPKIState struct {
	Version           int       `json:"version"`
	Source            PKISource `json:"source"`
	LifecycleOwner    string    `json:"lifecycle_owner"`
	RenewalMode       string    `json:"renewal_mode"`
	ServerFingerprint string    `json:"server_fingerprint"`
	ServerExpiresAt   time.Time `json:"server_expires_at"`
	ClientExpiresAt   time.Time `json:"client_expires_at,omitempty"`
	Health            string    `json:"health"`
	Warning           string    `json:"warning,omitempty"`
}

type managedPKIState struct {
	Version         int       `json:"version"`
	IssuerReference string    `json:"issuer_reference"`
	LifecycleOwner  string    `json:"lifecycle_owner"`
	RenewalMode     string    `json:"renewal_mode"`
	ServerSerial    string    `json:"server_serial"`
	ServerExpiresAt time.Time `json:"server_expires_at"`
	ClientSerial    string    `json:"client_serial,omitempty"`
	ClientExpiresAt time.Time `json:"client_expires_at,omitempty"`
}

func EnsureTLSMaterial(ctx context.Context, issuer Issuer, policy Policy, dir string, dnsNames ...string) (TLSMaterial, error) {
	if !policy.TLSRequired {
		return TLSMaterial{}, errors.New("BaseHarbor managed service access cannot disable TLS")
	}
	switch policy.PKISource {
	case PKIBYOC:
		return ensureStaticExternalMaterial(policy, dir)
	case PKIExternal:
		if strings.TrimSpace(policy.IssuerReference) == "" {
			return ensureStaticExternalMaterial(policy, dir)
		}
		if issuer == nil {
			return TLSMaterial{}, errors.New("issuer-backed external-pki requires an issuer adapter")
		}
		status, err := issuer.Status(ctx)
		if err != nil {
			return TLSMaterial{}, fmt.Errorf("inspect external PKI issuer: %w", err)
		}
		if !status.Ready {
			return TLSMaterial{}, errors.New("external PKI issuer is not ready")
		}
		if strings.TrimSpace(status.IssuerReference) != strings.TrimSpace(policy.IssuerReference) {
			return TLSMaterial{}, fmt.Errorf("external PKI issuer reference mismatch: policy=%q adapter=%q", policy.IssuerReference, status.IssuerReference)
		}
		return ensureIssuerMaterial(ctx, issuer, policy, dir, PKIExternal, dnsNames)
	case PKIManagedLocal:
		return ensureIssuerMaterial(ctx, issuer, policy, dir, PKIManagedLocal, dnsNames)
	default:
		return TLSMaterial{}, fmt.Errorf("unsupported PKI source %q", policy.PKISource)
	}
}

func ExistingTLSMaterial(policy Policy, dir string) (TLSMaterial, error) {
	if policy.PKISource == PKIBYOC || (policy.PKISource == PKIExternal && strings.TrimSpace(policy.IssuerReference) == "") {
		return externalTLSMaterial(policy)
	}
	if policy.PKISource != PKIManagedLocal && policy.PKISource != PKIExternal {
		return TLSMaterial{}, fmt.Errorf("unsupported PKI source %q", policy.PKISource)
	}
	material := issuerTLSMaterial(policy, dir, policy.PKISource)
	requireClient := policy.AuthenticationRequired && policy.Authentication == AuthenticationMTLS
	for _, path := range requiredMaterialPaths(material, requireClient) {
		if _, err := os.Stat(path); err != nil {
			return TLSMaterial{}, err
		}
	}
	if err := validateMaterial(material, requireClient); err != nil {
		return TLSMaterial{}, err
	}
	if !requireClient {
		material.ClientCertificate = ""
		material.ClientKey = ""
	}
	return material, nil
}

func ensureStaticExternalMaterial(policy Policy, dir string) (TLSMaterial, error) {
	material, err := externalTLSMaterial(policy)
	if err != nil {
		return TLSMaterial{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return TLSMaterial{}, fmt.Errorf("create static PKI lifecycle state directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return TLSMaterial{}, err
	}
	server, err := readCertificate(material.ServerCertificate)
	if err != nil {
		return TLSMaterial{}, err
	}
	sum := sha256.Sum256(server.Raw)
	state := staticPKIState{
		Version:           1,
		Source:            policy.PKISource,
		LifecycleOwner:    "operator",
		RenewalMode:       "replace-and-reconcile",
		ServerFingerprint: fmt.Sprintf("%x", sum[:]),
		ServerExpiresAt:   server.NotAfter.UTC(),
		Health:            "ok",
	}
	if policy.PKISource == PKIExternal {
		state.LifecycleOwner = "external"
	}
	remaining := time.Until(server.NotAfter)
	switch {
	case remaining <= 7*24*time.Hour:
		state.Health = "critical"
		state.Warning = "certificate expires within 7 days; replace the configured certificate/key material and reconcile before expiry"
	case remaining <= 30*24*time.Hour:
		state.Health = "warn"
		state.Warning = "certificate expires within 30 days; prepare replacement certificate/key material and reconcile"
	}
	if material.ClientCertificate != "" {
		if client, clientErr := readCertificate(material.ClientCertificate); clientErr == nil {
			state.ClientExpiresAt = client.NotAfter.UTC()
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return TLSMaterial{}, err
	}
	data = append(data, '\n')
	if err := writeAtomic(filepath.Join(dir, "static-state.json"), data, 0o600); err != nil {
		return TLSMaterial{}, err
	}
	return material, nil
}

func externalTLSMaterial(policy Policy) (TLSMaterial, error) {
	if policy.ServerCertificate == "" || policy.ServerKey == "" || policy.TrustBundle == "" {
		return TLSMaterial{}, errors.New("external TLS material is incomplete")
	}
	material := TLSMaterial{
		Source:            policy.PKISource,
		CA:                policy.TrustBundle,
		ServerCertificate: policy.ServerCertificate,
		ServerKey:         policy.ServerKey,
		ClientCertificate: policy.ClientCertificate,
		ClientKey:         policy.ClientKey,
		ServerName:        policy.ServerName,
	}
	if err := validateMaterial(material, policy.AuthenticationRequired && policy.Authentication == AuthenticationMTLS); err != nil {
		return TLSMaterial{}, err
	}
	return material, nil
}

func ensureIssuerMaterial(ctx context.Context, issuer Issuer, policy Policy, dir string, source PKISource, dnsNames []string) (TLSMaterial, error) {
	if issuer == nil {
		return TLSMaterial{}, errors.New("issuer-backed PKI requires an issuer")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return TLSMaterial{}, fmt.Errorf("create managed service PKI directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return TLSMaterial{}, err
	}

	material := issuerTLSMaterial(policy, dir, source)
	requireClient := policy.AuthenticationRequired && policy.Authentication == AuthenticationMTLS

	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		return TLSMaterial{}, fmt.Errorf("resolve service issuer trust bundle: %w", err)
	}
	if len(trust.PEM) == 0 {
		return TLSMaterial{}, errors.New("service issuer returned an empty trust bundle")
	}
	currentIssuerReference := firstNonEmpty(trust.IssuerReference, policy.IssuerReference)
	if currentIssuerReference == "" {
		return TLSMaterial{}, errors.New("issuer-backed PKI requires an issuer reference")
	}
	previousState, _ := readManagedPKIState(filepath.Join(dir, "state.json"))
	trustMatches := trustBundleMatchesFile(material.CA, trust.PEM)
	if valid, err := managedMaterialValid(material, dnsNames, requireClient); err != nil {
		return TLSMaterial{}, err
	} else if valid && trustMatches && previousState.IssuerReference == currentIssuerReference {
		return material, nil
	}

	names := uniqueNames(append([]string{policy.ServerName, "localhost"}, dnsNames...))
	serverRequest := CertificateRequest{
		CommonName: firstCertificateName(names),
		TTL:        30 * 24 * time.Hour,
	}
	for _, name := range names {
		if ip := net.ParseIP(name); ip != nil {
			serverRequest.IPAddresses = append(serverRequest.IPAddresses, ip)
			continue
		}
		serverRequest.DNSNames = append(serverRequest.DNSNames, name)
	}
	var serverCurrent IssuedCertificate
	if trustMatches && previousState.IssuerReference == currentIssuerReference {
		serverCurrent = currentIssuedCertificate(material.ServerCertificate, previousState.ServerSerial, previousState.ServerExpiresAt, previousState.IssuerReference)
	}
	server, err := issueOrRenew(ctx, issuer, serverCurrent, serverRequest)
	if err != nil {
		return TLSMaterial{}, fmt.Errorf("issue or renew managed service server certificate: %w", err)
	}

	var client IssuedCertificate
	if requireClient {
		clientRequest := CertificateRequest{
			CommonName: "baseharbor-service-client",
			TTL:        30 * 24 * time.Hour,
		}
		var clientCurrent IssuedCertificate
		if trustMatches && previousState.IssuerReference == currentIssuerReference {
			clientCurrent = currentIssuedCertificate(material.ClientCertificate, previousState.ClientSerial, previousState.ClientExpiresAt, previousState.IssuerReference)
		}
		client, err = issueOrRenew(ctx, issuer, clientCurrent, clientRequest)
		if err != nil {
			return TLSMaterial{}, fmt.Errorf("issue or renew managed service client certificate: %w", err)
		}
	}

	files := map[string]struct {
		data []byte
		mode os.FileMode
	}{
		material.CA:                {data: trust.PEM, mode: 0o644},
		material.ServerCertificate: {data: server.Certificate, mode: 0o644},
		material.ServerKey:         {data: server.PrivateKey, mode: 0o600},
	}
	if requireClient {
		files[material.ClientCertificate] = struct {
			data []byte
			mode os.FileMode
		}{data: client.Certificate, mode: 0o644}
		files[material.ClientKey] = struct {
			data []byte
			mode os.FileMode
		}{data: client.PrivateKey, mode: 0o600}
	}
	for path, file := range files {
		if len(file.data) == 0 {
			return TLSMaterial{}, fmt.Errorf("managed service issuer returned empty material for %s", filepath.Base(path))
		}
		if err := writeAtomic(path, file.data, file.mode); err != nil {
			return TLSMaterial{}, err
		}
	}
	if !requireClient {
		_ = os.Remove(material.ClientCertificate)
		_ = os.Remove(material.ClientKey)
		material.ClientCertificate = ""
		material.ClientKey = ""
	}

	state := managedPKIState{
		Version:         2,
		IssuerReference: firstNonEmpty(server.IssuerReference, currentIssuerReference),
		LifecycleOwner:  "issuer",
		RenewalMode:     "automatic-reconcile",
		ServerSerial:    server.Serial,
		ServerExpiresAt: server.ExpiresAt,
	}
	if requireClient {
		state.ClientSerial = client.Serial
		state.ClientExpiresAt = client.ExpiresAt
	}
	stateData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return TLSMaterial{}, err
	}
	stateData = append(stateData, '\n')
	if err := writeAtomic(filepath.Join(dir, "state.json"), stateData, 0o600); err != nil {
		return TLSMaterial{}, err
	}
	if err := validateMaterial(material, requireClient); err != nil {
		return TLSMaterial{}, fmt.Errorf("validate issued managed service TLS material: %w", err)
	}
	return material, nil
}

func readManagedPKIState(path string) (managedPKIState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return managedPKIState{}, err
	}
	var state managedPKIState
	if err := json.Unmarshal(data, &state); err != nil {
		return managedPKIState{}, err
	}
	if state.Version != 2 {
		return managedPKIState{}, fmt.Errorf("unsupported managed PKI state version %d", state.Version)
	}
	return state, nil
}

func trustBundleMatchesFile(path string, trustPEM []byte) bool {
	existing, err := readCertificate(path)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(trustPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return false
	}
	current, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return existing.Equal(current)
}

func currentIssuedCertificate(certPath, serial string, expiresAt time.Time, issuerReference string) IssuedCertificate {
	if strings.TrimSpace(serial) == "" || strings.TrimSpace(issuerReference) == "" {
		return IssuedCertificate{}
	}
	cert, err := readCertificate(certPath)
	if err != nil || !time.Now().Before(cert.NotAfter) {
		return IssuedCertificate{}
	}
	return IssuedCertificate{
		IssuerReference: issuerReference,
		Serial:          serial,
		ExpiresAt:       firstNonZeroTime(expiresAt, cert.NotAfter),
	}
}

func issueOrRenew(ctx context.Context, issuer Issuer, current IssuedCertificate, request CertificateRequest) (IssuedCertificate, error) {
	if strings.TrimSpace(current.Serial) != "" {
		return issuer.Renew(ctx, current, request)
	}
	return issuer.Issue(ctx, request)
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func issuerTLSMaterial(policy Policy, dir string, source PKISource) TLSMaterial {
	return TLSMaterial{
		Source:            source,
		CA:                filepath.Join(dir, "ca.pem"),
		ServerCertificate: filepath.Join(dir, "server-cert.pem"),
		ServerKey:         filepath.Join(dir, "server-key.pem"),
		ClientCertificate: filepath.Join(dir, "client-cert.pem"),
		ClientKey:         filepath.Join(dir, "client-key.pem"),
		ServerName:        policy.ServerName,
	}
}

func requiredMaterialPaths(material TLSMaterial, requireClient bool) []string {
	paths := []string{material.CA, material.ServerCertificate, material.ServerKey}
	if requireClient {
		paths = append(paths, material.ClientCertificate, material.ClientKey)
	}
	return paths
}

func managedMaterialValid(material TLSMaterial, dnsNames []string, requireClient bool) (bool, error) {
	for _, path := range requiredMaterialPaths(material, requireClient) {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return false, nil
		} else if err != nil {
			return false, err
		}
	}
	if err := validateMaterial(material, requireClient); err != nil {
		return false, nil
	}
	ca, err := readCertificate(material.CA)
	if err != nil {
		return false, nil
	}
	server, err := readCertificate(material.ServerCertificate)
	if err != nil || time.Now().Add(managedCertificateRenewalWindow).After(server.NotAfter) {
		return false, nil
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, name := range uniqueNames(append([]string{material.ServerName, "localhost"}, dnsNames...)) {
		if name == "" {
			continue
		}
		if _, err := server.Verify(x509.VerifyOptions{Roots: roots, DNSName: name, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			return false, nil
		}
	}
	if requireClient {
		client, err := readCertificate(material.ClientCertificate)
		if err != nil || time.Now().Add(managedCertificateRenewalWindow).After(client.NotAfter) {
			return false, nil
		}
		if _, err := client.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
			return false, nil
		}
	}
	return true, nil
}

func validateMaterial(material TLSMaterial, requireClient bool) error {
	ca, err := readCertificate(material.CA)
	if err != nil {
		return fmt.Errorf("read service TLS trust bundle: %w", err)
	}
	if !ca.IsCA {
		return errors.New("service trust certificate is not a CA")
	}
	server, err := readCertificate(material.ServerCertificate)
	if err != nil {
		return fmt.Errorf("read service TLS server certificate: %w", err)
	}
	if time.Now().After(server.NotAfter) {
		return errors.New("service TLS server certificate is expired")
	}
	if err := keyMatches(material.ServerCertificate, material.ServerKey); err != nil {
		return fmt.Errorf("service TLS server key: %w", err)
	}
	if requireClient {
		client, err := readCertificate(material.ClientCertificate)
		if err != nil {
			return fmt.Errorf("read service TLS client certificate: %w", err)
		}
		if time.Now().After(client.NotAfter) {
			return errors.New("service TLS client certificate is expired")
		}
		if err := keyMatches(material.ClientCertificate, material.ClientKey); err != nil {
			return fmt.Errorf("service TLS client key: %w", err)
		}
	}
	return nil
}

func readCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certificate PEM is invalid")
	}
	return x509.ParseCertificate(block.Bytes)
}

func keyMatches(certPath, keyPath string) error {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return errors.New("certificate and private key do not match")
	}
	return nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func uniqueNames(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstCertificateName(values []string) string {
	for _, value := range values {
		if value != "" && net.ParseIP(value) == nil {
			return value
		}
	}
	return "baseharbor-service"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
