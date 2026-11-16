package application

import (
	"context"
	"errors"
	"fmt"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func verifyCoreSharedPostgreSQL(ctx context.Context, executor bhruntime.SQLConsumerExecutor, shared SharedBackendFiles, state sharedBackendState, m Manifest) error {
	key := sharedBackendApplicationKey(m)
	if err := verifySharedPostgresStateOwnership(state, key); err != nil {
		return err
	}
	for _, resource := range state.Applications[key].SQL {
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return err
		}
		out, err := sharedCoreSQLQuery(ctx, executor, *state.CoreSQL, resource.Username, password, resource.Database, "SELECT 1;")
		if err != nil || out != "1" {
			return errors.New("shared application SQL authentication/readiness is not verified")
		}
		if err := verifyCoreSQLResourceOwnership(ctx, executor, state, resource); err != nil {
			return err
		}
		denied := []string{"postgres", "template1", "openbao", "baseharbor_identity"}
		for otherKey, app := range state.Applications {
			if otherKey != key {
				for _, other := range app.SQL {
					denied = append(denied, other.Database)
				}
			}
		}
		for _, database := range denied {
			if _, err := sharedCoreSQLQuery(ctx, executor, *state.CoreSQL, resource.Username, password, database, "SELECT 1;"); err == nil {
				return errors.New("shared application SQL can access another consumer or provider database")
			}
		}
	}
	return nil
}

func verifyCoreSQLResourceOwnership(ctx context.Context, executor bhruntime.SQLConsumerExecutor, state sharedBackendState, resource sharedPostgresResource) error {
	owner := ""
	for key, app := range state.Applications {
		for _, candidate := range app.SQL {
			if candidate.Database == resource.Database && candidate.Username == resource.Username {
				if owner != "" {
					return errors.New("shared SQL resource has ambiguous ownership")
				}
				owner = "baseharbor:shared:application:" + key + ":" + resource.Database
			}
		}
	}
	if owner == "" {
		return errors.New("shared SQL resource is not registered")
	}
	query := fmt.Sprintf("SELECT r.rolname FROM pg_database d JOIN pg_roles r ON r.oid=d.datdba WHERE d.datname=%s AND r.rolname=%s AND NOT r.rolsuper AND NOT r.rolcreatedb AND NOT r.rolcreaterole AND NOT r.rolreplication AND NOT r.rolbypassrls AND shobj_description(d.oid, 'pg_database')=%s AND shobj_description(r.oid, 'pg_authid')=%s;", quotePostgresLiteral(resource.Database), quotePostgresLiteral(resource.Username), quotePostgresLiteral(owner), quotePostgresLiteral(owner))
	out, err := sharedCoreSQLQuery(ctx, executor, *state.CoreSQL, "", "", "postgres", query)
	if err != nil || out != resource.Username {
		return errors.New("shared SQL database/role ownership is not verified; Core and foreign data are retained")
	}
	return nil
}
