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
			return controlplaneruntime.Run(ctx, cfg, store)
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
		AdminCredentials: os.Getenv("BASEHARBOR_EXECUTOR_S3_ADMIN_CREDENTIALS_FILE"),
		StateDir:         os.Getenv("BASEHARBOR_EXECUTOR_STATE_DIR"),
	}
}

func controlPlaneConfigFromEnv() (controlplaneruntime.Config, error) {
	audiences := splitNonEmpty(os.Getenv("BASEHARBOR_API_OIDC_AUDIENCES"))
	return controlplaneruntime.Config{
		ListenAddr:              os.Getenv("BASEHARBOR_API_LISTEN_ADDR"),
		DatabaseURL:             os.Getenv("BASEHARBOR_API_DATABASE_URL"),
		OIDCIssuer:              os.Getenv("BASEHARBOR_API_OIDC_ISSUER"),
		OIDCAudiences:           audiences,
		TLSCertFile:             os.Getenv("BASEHARBOR_API_TLS_CERT_FILE"),
		TLSKeyFile:              os.Getenv("BASEHARBOR_API_TLS_KEY_FILE"),
		TLSClientCAFile:         os.Getenv("BASEHARBOR_API_TLS_CLIENT_CA_FILE"),
		RuntimeAppName:          os.Getenv("BASEHARBOR_RUNTIME_APP_NAME"),
		RuntimeEnvironment:      os.Getenv("BASEHARBOR_RUNTIME_ENVIRONMENT"),
		RuntimeSecretsEnabled:   strings.EqualFold(strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_SECRETS_ENABLED")), "true"),
		RuntimeOpenBaoURL:       os.Getenv("BASEHARBOR_RUNTIME_OPENBAO_URL"),
		RuntimeCredentialsFile:  os.Getenv("BASEHARBOR_RUNTIME_OPENBAO_CREDENTIALS_FILE"),
		RuntimeTokenFile:        os.Getenv("BASEHARBOR_RUNTIME_TOKEN_FILE"),
		RuntimePermissionsFile:  os.Getenv("BASEHARBOR_RUNTIME_PERMISSIONS_FILE"),
		RuntimeServiceTokensFile: os.Getenv("BASEHARBOR_RUNTIME_SERVICE_TOKENS_FILE"),
		RuntimeExecutorURL:      os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_URL"),
		RuntimeExecutorCAFile:   os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_CA_FILE"),
		RuntimeExecutorCertFile: os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_CERT_FILE"),
		RuntimeExecutorKeyFile:  os.Getenv("BASEHARBOR_RUNTIME_EXECUTOR_KEY_FILE"),
		RuntimeOperationsDir:    os.Getenv("BASEHARBOR_RUNTIME_OPERATIONS_DIR"),
		RuntimeMetricsTargetsDir: os.Getenv("BASEHARBOR_RUNTIME_METRICS_TARGETS_DIR"),
		RuntimeDocsListenAddr:   os.Getenv("BASEHARBOR_RUNTIME_DOCS_LISTEN_ADDR"),
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
