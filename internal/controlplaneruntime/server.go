package controlplaneruntime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

var (
	ErrMissingDatabaseURL = errors.New("control-plane database URL is required")
	ErrMissingTLSFiles    = errors.New("control-plane TLS certificate and key are required")
)

type Config struct {
	ListenAddr               string
	DatabaseURL              string
	OIDCIssuer               string
	OIDCAudiences            []string
	TLSCertFile              string
	TLSKeyFile               string
	TLSClientCAFile          string
	RuntimeAppName           string
	RuntimeEnvironment       string
	RuntimeSecretsEnabled    bool
	RuntimeOpenBaoURL        string
	RuntimeCredentialsFile   string
	RuntimeTokenFile         string
	RuntimePermissionsFile   string
	RuntimeServiceTokensFile string
	RuntimeExecutorURL       string
	RuntimeExecutorCAFile    string
	RuntimeExecutorCertFile  string
	RuntimeExecutorKeyFile   string
	RuntimeOperationsDir     string
	RuntimeMetricsTargetsDir string
	RuntimeDocsListenAddr    string
	RuntimeBuildVersion      string
	RuntimeBuildCommit       string
	ShutdownTimeout          time.Duration
}

func (c Config) operatorAPIEnabled() bool {
	return strings.TrimSpace(c.OIDCIssuer) != "" || len(c.OIDCAudiences) > 0
}

func (c Config) boundRuntimeEnabled() bool {
	return strings.TrimSpace(c.RuntimeAppName) != ""
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.TLSCertFile) == "" || strings.TrimSpace(c.TLSKeyFile) == "" {
		return ErrMissingTLSFiles
	}
	if _, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile); err != nil {
		return fmt.Errorf("load control-plane TLS certificate: %w", err)
	}
	if c.boundRuntimeEnabled() {
		if c.operatorAPIEnabled() {
			return errors.New("per-application runtime broker cannot expose the operator API")
		}
		for label, value := range map[string]string{
			"runtime environment":           c.RuntimeEnvironment,
			"runtime token":                 c.RuntimeTokenFile,
			"runtime permissions":           c.RuntimePermissionsFile,
			"runtime client CA certificate": c.TLSClientCAFile,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s is required for a per-application runtime broker", label)
			}
		}
		for label, path := range map[string]string{
			"runtime token":       c.RuntimeTokenFile,
			"runtime permissions": c.RuntimePermissionsFile,
		} {
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("inspect %s: %w", label, err)
			}
		}
		if _, err := loadClientCAPool(c.TLSClientCAFile); err != nil {
			return err
		}
		if c.RuntimeSecretsEnabled {
			if strings.TrimSpace(c.RuntimeOpenBaoURL) == "" || strings.TrimSpace(c.RuntimeCredentialsFile) == "" {
				return errors.New("runtime OpenBao URL and AppRole credentials are required when runtime secrets are enabled")
			}
			if _, err := os.Stat(c.RuntimeCredentialsFile); err != nil {
				return fmt.Errorf("inspect runtime AppRole credentials: %w", err)
			}
			if _, err := openbao.NewApplicationRuntimeClient(c.RuntimeOpenBaoURL); err != nil {
				return err
			}
		} else if strings.TrimSpace(c.RuntimeOpenBaoURL) != "" || strings.TrimSpace(c.RuntimeCredentialsFile) != "" {
			return errors.New("runtime OpenBao configuration requires runtime secrets to be enabled")
		}
		return nil
	}
	if strings.TrimSpace(c.RuntimeDocsListenAddr) != "" {
		return errors.New("runtime docs listener requires a bound application runtime")
	}
	if strings.TrimSpace(c.RuntimeOpenBaoURL) != "" {
		if _, err := openbao.NewApplicationRuntimeClient(c.RuntimeOpenBaoURL); err != nil {
			return fmt.Errorf("validate runtime OpenBao endpoint: %w", err)
		}
	}
	if !c.operatorAPIEnabled() {
		return nil
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return ErrMissingDatabaseURL
	}
	return auth.Config{Issuer: c.OIDCIssuer, Audiences: c.OIDCAudiences}.Validate()
}

func (c Config) listenAddr() string {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return "127.0.0.1:8443"
	}
	return strings.TrimSpace(c.ListenAddr)
}

func (c Config) shutdownTimeout() time.Duration {
	if c.ShutdownTimeout <= 0 {
		return 10 * time.Second
	}
	return c.ShutdownTimeout
}

func Run(ctx context.Context, cfg Config, store application.Store) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	deps, err := prepareServerDependencies(ctx, cfg, store)
	if err != nil {
		return err
	}
	if deps.pool != nil {
		defer deps.pool.Close()
	}

	handler, err := buildServerHandler(ctx, cfg, deps)
	if err != nil {
		return err
	}
	server, err := newControlPlaneServer(cfg, handler)
	if err != nil {
		return err
	}
	docsServer, docsErrCh := startRuntimeDocsServer(cfg)
	return serveControlPlaneServers(ctx, cfg, server, docsServer, docsErrCh)
}

func runtimeTLSConfig(cfg Config) (*tls.Config, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if !cfg.boundRuntimeEnabled() {
		return config, nil
	}
	pool, err := loadClientCAPool(cfg.TLSClientCAFile)
	if err != nil {
		return nil, err
	}
	expectedURI := "spiffe://baseharbor/apps/" + cfg.RuntimeAppName + "/" + cfg.RuntimeEnvironment
	config.ClientAuth = tls.RequireAndVerifyClientCert
	config.ClientCAs = pool
	config.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("runtime client certificate is required")
		}
		for _, uri := range state.PeerCertificates[0].URIs {
			if uri.String() == expectedURI {
				return nil
			}
		}
		return errors.New("runtime client certificate identity does not match this application broker")
	}
	return config, nil
}

func loadClientCAPool(path string) (*x509.CertPool, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime client CA certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, errors.New("runtime client CA certificate is invalid")
	}
	return pool, nil
}

func writeNotReady(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("{\"status\":\"not_ready\"}\n"))
}
