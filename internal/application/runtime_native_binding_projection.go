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
