package runtime

import "strings"

// IsPortBindingConflict reports whether a Docker/Podman Compose failure is a
// host-port allocation conflict. Runtime engines phrase the same condition
// differently, so keep the classifier deliberately narrow but multi-engine.
func IsPortBindingConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"port is already allocated",
		"address already in use",
		"failed to bind host port",
		"bind for 127.0.0.1:",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
