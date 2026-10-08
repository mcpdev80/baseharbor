package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// CreateRenewal pins a one-use authorization to the currently active serial.
// A later revocation, scope change or competing replacement invalidates it.
func (s *ConnectorEnrollmentStore) CreateRenewal(ctx context.Context, grant targetenrollment.Grant) error {
	if ctx.Err() != nil || s == nil || s.pool == nil || grant.Scope.Validate() != nil {
		return targetenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, grant.Scope.TenantID, func(tx pgx.Tx) error {
		var previous string
		if err := tx.QueryRow(ctx, `SELECT certificate_serial FROM connector_nodes
WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3 AND runtime=$4
AND certificate_serial IS NOT NULL AND NOT certificate_revoked
AND certificate_expires_at > clock_timestamp() FOR UPDATE`, grant.Scope.TenantID,
			grant.Scope.NodeID, grant.Scope.TargetID, grant.Scope.Runtime).Scan(&previous); err != nil {
			return targetenrollment.ErrDenied
		}
		var pending bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connector_enrollment_grants
WHERE tenant_id=$1 AND node_id=$2 AND consumed_at IS NULL AND expires_at > clock_timestamp())`,
			grant.Scope.TenantID, grant.Scope.NodeID).Scan(&pending); err != nil {
			return err
		}
		if pending {
			return targetenrollment.ErrDenied
		}
		return insertConnectorGrant(ctx, tx, grant, &previous)
	})
}

func insertConnectorGrant(ctx context.Context, tx pgx.Tx, grant targetenrollment.Grant, previous *string) error {
	_, err := tx.Exec(ctx, `INSERT INTO connector_enrollment_grants
(token_digest, tenant_id, target_id, node_id, runtime, nonce_digest, expires_at,
certificate_ttl_seconds, previous_certificate_serial) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		grant.TokenDigest, grant.Scope.TenantID, grant.Scope.TargetID, grant.Scope.NodeID,
		grant.Scope.Runtime, grant.NonceDigest, grant.ExpiresAt, int64(grant.CertificateTTL/time.Second), previous)
	return err
}

func recordConnectorRenewal(ctx context.Context, tx pgx.Tx, scope targetenrollment.Scope, previous, serial string, expires time.Time) error {
	oldSerial, err := targetenrollment.NormalizeCertificateSerial(previous)
	if err != nil || oldSerial == serial {
		return targetenrollment.ErrDenied
	}
	var oldExpiry time.Time
	if err := tx.QueryRow(ctx, `SELECT certificate_expires_at FROM connector_nodes
WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3 AND runtime=$4
AND certificate_serial=$5 AND NOT certificate_revoked
AND certificate_expires_at > clock_timestamp() FOR UPDATE`, scope.TenantID,
		scope.NodeID, scope.TargetID, scope.Runtime, previous).Scan(&oldExpiry); err != nil {
		return targetenrollment.ErrDenied
	}
	// Retain exactly the most recent predecessor. A subsequent renewal retires
	// the earlier overlap rather than expanding admission indefinitely.
	if _, err := tx.Exec(ctx, `INSERT INTO connector_certificate_overlap
(tenant_id,node_id,certificate_serial,certificate_expires_at,admit_until,certificate_revoked)
VALUES ($1,$2,$3,$4,LEAST($4::timestamptz,clock_timestamp()+interval '5 minutes'),false)
ON CONFLICT (tenant_id,node_id) DO UPDATE SET certificate_serial=EXCLUDED.certificate_serial,
certificate_expires_at=EXCLUDED.certificate_expires_at,admit_until=EXCLUDED.admit_until,certificate_revoked=false`,
		scope.TenantID, scope.NodeID, oldSerial, oldExpiry); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE connector_nodes SET certificate_serial=$5,certificate_expires_at=$6
WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3 AND runtime=$4`,
		scope.TenantID, scope.NodeID, scope.TargetID, scope.Runtime, serial, expires)
	return err
}

var _ targetenrollment.RenewalStore = (*ConnectorEnrollmentStore)(nil)
