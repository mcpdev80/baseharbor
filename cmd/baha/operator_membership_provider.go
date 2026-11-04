package main

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

const operatorDatabaseURLEnv = "BASEHARBOR_API_DATABASE_URL"

type cliOperatorMembershipProvider struct {
	databaseURL string
	once        sync.Once
	pool        *pgxpool.Pool
	resolver    *database.IdentityTenantResolver
	err         error
}

func newCLIOperatorMembershipProvider() *cliOperatorMembershipProvider {
	return &cliOperatorMembershipProvider{databaseURL: strings.TrimSpace(os.Getenv(operatorDatabaseURLEnv))}
}

func (p *cliOperatorMembershipProvider) ResolveClaimMembership(ctx context.Context, principal *identity.Principal) ([]tenancy.Membership, error) {
	p.once.Do(func() {
		if p.databaseURL == "" {
			p.err = database.ErrMissingDSN
			return
		}
		p.pool, p.err = database.Open(ctx, database.Config{DSN: p.databaseURL, ConnectTimeout: 10 * time.Second})
		if p.err == nil {
			p.resolver = database.NewIdentityTenantResolver(p.pool)
		}
	})
	if p.err != nil {
		return nil, p.err
	}
	return p.resolver.ResolveClaimMembership(ctx, principal)
}

func (p *cliOperatorMembershipProvider) Close() {
	if p != nil && p.pool != nil {
		p.pool.Close()
	}
}

var _ operatorauth.MembershipProvider = (*cliOperatorMembershipProvider)(nil)
