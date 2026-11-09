package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// ensureLocalOnboardingTenant is an OS-owner bootstrap, never a replacement
// for verifying an OIDC browser user. It provisions the installation's tenant
// identity, not an external identity, membership, access token, or password.
// The existing local Core installation and its protected PostgreSQL credentials
// are a prerequisite; a remote/foreign Target cannot invoke this operation.
func ensureLocalOnboardingTenant(ctx context.Context, selected deployment.ResolvedTarget) (deployment.ResolvedTarget, error) {
	ownerCtx := withTargetOverride(ctx, selected.Name)
	files, err := initialOperatorLocalCoreFiles(ownerCtx, selected)
	if err != nil { return selected, err }
	dsn, err := locallyOwnedCorePostgresDSN(files)
	if err != nil { return selected, err }
	bootstrapCtx, cancel := context.WithTimeout(ownerCtx, 30*time.Second)
	defer cancel()
	pool, err := database.Open(bootstrapCtx, database.Config{DSN:dsn, ConnectTimeout:10*time.Second})
	if err != nil { return selected, errors.New("locally owned Core PostgreSQL bootstrap is unavailable") }
	defer pool.Close()
	if err := database.Migrate(bootstrapCtx, pool); err != nil {
		return selected, fmt.Errorf("prepare locally owned Core tenant schema: %w", err)
	}
	id, _, err := database.BootstrapLocalInstallationTenant(bootstrapCtx, pool, "baseharbor", "BaseHarbor")
	if err != nil { return selected, err }
	if strings.TrimSpace(selected.TenantID) != "" && selected.TenantID != id {
		return selected, errors.New("local Target tenant differs from authoritative Core tenancy; refusing enrollment")
	}
	cfg, expected, err := deployment.LoadConfigForMutation()
	if err != nil { return selected, err }
	def, ok := cfg.Targets[selected.Name]
	if !ok || def.Access.Reference == "" || def.Runtime.Provider != selected.RuntimeProvider {
		return selected, errors.New("locally owned Core Target changed during tenant bootstrap")
	}
	if def.TenantID != "" && def.TenantID != id {
		return selected, errors.New("another tenant is already bound to this Core Target")
	}
	if def.TenantID == "" {
		def.TenantID = id
		cfg.Targets[selected.Name] = def
		if err := cfg.SaveExpected(expected); err != nil {
			return selected, fmt.Errorf("persist protected local tenant binding: %w", err)
		}
	}
	selected.TenantID = id
	return selected, nil
}
