package application

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/providertopology"
)

// CheckRuntimeTopology prevents reconciliation from turning an existing data
// service into a different storage topology. Adding a new logical instance is
// allowed; changes to retained members require an explicit migration.
func CheckRuntimeTopology(files RuntimeFiles, m Manifest) error {
	services, err := providertopology.ServiceNames(files.Compose)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	groups := RuntimeTopologyGroups(m)
	check := func(base string, wanted int, postgres bool) error {
		actual := 0
		for name := range services {
			if group, known := groups[name]; known && group != groups[base] {
				continue
			}
			if name == base {
				actual++
				continue
			}
			prefix := base + "-"
			if postgres {
				prefix = base + "-member-"
			}
			if strings.HasPrefix(name, prefix) {
				ordinal, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
				if err == nil && ordinal > 0 {
					actual++
				}
			}
		}
		if actual > 0 && actual != wanted {
			return fmt.Errorf("provider topology change blocked: %s retains %d data members, requested %d; preserve existing intent or use an explicitly supported data migration", base, actual, wanted)
		}
		return nil
	}
	if !UsesSharedPostgreSQL(m) {
		for _, instance := range SQLInstanceNames(m) {
			if err := check(runtimeServiceName("postgres", instance), postgresMemberCount(m), true); err != nil {
				return err
			}
		}
	}
	if !UsesSharedValkey(m) {
		for _, instance := range ValkeyInstanceNames(m) {
			if err := check(runtimeServiceName("valkey", instance), valkeyMemberCount(m, instance), false); err != nil {
				return err
			}
		}
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		if err := check(runtimeServiceName("rabbitmq", instance), rabbitmqMemberCount(m), false); err != nil {
			return err
		}
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		if err := check(runtimeServiceName("mongodb", instance), mongodbMemberCount(m, instance), false); err != nil {
			return err
		}
	}
	return nil
}

func checkSharedValkeyTopology(state sharedBackendState, m Manifest) error {
	if !UsesSharedValkey(m) {
		return nil
	}
	app := state.Applications[sharedBackendApplicationKey(m)]
	for _, instance := range ValkeyInstanceNames(m) {
		if resource, exists := app.Cache[instance]; exists && sharedValkeyMemberCount(resource) != valkeyMemberCount(m, instance) {
			return fmt.Errorf("shared Valkey topology conflict: %s retains %d data members, requested %d; preserve existing intent or use an explicitly supported data migration", instance, sharedValkeyMemberCount(resource), valkeyMemberCount(m, instance))
		}
	}
	return nil
}

// RuntimeTopologyGroups keeps independent logical instances separate from
// replica ordinals in status/doctor, including names ending in a number.
func RuntimeTopologyGroups(m Manifest) map[string]string {
	groups := map[string]string{}
	for _, instance := range ValkeyInstanceNames(m) {
		root := valkeyAvailabilityComponent(m, instance) + "/" + instance
		base := runtimeServiceName("valkey", instance)
		groups[base] = root
		groups[sharedValkeyService(m, instance)] = root
		for n := 0; n < valkeyMemberCount(m, instance); n++ {
			groups[valkeyMemberServiceName(instance, n)] = root
			groups[valkeySentinelServiceName(instance, n)] = root
			app := sharedBackendAppState{Application: m.Name, Environment: m.Environment}
			groups[sharedValkeyMemberServiceName(app, instance, n)] = root
			groups[sharedValkeySentinelServiceName(app, instance, n)] = root
		}
	}
	for _, instance := range SQLInstanceNames(m) {
		groups[runtimeServiceName("postgres", instance)] = "sql/" + instance
	}
	groups[sharedPostgresService(m.Environment)] = "sql/environment-" + m.Environment
	for _, instance := range RabbitMQInstanceNames(m) {
		groups[runtimeServiceName("rabbitmq", instance)] = "messaging/" + instance
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		groups[runtimeServiceName("mongodb", instance)] = "document_database/" + instance
	}
	return groups
}
