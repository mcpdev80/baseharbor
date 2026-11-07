package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

const nodeAuthorityProfileVersion = "baseharbor.node-authority-profile/v1"

// Public, owner-local connection facts. No tokens, credentials or private keys.
type nodeAuthorityProfile struct {
	Version          string `json:"version"`
	Target           string `json:"target"`
	Environment      string `json:"environment"`
	CoreURL          string `json:"core_url"`
	CoreAddress      string `json:"core_address"`
	CAFile           string `json:"ca_file"`
	OperatorIssuer   string `json:"operator_issuer"`
	OperatorClientID string `json:"operator_client_id"`
}

func nodeAuthorityProfilePath(target string) (string, error) {
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "node-authority-profile.json"), nil
}

func loadNodeAuthorityProfile(target string) (nodeAuthorityProfile, error) {
	path, err := nodeAuthorityProfilePath(target)
	if err != nil {
		return nodeAuthorityProfile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nodeAuthorityProfile{}, fmt.Errorf("Core node authority profile missing: %w", err)
	}
	var profile nodeAuthorityProfile
	if json.Unmarshal(data, &profile) != nil || profile.Version != nodeAuthorityProfileVersion ||
		profile.Target != target || profile.Environment == "" || profile.CAFile == "" ||
		validateCoreURL(profile.CoreURL) != nil || profile.CoreAddress == "" ||
		profile.OperatorIssuer == "" || profile.OperatorClientID == "" {
		return nodeAuthorityProfile{}, errors.New("Core node authority profile invalid; start the owned Core API with connector enrollment enabled")
	}
	return profile, nil
}

func applyNodeAuthorityDefaults(opts nodeAddOptions, profile nodeAuthorityProfile) (nodeAddOptions, error) {
	if opts.Environment == "" {
		opts.Environment = profile.Environment
	}
	if opts.Environment != profile.Environment {
		return opts, errors.New("requested node environment disagrees with the owned Core")
	}
	if opts.CoreURL == "" {
		opts.CoreURL = profile.CoreURL
	}
	if opts.CoreAddress == "" {
		opts.CoreAddress = profile.CoreAddress
	}
	if opts.CAFile == "" {
		opts.CAFile = profile.CAFile
	}
	return opts, nil
}

// A custom endpoint may be used only with the installed Core trust anchor.
// Never send an operator ID token to a caller-supplied independent CA.
func validateNodeAuthorityTrustOverride(profile nodeAuthorityProfile, opts nodeAddOptions) error {
	owned, err := os.ReadFile(profile.CAFile)
	if err != nil || len(owned) == 0 {
		return errors.New("verified Core trust anchor is missing")
	}
	selected, err := os.ReadFile(opts.CAFile)
	if err != nil || len(selected) == 0 {
		return errors.New("selected Core CA file is unavailable")
	}
	if !bytes.Equal(bytes.TrimSpace(owned), bytes.TrimSpace(selected)) {
		return errors.New("custom Core CA differs from the owned authority; refusing to disclose operator credentials to an untrusted endpoint")
	}
	return nil
}

func publicNodeEndpoint(listen, override string, cert *x509.Certificate, scheme bool) (string, string, error) {
	if override != "" {
		if scheme {
			if err := validateCoreURL(override); err != nil {
				return "", "", err
			}
			u, _ := url.Parse(override)
			return override, u.Hostname(), nil
		}
		host, _, err := net.SplitHostPort(override)
		if err != nil {
			return "", "", err
		}
		return override, host, nil
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", "", fmt.Errorf("Core API/Connector listener must be host:port: %w", err)
	}
	host, err = discoverNodePublicHost(host, cert)
	if err != nil {
		return "", "", err
	}
	address := net.JoinHostPort(host, port)
	if scheme {
		return "https://" + address, host, nil
	}
	return address, host, nil
}

func discoverNodePublicHost(listen string, cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", errors.New("Core TLS certificate missing")
	}
	if usableNodeHost(listen) {
		if err := cert.VerifyHostname(listen); err != nil {
			return "", err
		}
		return listen, nil
	}
	var candidates []string
	for _, name := range cert.DNSNames {
		if usableNodeHost(name) {
			candidates = append(candidates, name)
		}
	}
	for _, ip := range cert.IPAddresses {
		if usableNodeHost(ip.String()) {
			candidates = append(candidates, ip.String())
		}
	}
	if len(candidates) != 1 {
		return "", errors.New("Core public name ambiguous or loopback-only; explicitly configure a public API URL and Connector address")
	}
	return candidates[0], nil
}

func usableNodeHost(host string) bool {
	host = strings.Trim(host, "[]")
	if host == "" || host == "localhost" || host == "0.0.0.0" || host == "::" || strings.Contains(host, "*") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast()
	}
	return !strings.ContainsAny(host, " /\\")
}

func verifiedNodeServerLeaf(certFile, keyFile string) (*x509.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil || len(pair.Certificate) == 0 {
		return nil, errors.New("Core server TLS keypair is unavailable")
	}
	return x509.ParseCertificate(pair.Certificate[0])
}

func publishNodeAuthorityProfile(ctx context.Context, cfg controlplaneruntime.Config) error {
	if !cfg.ConnectorEnrollmentEnabled {
		return nil
	}
	if cfg.ConnectorListenAddr == "" {
		return errors.New("Connector enrollment requires an outbound-session listener")
	}
	ownedCtx := withTargetOverride(ctx, cfg.ConnectorAuthorityTarget)
	target, err := effectiveTarget(ownedCtx)
	if err != nil || target.Name != cfg.ConnectorAuthorityTarget || target.AccessProvider != "local" {
		return errors.New("Core node authority must be the selected, locally owned Target")
	}
	if _, err := initialOperatorLocalCoreFiles(ownedCtx, target); err != nil {
		return err
	}
	environment := strings.TrimSpace(os.Getenv("BASEHARBOR_OPERATOR_ENVIRONMENT"))
	if environment == "" {
		environment = "dev"
	}
	oidc, found, err := resolveStoredOperatorAuthBoundary(target.Name, environment)
	if err != nil || !found || oidc.Issuer != cfg.OIDCIssuer {
		return errors.New("Core node authority requires the configured operator Keycloak issuer")
	}
	trust, managed, err := currentManagedTrustBundle(ownedCtx)
	if err != nil || !managed || len(trust.PEM) == 0 {
		return errors.New("Core managed public CA is unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trust.PEM) {
		return errors.New("Core managed public CA invalid")
	}
	api, err := verifiedNodeServerLeaf(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return err
	}
	connector, err := verifiedNodeServerLeaf(cfg.ConnectorTLSCertFile, cfg.ConnectorTLSKeyFile)
	if err != nil {
		return err
	}
	apiURL, apiHost, err := publicNodeEndpoint(cfg.ListenAddr, os.Getenv("BASEHARBOR_API_PUBLIC_URL"), api, true)
	if err != nil {
		return err
	}
	connectorAddress, connectorHost, err := publicNodeEndpoint(cfg.ConnectorListenAddr, os.Getenv("BASEHARBOR_CONNECTOR_PUBLIC_ADDRESS"), connector, false)
	if err != nil {
		return err
	}
	for _, check := range []struct {
		cert *x509.Certificate
		host string
	}{{api, apiHost}, {connector, connectorHost}} {
		if _, err := check.cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: check.host, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			return fmt.Errorf("Core server trust cannot be verified for the public endpoint: %w", err)
		}
	}
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	caPath := filepath.Join(root, "node-authority-ca.pem")
	if err := os.WriteFile(caPath, trust.PEM, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(caPath, 0o600); err != nil {
		return err
	}
	profile := nodeAuthorityProfile{
		Version: nodeAuthorityProfileVersion, Target: target.Name, Environment: environment,
		CoreURL: apiURL, CoreAddress: connectorAddress, CAFile: caPath,
		OperatorIssuer: cfg.OIDCIssuer, OperatorClientID: oidc.ClientID,
	}
	path, err := nodeAuthorityProfilePath(target.Name)
	if err != nil {
		return err
	}
	return writePrivateJSON(path, profile)
}
