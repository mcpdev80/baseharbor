package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// managedCoreTLSEndpoints validates the explicitly advertised LAN endpoints
// before issuing any certificate or publishing any profile. Listener and
// published ports must match; no loopback/wildcard identity is exportable.
func managedCoreTLSEndpoints(cfg controlplaneruntime.Config) (string, string, error) {
	apiRaw := strings.TrimSpace(os.Getenv("BASEHARBOR_API_PUBLIC_URL"))
	connectorRaw := strings.TrimSpace(os.Getenv("BASEHARBOR_CONNECTOR_PUBLIC_ADDRESS"))
	if apiRaw == "" {
		host, port, err := net.SplitHostPort(cfg.ListenAddr)
		if err == nil && usableNodeHost(host) && !strings.HasSuffix(strings.ToLower(host), ".localhost") {
			apiRaw = "https://" + net.JoinHostPort(host, port)
		}
	}
	if connectorRaw == "" && cfg.ConnectorEnrollmentEnabled {
		host, _, err := net.SplitHostPort(cfg.ListenAddr)
		if err == nil {
			_, port, err := net.SplitHostPort(cfg.ConnectorListenAddr)
			if err == nil && usableNodeHost(host) {
				connectorRaw = net.JoinHostPort(host, port)
			}
		}
	}
	if apiRaw == "" || (cfg.ConnectorEnrollmentEnabled && connectorRaw == "") {
		return "", "", errors.New("managed Core LAN TLS requires the advertised API URL and, only when enabled, the Connector address")
	}
	if err := validateCoreURL(apiRaw); err != nil {
		return "", "", fmt.Errorf("invalid public Core URL: %w", err)
	}
	apiURL, err := url.Parse(apiRaw)
	if err != nil {
		return "", "", err
	}
	apiHost := apiURL.Hostname()
	if !usableNodeHost(apiHost) || strings.HasSuffix(strings.ToLower(apiHost), ".localhost") {
		return "", "", errors.New("managed Core API requires a concrete reachable LAN name/IP")
	}
	apiPort := apiURL.Port()
	if apiPort == "" {
		apiPort = "443"
	}
	type listener struct{ label, address, port string }
	listeners := []listener{{"API", cfg.ListenAddr, apiPort}}
	var connectorHost string
	if cfg.ConnectorEnrollmentEnabled {
		var connectorPort string
		connectorHost, connectorPort, err = net.SplitHostPort(connectorRaw)
		if err != nil {
			return "", "", fmt.Errorf("invalid public connector address: %w", err)
		}
		if !usableNodeHost(connectorHost) || strings.HasSuffix(strings.ToLower(connectorHost), ".localhost") {
			return "", "", errors.New("managed Connector requires a reachable LAN name/IP")
		}
		listeners = append(listeners, listener{"connector", cfg.ConnectorListenAddr, connectorPort})
	}
	for _, endpoint := range listeners {
		listenHost, listenPort, err := net.SplitHostPort(endpoint.address)
		if err != nil {
			return "", "", fmt.Errorf("managed Core %s listener requires an explicit address: %w", endpoint.label, err)
		}
		ip := net.ParseIP(listenHost)
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
			return "", "", fmt.Errorf("managed Core %s listener must use a concrete LAN IP", endpoint.label)
		}
		advertised, err := strconv.Atoi(endpoint.port)
		if err != nil || advertised < 1 || advertised > 65535 || strconv.Itoa(advertised) != listenPort {
			return "", "", fmt.Errorf("managed Core %s listener and advertised port differ", endpoint.label)
		}
	}
	return apiHost, connectorHost, nil
}

// prepareManagedCoreServingTLS reuses the local installation's existing,
// authenticated OpenBao service issuer. No new CA, self-signed leaf, insecure
// client transport, or cross-installation signing authority is introduced.
// Opt-in is explicit; BYOC/externally prepared serving TLS remains supported.
func prepareManagedCoreServingTLS(ctx context.Context, cfg *controlplaneruntime.Config) error {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("BASEHARBOR_MANAGED_CORE_TLS")))
	if mode == "false" {
		return nil
	}
	if mode != "" && mode != "true" {
		return errors.New("BASEHARBOR_MANAGED_CORE_TLS must be true or false")
	}
	if mode != "true" && (cfg == nil || !cfg.WorkstationEnrollmentEnabled || cfg.ConnectorEnrollmentEnabled) {
		return nil
	}
	if cfg == nil || cfg.RuntimeAppName != "" ||
		(!cfg.ConnectorEnrollmentEnabled && !cfg.WorkstationEnrollmentEnabled) ||
		cfg.ConnectorAuthorityTarget == "" {
		return errors.New("managed Core LAN TLS requires an owned Core and connector or workstation enrollment")
	}
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" || cfg.ConnectorTLSCertFile != "" || cfg.ConnectorTLSKeyFile != "" || cfg.ConnectorTLSCAFile != "" {
		return errors.New("managed Core LAN TLS cannot override externally configured API/connector certificate material")
	}
	// Explicit endpoint settings remain authoritative. When none are supplied,
	// derive a single concrete LAN address for this owned Core. Never publish
	// a wildcard listener or a loopback-only invitation.
	if strings.TrimSpace(os.Getenv("BASEHARBOR_API_PUBLIC_URL")) == "" &&
		strings.TrimSpace(cfg.ListenAddr) == "" {
		address, err := discoverOwnedCoreLANAddress()
		if err != nil {
			return fmt.Errorf("discover managed Core LAN listener: %w", err)
		}
		cfg.ListenAddr = net.JoinHostPort(address, "18443")
	}
	apiHost, connectorHost, err := managedCoreTLSEndpoints(*cfg)
	if err != nil {
		return err
	}
	ownedCtx := withTargetOverride(ctx, cfg.ConnectorAuthorityTarget)
	target, err := effectiveTarget(ownedCtx)
	if err != nil || target.Name != cfg.ConnectorAuthorityTarget || (target.AccessProvider != "" && target.AccessProvider != "local") {
		return errors.New("managed Core LAN TLS requires the selected, locally owned installation Target")
	}
	files, err := initialOperatorLocalCoreFiles(ownedCtx, target)
	if err != nil {
		return fmt.Errorf("verify owned Core before managed LAN certificate issuance: %w", err)
	}
	// The managed local Core already owns a protected PostgreSQL database
	// binding. A new remote-serving process must not require its operator to
	// reconstruct the password or fabricate an external database URL.
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		dsn, err := locallyOwnedCorePostgresDSN(files)
		if err != nil {
			return fmt.Errorf("resolve locally owned Core database for operator API: %w", err)
		}
		cfg.DatabaseURL = dsn
	}
	runtime, err := detectRuntimeForTarget(ownedCtx, target)
	if err != nil {
		return err
	}
	issuer := openbao.NewServiceIssuer(runtime, files)
	status, err := issuer.Status(ownedCtx)
	if err != nil || !status.Ready {
		return errors.New("managed Core LAN TLS requires the existing READY OpenBao service issuer")
	}
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return err
	}
	issue := func(provider, host string, auth serviceaccess.AuthenticationMode) (serviceaccess.TLSMaterial, error) {
		policy, err := serviceaccess.Resolve("dev", provider, auth)
		if err != nil {
			return serviceaccess.TLSMaterial{}, err
		}
		if policy.PKISource != serviceaccess.PKIManagedLocal {
			return serviceaccess.TLSMaterial{}, errors.New("managed Core LAN TLS requires the existing managed-local PKI; use externally prepared TLS for other PKI sources")
		}
		policy.ServerName = host
		return serviceaccess.EnsureTLSMaterial(ownedCtx, issuer, policy, filepath.Join(root, "managed-serving", provider), host)
	}
	api, err := issue("core-api", apiHost, serviceaccess.AuthenticationOIDC)
	if err != nil {
		return fmt.Errorf("issue Core API serving TLS: %w", err)
	}
	cfg.TLSCertFile, cfg.TLSKeyFile = api.ServerCertificate, api.ServerKey
	if cfg.WorkstationEnrollmentEnabled {
		// This CA is the same managed trust anchor as the issued workstation
		// leaf; requesting a device certificate never bypasses OIDC on other routes.
		cfg.TLSClientCAFile = api.CA
	}
	if err := saveOwnedCoreLANEndpoint(target.Name, cfg.ListenAddr, apiHost, cfg.TLSCertFile); err != nil {
		return fmt.Errorf("publish verified managed LAN endpoint: %w", err)
	}
	if cfg.ConnectorEnrollmentEnabled {
		// Connector enrollment is a separate optional product. Core + Docker
		// on the same VM needs no Node Connector or connector TLS leaf.
		installationRoot, err := targetRuntimeStateRoot(target)
		if err != nil {
			return err
		}
		installation, err := coreinstallation.Load(installationRoot)
		if err != nil || !installation.Ready {
			return errors.New("managed Connector TLS requires current READY Core identity")
		}
		connector, err := issueManagedCoreConnectorTLS(ownedCtx, issuer, root, installation.ID, connectorHost)
		if err != nil {
			return fmt.Errorf("issue managed Connector TLS: %w", err)
		}
		cfg.ConnectorTLSCertFile, cfg.ConnectorTLSKeyFile, cfg.ConnectorTLSCAFile =
			connector.Certificate, connector.Key, connector.CA
	}
	return nil
}
