package controlplaneruntime

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	metricsprovider "github.com/mcpdev80/baseharbor/internal/metrics"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/runtimeapidocs"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
	"github.com/mcpdev80/baseharbor/internal/runtimeobservability"
	"github.com/mcpdev80/baseharbor/internal/runtimeresourceapi"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
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
			client, err := openbao.NewApplicationRuntimeClientWithCA(cfg.RuntimeOpenBaoURL, cfg.RuntimeOpenBaoCAFile)
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

func buildServerHandler(ctx context.Context, cfg Config, deps serverDependencies, machineExecutor machinehttp.Executor) (http.Handler, error) {
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

	if err := registerOperatorAPI(ctx, mux, cfg, deps, machineExecutor); err != nil {
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
				writeNotReady(w, "database", cfg.RuntimeBuildVersion, cfg.RuntimeBuildCommit)
				return
			}
		}
		if deps.boundRuntimeClient != nil {
			if err := deps.boundRuntimeClient.Check(checkCtx, cfg.RuntimeCredentialsFile); err != nil {
				log.Printf("runtime broker readiness failed: dependency=openbao error=%v", err)
				writeNotReady(w, "openbao", cfg.RuntimeBuildVersion, cfg.RuntimeBuildCommit)
				return
			}
		}
		if deps.boundExecutorClient != nil {
			if err := deps.boundExecutorClient.Check(checkCtx); err != nil {
				log.Printf("runtime broker readiness failed: dependency=runtime-executor error=%v", err)
				writeNotReady(w, "runtime-executor", cfg.RuntimeBuildVersion, cfg.RuntimeBuildCommit)
				return
			}
		} else if cfg.boundRuntimeEnabled() {
			// The broker process is bound but no executor client was built.
			// The missing client is optional ONLY when the valid broker mounts
			// show that this broker genuinely has no runtime-executor
			// dependency: the executor environment and the in-container
			// client are rendered from the same valid broker mounts and agree
			// (no executor URL configured). That is the documented default
			// shape (e.g. a secrets-only broker), and /readyz must report
			// READY from the dependencies that exist, never a blanket 503.
			//
			// Any disagreement IS a genuinely broken required dependency:
			// a mounted permissions.json that grants a capability the broker
			// cannot route without the executor (bound_runtime.go requires it
			// for every served runtime call) while no executor URL was
			// configured, or a configured URL whose client could not be
			// built. Both stay fail-closed RED and name runtime-executor.
			if executorDependencyRequired(cfg) {
				log.Printf("runtime broker readiness failed: dependency=runtime-executor error=%s", executorDependencyReason(cfg))
				writeNotReady(w, "runtime-executor", cfg.RuntimeBuildVersion, cfg.RuntimeBuildCommit)
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

// brokerRequiresExecutor reports whether the broker's valid mounted
// configuration declares the shared runtime executor as a dependency: the
// executor URL is configured, or the mounted permissions projection grants at
// least one runtime capability whose operations the broker cannot serve
// in-process (everything except the metrics provider, see bound_runtime.go).
// This is the same authority the request router uses, so readiness and
// serving agree on whether runtime-executor is a required dependency. The
// check runs on every probe because the CLI renders the executor environment
// from a manifest gate and the in-container client from a different one; the
// published 0.4.26 edge image showed that asymmetry as a permanent
// not_ready. An unreadable permissions projection needs no executor of its
// own: Config.Validate has already refused to start the broker without the
// mount, so readiness never invents a dependency from an invalid file.
func brokerRequiresExecutor(cfg Config) bool {
	if strings.TrimSpace(cfg.RuntimeExecutorURL) != "" {
		return true
	}
	authorizer, err := runtimeresourceapi.NewStaticAuthorizer(cfg.RuntimeAppName, cfg.RuntimePermissionsFile)
	if err != nil {
		return false
	}
	for capabilityName := range authorizer.Capabilities() {
		if capabilityName != metricsprovider.RuntimeCapabilityV1 {
			return true
		}
	}
	return false
}

// executorDependencyRequired reports whether the runtime-executor is a
// required dependency of this bound broker even though no client was built.
// It is true when the mounted configuration declares the dependency (a
// configured executor URL) or the permissions grant capabilities that only
// the executor can serve, while no client exists to address it.
func executorDependencyRequired(cfg Config) bool {
	return brokerRequiresExecutor(cfg)
}

// executorDependencyReason names, in the log, why the missing executor client
// blocks readiness.
func executorDependencyReason(cfg Config) string {
	if strings.TrimSpace(cfg.RuntimeExecutorURL) != "" {
		return "the broker was configured with a runtime executor URL but no client could be built"
	}
	return "the mounted runtime permissions grant capabilities that require the runtime executor, but no executor was configured"
}

type connectorEnrollmentProvider interface {
	ConnectorEnrollmentHTTP(context.Context, targetenrollment.Store, string) (http.Handler, error)
}

type workstationEnrollmentProvider interface {
	WorkstationEnrollmentHTTP(context.Context, ws.Store, *pgxpool.Pool, string) (http.Handler, error)
}

// operatorOIDCVerifier builds the operator API token verifier.
// cfg.OperatorOIDCTrustFile adds the managed operator CA on top of the system
// trust pool (additive only), so `baha serve` verifies a managed-Keycloak
// issuer exactly like `baha login` does; an empty value keeps the previous
// system-trust-only behavior. TLS verification is never disabled.
func operatorOIDCVerifier(ctx context.Context, cfg Config) (*auth.OIDCVerifier, error) {
	return auth.NewOIDCVerifier(ctx, auth.Config{Issuer: cfg.OIDCIssuer, Audiences: cfg.OIDCAudiences, TrustFile: cfg.OperatorOIDCTrustFile})
}

func registerOperatorAPI(ctx context.Context, mux *http.ServeMux, cfg Config, deps serverDependencies, machineExecutor machinehttp.Executor) error {
	if !cfg.operatorAPIEnabled() {
		return nil
	}
	if deps.pool == nil {
		return ErrMissingDatabaseURL
	}
	verifier, err := operatorOIDCVerifier(ctx, cfg)
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
	operatorMux := http.NewServeMux()
	if cfg.ConnectorEnrollmentEnabled {
		provider, ok := machineExecutor.(connectorEnrollmentProvider)
		if !ok {
			return errors.New("Connector enrollment requires the Core enrollment provider")
		}
		handler, err := provider.ConnectorEnrollmentHTTP(ctx, database.NewConnectorEnrollmentStore(deps.pool), cfg.ConnectorAuthorityTarget)
		if err != nil {
			return err
		}
		// Only the scoped one-use bootstrap exchange bypasses operator OIDC.
		// Grant creation remains behind the existing identity/tenant middleware.
		mux.Handle(targetenrollment.EnrollmentPath, handler)
		operatorMux.Handle(targetenrollment.AuthorizationPath, handler)
		operatorMux.Handle(targetenrollment.OwnerContextPath, handler)
		operatorMux.Handle(targetenrollment.RenewalAuthorizationPath, handler)
		operatorMux.Handle(targetenrollment.StatusPath, handler)
		operatorMux.Handle(targetenrollment.DisconnectPath, handler)
	}
	if cfg.WorkstationEnrollmentEnabled {
		provider, ok := machineExecutor.(workstationEnrollmentProvider)
		if !ok {
			return errors.New("Workstation enrollment requires the Core workstation-enrollment provider")
		}
		handler, err := provider.WorkstationEnrollmentHTTP(ctx, database.NewWorkstationEnrollmentStore(deps.pool), deps.pool, cfg.ConnectorAuthorityTarget)
		if err != nil {
			return err
		}
		// Only an already minted, short-lived workstation grant may cross
		// the OIDC boundary. The public endpoint cannot create grants or
		// access machine/operator APIs, and consumes the exact grant once.
		delegated, ok := handler.(interface {
			ServeDelegatedEnroll(http.ResponseWriter, *http.Request)
		})
		if !ok {
			return errors.New("workstation enrollment is missing the one-use grant boundary")
		}
		mux.HandleFunc("/api/v1/workstations/enroll", delegated.ServeDelegatedEnroll)
		// Grant creation stays behind the existing operator/tenant policy.
		operatorMux.Handle("/api/v1/workstations/bootstrap", handler)
		operatorMux.Handle("/api/v1/workstations/enroll", handler)
	}
	// Installation identity is disclosed only after the standard OIDC and
	// tenant middleware has authenticated the calling operator. The executor
	// pins the Core at startup; request Targets cannot alter this value.
	if identity, ok := machineExecutor.(interface {
		ManagementInstallation(context.Context) (string, string, error)
	}); ok {
		operatorMux.HandleFunc("GET /api/v1/management/installation", func(w http.ResponseWriter, r *http.Request) {
			tenant, valid := tenancy.FromContext(r.Context())
			if !valid || tenant.TenantID == "" || tenant.ExternalIdentityID == "" {
				http.Error(w, "operator tenant is unavailable", http.StatusForbidden)
				return
			}
			installationID, issuer, err := identity.ManagementInstallation(r.Context())
			if err != nil || installationID == "" || issuer == "" {
				http.Error(w, "Core installation identity is unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(struct {
				Version        string `json:"version"`
				InstallationID string `json:"installation_id"`
				IdentityIssuer string `json:"identity_issuer"`
				OperatorIssuer string `json:"operator_issuer"`
				TenantID       string `json:"tenant_id"`
			}{Version: "baseharbor.management-installation/v1", InstallationID: installationID, IdentityIssuer: issuer, OperatorIssuer: cfg.OIDCIssuer, TenantID: tenant.TenantID})
		})
	}
	operatorMux.Handle("/api/v1/apps/", secretHandler)
	if machineExecutor != nil {
		// Production boundary fail-closed (review finding 3): the execution
		// journal must persist to a configured state-root. An empty ops-dir would
		// silently fall back to in-memory storage, which is not a durable
		// state-root, so refuse to start rather than accept a silent fallback.
		if strings.TrimSpace(cfg.RuntimeOperationsDir) == "" {
			return ErrMissingOperationsDir
		}
		machineHandler, err := machinehttp.New(machineExecutor, cfg.RuntimeOperationsDir)
		if err != nil {
			return err
		}
		operatorMux.Handle("/api/v1/machine/", machineHandler)
	}
	protected, err := controlplaneapi.New(security, operatorMux)
	if err != nil {
		return err
	}
	if cfg.WorkstationEnrollmentEnabled {
		revoker, ok := machineExecutor.(workstationRevocationReader)
		if !ok {
			return errors.New("Core requires workstation OpenBao CRL authority for device access")
		}
		device, err := workstationDeviceMiddleware(deps.pool, cfg.TLSClientCAFile, revoker, operatorMux)
		if err != nil {
			return err
		}
		mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only a client that deliberately presented an enrolled leaf and
			// did not supply a human OIDC Bearer token selects device auth.
			// All browser/OIDC calls retain exactly the existing middleware.
			if r.TLS != nil && len(r.TLS.PeerCertificates) != 0 &&
				r.Header.Get("Authorization") == "" {
				device.ServeHTTP(w, r)
				return
			}
			protected.ServeHTTP(w, r)
		}))
	} else {
		mux.Handle("/api/", protected)
	}
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

func serveControlPlaneServers(ctx context.Context, cfg Config, server, docsServer *http.Server, docsErrCh <-chan error, connectorErrors ...<-chan error) error {
	var connectorErrCh <-chan error
	if len(connectorErrors) > 0 {
		connectorErrCh = connectorErrors[0]
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

	shutdownDocs := func() error {
		if docsServer == nil {
			return nil
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout())
		defer cancel()
		return docsServer.Shutdown(shutdownCtx)
	}

	select {
	case err := <-connectorErrCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout())
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		_ = shutdownDocs()
		return err
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
