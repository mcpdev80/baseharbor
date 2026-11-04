package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// useTargetCommand exposes the existing guided/persisted target activation.
// The target command remains the single implementation of selection rules.
//
// The explicit selection is bound to the single production management-connection
// boundary: a managed connection must be verified (live, current, not foreign
// or revoked) before the selection can be acted on. The connection, Target and
// installation are three distinct authorities (coreinstallation.ConnectionSelection);
// a Target without a verified connection fails closed with a typed error and
// persists no selection.
func useTargetCommand() *cli.Command {
	return &cli.Command{
		Name:    "use",
		Summary: "Select and persist the active Core or deployment target",
		Usage:   "baha use [TARGET]",
		Long:    "Select a configured Target interactively or persist an explicit selection. Equivalent to baha target activate.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			for _, child := range targetCommand().Children {
				if child.Name == "activate" && child.Run != nil {
					return child.Run(ctx, args, out, errOut)
				}
			}
			return errors.New("target activation is unavailable")
		},
	}
}

// bindExplicitSelectionToConnection is the explicit-selection helper
// (cmd/baha/use_target.go). It builds a coreinstallation.ConnectionSelection for
// the explicitly selected Target and verifies the management connection through
// the single production boundary.
//
// The production issuer and its live observation are supplied by the caller via
// the serviceaccess.Issuer boundary; a nil issuer is a missing connection and
// fails closed before any effect. The installation ID comes from the genuine
// installation that coreinstallation.Load validates on the production path.
//
// Scope note (Owner 2026-11-04): the live management-connection identity is only
// resolvable through resolveManagementConnectionIssuer(ctx, resolvedApplication,
// workload) (cmd/baha/application_service_issuer.go:89), which requires a full
// resolvedApplication (TargetStateRoot + bound Core). The `use`/`activate`
// command resolves only a deployment Target name and cannot produce one without
// fabricating a resolvedApplication — a boundary rebuild this contract forbids.
// So on the bare `use`/`activate` path this helper resolves with a nil issuer,
// which fails closed; the live liveness/identity check runs on the
// application-lifecycle path where resolvedApplication exists.
func bindExplicitSelectionToConnection(
	ctx context.Context,
	target string,
	installationID string,
	issuer serviceaccess.Issuer,
	observed serviceaccess.CertificateStatus,
	authoritativeConnection string,
) error {
	selection := coreinstallation.ConnectionSelection{
		InstallationID: installationID,
		ConnectionID:   "managed-connection",
		TargetID:       target,
	}
	// 1. Distinctness of the three selections: a Target without an installation
	// is not a valid selection and fails closed.
	if selection.InstallationID == "" {
		return machine.NewError(
			machine.ErrorConflict,
			fmt.Sprintf("target %q has no verified management connection; the installation identity is required", target),
			"Install a BaseHarbor Core for this Target (baha core install) so the management connection is present, then re-run baha use.",
			false,
		)
	}
	// 2. Verify through the single production boundary; every failure fails
	// closed with the boundary's own sentinel mapped to a typed error.
	if err := selection.Resolve(ctx, issuer, observed, authoritativeConnection); err != nil {
		if errors.Is(err, coreinstallation.ErrMissingAuthority) {
			return machine.Wrap(machine.ErrorAuthenticationFailed, err, "Establish a live management connection for the selected installation.", false)
		}
		if errors.Is(err, coreinstallation.ErrForeignAuthority) {
			return machine.Wrap(machine.ErrorPolicyDenied, err, "The management connection identity does not match the selected installation; select the correct one.", false)
		}
		if errors.Is(err, coreinstallation.ErrNotReady) {
			return machine.Wrap(machine.ErrorRuntimeUnavailable, err, "The management connection is not verified as ready; reconcile the Core and retry.", false)
		}
		return machine.Wrap(machine.ErrorVerificationFailed, err, "The management connection did not verify; the selection persists nothing.", false)
	}
	return nil
}

// deploymentTargetStateRoot is a thin wrapper so the selection helper stays
// decoupled from the deployment package's state-root lookup.
func deploymentTargetStateRoot(name string) (string, error) {
	return deployment.TargetStateRoot(name)
}

// verifyConnectionSelectionForTarget is the production-lifecycle entry point. It
// loads the genuine installation for a resolved Target, builds the
// ConnectionSelection and verifies the connection through the single boundary.
// It is called on the application-lifecycle path (where resolvedApplication /
// issuer exist), NOT from the bare `use`/`activate` command.
func verifyConnectionSelectionForTarget(
	ctx context.Context,
	resolved deployment.ResolvedTarget,
	issuer serviceaccess.Issuer,
	observed serviceaccess.CertificateStatus,
	installationID string,
) error {
	selection := coreinstallation.ConnectionSelection{
		InstallationID: installationID,
		ConnectionID:   "managed-connection",
		TargetID:       resolved.Name,
	}
	return selection.Resolve(ctx, issuer, observed, installationID)
}
