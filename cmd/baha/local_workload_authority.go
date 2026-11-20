package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

type localInvocationKey struct{}
type localApplicationIDKey struct{}
type localWorkloadOperationKey struct{}

func withLocalInvocation(ctx context.Context, transport string) context.Context {
	return context.WithValue(ctx, localInvocationKey{}, transport)
}

func withoutLocalInvocation(ctx context.Context) context.Context {
	return withLocalInvocation(ctx, "")
}

func localWorkloadScope(ctx context.Context, operation string) context.Context {
	if !operatorauth.LocalWorkloadOperation(operation) {
		operation = ""
	}
	return context.WithValue(ctx, localWorkloadOperationKey{}, operation)
}

// The opaque grant is invocation-local and binds the runtime check to typed
// operation and UID values. It protects against accidental internal misuse;
// the local OS user remains the security boundary. Portable labels never
// replace the existing remote/tenant/management/OIDC checks.
func verifiedLocalWorkloadAuthority(ctx context.Context, target, environment string) (operatorauth.LocalOperationGrant, bool, error) {
	return verifiedLocalWorkloadAuthorityWithRuntime(ctx, target, environment, detectRuntimeForTarget)
}

func verifiedLocalWorkloadAuthorityWithRuntime(ctx context.Context, target, environment string, resolve func(context.Context, deployment.ResolvedTarget) (bhruntime.RuntimeProvider, error)) (operatorauth.LocalOperationGrant, bool, error) {
	var authority operatorauth.LocalOperationGrant
	transport, _ := ctx.Value(localInvocationKey{}).(string)
	operation, _ := ctx.Value(localWorkloadOperationKey{}).(string)
	if (transport != "cli" && transport != "stdio") || !operatorauth.LocalWorkloadOperation(operation) {
		return authority, false, nil
	}
	if _, scoped := tenancy.FromContext(ctx); scoped {
		return authority, false, nil
	}
	if _, managed := ctx.Value(coreAuthorityContextKey{}).(deployment.ResolvedTarget); managed {
		return authority, false, nil
	}
	selected, err := effectiveTarget(withTargetOverride(ctx, target))
	if err != nil {
		return authority, false, err
	}
	if selected.Name != strings.TrimSpace(target) || (selected.AccessProvider != "" && selected.AccessProvider != "local") || selected.TenantID != "" {
		return authority, false, nil
	}
	if _, found, err := deployment.OperatorAuthForTargetEnvironment(target, environment); err != nil {
		return authority, false, err
	} else if found {
		return authority, false, nil
	}
	for _, name := range []string{operatorauth.EnvIssuer, operatorauth.EnvClientID, operatorauth.EnvScopes, operatorauth.EnvCallbackPort} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return authority, false, nil
		}
	}
	root, err := targetRuntimeStateRoot(selected)
	if err != nil {
		return authority, false, err
	}
	targetRoot, err := deployment.TargetStateRoot(selected.Name)
	if err != nil {
		return authority, false, err
	}
	for _, dir := range []string{targetRoot, root} {
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return authority, false, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return authority, false, errors.New("local runtime owner directories are unprotected or redirected; preserve state and recover its authority")
		}
	}
	if _, err := coreinstallation.Load(root); err == nil {
		return authority, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return authority, false, err
	}
	if _, err := os.Lstat(filepath.Join(root, "installation.json")); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return authority, false, err
		}
		return authority, false, errors.New("management installation ownership is incomplete; recover its existing authority before retry")
	}
	provider, err := resolve(ctx, selected)
	if err != nil {
		return authority, false, localRuntimeAuthorityUnavailable(err)
	}
	if !localRuntimeEngine(provider) {
		return authority, false, nil
	}
	operationContract, ok := machine.OperationByID(operation)
	if !ok {
		return nil, false, nil
	}
	environmentID, err := deployment.EnvironmentID(target, environment)
	if err != nil {
		return nil, false, err
	}
	applicationID, _ := ctx.Value(localApplicationIDKey{}).(string)
	if applicationID == "" {
		// Observation callers can report the runtime check but cannot issue a
		// grant without a canonical application UID.
		return nil, true, nil
	}
	grant, err := operatorauth.CheckLocalRuntimeGrant(operationContract, environmentID, applicationID, func() error {
		if err := validateTargetDockerBinding(ctx, selected, provider, false); err != nil {
			return err
		}
		if provider.Kind() == bhruntime.ProviderPodman {
			local, ok := provider.(interface{ VerifyLocalRuntimeAuthority(context.Context) error })
			if !ok {
				return errors.New("native local runtime observation is unavailable")
			}
			return local.VerifyLocalRuntimeAuthority(ctx)
		}
		return nil
	})
	if err != nil {
		return nil, false, localRuntimeAuthorityUnavailable(err)
	}
	return grant, true, nil
}

func localRuntimeEngine(provider bhruntime.RuntimeProvider) bool {
	if provider == nil {
		return false
	}
	if provider.Kind() == bhruntime.ProviderPodman {
		return true
	} // Requires the native observation above.
	engine := bhruntime.DockerEngineForProvider(provider)
	return provider.Kind() == bhruntime.ProviderDocker && engine != nil && engine.Verified && engine.DaemonID != "" && strings.HasPrefix(engine.Endpoint, "unix:///") && (engine.Mode == "rootless" || engine.Mode == "rootful")
}

func localRuntimeAuthorityUnavailable(cause error) error {
	return &machine.Error{Code: machine.ErrorRuntimeUnavailable, CauseCode: "local_runtime_authority_unverifiable", Message: "The selected local runtime authority could not be verified.", Next: "Verify the selected Target and its local engine binding, then retry; no OIDC service or fallback runtime was installed.", Cause: cause, Retryable: true}
}

func applicationBoundaryOperation(command string) string {
	switch command {
	case "init", "adopt":
		return "app.adopt"
	case "create", "configure", "environment", "connection":
		return "app." + command
	case "up":
		return "apply"
	case "down":
		return "app.stop"
	case "show":
		return "app.show"
	case "preflight":
		return "app.preflight"
	default:
		return command
	}
}
