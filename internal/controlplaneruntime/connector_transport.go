package controlplaneruntime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

func validateConnectorTransport(cfg Config) error {
	enabled := strings.TrimSpace(cfg.ConnectorListenAddr) != ""
	if !enabled {
		if cfg.ConnectorTLSCertFile != "" || cfg.ConnectorTLSKeyFile != "" || cfg.ConnectorTLSCAFile != "" {
			return errors.New("Connector TLS configuration requires the outbound-session listener")
		}
		return nil
	}
	if !cfg.ConnectorEnrollmentEnabled || cfg.boundRuntimeEnabled() || !cfg.operatorAPIEnabled() || cfg.DatabaseURL == "" {
		return errors.New("Connector sessions require protected Core enrollment, operator API and database")
	}
	_, _, err := connectorTLSConfiguration(cfg)
	return err
}

func connectorTLSConfiguration(cfg Config) (*tls.Config, string, error) {
	if cfg.ConnectorTLSCertFile == "" || cfg.ConnectorTLSKeyFile == "" || cfg.ConnectorTLSCAFile == "" {
		return nil, "", errors.New("Connector sessions require separate Core server certificate, key and node CA trust")
	}
	pair, err := tls.LoadX509KeyPair(cfg.ConnectorTLSCertFile, cfg.ConnectorTLSKeyFile)
	if err != nil || len(pair.Certificate) == 0 {
		return nil, "", errors.New("Connector Core TLS keypair is unavailable")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || leaf.IsCA || len(leaf.URIs) != 1 || !strings.HasPrefix(leaf.URIs[0].String(), "spiffe://baseharbor/platform/core/") ||
		!leaf.NotAfter.After(time.Now()) || leaf.NotBefore.After(time.Now()) || len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		return nil, "", errors.New("Connector Core server certificate requires one active Core SPIFFE identity and server-auth usage")
	}
	roots, err := loadClientCAPool(cfg.ConnectorTLSCAFile)
	if err != nil {
		return nil, "", errors.New("Connector node CA trust is unavailable")
	}
	configuration := &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, Certificates: []tls.Certificate{pair}, SessionTicketsDisabled: true}
	return configuration, leaf.URIs[0].String(), nil
}

type connectorSessionConsumer interface{ BindConnectorSessions(*targetsession.Pool) }

func startConnectorTransport(ctx context.Context, cfg Config, deps serverDependencies, executor machinehttp.Executor) (<-chan error, func(), error) {
	if cfg.ConnectorListenAddr == "" {
		return nil, func() {}, nil
	}
	consumer, ok := executor.(connectorSessionConsumer)
	if !ok || deps.pool == nil {
		return nil, nil, errors.New("Connector sessions require the authoritative Core executor and node registry")
	}
	configuration, identity, err := connectorTLSConfiguration(cfg)
	if err != nil {
		return nil, nil, err
	}
	configuration.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
		reloaded, current, err := connectorTLSConfiguration(cfg)
		if err != nil || current != identity {
			return nil, errors.New("Connector authority identity changed; restart with an explicitly selected authority")
		}
		return reloaded, nil
	}
	registry, err := targetenrollment.WithLiveTrust(database.NewConnectorEnrollmentStore(deps.pool),
		func(check context.Context) (*x509.CertPool, error) {
			if check.Err() != nil {
				return nil, check.Err()
			}
			return loadClientCAPool(cfg.ConnectorTLSCAFile)
		})
	if err != nil {
		return nil, nil, err
	}
	listener, err := net.Listen("tcp", cfg.ConnectorListenAddr)
	if err != nil {
		return nil, nil, errors.New("Core outbound Connector listener is unavailable")
	}
	pool := targetsession.NewPool()
	consumer.BindConnectorSessions(pool)
	lifetime, cancel := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() {
		finished <- pool.Serve(lifetime, listener, configuration, registry, identity)
	}()
	return finished, func() { cancel(); _ = listener.Close() }, nil
}
