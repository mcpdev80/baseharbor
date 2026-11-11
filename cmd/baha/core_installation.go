package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/hostresource"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

func selectInstallationRole(reader *bufio.Reader, out io.Writer) (coreinstallation.MachineRole, error) {
	fmt.Fprintln(out, "Is this installation running on a machine where you write code?")
	fmt.Fprintln(out, "  1. Yes, this is a development machine")
	fmt.Fprintln(out, "  2. No, this is a deployment machine")
	fmt.Fprint(out, "Choice [1]: ")
	answer, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "1", "yes", "y":
		return coreinstallation.Development, nil
	case "2", "no", "n":
		return coreinstallation.Deployment, nil
	default:
		return "", errors.New("choose development or deployment machine")
	}
}

func installCore(ctx context.Context, in io.Reader, out io.Writer, opts runtimeUpOptions) (coreinstallation.State, error) {
	if err := authorizeCurrentMCPContext(ctx, "control-plane.up", "", "", ""); err != nil {
		return coreinstallation.State{}, err
	}
	ctx = withAssumeYes(ctx, opts.Yes)
	interactive := !opts.Yes && !noInput(ctx) && readerIsTerminal(in)
	if interactive {
		in = bufio.NewReader(in)
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return coreinstallation.State{}, err
	}
	if err := checkBoundCoreTarget(ctx, target); err != nil {
		return coreinstallation.State{}, err
	}
	if target.AccessProvider != "" && target.AccessProvider != string(targetaccess.ProviderLocal) {
		return coreinstallation.State{}, machine.NewError(machine.ErrorCapabilityMissing,
			"Core bootstrap requires a local installation Target.", "Select the installation's local Core Target.", false)
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return coreinstallation.State{}, err
	}
	dataRoot, err := targetDataRoot(target)
	if err != nil {
		return coreinstallation.State{}, err
	}
	unlock, err := lockApplicationTargetLifecycle(resolvedApplication{Target: target, TargetStateRoot: dataRoot}, false)
	if err != nil {
		return coreinstallation.State{}, err
	}
	defer unlock()
	role := opts.MachineRole
	if role == "" {
		role = coreinstallation.MachineRole(os.Getenv("BASEHARBOR_INSTALLATION_MACHINE_ROLE"))
	}
	existing, err := coreinstallation.Load(root)
	if err == nil {
		if role == "" {
			role = existing.Spec.MachineRole
		}
		if !opts.HA {
			opts.HA = existing.Spec.HA
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return coreinstallation.State{}, err
	}
	if role == "" {
		if interactive {
			role, err = selectInstallationRole(bufio.NewReader(in), out)
			if err != nil {
				return coreinstallation.State{}, err
			}
		} else {
			role = coreinstallation.Development
		}
	}
	spec := coreinstallation.Spec{Target: target.Name, Runtime: target.RuntimeProvider, MachineRole: role, HA: opts.HA}
	var compose bhruntime.RuntimeProvider
	var files bhruntime.Files
	steps := coreinstallation.Steps{
		Preflight: func(ctx context.Context, state coreinstallation.State) error {
			// Existing owned resources are reconciled rather than budgeted a second time.
			estimate, err := hostresource.EstimateCore(opts.HA, state.Capabilities)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "BaseHarbor Core: SQL + Secrets + Identity. Console is optional.")
			fmt.Fprintln(out, "Placement: installation shared realization; application isolation can add provider instances and resource use.")
			fmt.Fprintln(out, "Measured Core idle/startup memory: unavailable until this runtime is sampled.")
			fmt.Fprintln(out, "Shared Docker reference: about 2.6 GiB idle; sampled startup peak up to 4.73 GiB. Other hosts/topologies may differ.")
			if !state.Capabilities["sql"] {
				path, _, err := preflightNewTargetRecoveryFile(ctx, opts.RecoveryFile)
				if err != nil {
					return err
				}
				opts.RecoveryFile = path
			}
			return runHostMemoryPreflight(ctx, in, out, bhruntime.ProviderKind(target.RuntimeProvider), estimate, true)
		},
		SQL: func(ctx context.Context, state coreinstallation.State) error {
			// Existing provider lifecycle already performs ownership, config and TLS checks.
			if err := runtimeUpGuidedProviders(ctx, in, out, opts); err != nil {
				return err
			}
			var err error
			compose, files, err = openBaoRuntime(ctx)
			return err
		},
		Secrets: func(ctx context.Context, state coreinstallation.State) error {
			return ensureRepositoryOpenBaoReady(ctx, in, out, io.Discard, opts)
		},
		Identity: func(ctx context.Context, state coreinstallation.State) (string, error) {
			dataDir, err := targetDataRoot(target)
			if err != nil {
				return "", err
			}
			return identityprovider.EnsureCoreIdentity(ctx, compose, platformopenbao.NewServiceIssuer(compose, files), dataDir, target.Name, state.ID, state.Spec.HA)
		},
		Verify: func(ctx context.Context, state coreinstallation.State) error {
			_, ready := health.Format(health.RuntimeChecksForFiles(files))
			if !ready {
				return machine.NewError(machine.ErrorVerificationFailed, "Core SQL/Secrets are not ready.", "Inspect the selected installation before continuing the application.", true)
			}
			if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
				return err
			}
			dataDir, err := targetDataRoot(target)
			if err != nil {
				return err
			}
			return identityprovider.VerifyCoreIdentity(ctx, dataDir, target.Name, state.ID, state.IdentityIssuer)
		},
	}
	if err := ensureTargetDockerBinding(ctx, target); err != nil {
		return coreinstallation.State{}, err
	}
	// Owned-authority passthrough: when this serve process is a management host
	// that already owns the durable installation state root (the server holds the
	// kernel lifecycle lock on it for its whole lifetime), execute the Core
	// lifecycle on the owned authority instead of acquiring a second,
	// non-reentrant lifecycle lock on the same root — which would dead-lock the
	// owning server. The management-host test proves the same-host lifecycle op
	// succeeds through this path.
	if owned := ownedHostStateRoot(ctx); owned != "" && owned == root {
		state, rerr := coreinstallation.Run(ctx, root, spec, steps)
		if rerr == nil {
			fmt.Fprintln(out, "BaseHarbor Core READY: SQL + Secrets + Identity.")
		}
		return state, rerr
	}
	state, err := coreinstallation.Run(ctx, root, spec, steps)
	if err == nil {
		fmt.Fprintln(out, "BaseHarbor Core READY: SQL + Secrets + Identity.")
	}
	return state, err
}

func ensureCoreRuntimeImage(ctx context.Context, target deployment.ResolvedTarget, out io.Writer) error {
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	return pullCoreRuntimeImage(ctx, provider, out)
}

func pullCoreRuntimeImage(ctx context.Context, provider bhruntime.RuntimeProvider, out io.Writer) error {
	image := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"))
	if image == "" {
		image = defaultRuntimeImage(version)
	}
	fmt.Fprintln(out, "Ensuring the BaseHarbor runtime image is available before Core setup.")
	if err := pullResolvedRuntimeImage(ctx, provider, image, ""); err != nil {
		return fmt.Errorf("ensure BaseHarbor runtime image before Core setup: %w", err)
	}
	fmt.Fprintln(out, "[OK] BaseHarbor runtime image is available (verified immutable registry identity).")
	return nil
}

func coreRequired() error {
	return &machine.Error{Code: machine.ErrorCapabilityMissing, CauseCode: "core_required", Message: "BaseHarbor needs its Core services before the first application can run.", Next: "Request the installation's Core bootstrap operation explicitly, then continue the original application operation.", Retryable: true}
}

func requireApplicationCore(ctx context.Context, in io.Reader, out io.Writer) error {
	target, remote, err := applicationCoreTarget(ctx)
	if err != nil {
		return err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	state, err := coreinstallation.Load(root)
	if err == nil && state.Ready {
		if state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
			return machine.NewError(machine.ErrorOwnershipAmbiguous, "Core belongs to a different installation selection.", "Select the owning installation.", false)
		}
		files, err := bhruntime.ExistingFilesForProject(root, targetRuntimeProjectName(target))
		if err != nil {
			return err
		}
		_, ready := health.Format(health.RuntimeChecksForFiles(files))
		if ready {
			// Probe real Identity discovery; cached READY is never proof of a live provider.
			issuer := state.IdentityIssuer
			dataDir, err := targetDataRoot(target)
			if err != nil {
				return err
			}
			return identityprovider.VerifyCoreIdentity(ctx, dataDir, target.Name, state.ID, issuer)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if remote {
		return coreRequired()
	}
	if noInput(ctx) || (!readerIsTerminal(in) && !isBufferedCoreInput(in)) {
		if !assumeYes(ctx) {
			return coreRequired()
		}
	}
	role := coreinstallation.MachineRole("")
	if !assumeYes(ctx) {
		reader, ok := in.(*bufio.Reader)
		if !ok {
			reader = bufio.NewReader(in)
		}
		fmt.Fprintln(out, "BaseHarbor needs its Core services before the first application can run.")
		yes, err := promptYesNo(reader, out, "Set them up now?", true)
		if err != nil {
			return err
		}
		if !yes {
			return coreRequired()
		}
		if state.ID == "" {
			role, err = selectInstallationRole(reader, out)
			if err != nil {
				return err
			}
		}
		in = reader
	}
	_, err = applicationCoreBootstrap(ctx, in, out, runtimeUpOptions{Yes: true, ControlPlaneOnly: true, MachineRole: role})
	return err
}

func isBufferedCoreInput(in io.Reader) bool { _, ok := in.(*bufio.Reader); return ok }

// Shared application prerequisite, injectable at construction boundaries in tests.
var applicationCorePrerequisite = requireApplicationCore

var applicationCoreBootstrap = installCore

type applicationInputKey struct{}

func applicationInput(ctx context.Context, fallback io.Reader) io.Reader {
	if in, ok := ctx.Value(applicationInputKey{}).(io.Reader); ok {
		return in
	}
	return fallback
}

func withApplicationInput(ctx context.Context, in io.Reader) context.Context {
	if readerIsTerminal(in) {
		return context.WithValue(ctx, applicationInputKey{}, bufio.NewReader(in))
	}
	return ctx
}
