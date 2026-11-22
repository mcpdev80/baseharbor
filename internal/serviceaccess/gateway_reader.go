package serviceaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Managed service-access gateway READER material (0428-091 P2). The gateway
// reconciliation in http_gateway.go projects the boundary's TLS material
// container-readable; this file owns what consumers of that projection may
// resolve: the authoritative reader view (GatewayReaderTLSMaterial) and the
// gateway access-alias naming, including the test-scoped alias dial hook the
// service-access transport consults.

// GatewayReaderTLSMaterial resolves a READER's service-access material from
// the gateway itself: the container-readable runtime projection (ca.pem,
// server.pem, server-key.pem and, wherever an identity is enrolled,
// client-cert.pem/client-key.pem) that EnsureHTTPGateway reconciled and the
// compose service mounts. It is authoritative for consumers of a MANAGED-
// LOCAL gateway: the projection is regenerated from the protected pki/ source
// on every reconcile, so it — never a stale source-tree file — names the
// identity the CURRENT boundary presents and trusts. A gateway whose
// reconciliation raised the posture to require client certificates
// (RequireClient specs in environments whose policy default marks
// authentication optional) carries its enrolled pair here; a projection
// without the required pair fails closed. An external or BYOC source is
// resolved by ExistingTLSMaterial from the operator's policy paths instead,
// unchanged. serverName is the identity the reader pins; requireClient is the
// client duty the reader's policy imposes.
func GatewayReaderTLSMaterial(serviceAccessDir, serverName string, requireClient bool) (TLSMaterial, error) {
	// The projection IS the reconciled gateway: a runtime tree without the
	// server pair it mounts is not reconciled material, and a reader must
	// never resolve a half-projected boundary.
	if _, err := os.Stat(filepath.Join(serviceAccessDir, "runtime", "server.pem")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TLSMaterial{}, ErrGatewayMaterialNotReconciled
		}
		return TLSMaterial{}, err
	}
	stateData, err := os.ReadFile(filepath.Join(serviceAccessDir, "state.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TLSMaterial{}, ErrGatewayMaterialNotReconciled
		}
		return TLSMaterial{}, fmt.Errorf("read the service-access gateway state: %w", err)
	}
	var state gatewayState
	if err := json.Unmarshal(stateData, &state); err != nil {
		return TLSMaterial{}, fmt.Errorf("decode the service-access gateway state: %w", err)
	}
	// A gateway that has EVER enforced client authentication keeps enforcing
	// it (the posture raise above and reconcileGatewayAuthentication both pin
	// that): a reader may never present LESS than the boundary demands.
	material := TLSMaterial{
		Source:            PKIManagedLocal,
		CA:                filepath.Join(serviceAccessDir, "runtime", "ca.pem"),
		ServerCertificate: filepath.Join(serviceAccessDir, "runtime", "server.pem"),
		ServerKey:         filepath.Join(serviceAccessDir, "runtime", "server-key.pem"),
		ServerName:        strings.TrimSpace(serverName),
	}
	clientDuty := requireClient || (state.AuthenticationRequired && state.Authentication == AuthenticationMTLS)
	if clientDuty {
		material.ClientCertificate = filepath.Join(serviceAccessDir, "runtime", "client-cert.pem")
		material.ClientKey = filepath.Join(serviceAccessDir, "runtime", "client-key.pem")
	}
	if err := validateMaterial(material, clientDuty); err != nil {
		return TLSMaterial{}, err
	}
	return material, nil
}

// ManagedGatewayAccessAlias is the fully-qualified backend-network alias of a
// managed service-access gateway. The service-access transport resolves it
// through managedGatewayAliasPort, which a test binary may map onto an
// in-process stand-in listener (SetManagedGatewayAliasResolverForTests) so the
// alias is dialed BY NAME with production TLS verification. Production
// resolution stays the container runtime's DNS.
func ManagedGatewayAccessAlias(gatewayServiceName string) string {
	return strings.TrimSpace(gatewayServiceName) + ".access.baseharbor.internal"
}

// ErrGatewayMaterialNotReconciled reports that no gateway material exists at
// the reference: the gateway this reader addresses has never been reconciled,
// so no authorized boundary identity exists to read.
var ErrGatewayMaterialNotReconciled = errors.New("service-access gateway material is not reconciled")

// SetManagedGatewayAliasResolverForTests maps one managed gateway access
// alias onto a loopback TCP port for the duration of a test and returns the
// restore function. The table is a test instrument by construction: only this
// function writes it, it maps only the alias of the named gateway service, and
// a production deployment never calls it — resolution there is the container
// runtime's DNS, exactly as the compose rendering registers the alias.
func SetManagedGatewayAliasResolverForTests(gatewayServiceName string, port int) (restore func()) {
	alias := strings.ToLower(ManagedGatewayAccessAlias(gatewayServiceName))
	managedGatewayAliasMu.Lock()
	previous := managedGatewayAliasOverrides[alias]
	had := managedGatewayAliasOverrides
	if had == nil {
		had = map[string]int{}
	}
	had[alias] = port
	managedGatewayAliasOverrides = had
	managedGatewayAliasMu.Unlock()
	return func() {
		managedGatewayAliasMu.Lock()
		defer managedGatewayAliasMu.Unlock()
		if previous == 0 {
			delete(managedGatewayAliasOverrides, alias)
			return
		}
		managedGatewayAliasOverrides[alias] = previous
	}
}

// managedGatewayAliasOverrides is the alias override table the service-access
// dialer consults.
var (
	managedGatewayAliasMu        sync.RWMutex
	managedGatewayAliasOverrides map[string]int
)

func managedGatewayAliasPort(host string) (int, bool) {
	managedGatewayAliasMu.RLock()
	defer managedGatewayAliasMu.RUnlock()
	port, ok := managedGatewayAliasOverrides[strings.ToLower(strings.TrimSuffix(host, "."))]
	return port, ok
}

// managedGatewayDialContext resolves the managed gateway alias through the
// test override table when one is installed, and through the system resolver
// otherwise (production: the container runtime's DNS).
func managedGatewayDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if host, _, err := net.SplitHostPort(address); err == nil {
		if mapped, ok := managedGatewayAliasPort(host); ok {
			// The alias stands an in-process gateway: dial the loopback
			// ADDRESS at the mapped port. The TLS ServerName stays the alias
			// the transport pinned (it comes from the tls.Config, not the
			// dial target), so the handshake still verifies the managed
			// certificate for the alias name.
			address = net.JoinHostPort("127.0.0.1", strconv.Itoa(mapped))
		}
	}
	return dialer.DialContext(ctx, network, address)
}
