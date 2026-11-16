package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// Discovery readiness can precede database-backed token readiness during HA
// convergence. Only transient server failures of this read-only authentication
// probe are retried. Keycloak also maps unexpected server exceptions, including
// stale JDBC connections after convergence, to one specific OAuth 400 response.
// Invalid credentials and all other client failures still fail closed.
func waitKeycloakAdminLogin(ctx context.Context, admin *keycloakAdmin) error {
	probe, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		err := admin.login(probe)
		if err == nil {
			return nil
		}
		if !transientKeycloakAdminLogin(err) {
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

func transientKeycloakAdminLogin(err error) bool {
	var status *keycloakAdminLoginError
	if !errors.As(err, &status) {
		return false
	}
	if status.Status >= 500 && status.Status <= 599 {
		return true
	}
	var problem struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	return status.Status == http.StatusBadRequest && json.Unmarshal([]byte(status.Body), &problem) == nil &&
		problem.Error == "unauthorized_client" && problem.Description == "Unexpected error when authenticating client"
}
