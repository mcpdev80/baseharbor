package application

import (
	"strconv"
	"strings"
)

// Each retained native realization owns its published port independently of
// the current shared binding. Both runtimes may coexist until verified handoff.
func ensureNativeBackendEndpointValues(values map[string]string, m Manifest) error {
	excluded := map[int]struct{}{}
	for key, value := range values {
		if strings.HasSuffix(key, "_HOST_PORT") && value != "" {
			if err := validatePortValue(value, key); err != nil {
				return err
			}
			port, _ := strconv.Atoi(value)
			excluded[port] = struct{}{}
		}
	}
	ensure := func(portKey, hostKey, nativeHost string, shared bool) error {
		ownerKey := strings.TrimSuffix(portKey, "HOST_PORT") + "APPLICATION_HOST_PORT"
		if values[ownerKey] == "" {
			if values[hostKey] == "" || values[hostKey] == nativeHost {
				values[ownerKey] = values[portKey]
			} else {
				port, err := allocateLoopbackPort(excluded)
				if err != nil {
					return err
				}
				values[ownerKey] = strconv.Itoa(port)
				excluded[port] = struct{}{}
			}
		}
		if !shared {
			values[portKey] = values[ownerKey]
			values[hostKey] = nativeHost
		}
		return nil
	}
	for _, instance := range SQLInstanceNames(m) {
		if err := ensure(postgresRuntimeKey(instance, "HOST_PORT"), postgresContainerHostKey(instance), postgresAccessService(instance), UsesSharedPostgreSQL(m)); err != nil {
			return err
		}
	}
	for _, instance := range ValkeyInstanceNames(m) {
		if err := ensure(valkeyRuntimeKey(instance, "HOST_PORT"), valkeyContainerHostKey(instance), valkeyAccessService(instance), UsesSharedValkey(m)); err != nil {
			return err
		}
	}
	return nil
}

// Each native PostgreSQL realization authenticates its retained data volume
// with the credential material recorded at first materialization. A shared
// placement phase legitimately redirects the consumed POSTGRES_DB/USER/
// PASSWORD slots to the provider-managed shared identity and, through the
// binding projection, overwrites the bindings/ files with the same shared-era
// values; neither may serve as the return source. Like the independently owned
// application host ports above, the dedicated credential material therefore
// keeps its own protected owner slots: the first apply captures them, the
// shared phase leaves them untouched, and a return to native placement restores
// the consumed slots from them. Absent owner slots (states created before this
// material existed) are captured from the current values and never invented.
func ensureNativeBackendCredentialValues(values map[string]string, m Manifest) error {
	shared := UsesSharedPostgreSQL(m)
	for _, instance := range SQLInstanceNames(m) {
		consumed := []string{
			postgresRuntimeKey(instance, "DB"),
			postgresRuntimeKey(instance, "USER"),
			postgresRuntimeKey(instance, "PASSWORD"),
		}
		owner := []string{
			postgresRuntimeKey(instance, "APPLICATION_DATABASE"),
			postgresRuntimeKey(instance, "APPLICATION_ROLE"),
			postgresRuntimeKey(instance, "APPLICATION_SECRET"),
		}
		for i := range consumed {
			current := strings.TrimSpace(values[consumed[i]])
			stored := strings.TrimSpace(values[owner[i]])
			if stored == "" && current != "" {
				values[owner[i]] = current
				continue
			}
			if !shared && stored != "" && current != stored {
				values[consumed[i]] = stored
			}
		}
	}
	return nil
}

// Native Valkey authenticates its default user through requirepass. Shared ACL
// identity remains in the existing protected owner for a later return; only the
// consumed native binding changes principal, with the same retained password.
func nativeRuntimeBindingValues(m Manifest, retained map[string]string) map[string]string {
	values := make(map[string]string, len(retained))
	for key, value := range retained {
		values[key] = value
	}
	if !UsesSharedPostgreSQL(m) {
		for _, instance := range SQLInstanceNames(m) {
			values[postgresContainerHostKey(instance)] = postgresAccessService(instance)
		}
	}
	if !UsesSharedValkey(m) {
		for _, instance := range ValkeyInstanceNames(m) {
			values[valkeyContainerHostKey(instance)] = valkeyAccessService(instance)
			if values[valkeyRuntimeKey(instance, "USER")] != "" {
				values[valkeyRuntimeKey(instance, "USER")] = "default"
			}
		}
	}
	return values
}
