package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
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

// bindExplicitSelectionToConnection resolves the genuine installation for the
// explicitly selected Target and verifies its management connection through the
// single production boundary (cmd/baha/use_target.go). It is the production
// entry point wired into `baha target activate`: it must run before any
// selection is persisted, so a Target without a verified management connection
// fails closed and persists no selection.
//
// The live management-connection identity is derived from the VERIFIED
// INSTALLATION plus the Target's own protected state through the reading
// resolver coreinstallation.ResolverForTargetSelection (card #1386, accepted
// #1386): it needs no resolvedApplication and therefore no application context,
// so it is reachable from the bare `use`/`activate` path that resolves only a
// Target name. The verified installation is loaded here through
// coreinstallation.Load on the Target state root; a Target that holds no
// genuine, verified Core installation fails closed with a typed conflict before
// any issuer material is produced.
//
// The installation and the Target are two distinct authorities (the management
// connection is the third, verified through the single boundary): a Target
// selection without a verified installation selects no local execution.
func bindExplicitSelectionToConnection(
	ctx context.Context,
	target string,
) error {
	// 1. The genuine installation that owns this Target's protected state lives
	// at the Target runtime root (deployment.TargetStateRoot/runtime), the same
	// accepted production root that
	// cmd/baha/application_management_dependency.go loads. coreinstallation.Load
	// validates it as a genuine baseharbor Core installation (Owner ==
	// "baseharbor"); an absent or foreign root fails closed before any
	// verification work.
	// A remote execution Target is not itself a local Core installation.
	// Until a verified remote management-connection binder is configured,
	// refuse selection rather than inventing a local Core or advising a
	// developer-only workstation to install one.
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	resolved, err := cfg.ResolveTarget(target, "")
	if err != nil {
		return err
	}
	if resolved.AccessProvider != "" && resolved.AccessProvider != "local" {
		return machine.NewError(machine.ErrorCapabilityMissing,
			fmt.Sprintf("target %q requires a verified remote Core management connection", target),
			"Configure and verify the external Core management connection and its operator identity; no local Core will be installed or substituted.", false)
	}
	root, err := targetRuntimeStateRootByName(target)
	if err != nil {
		return err
	}
	// The plain target-state root the Core owns (deployment.TargetStateRoot(name));
	// the Core's Keycloak core issuer lives under providers/keycloak/shared/core
	// there, never under the runtime/ sub-root. VerifyCoreConnection reads it from
	// the plain root.
	plainTargetRoot, err := deployment.TargetStateRoot(target)
	if err != nil {
		return err
	}
	state, err := coreinstallation.Load(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return machine.NewError(
				machine.ErrorConflict,
				fmt.Sprintf("target %q has no verified management connection; the installation identity is required", target),
				"Install a BaseHarbor Core for this Target with `baha up --control-plane-only` so the management connection is present, then re-run baha use.",
				false,
			)
		}
		return machine.NewError(
			machine.ErrorConflict,
			fmt.Sprintf("target %q has no verified management connection: %v", target, err),
			"Install a BaseHarbor Core for this Target with `baha up --control-plane-only` so the management connection is present, then re-run baha use.",
			false,
		)
	}
	// 2. Verify the Core's live management connection through the real Core
	// identity: the verified Core keeps its management-connection identity at
	// its Keycloak discovery issuer (state.IdentityIssuer, an HTTPS URL such as
	// https://identity.localhost:<port>/realms/baseharbor), NOT at a local base
	// CA. VerifyCoreConnection (cmd/baha/verify_core_connection.go) reads the
	// Core's existing Keycloak public TLS material from disk and proves the
	// observed identity matches the verified Core — the accepted, read-only
	// production boundary that never fabricates issuer material or opens a
	// second trust source. A missing/unreadable Core issuer, a foreign target,
	// an unverifiable installation or an identity mismatch fails closed with a
	// named sentinel mapped to a typed rejection below.
	if err := VerifyCoreConnection(ctx, state, plainTargetRoot); err != nil {
		if errors.Is(err, ErrKeycloakCoreIssuerUnavailable) {
			return machine.NewError(
				machine.ErrorConflict,
				fmt.Sprintf("target %q has no verified management connection: %v", target, err),
				"Verify the BaseHarbor Core for this Target with `baha up --control-plane-only` so its management connection is reachable, then re-run baha use.",
				false,
			)
		}
		if errors.Is(err, coreinstallation.ErrForeignTarget) {
			return machine.Wrap(machine.ErrorPolicyDenied, err, "The management connection belongs to a different Target; select the correct one.", false)
		}
		if errors.Is(err, coreinstallation.ErrUnverifiableInstallation) {
			return machine.NewError(
				machine.ErrorConflict,
				fmt.Sprintf("target %q has no verified management connection: %v", target, err),
				"Verify the BaseHarbor Core for this Target with `baha up --control-plane-only` so its management connection is reachable, then re-run baha use.",
				false,
			)
		}
		if errors.Is(err, ErrCoreIssuerMismatch) {
			return machine.Wrap(machine.ErrorPolicyDenied, err, "The management connection identity does not match the selected installation; select the correct one.", false)
		}
		return machine.Wrap(machine.ErrorVerificationFailed, err, "The management connection did not verify; the selection persists nothing.", false)
	}
	// 3. The management connection is verified. Assert the three distinct
	// authorities before persisting the selection: the installation, the
	// management connection and the Target are never interchanged.
	selection := coreinstallation.ConnectionSelection{
		InstallationID: state.ID,
		ConnectionID:   "managed-connection",
		TargetID:       target,
	}
	if err := selection.AssertDistinct(); err != nil {
		return machine.Wrap(machine.ErrorConflict, err, "The Target, connection and installation are distinct; select a distinct Target.", false)
	}
	return nil
}
