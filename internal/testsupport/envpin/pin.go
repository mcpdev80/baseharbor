// Package envpin builds child process environments so that a controlled key is
// never overridable by an inherited copy of the same key.
//
// It exists as a shared helper because the pinned-env call sites span multiple
// packages (corebackup/etcd, coreupdate, stableid). internal/testsupport is the
// shared runtime/support package and imports none of those, so envpin here has
// no import-cycle risk.
package envpin

import (
	"os"
	"strings"
)

// PinnedEnv builds the child environment so controlled keys are never overridable
// by the parent environment. It copies os.Environ(), removes every key named in
// pins, and appends the pin values last. Dropping the inherited copy is
// mandatory: with Go's env-dedup (last value wins) an inherited ETCDCTL_* key
// placed before the pin would win and silently override the pin, making the
// pinned endpoint and TLS trust swappable by a parent process. The result is
// therefore independent of Go's dedup rule.
//
// Every inherited key that carries the ETCDCTL_ prefix but is NOT one of the
// passed pins is also dropped. etcdctl maps its global flags onto ETCDCTL_*
// variables (for example ETCDCTL_INSECURE_SKIP_TLS_VERIFY selects the
// --insecure-skip-tls-verify global flag), and flags such as insecure-skip-tls,
// username and password are never pinned by a call site. An inherited copy of
// any of these would otherwise reach the child and weaken its TLS verification
// even though the CA, cert and key are pinned. Dropping the whole unpinned
// ETCDCTL_ prefix keeps that door shut while ordinary, non-ETCDCTL_ parent
// variables keep being inherited.
func PinnedEnv(pins ...string) []string {
	dropped := make(map[string]struct{}, len(pins))
	for _, p := range pins {
		if i := indexByte(p, '='); i >= 0 {
			dropped[p[:i]] = struct{}{}
		}
	}
	out := make([]string, 0, len(os.Environ())+len(pins))
	for _, kv := range os.Environ() {
		key := kv
		if i := indexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		// Drop an inherited copy of any explicitly pinned key (PATH, the pinned
		// ETCDCTL_* values, ...) so the pin wins unambiguously, then drop every
		// other inherited key carrying the ETCDCTL_ prefix: etcdctl maps its
		// global flags onto ETCDCTL_* variables, and flags such as
		// insecure-skip-tls, username and password are never pinned by a call
		// site, so an inherited copy could reach the child and weaken its TLS
		// verification even though the CA, cert and key are pinned.
		if strings.HasPrefix(key, "ETCDCTL_") {
			if _, pinned := dropped[key]; pinned {
				continue
			}
			continue
		}
		if _, bad := dropped[key]; bad {
			continue
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
