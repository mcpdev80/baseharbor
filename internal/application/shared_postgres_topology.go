package application

import "fmt"

func sharedPostgresMemberCount(state sharedBackendState) int {
	if state.PostgresMembers > 0 {
		return state.PostgresMembers
	}
	return 1
}

func selectSharedPostgresTopology(state *sharedBackendState, m Manifest) error {
	if !UsesSharedPostgreSQL(m) {
		return nil
	}
	requirement := AvailabilityIntent(m).Resolve("sql")
	if !requirement.HA && requirement.Instances > 1 {
		return fmt.Errorf("shared PostgreSQL single-instance realization cannot satisfy instances=%d with ha:false", requirement.Instances)
	}
	wanted := postgresMemberCount(m)
	if wanted != 1 && (wanted < 3 || wanted%2 == 0) {
		return fmt.Errorf("shared PostgreSQL HA requires an odd member count of at least three")
	}
	if state.PostgresMembers != 0 && state.PostgresMembers != wanted {
		return fmt.Errorf("shared PostgreSQL topology conflict: existing provider has %d member(s), application requests %d; use another target/environment or explicitly recreate the provider after backing up its data", state.PostgresMembers, wanted)
	}
	state.PostgresMembers = wanted
	return nil
}

// CheckSharedPostgresTopologyAt is read-only and rejects changing a retained
// provider's storage realization before runtime or credential mutation.
func CheckSharedPostgresTopologyAt(dataDir, namespace string, m Manifest) error {
	if !HasSharedBackends(m) {
		return nil
	}
	files := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(files.State, m.Environment)
	if err != nil {
		return err
	}
	if state.CoreSQL != nil && UsesSharedPostgreSQL(m) {
		requirement := AvailabilityIntent(m).Resolve("sql")
		members := 1
		if state.CoreSQL.HA {
			members = 3
		}
		if requirement.HA && !state.CoreSQL.HA || requirement.Instances > 0 && requirement.Instances != members {
			return fmt.Errorf("shared PostgreSQL intent differs from the existing Core topology; explicit matching Core or application placement is required")
		}
		return checkSharedValkeyTopology(state, m)
	}
	if err := checkSharedValkeyTopology(state, m); err != nil {
		return err
	}
	return selectSharedPostgresTopology(&state, m)
}
