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
	if apiRaw == "" || connectorRaw == "" {
		return "", "", errors.New("managed Core LAN TLS requires explicit BASEHARBOR_API_PUBLIC_URL and BASEHARBOR_CONNECTOR_PUBLIC_ADDRESS")
	}
	if err := validateCoreURL(apiRaw); err != nil {
		return "", "", fmt.Errorf("invalid public Core URL: %w", err)
	}
	apiURL, err := url.Parse(apiRaw)
	if err != nil {
		return "", "", err
	}
	apiHost := apiURL.Hostname()
	connectorHost, connectorPort, err := net.SplitHostPort(connectorRaw)
	if err != nil {
		return "", "", fmt.Errorf("invalid public connector address: %w", err)
	}
	if !usableNodeHost(apiHost) || !usableNodeHost(connectorHost) {
		return "", "", errors.New("Core LAN TLS requires public non-loopback API and connector names/IPs")
	}
	apiPort := apiURL.Port()
	if apiPort == "" {
		apiPort = "443"
	}
	for _, endpoint := range []struct {
		label  string
		listen string
		port   string
	}{{"API", cfg.ListenAddr, apiPort}, {"connector", cfg.ConnectorListenAddr, connectorPort}} {
		listenHost, listenPort, err := net.SplitHostPort(endpoint.listen)
		if err != nil {
			return "", "", fmt.Errorf("managed Core %s listener requires an explicit address: %w", endpoint.label, err)
		}
		ip := net.ParseIP(listenHost)
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
			return "", "", fmt.Errorf("managed Core %s listener requires a concrete non-loopback IP, not a hostname or wildcard", endpoint.label)
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
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("BASEHARBOR_MANAGED_CORE_TLS")), "true") {
		return nil
	}
	if cfg == nil || cfg.RuntimeAppName != "" || !cfg.ConnectorEnrollmentEnabled || cfg.ConnectorAuthorityTarget == "" {
		return errors.New("managed Core LAN TLS requires a locally owned Core with connector enrollment enabled")
	}
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" || cfg.ConnectorTLSCertFile != "" || cfg.ConnectorTLSKeyFile != "" || cfg.ConnectorTLSCAFile != "" {
		return errors.New("managed Core LAN TLS cannot override externally configured API/connector certificate material")
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
	if _, err := initialOperatorLocalCoreFiles(ownedCtx, target); err != nil {
		return fmt.Errorf("verify owned Core before managed LAN certificate issuance: %w", err)
	}
	runtime, err := detectRuntimeForTarget(ownedCtx, target)
	if err != nil {
		return err
	}
	files, err := existingTargetRuntimeFiles(ownedCtx)
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
	connector, err := issue("core-connector", connectorHost, serviceaccess.AuthenticationMTLS)
	if err != nil {
		return fmt.Errorf("issue Core connector serving TLS: %w", err)
	}
	cfg.TLSCertFile, cfg.TLSKeyFile = api.ServerCertificate, api.ServerKey
	cfg.ConnectorTLSCertFile, cfg.ConnectorTLSKeyFile, cfg.ConnectorTLSCAFile =
		connector.ServerCertificate, connector.ServerKey, connector.CA
	return nil
}
