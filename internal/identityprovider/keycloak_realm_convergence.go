package identityprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type keycloakRealmResponseError struct {
	status    int
	operation string
}

func (e *keycloakRealmResponseError) Error() string {
	return fmt.Sprintf("%s Keycloak realm: HTTP %d", e.operation, e.status)
}

// A server error can follow an applied write. Re-enter through the realm read
// and ownership check rather than replaying a POST or PUT blindly.
func (a *keycloakAdmin) reconcileRealm(ctx context.Context, desired keycloakRealm) error {
	probe, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for {
		err := a.reconcileRealmOnce(probe, desired)
		if err == nil {
			return nil
		}
		var response *keycloakRealmResponseError
		if !errors.As(err, &response) || (response.status != http.StatusInternalServerError &&
			response.status != http.StatusBadGateway && response.status != http.StatusServiceUnavailable &&
			response.status != http.StatusGatewayTimeout) {
			return err
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-probe.Done():
			timer.Stop()
			return errors.Join(probe.Err(), err)
		case <-timer.C:
		}
	}
}
