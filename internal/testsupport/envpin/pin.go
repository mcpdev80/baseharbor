// Package envpin builds child process environments so that a controlled key is
// never overridable by an inherited copy of the same key.
//
// It exists as a shared helper because the pinned-env call sites span multiple
// packages (corebackup/etcd, coreupdate, stableid). internal/testsupport is the
// shared runtime/support package and imports none of those, so envpin here has
// no import-cycle risk.
package envpin

import "os"

// PinnedEnv builds the child environment so controlled keys are never overridable
// by the parent environment. It copies os.Environ(), removes every key named in
// pins, and appends the pin values last. Dropping the inherited copy is
// mandatory: with Go's env-dedup (last value wins) an inherited ETCDCTL_* key
// placed before the pin would win and silently override the pin, making the
// pinned endpoint and TLS trust swappable by a parent process. The result is
// therefore independent of Go's dedup rule.
func PinnedEnv(pins ...string) []string {
	dropped := make(map[string]struct{}, len(pins))
	for _, p := range pins {
		if i := indexByte(p, '='); i >= 0 {
			dropped[p[:i]] = struct{}{}
		}
	}
	out := make([]string, 0, len(os.Environ())+len(pins))
	for _, kv := range os.Environ() {
		if i := indexByte(kv, '='); i >= 0 {
			if _, bad := dropped[kv[:i]]; bad {
				continue
			}
		}
		out = append(out, kv)
	}
	return append(out, pins...)
}

// indexByte returns the offset of the first byte c in s, or -1 when absent.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
