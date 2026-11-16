package auth

import (
	"context"
	"errors"

	"github.com/mcpdev80/baseharbor/internal/identity"
)

var ErrInvalidToken = errors.New("invalid or expired token")

// Verifier converts a verified OIDC/JWT token into BaseHarbor's provider-neutral identity.
// Implementations must validate signature, issuer, audience, expiry and not-before claims.
type Verifier interface {
	Verify(ctx context.Context, rawToken string) (*identity.Principal, error)
}
