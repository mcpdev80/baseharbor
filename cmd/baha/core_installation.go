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
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/hostresource"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	"github.com/mcpdev80/baseharbor/internal/machine"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return coreinstallation.State{}, err
	}
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
			if _, err := detectRuntimeForTarget(ctx, target); err != nil {
				return err
			}
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
			return identityprovider.EnsureCoreIdentity(ctx, compose, platformopenbao.NewServiceIssuer(compose, files), dataDir, target.Name, state.ID)
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
	state, err := coreinstallation.Run(ctx, root, spec, steps)
	if err == nil {
		fmt.Fprintln(out, "BaseHarbor Core READY: SQL + Secrets + Identity.")
	}
	return state, err
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
