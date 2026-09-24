package runtimeexecutor

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

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/runtimeobservability"
)

type Config struct {
	ListenAddr       string
	TLSCertFile      string
	TLSKeyFile       string
	TLSClientCAFile  string
	S3Endpoint       string
	S3CAFile         string
	AdminCredentials string
	StateDir         string
	ShutdownTimeout  time.Duration
}

func (c Config) Validate() error {
	for label, value := range map[string]string{
		"TLS certificate":            c.TLSCertFile,
		"TLS private key":            c.TLSKeyFile,
		"TLS client CA":              c.TLSClientCAFile,
		"S3 endpoint":                c.S3Endpoint,
		"S3 trust bundle":            c.S3CAFile,
		"S3 admin credentials":       c.AdminCredentials,
		"runtime resource state dir": c.StateDir,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required for runtime executor", label)
		}
	}
	if _, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile); err != nil {
		return fmt.Errorf("load runtime executor TLS certificate: %w", err)
	}
	if _, err := loadCAPool(c.TLSClientCAFile); err != nil {
		return err
	}
	if _, err := loadCAPool(c.S3CAFile); err != nil {
		return fmt.Errorf("load runtime executor S3 trust bundle: %w", err)
	}
	if _, err := objectstorage.LoadContainerAdminCredentials(c.AdminCredentials); err != nil {
		return fmt.Errorf("load runtime executor S3 admin credentials: %w", err)
	}
	return nil
}

func (c Config) listenAddr() string {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return "0.0.0.0:9443"
	}
	return strings.TrimSpace(c.ListenAddr)
}

func (c Config) shutdownTimeout() time.Duration {
	if c.ShutdownTimeout <= 0 {
		return 10 * time.Second
	}
	return c.ShutdownTimeout
}

func Run(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	admin, err := objectstorage.LoadContainerAdminCredentials(cfg.AdminCredentials)
	if err != nil {
		return err
	}
	s3Roots, err := loadCAPool(cfg.S3CAFile)
	if err != nil {
		return err
	}
	s3Transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: s3Roots}}
	defer s3Transport.CloseIdleConnections()
	client := &http.Client{Transport: s3Transport, Timeout: 30 * time.Second}
	resources, err := objectstorage.NewRuntimeResourceManager(cfg.StateDir, cfg.S3Endpoint, client, admin)
	if err != nil {
		return err
	}
	handler, err := NewHandler(resources)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/internal/", handler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := resources.Check(checkCtx); err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "provider not ready")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	observer, err := runtimeobservability.NewFromEnvironment(runtimeobservability.Config{
		Component: "runtime-executor",
	})
	if err != nil {
		return fmt.Errorf("configure runtime executor observability: %w", err)
	}
	mux.Handle("GET /metrics", observer.MetricsHandler())

	pool, err := loadCAPool(cfg.TLSClientCAFile)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.listenAddr(),
		Handler:           observer.Wrap(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:       tls.VersionTLS12,
			ClientAuth:       tls.RequireAndVerifyClientCert,
			ClientCAs:        pool,
			VerifyConnection: verifyRuntimeClientIdentity,
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
			return fmt.Errorf("shutdown runtime executor: %w", err)
		}
		return <-errCh
	}
}

func verifyRuntimeClientIdentity(state tls.ConnectionState) error {
	if len(state.PeerCertificates) == 0 {
		return errors.New("runtime executor client certificate is required")
	}
	for _, identity := range state.PeerCertificates[0].URIs {
		if _, _, ok := parseWorkloadURI(identity); ok {
			return nil
		}
		if identity != nil && identity.String() == openbao.RuntimeExecutorObserverSPIFFE {
			return nil
		}
	}
	return errors.New("runtime executor client certificate must carry a BaseHarbor workload or observer SPIFFE identity")
}

func loadCAPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime executor client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, errors.New("runtime executor client CA is invalid")
	}
	return pool, nil
}
