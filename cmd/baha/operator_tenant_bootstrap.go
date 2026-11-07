package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identity"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

var errInitialOperatorLocalOnly = errors.New("initial operator tenant requires the locally owned, ready management Core")

// initialOperatorLocalCoreFiles is an owner-only preflight. A selected remote
// Target, uninitialized Core or foreign root cannot grant first-tenant powers.
// Protected PostgreSQL bootstrap credentials are never sent to a remote node.
func initialOperatorLocalCoreFiles(ctx context.Context, target deployment.ResolvedTarget) (bhruntime.Files, error) {
	if target.AccessProvider != "" && target.AccessProvider != string(targetaccess.ProviderLocal) {
		return bhruntime.Files{}, errInitialOperatorLocalOnly
	}
	if err := checkBoundCoreTarget(ctx, target); err != nil {
		return bhruntime.Files{}, err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return bhruntime.Files{}, err
	}
	state, err := coreinstallation.Load(root)
	if err != nil || !state.Ready || state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
		return bhruntime.Files{}, errInitialOperatorLocalOnly
	}
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return bhruntime.Files{}, err
	}
	return files, nil
}

// initializeLocalOperatorTenant runs only after an OIDC code+PKCE login has
// returned a cryptographically verified principal. The caller explicitly opts
// in using --init-tenant, and must own the protected local Core installation.
func initializeLocalOperatorTenant(ctx context.Context, target deployment.ResolvedTarget, principal *identity.Principal, tenantSlug, tenantName string) (database.InitialOperatorTenant, error) {
	files, err := initialOperatorLocalCoreFiles(ctx, target)
	if err != nil {
		return database.InitialOperatorTenant{}, err
	}
	if err := database.ValidateInitialOperatorBootstrap(principal, tenantSlug, tenantName, time.Now()); err != nil {
		return database.InitialOperatorTenant{}, err
	}
	cfg, err := bhruntime.LoadConfig(files.Env)
	if err != nil {
		return database.InitialOperatorTenant{}, errors.New("protected Core PostgreSQL configuration is unavailable")
	}
	policy, err := serviceaccess.Resolve("prod", "control-plane-postgresql", serviceaccess.AuthenticationNative)
	if err != nil {
		return database.InitialOperatorTenant{}, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(filepath.Dir(files.Compose), "providers", "postgresql", "service-access", "pki"))
	if err != nil || strings.TrimSpace(material.CA) == "" {
		return database.InitialOperatorTenant{}, errors.New("verified Core PostgreSQL trust material is unavailable")
	}
	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.PostgresUser, cfg.PostgresPassword),
		Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.PostgresPort)),
		Path:   "/" + cfg.PostgresDB,
	}
	query := url.Values{}
	query.Set("sslmode", "verify-full")
	query.Set("sslrootcert", material.CA)
	dsn.RawQuery = query.Encode()
	pool, err := database.Open(ctx, database.Config{DSN: dsn.String(), ConnectTimeout: 10 * time.Second})
	if err != nil {
		return database.InitialOperatorTenant{}, errors.New("local operator bootstrap database connection failed; inspect protected Core diagnostics")
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return database.InitialOperatorTenant{}, fmt.Errorf("prepare initial operator schema: %w", err)
	}
	result, err := database.BootstrapInitialOperatorTenant(ctx, pool, principal, tenantSlug, tenantName)
	if err != nil {
		return database.InitialOperatorTenant{}, err
	}
	return result, nil
}
