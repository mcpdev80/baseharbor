package controlplaneruntime

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	"github.com/mcpdev80/baseharbor/internal/runtimeapidocs"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/runtimeobservability"
)

type serverDependencies struct {
	pool                  *pgxpool.Pool
	operatorSecretService *applicationsecret.Service
	runtimeSecrets        applicationruntimeapi.SecretService
	runtimeVerifier       applicationruntimeapi.RuntimeVerifier
	boundRuntimeClient    *openbao.ApplicationRuntimeClient
	boundExecutorClient   *runtimeexecutor.Client
}

func prepareServerDependencies(ctx context.Context, cfg Config, store application.Store) (serverDependencies, error) {
	deps := serverDependencies{}
	if strings.TrimSpace(cfg.DatabaseURL) != "" {
		pool, err := database.Open(ctx, database.Config{DSN: cfg.DatabaseURL, ConnectTimeout: 10 * time.Second})
		if err != nil {
			return serverDependencies{}, fmt.Errorf("open control-plane database: %w", err)
		}
		if err := database.VerifySchemaReady(ctx, pool); err != nil {
			pool.Close()
			return serverDependencies{}, fmt.Errorf("verify control-plane schema: %w", err)
		}
		deps.pool = pool
	}

	deps.operatorSecretService = applicationsecret.New(store)
	deps.runtimeSecrets = deps.operatorSecretService
	deps.runtimeVerifier = applicationruntimeauth.New(store)

	if cfg.boundRuntimeEnabled() {
		verifier, err := applicationruntimeauth.NewStatic(cfg.RuntimeAppName, cfg.RuntimeTokenFile)
		if err != nil {
			deps.close()
			return serverDependencies{}, err
		}
		deps.runtimeVerifier = verifier
		deps.runtimeSecrets = nil

		if cfg.RuntimeSecretsEnabled {
			client, err := openbao.NewApplicationRuntimeClient(cfg.RuntimeOpenBaoURL)
			if err != nil {
				deps.close()
				return serverDependencies{}, err
			}
			bound, err := applicationsecret.NewBoundRuntimeService(cfg.RuntimeAppName, cfg.RuntimeEnvironment, cfg.RuntimeCredentialsFile, client)
			if err != nil {
				deps.close()
				return serverDependencies{}, err
			}
			deps.boundRuntimeClient = client
			deps.runtimeSecrets = bound
		}
		if strings.TrimSpace(cfg.RuntimeExecutorURL) != "" {
			client, err := runtimeexecutor.NewClient(runtimeexecutor.ClientConfig{
				URL:      cfg.RuntimeExecutorURL,
				CAFile:   cfg.RuntimeExecutorCAFile,
				CertFile: cfg.RuntimeExecutorCertFile,
				KeyFile:  cfg.RuntimeExecutorKeyFile,
			})
			if err != nil {
				deps.close()
				return serverDependencies{}, err
			}
			deps.boundExecutorClient = client
		}
		return deps, nil
	}

	if strings.TrimSpace(cfg.RuntimeOpenBaoURL) != "" {
		client, err := openbao.NewApplicationRuntimeClient(cfg.RuntimeOpenBaoURL)
		if err != nil {
			deps.close()
			return serverDependencies{}, fmt.Errorf("create runtime OpenBao client: %w", err)
		}
		deps.runtimeSecrets = applicationsecret.NewRuntime(store, client)
	}
	return deps, nil
}

func (d serverDependencies) close() {
	if d.pool != nil {
		d.pool.Close()
	}
}

func buildServerHandler(ctx context.Context, cfg Config, deps serverDependencies) (http.Handler, error) {
	runtimeHandler, err := buildRuntimeHandler(ctx, cfg, deps)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	registerHealthHandlers(mux, cfg, deps)
	mux.Handle("/runtime/", runtimeHandler)

	var serverHandler http.Handler = mux
	if cfg.boundRuntimeEnabled() {
		observer, err := runtimeobservability.NewFromEnvironment(runtimeobservability.Config{
			Component:   "runtime-broker",
			Application: cfg.RuntimeAppName,
			Environment: cfg.RuntimeEnvironment,
		})
		if err != nil {
			return nil, fmt.Errorf("configure runtime broker observability: %w", err)
		}
		mux.Handle("GET /metrics", observer.MetricsHandler())
		serverHandler = observer.Wrap(mux)
	}

	if err := registerOperatorAPI(ctx, mux, cfg, deps); err != nil {
		return nil, err
	}
	return serverHandler, nil
}

func buildRuntimeHandler(ctx context.Context, cfg Config, deps serverDependencies) (http.Handler, error) {
	if cfg.boundRuntimeEnabled() {
		return buildBoundRuntimeHandler(ctx, cfg, deps.runtimeSecrets, deps.runtimeVerifier)
	}
	return applicationruntimeapi.New(deps.runtimeSecrets, deps.runtimeVerifier)
}

func registerHealthHandlers(mux *http.ServeMux, cfg Config, deps serverDependencies) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if deps.pool != nil {
			if err := database.Ping(checkCtx, deps.pool); err != nil {
				writeNotReady(w)
				return
			}
		}
		if deps.boundRuntimeClient != nil {
			if err := deps.boundRuntimeClient.Check(checkCtx, cfg.RuntimeCredentialsFile); err != nil {
				writeNotReady(w)
				return
			}
		}
		if deps.boundExecutorClient != nil {
			if err := deps.boundExecutorClient.Check(checkCtx); err != nil {
				writeNotReady(w)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := struct {
			Status  string `json:"status"`
			Version string `json:"version,omitempty"`
			Commit  string `json:"commit,omitempty"`
		}{Status: "ready"}
		if cfg.boundRuntimeEnabled() {
			response.Version = strings.TrimSpace(cfg.RuntimeBuildVersion)
			response.Commit = strings.TrimSpace(cfg.RuntimeBuildCommit)
		}
		_ = json.NewEncoder(w).Encode(response)
	})
}

func registerOperatorAPI(ctx context.Context, mux *http.ServeMux, cfg Config, deps serverDependencies) error {
	if !cfg.operatorAPIEnabled() {
		return nil
	}
	if deps.pool == nil {
		return ErrMissingDatabaseURL
	}
	verifier, err := auth.NewOIDCVerifier(ctx, auth.Config{Issuer: cfg.OIDCIssuer, Audiences: cfg.OIDCAudiences})
	if err != nil {
		return err
	}
	resolver := database.NewIdentityTenantResolver(deps.pool)
	security, err := httpsecurity.New(verifier, resolver)
	if err != nil {
		return err
	}
	secretHandler, err := applicationsecretapi.New(
		deps.operatorSecretService,
		database.NewApplicationOwnershipStore(deps.pool),
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
	return nil
}

func newControlPlaneServer(cfg Config, handler http.Handler) (*http.Server, error) {
	tlsConfig, err := runtimeTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &http.Server{
		Addr:              cfg.listenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		TLSConfig:         tlsConfig,
	}, nil
}

func startRuntimeDocsServer(cfg Config) (*http.Server, <-chan error) {
	addr := strings.TrimSpace(cfg.RuntimeDocsListenAddr)
	if addr == "" {
		return nil, nil
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           runtimeapidocs.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	ch := make(chan error, 1)
	go func() {
		err := server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			ch <- err
			return
		}
		ch <- nil
	}()
	return server, ch
}

func serveControlPlaneServers(ctx context.Context, cfg Config, server, docsServer *http.Server, docsErrCh <-chan error) error {
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	shutdownDocs := func() error {
		if docsServer == nil {
			return nil
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout())
		defer cancel()
		return docsServer.Shutdown(shutdownCtx)
	}

	select {
	case err := <-errCh:
		_ = shutdownDocs()
		return err
	case err := <-docsErrCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout())
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		if err != nil {
			return fmt.Errorf("runtime docs server: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout())
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown control-plane server: %w", err)
		}
		if err := shutdownDocs(); err != nil {
			return fmt.Errorf("shutdown runtime docs server: %w", err)
		}
		return <-errCh
	}
}
