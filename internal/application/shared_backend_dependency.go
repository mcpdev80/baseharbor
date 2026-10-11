package application

import (
	"errors"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Preserve an existing selected SQL dependency; new local deployments can use
// the existing standalone shared-provider renderer with an independent issuer.
// No provider, data or trust migration is inferred from capability changes.
func selectSharedBackendDependency(state *sharedBackendState, m Manifest, issuer serviceaccess.Issuer, namespace string) (bhruntime.Files, error) {
	if issuer == nil {
		return bhruntime.Files{}, errors.New("shared providers require a verified service-access issuer")
	}
	state.Environment = "core"
	if err := checkSharedValkeyTopology(*state, m); err != nil {
		return bhruntime.Files{}, err
	}
	dependency, ok := issuer.(bhruntime.CoreDependencyProvider)
	if !ok {
		if state.CoreSQL != nil {
			return bhruntime.Files{}, errors.New("existing shared SQL binding requires its selected provider; refusing implicit migration")
		}
		if err := selectSharedPostgresTopology(state, m); err != nil {
			return bhruntime.Files{}, err
		}
		return bhruntime.Files{}, nil
	}
	core, err := dependency.CoreRuntimeFiles()
	if err != nil {
		return bhruntime.Files{}, err
	}
	if core.Project != bhruntime.SharedProjectName(namespace) || core.ResourceProject != bhruntime.SharedResourceProjectName(namespace) {
		return bhruntime.Files{}, errors.New("shared backend dependency does not belong to the selected Target")
	}
	if sharedBackendUsesStandalonePostgres(*state) {
		return bhruntime.Files{}, errors.New("retained standalone providers cannot be implicitly migrated into management SQL")
	}
	if state.CoreSQL != nil && (state.CoreSQL.Project != core.Project || state.CoreSQL.Compose != core.Compose || state.CoreSQL.Env != core.Env) {
		return bhruntime.Files{}, errors.New("shared backend SQL binding changed; explicit migration is required")
	}
	if UsesSharedPostgreSQL(m) {
		requirement := AvailabilityIntent(m).Resolve("sql")
		members := 1
		if core.HA {
			members = 3
		}
		if requirement.HA && !core.HA || requirement.Instances > 0 && requirement.Instances != members {
			return bhruntime.Files{}, errors.New("shared PostgreSQL intent differs from the selected SQL topology")
		}
		state.PostgresMembers = members
	}
	state.CoreSQL = &core
	return core, nil
}
