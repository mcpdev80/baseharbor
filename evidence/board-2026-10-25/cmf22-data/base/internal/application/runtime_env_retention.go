package application

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

var protectedRuntimeEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateProtectedRuntimeValues(values map[string]string) error {
	for key, value := range values {
		if !protectedRuntimeEnvKey.MatchString(key) || strings.ContainsAny(value, "\r\n\x00") {
			return errors.New("protected application runtime environment contains an invalid entry; retained material was not rewritten")
		}
	}
	return nil
}

// runtime.env is the existing protected credential/recovery owner. Removing a
// capability stops its use, not this material. Workload and binding projection
// still select only the current Manifest's capabilities.
func appendRetainedRuntimeValues(b *strings.Builder, values map[string]string) {
	emitted := map[string]bool{}
	for _, line := range strings.Split(b.String(), "\n") {
		if key, _, ok := strings.Cut(line, "="); ok {
			emitted[key] = true
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if !emitted[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString(key + "=" + values[key] + "\n")
	}
}
