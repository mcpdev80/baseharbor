package main

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
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
			cfg, err := controlPlaneConfigFromEnv()
			if err != nil {
				return err
			}
			return controlplaneruntime.Run(ctx, cfg, store)
		},
	}
}

func controlPlaneConfigFromEnv() (controlplaneruntime.Config, error) {
	audiences := splitNonEmpty(os.Getenv("BASEHARBOR_API_OIDC_AUDIENCES"))
	return controlplaneruntime.Config{
		ListenAddr:             os.Getenv("BASEHARBOR_API_LISTEN_ADDR"),
		DatabaseURL:            os.Getenv("BASEHARBOR_API_DATABASE_URL"),
		OIDCIssuer:             os.Getenv("BASEHARBOR_API_OIDC_ISSUER"),
		OIDCAudiences:          audiences,
		TLSCertFile:            os.Getenv("BASEHARBOR_API_TLS_CERT_FILE"),
		TLSKeyFile:             os.Getenv("BASEHARBOR_API_TLS_KEY_FILE"),
		TLSClientCAFile:        os.Getenv("BASEHARBOR_API_TLS_CLIENT_CA_FILE"),
		RuntimeAppName:         os.Getenv("BASEHARBOR_RUNTIME_APP_NAME"),
		RuntimeEnvironment:     os.Getenv("BASEHARBOR_RUNTIME_ENVIRONMENT"),
		RuntimeOpenBaoURL:      os.Getenv("BASEHARBOR_RUNTIME_OPENBAO_URL"),
		RuntimeCredentialsFile: os.Getenv("BASEHARBOR_RUNTIME_OPENBAO_CREDENTIALS_FILE"),
		RuntimeTokenFile:       os.Getenv("BASEHARBOR_RUNTIME_TOKEN_FILE"),
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
