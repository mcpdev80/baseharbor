package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func ensureOperatorAuthForBoundary(ctx context.Context, target, environment string) error {
	if !operatorauth.ManagedEnvironment(environment) || !operatorauth.EnforcementEnabled(ctx) {
		return nil
	}
	cfg, err := resolveOperatorAuthBoundaryConfig(ctx, target, environment)
	if err != nil {
		return err
	}
	_, err = operatorauth.Ensure(ctx, target, environment, cfg)
	if err != nil {
		return err
	}
	if cfg.Provider == "managed-keycloak" {
		return finalizeManagedOperatorKeycloak(ctx, target, environment)
	}
	return nil
}

func resolveStoredOperatorAuthBoundary(target, environment string) (operatorauth.Config, bool, error) {
	stored, found, err := deployment.OperatorAuthForTargetEnvironment(target, environment)
	if err != nil {
		return operatorauth.Config{}, false, err
	}
	if !found {
		return operatorauth.Config{}, false, nil
	}
	cfg, err := operatorAuthRuntimeConfig(stored)
	if err != nil {
		return operatorauth.Config{}, false, err
	}
	return cfg, true, nil
}

func resolveOperatorAuthBoundaryConfig(ctx context.Context, target, environment string) (operatorauth.Config, error) {
	stored, found, err := deployment.OperatorAuthForTargetEnvironment(target, environment)
	if err != nil {
		return operatorauth.Config{}, err
	}
	if found {
		return operatorAuthRuntimeConfig(stored)
	}

	if envCfg, envErr := operatorauth.ConfigFromEnv(); envErr == nil {
		if persistErr := persistOperatorAuthBoundary(target, environment, envCfg); persistErr != nil {
			return operatorauth.Config{}, persistErr
		}
		return envCfg, nil
	}

	if noInput(ctx) || assumeYes(ctx) {
		return operatorauth.Config{}, operatorauth.ErrConfigurationRequired
	}
	interactive, ok := operatorauth.InteractiveFromContext(ctx)
	if !ok || interactive.In == nil {
		return operatorauth.Config{}, operatorauth.ErrConfigurationRequired
	}

	fmt.Fprintf(interactive.Out, "%s requires authenticated BaseHarbor operator access.\n", strings.ToUpper(environment))
	configure, err := promptYesNo(bufio.NewReader(interactive.In), interactive.Out, "Operator authentication is not configured. Configure it now?", true)
	if err != nil {
		return operatorauth.Config{}, err
	}
	if !configure {
		return operatorauth.Config{}, operatorauth.ErrConfigurationRequired
	}
	return bootstrapOperatorAuthBoundary(ctx, target, environment, interactive)
}

func bootstrapOperatorAuthBoundary(ctx context.Context, target, environment string, interactive operatorauth.Interactive) (operatorauth.Config, error) {
	reader := bufio.NewReader(interactive.In)
	fmt.Fprintln(interactive.Out, "Operator authentication:")
	fmt.Fprintln(interactive.Out, "  1. Managed/shared Keycloak")
	fmt.Fprintln(interactive.Out, "  2. External OIDC provider")
	choice, err := promptLine(reader, interactive.Out, "Selection", "1")
	if err != nil {
		return operatorauth.Config{}, err
	}

	var cfg operatorauth.Config
	switch strings.TrimSpace(choice) {
	case "1", "managed", "managed-keycloak", "keycloak":
		cfg, err = bootstrapManagedOperatorKeycloak(ctx, target, environment)
	case "2", "external", "external-oidc":
		cfg, err = bootstrapExternalOperatorOIDC(reader, interactive.Out)
	default:
		return operatorauth.Config{}, fmt.Errorf("unsupported operator authentication selection %q", choice)
	}
	if err != nil {
		return operatorauth.Config{}, err
	}
	if err := persistOperatorAuthBoundary(target, environment, cfg); err != nil {
		return operatorauth.Config{}, err
	}
	return cfg, nil
}

func bootstrapExternalOperatorOIDC(reader *bufio.Reader, out interface{ Write([]byte) (int, error) }) (operatorauth.Config, error) {
	issuer, err := promptLine(reader, out, "OIDC issuer", "")
	if err != nil {
		return operatorauth.Config{}, err
	}
	clientID, err := promptLine(reader, out, "OIDC native client ID", "")
	if err != nil {
		return operatorauth.Config{}, err
	}
	cfg := operatorauth.Config{
		Provider: "external-oidc",
		Issuer:   strings.TrimRight(strings.TrimSpace(issuer), "/"),
		ClientID: strings.TrimSpace(clientID),
		Scopes:   []string{"openid", "profile", "email"},
	}
	if err := cfg.Validate(); err != nil {
		return operatorauth.Config{}, err
	}
	return cfg, nil
}

func operatorAuthRuntimeConfig(stored deployment.OperatorAuthEnvironmentConfig) (operatorauth.Config, error) {
	cfg := operatorauth.Config{
		Provider:     strings.TrimSpace(stored.Provider),
		Issuer:       strings.TrimRight(strings.TrimSpace(stored.Issuer), "/"),
		ClientID:     strings.TrimSpace(stored.ClientID),
		Scopes:       append([]string(nil), stored.Scopes...),
		CallbackPort: stored.CallbackPort,
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email"}
	}
	if err := cfg.Validate(); err != nil {
		return operatorauth.Config{}, err
	}
	return cfg, nil
}

func persistOperatorAuthBoundary(target, environment string, cfg operatorauth.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	err := deployment.SetOperatorAuthForTargetEnvironment(target, environment, deployment.OperatorAuthEnvironmentConfig{
		Provider:     cfg.Provider,
		Issuer:       cfg.Issuer,
		ClientID:     cfg.ClientID,
		Scopes:       append([]string(nil), cfg.Scopes...),
		CallbackPort: cfg.CallbackPort,
	})
	if err != nil {
		if strings.TrimSpace(target) == "local" && strings.Contains(err.Error(), "not configured") {
			return errors.New("managed test/prod operator authentication requires an explicit BaseHarbor target; create one with 'baha target create' or configure OIDC through environment inputs for this invocation")
		}
		return err
	}
	return nil
}

func bootstrapManagedOperatorKeycloak(ctx context.Context, target, environment string) (operatorauth.Config, error) {
	configured, err := deployment.LoadConfig()
	if err != nil {
		return operatorauth.Config{}, err
	}
	resolvedTarget, err := configured.ResolveTarget(target, "")
	if err != nil {
		return operatorauth.Config{}, err
	}
	compose, err := detectRuntimeForTarget(ctx, resolvedTarget)
	if err != nil {
		return operatorauth.Config{}, err
	}
	platformFiles, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return operatorauth.Config{}, errors.New("managed operator Keycloak requires the BaseHarbor control plane to be ready; run 'baha up --control-plane-only' first or choose an external OIDC provider")
	}
	issuer := openbao.NewServiceIssuer(compose, platformFiles)
	status, err := issuer.Status(ctx)
	if err != nil || !status.Ready {
		if err != nil {
			return operatorauth.Config{}, fmt.Errorf("managed operator Keycloak requires ready BaseHarbor PKI: %w", err)
		}
		return operatorauth.Config{}, errors.New("managed operator Keycloak requires ready BaseHarbor PKI")
	}
	dataDir, err := deployment.TargetStateRoot(target)
	if err != nil {
		return operatorauth.Config{}, err
	}
	managed, err := identityprovider.EnsureManagedOperatorOIDC(ctx, compose, issuer, dataDir, target, target, environment)
	if err != nil {
		return operatorauth.Config{}, err
	}
	cfg := operatorauth.Config{
		Provider:     "managed-keycloak",
		Issuer:       managed.Issuer,
		ClientID:     managed.ClientID,
		Scopes:       []string{"openid", "profile", "email"},
		CallbackPort: managed.CallbackPort,
	}
	if err := cfg.Validate(); err != nil {
		return operatorauth.Config{}, err
	}
	return cfg, nil
}

func finalizeManagedOperatorKeycloak(ctx context.Context, target, environment string) error {
	configured, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	resolvedTarget, err := configured.ResolveTarget(target, "")
	if err != nil {
		return err
	}
	compose, err := detectRuntimeForTarget(ctx, resolvedTarget)
	if err != nil {
		return err
	}
	platformFiles, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return err
	}
	issuer := openbao.NewServiceIssuer(compose, platformFiles)
	dataDir, err := deployment.TargetStateRoot(target)
	if err != nil {
		return err
	}
	return identityprovider.FinalizeManagedOperatorOIDC(ctx, compose, issuer, dataDir, target, target, environment)
}
