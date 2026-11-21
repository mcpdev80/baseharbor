package envpin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnpinnedETCDCTLKeyDropped is the fail-first child-process regression for
// card 0428-222 (PR #1749 security re-review). It proves that an inherited
// ETCDCTL_* key that is NOT among the pins never reaches the child, even though
// PinnedEnv copies os.Environ(). With the pre-fix helper (which kept every
// unpinned parent key) this test is RED: an inherited
// ETCDCTL_INSECURE_SKIP_TLS_VERIFY=true survives in the child env. After the
// filter fix the child env contains no unpinned ETCDCTL_* key at all.
//
// The retained inherited ETCDCTL_CACERT assertion (from TestPinnedEnvDropsInheritedKeys
// and TestChildProcessEnvShowsPinNotInherited) is covered here too: the CA pin
// is pinned explicitly and must win, while an inherited non-pinned CA key
// stays dropped.
func TestUnpinnedETCDCTLKeyDropped(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "child_env.txt")

	// writeEnv writes the received env to a file and exits cleanly.
	wrapper := "#!/bin/sh\n" +
		"env > " + quoteShell(out) + "\n" +
		"exit 0\n"
	wrapperPath := filepath.Join(root, "echo")
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}

	// Restore any inherited values of the keys we set.
	sets := []string{
		"ETCDCTL_INSECURE_SKIP_TLS_VERIFY=true",
		"ETCDCTL_CACERT=/inherited/ca.pem",
		"ETCDCTL_CERT=/inherited/cert.pem",
		"ETCDCTL_KEY=/inherited/key.pem",
		"PATH=/inherited/bin:/inherited/localbin",
		"NORMAL_EXPORTED_VAR=keep-me",
	}
	restore := map[string]string{}
	had := map[string]bool{}
	for _, kv := range sets {
		key := kv[:strings.IndexByte(kv, '=')]
		if p, ok := os.LookupEnv(key); ok {
			restore[key] = p
			had[key] = true
		}
		if err := os.Setenv(key, kv[strings.IndexByte(kv, '=')+1:]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for key, prev := range restore {
			if had[key] {
				_ = os.Setenv(key, prev)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})

	// Pin only the approved etcdctl keys plus a safe PATH.
	childEnv := PinnedEnv(
		"PATH=/usr/bin:/bin",
		"ETCDCTL_API=3",
		"ETCDCTL_ENDPOINTS=https://etcd-1.internal:2379",
		"ETCDCTL_CACERT=/pins/ca.pem",
		"ETCDCTL_CERT=/pins/cert.pem",
		"ETCDCTL_KEY=/pins/key.pem",
	)

	cmd := exec.Command(wrapperPath)
	cmd.Env = childEnv
	if _, err := cmd.Output(); err != nil {
		t.Fatalf("wrapper failed: %v", err)
	}

	got := readFile(t, out)
	lines := splitLines(got)

	// Every unpinned ETCDCTL_* key must be ABSENT from the child env.
	for _, key := range []string{"ETCDCTL_INSECURE_SKIP_TLS_VERIFY", "ETCDCTL_USERNAME", "ETCDCTL_PASSWORD"} {
		for _, line := range lines {
			if strings.HasPrefix(line, key+"=") {
				t.Fatalf("unpinned inherited %s leaked into child env:\n%s", key, got)
			}
		}
	}

	// The explicitly pinned CA must win over the inherited opposite value.
	if !containsLine(lines, "ETCDCTL_CACERT=/pins/ca.pem") {
		t.Fatalf("pinned ETCDCTL_CACERT missing or overridden; child env:\n%s", got)
	}
	if containsLine(lines, "ETCDCTL_CACERT=/inherited/ca.pem") {
		t.Fatalf("inherited ETCDCTL_CACERT must not reach child; child env:\n%s", got)
	}

	// The pinned, listed ETCDCTL keys must all be present with pin values.
	for _, want := range []string{
		"ETCDCTL_API=3",
		"ETCDCTL_ENDPOINTS=https://etcd-1.internal:2379",
		"ETCDCTL_CACERT=/pins/ca.pem",
		"ETCDCTL_CERT=/pins/cert.pem",
		"ETCDCTL_KEY=/pins/key.pem",
	} {
		if !containsLine(lines, want) {
			t.Fatalf("expected pinned key %q in child env;\n%s", want, got)
		}
	}

	// Normally exported parent variables must still be inherited (no regression).
	if !containsLine(lines, "NORMAL_EXPORTED_VAR=keep-me") {
		t.Fatalf("normally exported parent var must be inherited; child env:\n%s", got)
	}
	// The pinned PATH must win over the inherited PATH.
	if containsLine(lines, "PATH=/inherited/bin:/inherited/localbin") {
		t.Fatalf("inherited PATH must not reach child; child env:\n%s", got)
	}
	if !containsLine(lines, "PATH=/usr/bin:/bin") {
		t.Fatalf("pinned PATH missing; child env:\n%s", got)
	}
}

// TestUnpinnedETCDCTLKeyDroppedDirect asserts the same property directly against
// the returned env slice so the regression can run without an external wrapper
// binary in packages that cannot spawn one.
func TestUnpinnedETCDCTLKeyDroppedDirect(t *testing.T) {
	sets := []string{
		"ETCDCTL_INSECURE_SKIP_TLS_VERIFY=true",
		"ETCDCTL_USERNAME=admin",
		"ETCDCTL_PASSWORD=secret",
		"NORMAL_EXPORTED_VAR=keep-me",
	}
	restore := map[string]string{}
	had := map[string]bool{}
	for _, kv := range sets {
		key := kv[:strings.IndexByte(kv, '=')]
		if p, ok := os.LookupEnv(key); ok {
			restore[key] = p
			had[key] = true
		}
		if err := os.Setenv(key, kv[strings.IndexByte(kv, '=')+1:]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for key, prev := range restore {
			if had[key] {
				_ = os.Setenv(key, prev)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})

	got := PinnedEnv(
		"PATH=/usr/bin:/bin",
		"ETCDCTL_API=3",
		"ETCDCTL_CACERT=/pins/ca.pem",
		"ETCDCTL_CERT=/pins/cert.pem",
		"ETCDCTL_KEY=/pins/key.pem",
	)

	for _, kv := range got {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key := kv[:i]
			if strings.HasPrefix(key, "ETCDCTL_") && key != "ETCDCTL_API" &&
				key != "ETCDCTL_CACERT" && key != "ETCDCTL_CERT" && key != "ETCDCTL_KEY" {
				t.Fatalf("unpinned inherited key %s=%s must be dropped by PinnedEnv", key, kv[i+1:])
			}
		}
	}
	if !containsLine(splitLines(strings.Join(got, "\n")), "NORMAL_EXPORTED_VAR=keep-me") {
		t.Fatalf("normally exported parent var must be inherited; got:\n%s", strings.Join(got, "\n"))
	}
}

func quoteShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}
