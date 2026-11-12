package database

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// RegisterWorkstationUser stores the non-secret directory metadata for a
// Keycloak-owned account. An exact active duplicate is idempotent to support
// recovery after invitation delivery; differing or offboarded identities fail closed.
func (s *WorkstationEnrollmentStore) RegisterWorkstationUser(ctx context.Context, tenantID string, user WorkstationUser) error {
	if s == nil || s.pool == nil || validWorkstationTenant(tenantID) != nil || !validWorkstationUser(user) {
		return ws.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		email := strings.ToLower(user.Email)
		if _, err := tx.Exec(ctx, `INSERT INTO workstation_users
(tenant_id, login, display_name, email, keycloak_user_id)
VALUES ($1,$2,$3,$4,$5) ON CONFLICT (tenant_id,login) DO NOTHING`, tenantID, user.Login, user.DisplayName, email, user.KeycloakID); err != nil {
			return err
		}
		var display, storedEmail, keycloakID, status string
		if err := tx.QueryRow(ctx, `SELECT display_name,email,keycloak_user_id,status FROM workstation_users WHERE tenant_id=$1 AND login=$2`, tenantID, user.Login).Scan(&display, &storedEmail, &keycloakID, &status); err != nil {
			return err
		}
		if display != user.DisplayName || storedEmail != email || keycloakID != user.KeycloakID || status != "active" {
			return ws.ErrDenied
		}
		return nil
	})
}

// WorkstationUsers returns the installation's non-secret user directory,
// including the number of registered devices per person.
func (s *WorkstationEnrollmentStore) WorkstationUsers(ctx context.Context, tenantID string) ([]WorkstationUser, map[string]int, error) {
	if s == nil || s.pool == nil || validWorkstationTenant(tenantID) != nil {
		return nil, nil, ws.ErrDenied
	}
	var users []WorkstationUser
	devices := map[string]int{}
	err := WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT u.login,u.display_name,u.email,u.keycloak_user_id,u.status,u.created_at,u.offboarded_at,
COUNT(r.station)
FROM workstation_users u LEFT JOIN workstation_enrollment_registry r
ON r.tenant_id=u.tenant_id AND r.owner_login=u.login
WHERE u.tenant_id=$1
GROUP BY u.login,u.display_name,u.email,u.keycloak_user_id,u.status,u.created_at,u.offboarded_at
ORDER BY u.login`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var user WorkstationUser
			var count int
			if err := rows.Scan(&user.Login, &user.DisplayName, &user.Email, &user.KeycloakID, &user.Status, &user.CreatedAt, &user.OffboardedAt, &count); err != nil {
				return err
			}
			users = append(users, user)
			devices[user.Login] = count
		}
		return rows.Err()
	})
	if err != nil {
		return nil, nil, ws.ErrDenied
	}
	return users, devices, nil
}

// WorkstationUserByLogin resolves one installation-owned person.
func (s *WorkstationEnrollmentStore) WorkstationUserByLogin(ctx context.Context, tenantID, login string) (WorkstationUser, bool, error) {
	if s == nil || s.pool == nil || validWorkstationTenant(tenantID) != nil || strings.TrimSpace(login) != login || login == "" {
		return WorkstationUser{}, false, ws.ErrDenied
	}
	var user WorkstationUser
	err := WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT login,display_name,email,keycloak_user_id,status,created_at,offboarded_at
FROM workstation_users WHERE tenant_id=$1 AND login=$2`, tenantID, login).
			Scan(&user.Login, &user.DisplayName, &user.Email, &user.KeycloakID, &user.Status, &user.CreatedAt, &user.OffboardedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkstationUser{}, false, nil
	}
	if err != nil {
		return WorkstationUser{}, false, ws.ErrDenied
	}
	return user, true, nil
}

// OffboardWorkstationUser marks an account inactive while retaining its audit
// row. Call only after the Keycloak account is disabled and owned device leaves
// have been revoked; repeated calls are safe for a partial-offboard retry.
func (s *WorkstationEnrollmentStore) OffboardWorkstationUser(ctx context.Context, tenantID, login string) error {
	if s == nil || s.pool == nil || validWorkstationTenant(tenantID) != nil || strings.TrimSpace(login) != login || login == "" {
		return ws.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM workstation_enrollment_grants WHERE tenant_id=$1 AND owner_login=$2 AND consumed_at IS NULL`, tenantID, login); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE workstation_users SET status='offboarded',offboarded_at=COALESCE(offboarded_at,clock_timestamp())
WHERE tenant_id=$1 AND login=$2`, tenantID, login)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (s *WorkstationEnrollmentStore) WorkstationOwnerStations(ctx context.Context, tenantID, login string) ([]WorkstationRegistryFact, error) {
	if s == nil || s.pool == nil || validWorkstationTenant(tenantID) != nil || strings.TrimSpace(login) != login || login == "" {
		return nil, ws.ErrDenied
	}
	var facts []WorkstationRegistryFact
	err := WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT station,COALESCE(owner_login,''),issued_at,expires_at
FROM workstation_enrollment_registry WHERE tenant_id=$1 AND owner_login=$2 ORDER BY station`, tenantID, login)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var fact WorkstationRegistryFact
			if err := rows.Scan(&fact.Station, &fact.OwnerLogin, &fact.IssuedAt, &fact.ExpiresAt); err != nil {
				return err
			}
			facts = append(facts, fact)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, ws.ErrDenied
	}
	return facts, nil
}

func validWorkstationTenant(tenantID string) error {
	return (ws.Scope{TenantID: tenantID, Station: "validation"}).Validate()
}

func validWorkstationUser(user WorkstationUser) bool {
	return strings.TrimSpace(user.Login) == user.Login && len(user.Login) > 0 && len(user.Login) <= 128 &&
		strings.TrimSpace(user.DisplayName) == user.DisplayName && len(user.DisplayName) > 0 && len(user.DisplayName) <= 200 &&
		strings.TrimSpace(user.Email) == user.Email && strings.Contains(user.Email, "@") &&
		strings.TrimSpace(user.KeycloakID) == user.KeycloakID && user.KeycloakID != "" && user.Status == ""
}
