package containersecurity

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSameCapabilitiesRequiresExactCapabilitySet(t *testing.T) {
	if !sameCapabilities([]string{"CAP_CHOWN", "SETUID"}, []string{"chown", "cap_setuid"}) {
		t.Fatal("capability names should compare case-insensitively with optional CAP_ prefix")
	}
	for _, actual := range [][]string{
		{"CHOWN"},
		{"CHOWN", "SETUID", "DAC_READ_SEARCH"},
		{"CHOWN", "CHOWN"},
	} {
		if sameCapabilities(actual, []string{"CHOWN", "SETUID"}) {
			t.Fatalf("unexpected capability set accepted: %v", actual)
		}
	}
}

func TestValidateEffectiveCapabilitiesAllowsOnlyExplicitPodmanEntrypointSet(t *testing.T) {
	allowed := []string{"CHOWN", "DAC_READ_SEARCH", "FOWNER", "SETGID", "SETUID"}
	if err := validateEffectiveCapabilities([]string{"CAP_CHOWN", "DAC_READ_SEARCH", "FOWNER", "SETGID", "SETUID"}, allowed); err != nil {
		t.Fatalf("bounded entrypoint capabilities rejected: %v", err)
	}
	for _, actual := range [][]string{
		{"CHOWN", "DAC_READ_SEARCH", "FOWNER", "SETGID"},
		{"CHOWN", "DAC_READ_SEARCH", "FOWNER", "SETGID", "SETUID", "NET_ADMIN"},
	} {
		if err := validateEffectiveCapabilities(actual, allowed); err == nil {
			t.Fatalf("unexpected effective capability set accepted: %v", actual)
		}
	}
	if err := validateEffectiveCapabilities([]string{"CHOWN"}, nil); err == nil {
		t.Fatal("nonempty capability set accepted when no entrypoint allowance is configured")
	}
}

func TestPodmanOwnershipUsesInspectedRunningState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command fixture")
	}
	root := t.TempDir()
	script := `#!/bin/sh
if [ -n "${XDG_CONFIG_HOME:-}" ] || [ -n "${XDG_DATA_HOME:-}" ]; then exit 2; fi
if [ "${XDG_RUNTIME_DIR:-}" != 'runtime-session' ]; then exit 3; fi
case "$*" in
  'container ls -aq') printf 'live\nstopped\n' ;;
  'container inspect live') printf '%s\n' '[{"Config":{"Labels":{"com.docker.compose.project":"owned","com.docker.compose.service":"sql"}},"State":{"Running":true}}]' ;;
  'container inspect stopped') printf '%s\n' '[{"Config":{"Labels":{"com.docker.compose.project":"owned","com.docker.compose.service":"sql"}},"State":{"Running":false}}]' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(root, "podman"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "podman")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", "runtime-session")
	id, err := composeServiceContainerID(context.Background(), "owned", "sql")
	if err != nil || id != "live" {
		t.Fatalf("live owned selection: id=%s err=%v", id, err)
	}
	if _, err := composeServiceContainerID(context.Background(), "foreign", "sql"); err == nil {
		t.Fatal("foreign ownership accepted")
	}
}
