package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// configureAutomaticOwnedCoreLAN is a default for a READY Core, never a
// migration of a BYOC server or an application-scoped broker. Existing TLS
// inputs and explicit enrollment configuration retain their authority.
func configureAutomaticOwnedCoreLAN(ctx context.Context, cfg *controlplaneruntime.Config) error {
	if cfg == nil {
		return errors.New("Core serving configuration is missing")
	}
	if cfg.RuntimeAppName != "" || cfg.ConnectorEnrollmentEnabled ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("BASEHARBOR_MANAGED_CORE_TLS")), "false") {
		return nil
	}
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" || cfg.TLSClientCAFile != "" {
		return nil
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		// A serve process without a selected local installation retains the
		// original explicit configuration/error behavior.
		return nil
	}
	if target.AccessProvider != "" && target.AccessProvider != "local" {
		return nil
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	state, err := coreinstallation.Load(root)
	if err != nil || !state.Ready || state.Spec.Target != target.Name {
		return nil
	}
	if cfg.ConnectorAuthorityTarget != "" && cfg.ConnectorAuthorityTarget != target.Name {
		return fmt.Errorf("Core enrollment authority %q differs from selected READY installation %q",
			cfg.ConnectorAuthorityTarget, target.Name)
	}
	cfg.ConnectorAuthorityTarget = target.Name
	cfg.WorkstationEnrollmentEnabled = true

	if strings.TrimSpace(cfg.ListenAddr) == "" {
		address, err := discoverOwnedCoreLANAddress()
		if err != nil {
			return err
		}
		cfg.ListenAddr = net.JoinHostPort(address, "18443")
	}
	if strings.TrimSpace(cfg.RuntimeOperationsDir) == "" {
		dataRoot, err := deployment.TargetStateRoot(target.Name)
		if err != nil {
			return err
		}
		cfg.RuntimeOperationsDir = filepath.Join(dataRoot, "machine-operations")
	}
	return nil
}
