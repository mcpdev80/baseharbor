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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationruntimeapi"
	"github.com/mcpdev80/baseharbor/internal/applicationruntimeauth"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/applicationsecretapi"
	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/authorization"
	"github.com/mcpdev80/baseharbor/internal/controlplaneapi"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/httpsecurity"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

var (
	ErrMissingDatabaseURL = errors.New("control-plane database URL is required")
	ErrMissingTLSFiles    = errors.New("control-plane TLS certificate and key are required")
)

type Config struct {
	ListenAddr             string
	DatabaseURL            string
	OIDCIssuer             string
	OIDCAudiences          []string
	TLSCertFile            string
	TLSKeyFile             string
	TLSClientCAFile        string
	RuntimeAppName         string
	RuntimeEnvironment     string
	RuntimeOpenBaoURL      string
	RuntimeCredentialsFile string
	RuntimeTokenFile       string
	ShutdownTimeout        time.Duration
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
			"runtime OpenBao URL":           c.RuntimeOpenBaoURL,
			"runtime AppRole credentials":   c.RuntimeCredentialsFile,
			"runtime token":                 c.RuntimeTokenFile,
			"runtime client CA certificate": c.TLSClientCAFile,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s is required for a per-application runtime broker", label)
			}
		}
		if _, err := os.Stat(c.RuntimeCredentialsFile); err != nil {
			return fmt.Errorf("inspect runtime AppRole credentials: %w", err)
		}
		if _, err := os.Stat(c.RuntimeTokenFile); err != nil {
			return fmt.Errorf("inspect runtime token: %w", err)
		}
		if _, err := loadClientCAPool(c.TLSClientCAFile); err != nil {
			return err
		}
		if _, err := openbao.NewApplicationRuntimeClient(c.RuntimeOpenBaoURL); err != nil {
			return err
		}
		return nil
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

	var pool *pgxpool.Pool
	if strings.TrimSpace(cfg.DatabaseURL) != "" {
		var err error
		pool, err = database.Open(ctx, database.Config{DSN: cfg.DatabaseURL, ConnectTimeout: 10 * time.Second})
		if err != nil {
			return fmt.Errorf("open control-plane database: %w", err)
		}
		defer pool.Close()
		if err := database.VerifySchemaReady(ctx, pool); err != nil {
			return fmt.Errorf("verify control-plane schema: %w", err)
		}
	}

	operatorSecretService := applicationsecret.New(store)
	var runtimeSecrets applicationruntimeapi.SecretService = operatorSecretService
	var runtimeVerifier applicationruntimeapi.RuntimeVerifier = applicationruntimeauth.New(store)
	var boundRuntimeClient *openbao.ApplicationRuntimeClient
	if cfg.boundRuntimeEnabled() {
		client, err := openbao.NewApplicationRuntimeClient(cfg.RuntimeOpenBaoURL)
		if err != nil {
			return err
		}
		bound, err := applicationsecret.NewBoundRuntimeService(cfg.RuntimeAppName, cfg.RuntimeEnvironment, cfg.RuntimeCredentialsFile, client)
		if err != nil {
			return err
		}
		verifier, err := applicationruntimeauth.NewStatic(cfg.RuntimeAppName, cfg.RuntimeTokenFile)
		if err != nil {
			return err
		}
		boundRuntimeClient = client
		runtimeSecrets = bound
		runtimeVerifier = verifier
	} else if strings.TrimSpace(cfg.RuntimeOpenBaoURL) != "" {
		client, err := openbao.NewApplicationRuntimeClient(cfg.RuntimeOpenBaoURL)
		if err != nil {
			return fmt.Errorf("create runtime OpenBao client: %w", err)
		}
		runtimeSecrets = applicationsecret.NewRuntime(store, client)
	}
	runtimeHandler, err := applicationruntimeapi.New(runtimeSecrets, runtimeVerifier)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if pool != nil {
			if err := database.Ping(checkCtx, pool); err != nil {
				writeNotReady(w)
				return
			}
		}
		if boundRuntimeClient != nil {
			if err := boundRuntimeClient.Check(checkCtx, cfg.RuntimeCredentialsFile); err != nil {
				writeNotReady(w)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ready\"}\n"))
	})
	mux.Handle("/runtime/", runtimeHandler)

	if cfg.operatorAPIEnabled() {
		if pool == nil {
			return ErrMissingDatabaseURL
		}
		verifier, err := auth.NewOIDCVerifier(ctx, auth.Config{Issuer: cfg.OIDCIssuer, Audiences: cfg.OIDCAudiences})
		if err != nil {
			return err
		}
		resolver := database.NewIdentityTenantResolver(pool)
		security, err := httpsecurity.New(verifier, resolver)
		if err != nil {
			return err
		}
		secretHandler, err := applicationsecretapi.New(
			operatorSecretService,
			database.NewApplicationOwnershipStore(pool),
			authorization.NewService(),
		)
		if err != nil {
			return err
		}
		protected, err := controlplaneapi.New(security, secretHandler)
		if err != nil {
			return err
		}
		mux.Handle("/api/", protected)
	}

	tlsConfig, err := runtimeTLSConfig(cfg)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.listenAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		TLSConfig:         tlsConfig,
	}

	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout())
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown control-plane server: %w", err)
		}
		return <-errCh
	}
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
