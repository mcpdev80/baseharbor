package operatorauth

import (
	"context"
	"strings"
)

type localRuntimeAuthorityKey struct{}

// LocalRuntimeAuthority is invocation evidence supplied by a trusted adapter
// after observing its local runtime binding and explicit authentication policy.
// It is not a principal, persisted identity, or machine-request input. One
// operation on one Target/environment is covered; tenant/Core operations are
// never authorized through this authority.
type LocalRuntimeAuthority struct {
	Operation   string
	Target      string
	Environment string
	Runtime     string
}

func LocalWorkloadOperation(operation string) bool {
	switch operation {
	case "app.create", "app.adopt", "app.configure", "app.new", "app.show", "app.environment", "app.connection", "app.preflight", "app.stop", "plan", "apply", "status", "doctor", "observe", "evidence", "update", "repair", "backup", "restore", "destroy", "workspace.show", "workspace.init", "workspace.map", "workspace.resolve", "workspace.status", "workspace.update":
		return true
	default:
		return false
	}
}

func WithVerifiedLocalRuntimeAuthority(ctx context.Context, authority LocalRuntimeAuthority) context.Context {
	authority.Operation = strings.TrimSpace(authority.Operation)
	authority.Target = strings.TrimSpace(authority.Target)
	authority.Environment = strings.ToLower(strings.TrimSpace(authority.Environment))
	return context.WithValue(ctx, localRuntimeAuthorityKey{}, authority)
}

func WithoutLocalRuntimeAuthority(ctx context.Context) context.Context {
	return context.WithValue(ctx, localRuntimeAuthorityKey{}, LocalRuntimeAuthority{})
}

func localRuntimeAuthorityMatches(ctx context.Context, request AuthorizationRequest) bool {
	authority, ok := ctx.Value(localRuntimeAuthorityKey{}).(LocalRuntimeAuthority)
	return ok && LocalWorkloadOperation(authority.Operation) && authority.Operation == request.Operation.ID && authority.Target != "" && authority.Target == strings.TrimSpace(request.Context.Target) && authority.Environment != "" && authority.Environment == strings.ToLower(strings.TrimSpace(request.Context.Environment)) && (authority.Runtime == "docker" || authority.Runtime == "podman")
}
