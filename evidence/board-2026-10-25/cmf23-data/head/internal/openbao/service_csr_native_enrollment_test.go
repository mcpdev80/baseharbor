package openbao

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// The caller validates the isolated native fixture DSN before this helper runs.
// OpenBao storage and Core admission use separate tables in that disposable DB.
func nativeBaoEnrollmentAuthority(t *testing.T, ctx context.Context, dsn string, issuer serviceaccess.CSRIssuer) (*targetenrollment.Authority, *database.ConnectorEnrollmentStore) {
	t.Helper()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal("native enrollment migration failed", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO tenants(id,slug,name) VALUES('11111111-1111-4111-8111-111111111111','native-pki','Native PKI fixture')"); err != nil {
		t.Fatal("isolated enrollment tenant setup failed", err)
	}
	registry := database.NewConnectorEnrollmentStore(pool)
	authority, err := targetenrollment.New(registry, issuer)
	if err != nil {
		t.Fatal(err)
	}
	return authority, registry
}

func nativeBaoEnroll(t *testing.T, ctx context.Context, authority *targetenrollment.Authority, scope targetenrollment.Scope, csr []byte, renewal bool) serviceaccess.IssuedCertificate {
	t.Helper()
	create := authority.Create
	if renewal {
		create = authority.CreateRenewal
	}
	grant, err := create(ctx, scope, time.Minute, time.Hour)
	if err != nil {
		t.Fatal("native enrollment authorization failed", err)
	}
	request := targetenrollment.Request{Scope: scope, Token: grant.Token, Nonce: grant.Nonce, CSRPEM: csr}
	result, err := authority.Enroll(ctx, request)
	if err != nil {
		t.Fatal("native persisted CSR exchange failed", err)
	}
	if _, err := authority.Enroll(ctx, request); err == nil {
		t.Fatal("native enrollment authorization replay succeeded")
	}
	return result.Certificate
}
