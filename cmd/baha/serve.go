package main

import (
	"context"
	"errors"
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
		Summary: "Run the TLS-protected BaseHarbor control-plane API",
		Usage:   "baha serve",
		Long:    "Start the BaseHarbor control-plane HTTP API after PostgreSQL, OIDC discovery, TLS certificate loading and security dependency preflight succeed.",
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
	cfg := controlplaneruntime.Config{
		ListenAddr:    os.Getenv("BASEHARBOR_API_LISTEN_ADDR"),
		DatabaseURL:   os.Getenv("BASEHARBOR_API_DATABASE_URL"),
		OIDCIssuer:    os.Getenv("BASEHARBOR_API_OIDC_ISSUER"),
		OIDCAudiences: audiences,
		TLSCertFile:   os.Getenv("BASEHARBOR_API_TLS_CERT_FILE"),
		TLSKeyFile:    os.Getenv("BASEHARBOR_API_TLS_KEY_FILE"),
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return controlplaneruntime.Config{}, errors.New("BASEHARBOR_API_DATABASE_URL is required")
	}
	return cfg, nil
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
