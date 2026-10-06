package identityprovider

import (
	"context"
	"errors"
	"time"
)

// Discovery readiness can precede database-backed token readiness during HA
// convergence. Only transient server failures of this read-only authentication
// probe are retried. Invalid credentials and other client failures fail closed.
func waitKeycloakAdminLogin(ctx context.Context, admin *keycloakAdmin) error {
	probe, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		err := admin.login(probe)
		if err == nil {
			return nil
		}
		var status *keycloakAdminLoginError
		if !errors.As(err, &status) || status.Status < 500 || status.Status > 599 {
			return err
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-probe.Done():
			timer.Stop()
			return errors.Join(probe.Err(), err)
		case <-timer.C:
		}
	}
}
