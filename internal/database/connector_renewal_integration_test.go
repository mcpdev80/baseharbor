package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

func renewalDatabase(t *testing.T) (context.Context, *pgxpool.Pool, targetenrollment.Scope) {
	t.Helper()
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BASEHARBOR_TEST_DATABASE_URL is required for persistent renewal qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := resetTestDatabase(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "lab", NodeID: "node-a", Runtime: "docker"}
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id,slug,name) VALUES ($1,'renewal','Renewal'),($2,'foreign','Foreign')`,
		scope.TenantID, "22222222-2222-4222-8222-222222222222"); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, scope
}

func renewalGrant(scope targetenrollment.Scope, token, nonce string) targetenrollment.Grant {
	return targetenrollment.Grant{Scope: scope, TokenDigest: repeatDigest(token), NonceDigest: repeatDigest(nonce),
		ExpiresAt: time.Now().Add(time.Minute), CertificateTTL: time.Hour}
}

func consumeRenewalGrant(t *testing.T, ctx context.Context, store *ConnectorEnrollmentStore, grant targetenrollment.Grant) {
	t.Helper()
	if _, err := store.Consume(ctx, grant.Scope, grant.TokenDigest, grant.NonceDigest, repeatDigest("f"), time.Now()); err != nil {
		t.Fatal("authorized one-use grant did not consume", err)
	}
}

func TestConnectorRenewalPersistentOverlapRetirementAndConcurrentRevocation(t *testing.T) {
	ctx, pool, scope := renewalDatabase(t)
	store := NewConnectorEnrollmentStore(pool)
	initial := renewalGrant(scope, "a", "b")
	if err := store.Create(ctx, initial); err != nil {
		t.Fatal(err)
	}
	consumeRenewalGrant(t, ctx, store, initial)
	expires := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	if err := store.RecordIssued(ctx, scope, initial.TokenDigest, "00:AB:CD", expires); err != nil {
		t.Fatal(err)
	}
	renewal := renewalGrant(scope, "c", "d")
	if err := store.Create(ctx, renewal); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("initial enrollment replaced an issued identity", err)
	}
	if err := store.CreateRenewal(ctx, renewal); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRenewal(ctx, renewalGrant(scope, "e", "f")); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("competing pending renewal was accepted", err)
	}
	for _, dimension := range []string{"tenant", "target", "node", "runtime"} {
		foreign := scope
		switch dimension {
		case "tenant":
			foreign.TenantID = "22222222-2222-4222-8222-222222222222"
		case "target":
			foreign.TargetID = "foreign"
		case "node":
			foreign.NodeID = "foreign"
		case "runtime":
			foreign.Runtime = "podman"
		}
		if err := store.CreateRenewal(ctx, renewalGrant(foreign, "e", "f")); !errors.Is(err, targetenrollment.ErrDenied) {
			t.Fatal("renewal escaped immutable scope", dimension, err)
		}
	}
	consumeRenewalGrant(t, ctx, store, renewal)
	if err := store.RecordIssued(ctx, scope, renewal.TokenDigest, "ef01", expires); err != nil {
		t.Fatal(err)
	}
	for _, serial := range []string{"abcd", "ef01"} {
		if err := store.AdmitCertificate(ctx, scope, serial, expires); err != nil {
			t.Fatal("bounded overlap denied an active certificate", serial, err)
		}
	}
	if err := store.RecordIssued(ctx, scope, renewal.TokenDigest, "1234", expires); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("consumed renewal was applied a second time", err)
	}
	var count int
	var maximum bool
	if err := pool.QueryRow(ctx, `SELECT count(*),bool_and(admit_until<=clock_timestamp()+interval '5 minutes')
FROM connector_certificate_overlap WHERE tenant_id=$1 AND node_id=$2`, scope.TenantID, scope.NodeID).Scan(&count, &maximum); err != nil || count != 1 || !maximum {
		t.Fatal("overlap slot is not uniquely time bounded", count, maximum, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE connector_certificate_overlap SET admit_until=clock_timestamp()-interval '1 second'
WHERE tenant_id=$1 AND node_id=$2`, scope.TenantID, scope.NodeID); err != nil {
		t.Fatal(err)
	}
	if err := store.AdmitCertificate(ctx, scope, "abcd", expires); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("expired predecessor was still admitted", err)
	}
	if err := store.AdmitCertificate(ctx, scope, "ef01", expires); err != nil {
		t.Fatal("overlap retirement damaged current admission", err)
	}
	inFlight := renewalGrant(scope, "1", "2")
	if err := store.CreateRenewal(ctx, inFlight); err != nil {
		t.Fatal(err)
	}
	consumeRenewalGrant(t, ctx, store, inFlight)
	if err := store.RevokeCertificate(ctx, scope, "ef01"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordIssued(ctx, scope, inFlight.TokenDigest, "3456", expires); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("in-flight signing resurrected a revoked node", err)
	}
	if err := store.CreateRenewal(ctx, renewalGrant(scope, "3", "4")); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("revoked node could authorize its own replacement", err)
	}
	for _, serial := range []string{"abcd", "ef01", "3456"} {
		if err := store.AdmitCertificate(ctx, scope, serial, expires); !errors.Is(err, targetenrollment.ErrDenied) {
			t.Fatal("revoked identity retained admission", serial, err)
		}
	}
	pool.Close()
	reopened, err := pgxpool.New(ctx, os.Getenv("BASEHARBOR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := NewConnectorEnrollmentStore(reopened).AdmitCertificate(ctx, scope, "ef01", expires); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("restart forgot persisted renewal revocation", err)
	}
}

func TestConnectorRenewalPredecessorRevocationKeepsReplacementAndBlocksRevokedGrant(t *testing.T) {
	ctx, pool, scope := renewalDatabase(t)
	store := NewConnectorEnrollmentStore(pool)
	expires := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	initial, renewal := renewalGrant(scope, "a", "b"), renewalGrant(scope, "c", "d")
	if err := store.Create(ctx, initial); err != nil {
		t.Fatal(err)
	}
	consumeRenewalGrant(t, ctx, store, initial)
	if err := store.RecordIssued(ctx, scope, initial.TokenDigest, "abcd", expires); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRenewal(ctx, renewal); err != nil {
		t.Fatal(err)
	}
	consumeRenewalGrant(t, ctx, store, renewal)
	if err := store.RecordIssued(ctx, scope, renewal.TokenDigest, "ef01", expires); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeCertificate(ctx, scope, "abcd"); err != nil {
		t.Fatal(err)
	}
	if err := store.AdmitCertificate(ctx, scope, "abcd", expires); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("revoked predecessor was admitted", err)
	}
	if err := store.AdmitCertificate(ctx, scope, "ef01", expires); err != nil {
		t.Fatal("predecessor revocation revoked its replacement", err)
	}
	pending := renewalGrant(scope, "1", "2")
	if err := store.CreateRenewal(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeCertificate(ctx, scope, "ef01"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(ctx, scope, pending.TokenDigest, pending.NonceDigest, repeatDigest("f"), time.Now()); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("grant consumed after its authorized current certificate was revoked", err)
	}
}
