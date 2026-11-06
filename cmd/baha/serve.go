package main

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/connectivityrelay"
	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
)

func serveCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "serve",
		Summary: "Run the TLS-protected BaseHarbor runtime/control-plane API",
		Usage:   "baha serve",
		Long:    "Starts the TLS-protected BaseHarbor application runtime API. A managed per-application broker is bound to one app/environment, requires mTLS plus the app runtime token, and authenticates to OpenBao only with that application's AppRole. Configure OIDC audiences plus a database URL only for the separate operator control-plane API.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha serve does not accept arguments", "Run 'baha serve --help' for usage.")
			}
			if strings.EqualFold(strings.TrimSpace(os.Getenv("BASEHARBOR_CONNECTIVITY_RELAY_MODE")), "true") {
				return connectivityrelay.Run(ctx, connectivityrelay.Config{
					ListenAddr: os.Getenv("BASEHARBOR_RELAY_LISTEN_ADDR"),
					TargetAddr: os.Getenv("BASEHARBOR_RELAY_TARGET_ADDR"),
					HealthAddr: os.Getenv("BASEHARBOR_RELAY_HEALTH_ADDR"),
				})
			}
			if strings.EqualFold(strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_MODE")), "true") {
				return runtimeexecutor.Run(ctx, runtimeExecutorConfigFromEnv())
			}
			cfg, err := controlPlaneConfigFromEnv()
			if err != nil {
				return err
			}
			executor := newBahaMachineExecutor(store)
			var startup context.Context
			if cfg.RuntimeAppName == "" {
				startup = ctx
				if cfg.ConnectorEnrollmentEnabled {
					startup = withTargetOverride(startup, cfg.ConnectorAuthorityTarget)
				}
				executor, err = newInstallationMachineExecutor(startup, store)
				if err != nil {
					return err
				}
			}
			// Durable management host: prove this process is the single active
			// writer of one durable installation state root before the
			// control-plane server binds. A missing root is a bootstrap: the
			// caller provisions a fresh durable installation while holding an
			// atomic bootstrap lock, so a second competing fresh bootstrap is
			// refused with ErrManagementHostOwnedFreshRoot. A foreign or locked
			// root fails closed with no local fallback. The verified state root
			// is pinned into the startup context that reaches the executor and
			// server below.
			host, herr := managementHostForServe(ctx)
			if herr != nil {
				// A real target/root resolution error must fail closed, not
				// start the server unverified. Only the intentional (nil, nil)
				// no-host case (a remote or non-management target) passes
				// through, and that is a nil herr and a nil host.
				return herr
			}
			if host != nil {
				bootstrap, unlock, verr := host.Verify()
				if verr != nil {
					if unlock != nil {
						unlock()
					}
					return verr
				}
				if bootstrap {
					// Bootstrap: retain EXCLUSIVE ownership of the fresh durable
					// state root across provisioning and the bind. The bootstrap
					// lock (distinct kernel lock from the lifecycle lock) is held
					// here; the management-host test proves a second competing
					// fresh bootstrap is refused with ErrManagementHostOwnedFreshRoot.
					// The caller provisions the fresh durable installation on this
					// root while the lock is held, then the server becomes the
					// durable authority and holds the kernel lifecycle lock for its
					// whole lifetime, so an independent writer stays excluded while
					// lifecycle operations routed through this host execute on the
					// owned authority (see coreInstallUnderManagedHost). The lock is
					// released on server exit via the deferred unlock, so bootstrap
					// ownership transitions to lifetime ownership with no unowned
					// gap between provisioning and bind.
					defer func() { unlock() }()
				} else {
					// Attach: pin the owned durable state root into the startup
					// context so the executor and server never select a
					// different root than the one this host owns.
					startup = withManagementHostAttached(startup, host.StateRoot())
				}
			}
			// Pin the startup context (management-host attachment, connector
			// override, authority) through controlplaneruntime.Run instead of
			// the original ctx, so the selected root reaches every consumer.
			return controlplaneruntime.Run(startup, cfg, store, executor)
		},
	}
}

func runtimeExecutorConfigFromEnv() runtimeexecutor.Config {
	return runtimeexecutor.Config{
		ListenAddr:       os.Getenv("BASEHARBOR_EXECUTOR_LISTEN_ADDR"),
		TLSCertFile:      os.Getenv("BASEHARBOR_EXECUTOR_TLS_CERT_FILE"),
		TLSKeyFile:       os.Getenv("BASEHARBOR_EXECUTOR_TLS_KEY_FILE"),
		TLSClientCAFile:  os.Getenv("BASEHARBOR_EXECUTOR_TLS_CLIENT_CA_FILE"),
		S3Endpoint:       os.Getenv("BASEHARBOR_EXECUTOR_S3_ENDPOINT"),
		S3CAFile:         os.Getenv("BASEHARBOR_EXECUTOR_S3_CA_FILE"),
		AdminCredentials: os.Getenv("BASEHARBOR_EXECUTOR_S3_ADMIN_CREDENTIALS_FILE"),
		StateDir:         os.Getenv("BASEHARBOR_EXECUTOR_STATE_DIR"),
	}
}

func controlPlaneConfigFromEnv() (controlplaneruntime.Config, error) {
	audiences := splitNonEmpty(os.Getenv("BASEHARBOR_API_OIDC_AUDIENCES"))
	return controlplaneruntime.Config{
		ListenAddr:                 os.Getenv("BASEHARBOR_API_LISTEN_ADDR"),
		DatabaseURL:                os.Getenv("BASEHARBOR_API_DATABASE_URL"),
		ConnectorEnrollmentEnabled: strings.EqualFold(strings.TrimSpace(os.Getenv("BASEHARBOR_CONNECTOR_ENROLLMENT_ENABLED")), "true"),
		ConnectorAuthorityTarget:   os.Getenv("BASEHARBOR_CONNECTOR_AUTHORITY_TARGET"),
		ConnectorListenAddr:        os.Getenv("BASEHARBOR_CONNECTOR_LISTEN_ADDR"),
		ConnectorTLSCertFile:       os.Getenv("BASEHARBOR_CONNECTOR_TLS_CERT_FILE"),
		ConnectorTLSKeyFile:        os.Getenv("BASEHARBOR_CONNECTOR_TLS_KEY_FILE"),
		ConnectorTLSCAFile:         os.Getenv("BASEHARBOR_CONNECTOR_TLS_CA_FILE"),
		OIDCIssuer:                 os.Getenv("BASEHARBOR_API_OIDC_ISSUER"),
		OIDCAudiences:              audiences,
		TLSCertFile:                os.Getenv("BASEHARBOR_API_TLS_CERT_FILE"),
		TLSKeyFile:                 os.Getenv("BASEHARBOR_API_TLS_KEY_FILE"),
		TLSClientCAFile:            os.Getenv("BASEHARBOR_API_TLS_CLIENT_CA_FILE"),
		RuntimeAppName:             os.Getenv("BASEHARBOR_RUNTIME_APP_NAME"),
		RuntimeEnvironment:         os.Getenv("BASEHARBOR_RUNTIME_ENVIRONMENT"),
		RuntimeSecretsEnabled:      strings.EqualFold(strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_SECRETS_ENABLED")), "true"),
		RuntimeOpenBaoURL:          os.Getenv("BASEHARBOR_RUNTIME_OPENBAO_URL"),
		RuntimeOpenBaoCAFile:       os.Getenv("BASEHARBOR_RUNTIME_OPENBAO_CA_FILE"),
		RuntimeCredentialsFile:     os.Getenv("BASEHARBOR_RUNTIME_OPENBAO_CREDENTIALS_FILE"),
		RuntimeTokenFile:           os.Getenv("BASEHARBOR_RUNTIME_TOKEN_FILE"),
		RuntimePermissionsFile:     os.Getenv("BASEHARBOR_RUNTIME_PERMISSIONS_FILE"),
		RuntimeServiceTokensFile:   os.Getenv("BASEHARBOR_RUNTIME_SERVICE_TOKENS_FILE"),
		RuntimeExecutorURL:         os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_URL"),
		RuntimeExecutorCAFile:      os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_CA_FILE"),
		RuntimeExecutorCertFile:    os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_CERT_FILE"),
		RuntimeExecutorKeyFile:     os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_KEY_FILE"),
		RuntimeOperationsDir:       os.Getenv("BASEHARBOR_RUNTIME_OPERATIONS_DIR"),
		RuntimeMetricsTargetsDir:   os.Getenv("BASEHARBOR_RUNTIME_METRICS_TARGETS_DIR"),
		RuntimeDocsListenAddr:      os.Getenv("BASEHARBOR_RUNTIME_DOCS_LISTEN_ADDR"),
		RuntimeBuildVersion:        version,
		RuntimeBuildCommit:         commit,
	}, nil
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
