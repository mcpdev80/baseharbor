package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

type bahaMachineExecutor struct {
	store             application.Store
	connectorSessions *targetsession.Pool
	coreAuthority     *deployment.ResolvedTarget
	// ownedStateRoot is the durable installation state root this management-host
	// serve process owns: the server holds the kernel lifecycle lock on it for
	// the whole server lifetime, so lifecycle operations routed through THIS
	// executor execute on the owned authority. It is nil for a per-application
	// runtime broker (which owns no installation) and for the local no-login
	// path, where nothing binds.
	ownedStateRoot string
}

// Bound once during Core startup, before accepting any machine request.
//
// The management-host serve process holds the kernel lifecycle lock on
// ownedStateRoot for its whole lifetime. Lifecycle operations routed through
// THIS executor execute on the owned authority. The executor must NOT acquire
// a second, non-reentrant per-operation lifecycle lock on the same root: that
// would dead-lock against the server itself, because the kernel lock is held
// for the server's whole lifetime. The owning process already authorises the
// authority for the whole request, so no second lock is required.

// ManagementInstallation exposes only the startup-pinned local Core identity.
// It must never resolve installation identity from a request's Target.
func (e *bahaMachineExecutor) ManagementInstallation(context.Context) (string, string, error) {
    if e.coreAuthority == nil || (e.coreAuthority.AccessProvider != "" && e.coreAuthority.AccessProvider != "local") {
        return "", "", errors.New("management API has no pinned local Core authority")
    }
    root := e.ownedStateRoot
    if root == "" {
        var err error
        root, err = targetRuntimeStateRoot(*e.coreAuthority)
        if err != nil { return "", "", err }
    }
    state, err := coreinstallation.Load(root)
    if err != nil { return "", "", err }
    if !state.Ready || state.Spec.Target != e.coreAuthority.Name || state.Spec.Runtime != e.coreAuthority.RuntimeProvider ||
       state.IdentityIssuer == "" {
        return "", "", errors.New("startup Core installation identity is not ready")
    }
    return state.ID, state.IdentityIssuer, nil
}

func (e *bahaMachineExecutor) BindConnectorSessions(pool *targetsession.Pool) {
	e.connectorSessions = pool
}

// BindOwnedStateRoot pins the durable installation state root this
// management-host serve process owns, so lifecycle operations routed through
// this executor execute on the owned authority. The server holds the kernel
// lifecycle lock on this root for its whole lifetime, so the executor must NOT
// acquire a second, non-reentrant per-operation lifecycle lock on the same
// root (which would dead-lock against the server itself); the owning process
// already authorises the authority for the whole request.
func (e *bahaMachineExecutor) BindOwnedStateRoot(root string) {
	if strings.TrimSpace(root) == "" {
		return
	}
	e.ownedStateRoot = root
}

func newBahaMachineExecutor(store application.Store) machinehttp.Executor {
	return &bahaMachineExecutor{store: store}
}

func (e *bahaMachineExecutor) Execute(
	ctx context.Context,
	operation machine.Operation,
	operationContext machine.OperationContext,
	input json.RawMessage,
	report machinehttp.ProgressReporter,
) (json.RawMessage, error) {
	ctx = withoutLocalInvocation(ctx)
	ctx = targetsession.WithPool(ctx, e.connectorSessions)
	if e.coreAuthority != nil {
		ctx = withCoreAuthority(ctx, *e.coreAuthority)
	}
	ctx = withOrganizationEnvironment(ctx, operationContext.Environment)
	opts := cli.OutputOptionsFromContext(ctx)
	opts.NonInteractive = true
	opts.Quiet = true
	opts.Plain = true
	ctx = cli.WithOutputOptions(ctx, opts)

	// Owned-authority passthrough: when this executor belongs to a management
	// host that owns a durable installation state root, inject that ownership so
	// lifecycle operations execute on the owned authority. The server already
	// holds the kernel lifecycle lock on this root for its whole lifetime, so no
	// second lock is acquired here (that would dead-lock the server).
	if e.ownedStateRoot != "" {
		ctx = withOwnedHostState(ctx, e.ownedStateRoot)
	}

	if report != nil {
		report(machine.OperationProgress{Stage: "accepted", Message: "Operation accepted by BaseHarbor Core."})
	}

	var (
		result any
		err    error
	)
	switch httpOperationKinds[operation.ID] {
	case httpRead:
		result, err = e.executeHTTPRead(ctx, operation.ID, operationContext, input)
	case httpRuntimeMutation:
		result, err = e.executeHTTPRuntimeMutation(ctx, operation.ID, operationContext, input)
	case httpPlatformMutation:
		result, err = e.executeHTTPPlatformMutation(ctx, operation.ID, operationContext, input, report)
	case httpLifecycle:
		result, err = e.executeHTTPLifecycle(ctx, operation.ID, operationContext, input, report)
	default:
		return nil, machine.NewError(machine.ErrorUnsupported, "Machine operation is not implemented by the HTTP semantic executor.", "Use machine discovery and select an implemented operation.", false)
	}

	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, machine.Wrap(machine.ErrorInternal, err, "Retry the operation.", false)
	}
	return data, nil
}

func decodeHTTPInput(input json.RawMessage, target any) error {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	invalid := func() error {
		return machine.NewError(machine.ErrorValidationFailed, "Invalid machine operation input.", "Send valid JSON input for the selected operation.", false)
	}
	if machine.ValidateJSONObject(input, 1<<20) != nil {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return invalid()
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return invalid()
	}
	return nil
}

func bindHTTPSelector(label, authorized, supplied string) (string, error) {
	authorized = strings.TrimSpace(authorized)
	supplied = strings.TrimSpace(supplied)
	if authorized != "" && supplied != "" && supplied != authorized {
		return "", &machine.Error{
			Code:      machine.ErrorPolicyDenied,
			CauseCode: "operation_context_mismatch",
			Message:   "Machine input attempts to escape the authorized operation context.",
			Resource:  label,
			Next:      "Use selectors that match the authorized execution context.",
		}
	}
	if authorized != "" {
		return authorized, nil
	}
	return supplied, nil
}

func bindHTTPApplicationInput(operationContext machine.OperationContext, input *machineApplicationInput) error {
	var err error
	input.Target, err = bindHTTPSelector("target", operationContext.Target, input.Target)
	if err != nil {
		return err
	}
	input.Environment, err = bindHTTPSelector("environment", operationContext.Environment, input.Environment)
	if err != nil {
		return err
	}
	input.Name, err = bindHTTPSelector("application", operationContext.Application, input.Name)
	return err
}

func bindHTTPApplicationSelectors(operationContext machine.OperationContext, target, environment, name *string) error {
	var err error
	*target, err = bindHTTPSelector("target", operationContext.Target, *target)
	if err != nil {
		return err
	}
	*environment, err = bindHTTPSelector("environment", operationContext.Environment, *environment)
	if err != nil {
		return err
	}
	*name, err = bindHTTPSelector("application", operationContext.Application, *name)
	return err
}
