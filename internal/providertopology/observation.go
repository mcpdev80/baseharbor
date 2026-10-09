package providertopology

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/availability"
)

// Observation separates live native provider roles from data replication proof.
// Multiple independently stored Prometheus TSDBs and stateless receivers are
// explicitly not reported as replicated datasets.
type Observation struct {
	Provider, Component, Replication string
	Instance                         string
	RequestedHA                      bool
	Members, DataMembers, Helpers    int
	DeclaredMembers, DeclaredHelpers int
	MemberNames, HelperNames         []string
}

func (o Observation) Detail() string {
	active := o.Members > 1
	if o.DeclaredMembers <= 1 && o.Replication == "configured-not-proven" {
		o.Replication = "0"
	}
	return fmt.Sprintf("provider=%s instance=%s ha-requested=%t ha-active=%t data-members=%d service-members=%d replicas=%s auxiliary-services=%d/%d configured-service-members=%d members=[%s] helpers=[%s] process-redundancy=%t failover-proof=not-collected host-failure-tolerance=false", o.Provider, o.Instance, o.RequestedHA, active, o.DataMembers, o.Members, o.Replication, o.Helpers, o.DeclaredHelpers, o.DeclaredMembers, strings.Join(o.MemberNames, ","), strings.Join(o.HelperNames, ","), active)
}

// Observe uses protected Compose ownership and the native running-service
// inventory. It never infers membership from the total container count.
func Observe(compose string, running []string, intent availability.Intent, explicitGroups ...map[string]string) ([]Observation, error) {
	services, err := ServiceNames(compose)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, name := range running {
		live[name] = true
	}
	groupNames := map[string]string{}
	if len(explicitGroups) > 0 {
		groupNames = explicitGroups[0]
	}
	groups := map[string]*Observation{}
	for name, raw := range services {
		spec, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid retained service role for %s", name)
		}
		image, _ := spec["image"].(string)
		provider, component, replication, data, member := classifyRole(name, image, spec["command"])
		if provider == "" {
			continue
		}
		instance := provider
		if provider == "postgresql" || provider == "valkey" || provider == "rabbitmq" || provider == "mongodb" {
			instance = datastoreRoot(name, services, groupNames)
			if strings.HasPrefix(instance, "key_value/") {
				component = "key_value"
			}
		}
		key := provider + "/" + instance
		o := groups[key]
		if o == nil {
			o = &Observation{Provider: provider, Instance: instance, Component: component, Replication: replication, RequestedHA: intent.Resolve(component).HA}
			groups[key] = o
		}
		if member {
			o.DeclaredMembers++
		} else {
			o.DeclaredHelpers++
		}
		if !live[name] {
			continue
		}
		if member {
			o.Members++
			o.MemberNames = append(o.MemberNames, name)
		} else {
			o.Helpers++
			o.HelperNames = append(o.HelperNames, name)
		}
		if data {
			o.DataMembers++
		}
	}
	var result []Observation
	for _, o := range groups {
		sort.Strings(o.MemberNames)
		sort.Strings(o.HelperNames)
		result = append(result, *o)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Provider+result[i].Instance < result[j].Provider+result[j].Instance
	})
	return result, nil
}

func classifyRole(name, image string, command any) (provider, component, replication string, data, member bool) {
	auxiliary := strings.HasSuffix(name, "-admin") || strings.HasSuffix(name, "-init") || strings.Contains(name, "-access") || strings.Contains(name, "-sentinel") || strings.Contains(name, "-etcd-") || strings.HasSuffix(name, "-ui") || strings.Contains(name, "pgadmin") || strings.Contains(name, "cache-ui") || strings.Contains(name, "redisinsight")
	switch {
	case strings.HasPrefix(name, "seaweedfs-"):
		return "seaweedfs", "object_storage", "configured-not-proven", strings.HasPrefix(name, "seaweedfs-node-"), strings.HasPrefix(name, "seaweedfs-node-")
	case strings.Contains(image, "prometheus:") || strings.HasPrefix(name, "prometheus"):
		member = strings.Contains(image, "prometheus:") && !auxiliary
		return "prometheus", "metrics", "0-independent-tsdb", member, member
	case strings.Contains(image, "opentelemetry-collector") || strings.HasPrefix(name, "otel-collector"):
		return "otel-collector", "telemetry", "0-stateless", false, !auxiliary
	case strings.HasPrefix(name, "keycloak-db"):
		member = !auxiliary && (strings.Contains(image, "postgres:") || strings.Contains(image, "spilo-"))
		return "keycloak-sql", "identity", "configured-not-proven", member, member
	case strings.Contains(image, "keycloak:"):
		return "keycloak", "identity", "0-shared-sql", false, !auxiliary
	case strings.Contains(image, "pgadmin") || strings.Contains(name, "pgadmin") || strings.Contains(image, "postgres:") || strings.Contains(image, "spilo-") || strings.HasPrefix(name, "postgres") || strings.HasPrefix(name, "shared-postgres"):
		member = !auxiliary && (strings.Contains(image, "postgres:") || strings.Contains(image, "spilo-"))
		return "postgresql", "sql", "configured-not-proven", member, member
	case strings.Contains(image, "openbao:") || strings.HasPrefix(name, "openbao"):
		return "openbao", "secrets", "0-shared-sql", false, !auxiliary && strings.Contains(image, "openbao:")
	case strings.Contains(name, "cache-ui") || strings.Contains(image, "redisinsight") || strings.Contains(image, "valkey:") || strings.HasPrefix(name, "valkey") || strings.HasPrefix(name, "shared-valkey"):
		member = !auxiliary && strings.Contains(image, "valkey:")
		return "valkey", "cache", "configured-not-proven", member, member
	case strings.Contains(image, "rabbitmq:") || strings.HasPrefix(name, "rabbitmq"):
		member = !auxiliary && strings.Contains(image, "rabbitmq:")
		return "rabbitmq", "messaging", "configured-not-proven", member, member
	case strings.Contains(image, "mongo:") || strings.HasPrefix(name, "mongodb"):
		member = !auxiliary && strings.Contains(image, "mongo:")
		return "mongodb", "document_database", "configured-not-proven", member, member
	case strings.Contains(image, "loki:") || strings.Contains(name, "loki") || strings.Contains(image, "alloy:"):
		member = strings.Contains(image, "loki:") && !auxiliary
		return "loki", "logs", "configured-not-proven", member, member
	case strings.Contains(image, "tempo:") || strings.HasPrefix(name, "tempo"):
		if strings.Contains(image, "redpandadata/redpanda:") {
			return "tempo-kafka", "traces", "configured-not-proven", !auxiliary, !auxiliary
		}
		member = strings.Contains(image, "tempo:") && !auxiliary
		text := fmt.Sprint(command)
		data = member && (name == "tempo" || strings.Contains(text, "live-store") || strings.Contains(text, "block-builder"))
		return "tempo", "traces", "configured-not-proven", data, member
	case strings.Contains(image, "redpandadata/redpanda:"):
		return "tempo-kafka", "traces", "configured-not-proven", !auxiliary, !auxiliary
	}
	return "", "", "", false, false
}

func datastoreRoot(name string, services map[string]any, explicit map[string]string) string {
	if root, exists := explicit[name]; exists {
		return root
	}
	// Member and etcd names have an unambiguous provider-owned ordinal.
	for _, marker := range []string{"-member-", "-etcd-", "-sentinel-"} {
		if i := strings.LastIndex(name, marker); i >= 0 {
			if n, err := strconv.Atoi(name[i+len(marker):]); err == nil && n > 0 {
				return datastoreRoot(name[:i], services, explicit)
			}
		}
	}
	for _, suffix := range []string{"-access", "-admin", "-init", "-sentinel", "-ui"} {
		if strings.HasSuffix(name, suffix) {
			return datastoreRoot(strings.TrimSuffix(name, suffix), services, explicit)
		}
	}
	if i := strings.LastIndex(name, "-"); i >= 0 {
		if n, err := strconv.Atoi(name[i+1:]); err == nil && n > 1 {
			base := name[:i]
			if _, exists := services[base]; exists {
				return datastoreRoot(base, services, explicit)
			}
		}
	}
	return name
}
