package controlplaneapi

import (
	"errors"
	"net/http"

	"github.com/mcpdev80/baseharbor/internal/httpsecurity"
)

// New composes the control-plane HTTP handler behind the mandatory request
// security chain. Protected API handlers must not be exposed directly.
func New(security *httpsecurity.Middleware, protected http.Handler) (http.Handler, error) {
	if security == nil || protected == nil {
		return nil, errors.New("control-plane API dependencies are required")
	}
	return security.Protect(protected), nil
}
