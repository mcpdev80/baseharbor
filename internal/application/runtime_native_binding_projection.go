package application

// Native Valkey authenticates its default user through requirepass. Shared ACL
// identity remains in the existing protected owner for a later return; only the
// consumed native binding changes principal, with the same retained password.
func nativeRuntimeBindingValues(m Manifest, retained map[string]string) map[string]string {
	values := make(map[string]string, len(retained))
	for key, value := range retained {
		values[key] = value
	}
	if !UsesSharedValkey(m) {
		for _, instance := range ValkeyInstanceNames(m) {
			if values[valkeyRuntimeKey(instance, "USER")] != "" {
				values[valkeyRuntimeKey(instance, "USER")] = "default"
			}
		}
	}
	return values
}
