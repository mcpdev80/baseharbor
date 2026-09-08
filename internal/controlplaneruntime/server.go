package controlplaneruntime

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/applicationsecretapi"
	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/authorization"
	"github.com/mcpdev80/baseharbor/internal/controlplaneapi"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/httpsecurity"
)

var (
	ErrMissingDatabaseURL = errors.New("control-plane database URL is required")
	ErrMissingTLSFiles    = errors.New("control-plane TLS certificate and key are required")
)

type Config struct {
	ListenAddr      string
	DatabaseURL     string
	OIDCIssuer      string
	OIDCAudiences   []string
	TLSCertFile     string
	TLSKeyFile      string
	ShutdownTimeout time.Duration
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return ErrMissingDatabaseURL
	}
	if strings.TrimSpace(c.TLSCertFile) == "" || strings.TrimSpace(c.TLSKeyFile) == "" {
		return ErrMissingTLSFiles
	}
	if _, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile); err != nil {
		return fmt.Errorf("load control-plane TLS certificate: %w", err)
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

	pool, err := database.Open(ctx, database.Config{DSN: cfg.DatabaseURL, ConnectTimeout: 10 * time.Second})
	if err != nil {
		return fmt.Errorf("open control-plane database: %w", err)
	}
	defer pool.Close()
	if err := database.VerifySchemaReady(ctx, pool); err != nil {
		return fmt.Errorf("verify control-plane schema: %w", err)
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
		applicationsecret.New(store),
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := database.Ping(checkCtx, pool); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("{\"status\":\"not_ready\"}\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ready\"}\n"))
	})
	mux.Handle("/api/", protected)

	server := &http.Server{
		Addr:              cfg.listenAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
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
