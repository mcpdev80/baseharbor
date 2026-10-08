package database

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

func TestConnectorEnrollmentPersistentAtomicScope(t *testing.T) {
	dsn := os.Getenv("BASEHARBOR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BASEHARBOR_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := resetTestDatabase(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "lab", NodeID: "node-a", Runtime: "docker"}
	if _, err := pool.Exec(ctx, "INSERT INTO tenants (id,slug,name) VALUES ($1,'enrollment','Enrollment'), ($2,'foreign','Foreign')", scope.TenantID, "22222222-2222-4222-8222-222222222222"); err != nil {
		t.Fatal(err)
	}
	store := NewConnectorEnrollmentStore(pool)
	grant := targetenrollment.Grant{Scope: scope, TokenDigest: repeatDigest("a"), NonceDigest: repeatDigest("b"), ExpiresAt: time.Now().Add(time.Minute), CertificateTTL: time.Hour}
	if err := store.Create(ctx, grant); err != nil {
		t.Fatal(err)
	}

	if err := store.RecordIssued(ctx, scope, grant.TokenDigest, "abcd", time.Now().Add(time.Hour)); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("certificate recorded before bootstrap consumption")
	}
	for _, field := range []string{"pending", "target", "runtime"} {
		other := grant
		other.TokenDigest = repeatDigest("c")
		if field == "target" {
			other.Scope.TargetID = "foreign"
		}
		if field == "runtime" {
			other.Scope.Runtime = "podman"
		}
		if err := store.Create(ctx, other); !errors.Is(err, targetenrollment.ErrDenied) {
			t.Fatalf("ambiguous node authorization (%s) admitted: %v", field, err)
		}
	}
	// Explicit tenant binding is required even when a privileged DB test account
	// bypasses RLS. Production RLS is an additional boundary, not the only one.
	for _, field := range []string{"tenant", "target", "node", "runtime", "nonce"} {
		other := scope
		nonce := grant.NonceDigest
		switch field {
		case "tenant":
			other.TenantID = "22222222-2222-4222-8222-222222222222"
		case "target":
			other.TargetID = "foreign"
		case "node":
			other.NodeID = "foreign"
		case "runtime":
			other.Runtime = "podman"
		case "nonce":
			nonce = repeatDigest("c")
		}
		if _, err := store.Consume(ctx, other, grant.TokenDigest, nonce, repeatDigest("d"), time.Now()); !errors.Is(err, targetenrollment.ErrDenied) {
			t.Fatalf("%s scope mismatch admitted: %v", field, err)
		}
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			if _, err := store.Consume(ctx, scope, grant.TokenDigest, grant.NonceDigest, repeatDigest("d"), time.Now()); err == nil {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("simultaneous DB successes = %d", successes.Load())
	}
	pool.Close()
	reopened, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := NewConnectorEnrollmentStore(reopened).Consume(ctx, scope, grant.TokenDigest, grant.NonceDigest, repeatDigest("e"), time.Now()); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatalf("replay after pool restart: %v", err)
	}
	var storedCSR string
	if err := reopened.QueryRow(ctx, "SELECT csr_digest FROM connector_enrollment_grants WHERE token_digest=$1", grant.TokenDigest).Scan(&storedCSR); err != nil || storedCSR != repeatDigest("d") {
		t.Fatalf("persisted consumed CSR = %q, %v", storedCSR, err)
	}

	if err := NewConnectorEnrollmentStore(reopened).RecordIssued(ctx, scope, repeatDigest("e"), "abcd", time.Now().Add(time.Hour)); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("unrelated token registered node certificate")
	}
	expires := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	if err := NewConnectorEnrollmentStore(reopened).RecordIssued(ctx, scope, grant.TokenDigest, "00:AB:CD", expires); err != nil {
		t.Fatal(err)
	}
	if err := NewConnectorEnrollmentStore(reopened).RecordIssued(ctx, scope, grant.TokenDigest, "ef01", time.Now().Add(time.Hour)); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("duplicate certificate replaced enrolled identity")
	}
	duplicate := grant
	duplicate.TokenDigest = repeatDigest("e")
	if err := NewConnectorEnrollmentStore(reopened).Create(ctx, duplicate); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("enrolled node accepted fresh bootstrap instead of renewal")
	}
	registry := NewConnectorEnrollmentStore(reopened)
	if err := registry.AdmitCertificate(ctx, scope, "abcd", expires); err != nil {
		t.Fatal("persisted certificate was not admitted after store restart", err)
	}
	for _, field := range []string{"tenant", "target", "node", "runtime", "serial", "expiry"} {
		other := scope
		serial, notAfter := "abcd", expires
		switch field {
		case "tenant":
			other.TenantID = "22222222-2222-4222-8222-222222222222"
		case "target":
			other.TargetID = "foreign"
		case "node":
			other.NodeID = "foreign"
		case "runtime":
			other.Runtime = "podman"
		case "serial":
			serial = "cafe"
		case "expiry":
			notAfter = notAfter.Add(time.Second)
		}
		if err := registry.AdmitCertificate(ctx, other, serial, notAfter); !errors.Is(err, targetenrollment.ErrDenied) {
			t.Fatal("certificate registry admitted scope/material drift", field, err)
		}
		if field != "expiry" {
			if err := registry.RevokeCertificate(ctx, other, serial); !errors.Is(err, targetenrollment.ErrDenied) {
				t.Fatal("foreign certificate scope revoked a node", field, err)
			}
		}
	}
	if err := registry.RevokeCertificate(ctx, scope, "00:AB:CD"); err != nil {
		t.Fatal(err)
	}
	if err := registry.RevokeCertificate(ctx, scope, "abcd"); err != nil {
		t.Fatal("certificate revocation was not idempotent", err)
	}
	if err := NewConnectorEnrollmentStore(reopened).AdmitCertificate(ctx, scope, "abcd", expires); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("persisted revocation admitted a still-unexpired CA-trusted certificate", err)
	}
	if err := registry.Create(ctx, duplicate); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("revoked node accepted bootstrap reuse", err)
	}
	grant.Scope.NodeID = "expired-node"
	grant.TokenDigest = repeatDigest("f")
	grant.ExpiresAt = time.Now().Add(-time.Minute)
	if err := NewConnectorEnrollmentStore(reopened).Create(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConnectorEnrollmentStore(reopened).Consume(ctx, grant.Scope, grant.TokenDigest, grant.NonceDigest, repeatDigest("d"), time.Now().Add(-time.Hour)); !errors.Is(err, targetenrollment.ErrDenied) {
		t.Fatal("expired grant accepted through caller clock rollback")
	}
}

func repeatDigest(char string) string {
	value := ""
	for range 64 {
		value += char
	}
	return value
}
