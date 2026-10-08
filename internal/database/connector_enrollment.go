package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

type ConnectorEnrollmentStore struct{ pool *pgxpool.Pool }

func NewConnectorEnrollmentStore(pool *pgxpool.Pool) *ConnectorEnrollmentStore {
	return &ConnectorEnrollmentStore{pool: pool}
}

func (s *ConnectorEnrollmentStore) Create(ctx context.Context, grant targetenrollment.Grant) error {
	if s == nil || s.pool == nil || grant.Scope.Validate() != nil {
		return targetenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, grant.Scope.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO connector_nodes (tenant_id,node_id,target_id,runtime)
VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, grant.Scope.TenantID, grant.Scope.NodeID, grant.Scope.TargetID, grant.Scope.Runtime); err != nil {
			return err
		}
		var target, runtime string
		var issued bool
		if err := tx.QueryRow(ctx, `SELECT target_id,runtime,certificate_serial IS NOT NULL FROM connector_nodes
WHERE tenant_id=$1 AND node_id=$2 FOR UPDATE`, grant.Scope.TenantID, grant.Scope.NodeID).Scan(&target, &runtime, &issued); err != nil {
			return err
		}
		if issued || target != grant.Scope.TargetID || runtime != grant.Scope.Runtime {
			return targetenrollment.ErrDenied
		}
		var pending bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connector_enrollment_grants
WHERE tenant_id=$1 AND node_id=$2 AND consumed_at IS NULL AND expires_at > clock_timestamp())`, grant.Scope.TenantID, grant.Scope.NodeID).Scan(&pending); err != nil {
			return err
		}
		if pending {
			return targetenrollment.ErrDenied
		}

		return insertConnectorGrant(ctx, tx, grant, nil)
	})
}

func (s *ConnectorEnrollmentStore) Consume(ctx context.Context, scope targetenrollment.Scope, tokenDigest, nonceDigest, csrDigest string, now time.Time) (targetenrollment.Grant, error) {
	grant := targetenrollment.Grant{Scope: scope, TokenDigest: tokenDigest, NonceDigest: nonceDigest}
	if s == nil || s.pool == nil || scope.Validate() != nil {
		return grant, targetenrollment.ErrDenied
	}
	err := WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		var seconds int64
		err := tx.QueryRow(ctx, `UPDATE connector_enrollment_grants SET consumed_at=clock_timestamp(), csr_digest=$7
WHERE token_digest=$1 AND nonce_digest=$2 AND target_id=$3 AND node_id=$4 AND runtime=$5
AND tenant_id=$8
AND consumed_at IS NULL AND expires_at > GREATEST($6, clock_timestamp())
AND (previous_certificate_serial IS NULL OR EXISTS(SELECT 1 FROM connector_nodes node
WHERE node.tenant_id=$8 AND node.node_id=$4 AND node.target_id=$3 AND node.runtime=$5
AND node.certificate_serial=connector_enrollment_grants.previous_certificate_serial
AND NOT node.certificate_revoked AND node.certificate_expires_at > clock_timestamp()))
RETURNING expires_at, certificate_ttl_seconds`, tokenDigest, nonceDigest, scope.TargetID, scope.NodeID, scope.Runtime, now, csrDigest, scope.TenantID).Scan(&grant.ExpiresAt, &seconds)
		if errors.Is(err, pgx.ErrNoRows) {
			return targetenrollment.ErrDenied
		}
		grant.CertificateTTL = time.Duration(seconds) * time.Second
		return err
	})
	if err != nil {
		return targetenrollment.Grant{}, targetenrollment.ErrDenied
	}
	return grant, nil
}

var _ targetenrollment.Store = (*ConnectorEnrollmentStore)(nil)

// RecordIssued commits the exact authenticated node identity before returning
// certificate material. Renewal additionally compares its authorized old serial
// and records one bounded predecessor in the same tenant-local transaction.
func (s *ConnectorEnrollmentStore) RecordIssued(ctx context.Context, scope targetenrollment.Scope, tokenDigest, serial string, expires time.Time) error {
	serial, err := targetenrollment.NormalizeCertificateSerial(serial)
	if ctx.Err() != nil || s == nil || s.pool == nil || scope.Validate() != nil || err != nil || !expires.After(time.Now()) {
		return targetenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		var previous *string
		if err := tx.QueryRow(ctx, `SELECT previous_certificate_serial FROM connector_enrollment_grants
WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3 AND runtime=$4 AND token_digest=$5
AND consumed_at IS NOT NULL FOR UPDATE`, scope.TenantID, scope.NodeID, scope.TargetID,
			scope.Runtime, tokenDigest).Scan(&previous); err != nil {
			return targetenrollment.ErrDenied
		}
		if previous != nil {
			return recordConnectorRenewal(ctx, tx, scope, *previous, serial, expires)
		}
		command, err := tx.Exec(ctx, `UPDATE connector_nodes SET certificate_serial=$6, certificate_expires_at=$7
WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3 AND runtime=$4 AND certificate_serial IS NULL
AND EXISTS (SELECT 1 FROM connector_enrollment_grants WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3
AND runtime=$4 AND token_digest=$5 AND consumed_at IS NOT NULL)`, scope.TenantID, scope.NodeID, scope.TargetID, scope.Runtime, tokenDigest, serial, expires)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return targetenrollment.ErrDenied
		}
		return nil
	})
}

// AdmitCertificate checks persisted identity and revocation in a tenant-local
// transaction. Explicit predicates remain mandatory for privileged DB callers.
func (s *ConnectorEnrollmentStore) AdmitCertificate(ctx context.Context, scope targetenrollment.Scope, serial string, expires time.Time) error {
	normalized, err := targetenrollment.NormalizeCertificateSerial(serial)
	if ctx.Err() != nil || s == nil || s.pool == nil || scope.Validate() != nil || err != nil || !expires.After(time.Now()) {
		return targetenrollment.ErrDenied
	}
	err = WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		var admitted bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM connector_nodes
WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3 AND runtime=$4
AND NOT certificate_revoked AND (
(ltrim(replace(lower(certificate_serial), ':', ''), '0')=$5 AND certificate_expires_at=$6
AND certificate_expires_at > clock_timestamp()) OR EXISTS(
SELECT 1 FROM connector_certificate_overlap overlap
WHERE overlap.tenant_id=connector_nodes.tenant_id AND overlap.node_id=connector_nodes.node_id
AND overlap.certificate_serial=$5 AND overlap.certificate_expires_at=$6
AND overlap.admit_until > clock_timestamp() AND NOT overlap.certificate_revoked)))`,
			scope.TenantID, scope.NodeID, scope.TargetID, scope.Runtime, normalized, expires).Scan(&admitted); err != nil {
			return err
		}
		if !admitted {
			return targetenrollment.ErrDenied
		}
		return nil
	})
	if err != nil {
		return targetenrollment.ErrDenied
	}
	return nil
}

// RevokeCertificate persists the admission denial before provider revocation.
// A provider outage must never make a retired node identity active again.
func (s *ConnectorEnrollmentStore) RevokeCertificate(ctx context.Context, scope targetenrollment.Scope, serial string) error {
	normalized, err := targetenrollment.NormalizeCertificateSerial(serial)
	if ctx.Err() != nil || s == nil || s.pool == nil || scope.Validate() != nil || err != nil {
		return targetenrollment.ErrDenied
	}
	return WithTenantTx(ctx, s.pool, scope.TenantID, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, `SELECT certificate_serial FROM connector_nodes
WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3 AND runtime=$4 FOR UPDATE`,
			scope.TenantID, scope.NodeID, scope.TargetID, scope.Runtime).Scan(&current); err != nil {
			return targetenrollment.ErrDenied
		}
		current, err := targetenrollment.NormalizeCertificateSerial(current)
		if err != nil {
			return targetenrollment.ErrDenied
		}
		if current != normalized {
			command, err := tx.Exec(ctx, `UPDATE connector_certificate_overlap SET certificate_revoked=true
WHERE tenant_id=$1 AND node_id=$2 AND certificate_serial=$3`, scope.TenantID, scope.NodeID, normalized)
			if err != nil || command.RowsAffected() != 1 {
				return targetenrollment.ErrDenied
			}
			return nil
		}
		command, err := tx.Exec(ctx, `UPDATE connector_nodes SET certificate_revoked=true
WHERE tenant_id=$1 AND node_id=$2 AND target_id=$3 AND runtime=$4`, scope.TenantID, scope.NodeID, scope.TargetID, scope.Runtime)
		if err != nil || command.RowsAffected() != 1 {
			return targetenrollment.ErrDenied
		}
		return nil
	})
}

var _ targetenrollment.NodeRegistry = (*ConnectorEnrollmentStore)(nil)
