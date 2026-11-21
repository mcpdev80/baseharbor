package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestOperatorAuthBoundaryRequiresOperatorForEveryEnvironment(t *testing.T) {
	t.Setenv(operatorauth.EnvIssuer, "")
	t.Setenv(operatorauth.EnvClientID, "")
	t.Setenv(operatorauth.EnvScopes, "")
	t.Setenv(operatorauth.EnvCallbackPort, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	ctx := operatorauth.WithEnforcement(context.Background())
	ctx = cli.WithOutputOptions(ctx, cli.OutputOptions{NonInteractive: true})

	for _, environment := range []string{"dev", "test", "prod"} {
		err := ensureOperatorAuthForBoundary(ctx, "local", environment)
		if !errors.Is(err, operatorauth.ErrConfigurationRequired) {
			t.Fatalf("%s must fail closed without operator OIDC configuration, got %v", environment, err)
		}
	}
}
