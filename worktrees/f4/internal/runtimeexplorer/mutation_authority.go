package runtimeexplorer

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// MutationAuthority is supplied by the existing Target/deployment owner. The
// returned verifier also runs after a failed backend command: partial physical
// effects must not disappear behind a command error. It owns no persistent state.
type MutationAuthority interface {
	AuthorizeRuntimeMutation(context.Context, Resource, OperationRequest) (MutationVerifier, error)
}

type MutationVerifier func(context.Context, Resource, error) (*application.ProviderImpactScopeResult, error)
