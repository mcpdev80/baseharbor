package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/onboardingbundle"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// issueLocalWorkstationInvitation uses the protected, locally owned Core
// installation as the only first-party issuer of one-use workstation grants.
// There is no remote tenant or operator-login fallback. Only the local Core
// owner with access to the protected runtime files/database can reach this
// helper; it never returns bearer tokens in stdout or logs.
func issueLocalWorkstationInvitation(ctx context.Context, target deployment.ResolvedTarget, station, coreOrigin, outputFile string, certificateTTL time.Duration, ownerLogin string, user *identityprovider.ManagedOperatorUser, initialPassword []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := initialOperatorLocalCoreFiles(ctx, target); err != nil {
		return fmt.Errorf("only the protected local Core owner may issue workstation invitations: %w", err)
	}
	scope := ws.Scope{TenantID: target.TenantID, Station: station}
	if err := scope.Validate(); err != nil {
		return errors.New("Core needs a valid local installation tenant and workstation label before pairing")
	}
	// Fail early on malformed/untrusted output so a grant is not minted only
	// to be discarded during local validation.
	if strings.TrimSpace(coreOrigin) == "" || strings.TrimSpace(outputFile) == "" {
		return errors.New("managed Core HTTPS origin and private invitation file are required")
	}
	if _, err := os.Lstat(outputFile); err == nil {
		return errors.New("invitation output already exists; choose a new private file path")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect invitation output path: %w", err)
	}
	parent, err := os.Stat(filepath.Dir(outputFile))
	if err != nil || !parent.IsDir() || parent.Mode().Perm()&0o077 != 0 {
		return errors.New("invitation output directory must exist and be private (mode 0700 or stricter)")
	}
	check, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	provider, files, err := openBaoRuntime(check)
	if err != nil {
		return errors.New("managed local Core Secrets authority is not available")
	}
	status, err := platformopenbao.Inspect(check, provider, files)
	if err != nil || !status.Initialized || status.Sealed || platformopenbao.CheckManager(check, provider, files) != nil {
		return errors.New("managed local OpenBao must be initialized, unsealed and owned before invitation export")
	}
	issuer := platformopenbao.NewServiceIssuer(provider, files)
	trust, err := issuer.TrustBundle(check)
	if err != nil {
		return errors.New("managed Core CA trust is not available")
	}
	installationRoot, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	installation, err := coreinstallation.Load(installationRoot)
	if err != nil || !installation.Ready || installation.ID == "" {
		return errors.New("managed Core installation identity is not READY")
	}
	// Dummy but correctly shaped grant values validate the public transport
	// before the protected real grant is durably created.
	bundle := onboardingbundle.Bundle{
		ContractVersion:    onboardingbundle.Contract,
		CoreURL:            coreOrigin,
		CAPEM:              string(trust.PEM),
		InstallationID:     scope.TenantID,
		CoreInstallationID: installation.ID,
		CoreTarget:         target.Name,
		RuntimeProvider:    target.RuntimeProvider,
		Station:            scope.Station,
		Token:              strings.Repeat("A", 48),
		Nonce:              strings.Repeat("B", 48),
		ExpiresAt:          time.Now().Add(5 * time.Minute),
	}
	if err := bundle.Validate(time.Now()); err != nil {
		return fmt.Errorf("invalid managed Core invitation authority: %w", err)
	}
	dsn, err := locallyOwnedCorePostgresDSN(files)
	if err != nil {
		return err
	}
	pool, err := database.Open(check, database.Config{DSN: dsn, ConnectTimeout: 10 * time.Second})
	if err != nil {
		return errors.New("local Core enrollment database is not available")
	}
	defer pool.Close()
	// Existing Core installations may already have a tenant while still
	// carrying an older schema. Apply embedded migrations before creating a
	// grant so supported certificate lifetimes are accepted by PostgreSQL.
	if err := database.Migrate(check, pool); err != nil {
		return fmt.Errorf("prepare Core schema for workstation invitation: %w", err)
	}
	store := database.NewWorkstationEnrollmentStore(pool)
	if err := store.EnsureWorkstationNameAvailable(check, scope); err != nil {
		return errors.New("workstation name is already registered or has a pending invitation; choose a unique device name")
	}
	if user != nil {
		if ownerLogin == "" || user.Login != ownerLogin || user.ID != "" {
			return errors.New("new workstation owner identity is incomplete")
		}
		identityRuntime, ok := provider.(identityprovider.KeycloakRuntime)
		if !ok {
			return errors.New("selected runtime cannot manage the Core identity provider")
		}
		dataDir, err := deployment.TargetStateRoot(target.Name)
		if err != nil {
			return err
		}
		created, err := identityprovider.InviteManagedOperatorUser(check, identityRuntime, issuer, dataDir, target.Name, target.Name, onboardingEnvironment, user.Login, user.DisplayName, user.Email, string(initialPassword))
		if err != nil {
			return fmt.Errorf("create Keycloak user and set initial password: %w", err)
		}
		*user = created
		if err := store.RegisterWorkstationUser(check, target.TenantID, database.WorkstationUser{Login: user.Login, DisplayName: user.DisplayName, Email: user.Email, KeycloakID: user.ID}); err != nil {
			return fmt.Errorf("persist new user directory entry: %w", err)
		}
	}
	authority, err := ws.New(store, issuer, store)
	if err != nil {
		return err
	}
	grant, err := authority.CreateForOwner(check, scope, 5*time.Minute, certificateTTL, ownerLogin)
	if err != nil {
		return fmt.Errorf("issue single-use workstation authorization: %w", err)
	}
	bundle.Token = grant.Token
	bundle.Nonce = grant.Nonce
	bundle.ExpiresAt = grant.ExpiresAt
	if err := onboardingbundle.Write(outputFile, bundle, time.Now()); err != nil {
		return fmt.Errorf("secure invitation file not written; the unused grant expires automatically: %w", err)
	}
	return nil
}
