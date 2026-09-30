package runtime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunningServicesProjectUsesBatchedRuntimeContainerState(t *testing.T) {
	dir := t.TempDir()
	runtimePath := filepath.Join(dir, "runtime")
	logPath := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
set -eu

printf '%s\n' "$*" >> "$CALL_LOG"

if [ "$1" = "container" ] && [ "$2" = "ls" ]; then
	printf '%s\n' id1 id2 id3 id4
	exit 0
fi

if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then
	printf '%s\n' \
	  '/demo-api|baseharbor-demo-dev|<no value>|api|<no value>|true|healthy|running|0|' \
	  '/demo-worker|<no value>|baseharbor-demo-dev|<no value>|worker|true||running|0|' \
	  '/other-api|baseharbor-other-dev|<no value>|api|<no value>|true|healthy|running|0|' \
	  '/stopped-api|baseharbor-demo-dev|<no value>|stopped|<no value>|false||exited|0|'
	exit 0
fi

printf 'unexpected arguments: %s\n' "$*" >&2
exit 2
`
	if err := os.WriteFile(runtimePath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CALL_LOG", logPath)
	compose := Compose{command: runtimePath}
	got, err := compose.RunningServicesProject(context.Background(), "baseharbor-demo-dev", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"api", "worker"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("running services = %#v, want %#v", got, want)
	}

	rawCalls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(rawCalls)), "\n")
	if len(calls) != 2 {
		t.Fatalf("runtime calls = %d, want 2; calls=%q", len(calls), calls)
	}
	if !strings.HasPrefix(calls[0], "container ls -aq") {
		t.Fatalf("first runtime call = %q, want batched container list", calls[0])
	}
	if !strings.HasPrefix(calls[1], "container inspect --format ") {
		t.Fatalf("second runtime call = %q, want one batched inspect", calls[1])
	}
	for _, id := range []string{"id1", "id2", "id3", "id4"} {
		if !strings.Contains(calls[1], id) {
			t.Fatalf("batched inspect call %q does not include %s", calls[1], id)
		}
	}
}

func TestFirstRuntimeLabelPrefersDockerAndFallsBackToPodman(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want string
	}{
		{name: "docker", in: []string{"project-a", "project-b"}, want: "project-a"},
		{name: "podman fallback", in: []string{"<no value>", "project-b"}, want: "project-b"},
		{name: "empty fallback", in: []string{"", "project-b"}, want: "project-b"},
		{name: "none", in: []string{"<no value>", ""}, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstRuntimeLabel(tc.in...); got != tc.want {
				t.Fatalf("firstRuntimeLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestListRuntimeContainersCapturesTerminalStartEvidence(t *testing.T) {
	dir := t.TempDir()
	runtimePath := filepath.Join(dir, "runtime")
	script := `#!/bin/sh
set -eu
if [ "$1" = "container" ] && [ "$2" = "ls" ]; then
	printf '%s\n' failed1
	exit 0
fi
if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then
	printf '%s\n' '/demo-app|baseharbor-demo-dev|<no value>|demo-app|<no value>|false||created|128|failed to set up container networking: port is already allocated'
	exit 0
fi
exit 2
`
	if err := os.WriteFile(runtimePath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	compose := Compose{command: runtimePath}
	containers, err := compose.ListRuntimeContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 1 {
		t.Fatalf("containers = %#v", containers)
	}
	got := containers[0]
	if got.State != "created" || got.ExitCode != 128 || !strings.Contains(got.Error, "port is already allocated") {
		t.Fatalf("terminal evidence lost: %#v", got)
	}
}


func TestDestroyProjectRemoveOrphansRemovesOwnedResourcesOutsideCurrentComposeModel(t *testing.T) {
	dir := t.TempDir()
	runtimePath := filepath.Join(dir, "runtime")
	stateDir := filepath.Join(dir, "state")
	logPath := filepath.Join(dir, "calls.log")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
printf '%s
' "$*" >> "$CALL_LOG"

case "$1 $2" in
  "container ls")
    if [ ! -f "$STATE_DIR/containers-removed" ]; then
      printf '%s
' current orphan unrelated
    else
      printf '%s
' unrelated
    fi
    exit 0
    ;;
  "network ls")
    if [ ! -f "$STATE_DIR/network-removed" ]; then
      printf '%s
' shared-net unrelated-net
    else
      printf '%s
' unrelated-net
    fi
    exit 0
    ;;
  "volume ls")
    if [ ! -f "$STATE_DIR/volume-removed" ]; then
      printf '%s
' shared-vol
    fi
    exit 0
    ;;
  "container inspect")
    if [ ! -f "$STATE_DIR/containers-removed" ]; then
      printf '%s
'         '/current|bh-local-shared|<no value>'         '/orphan|bh-local-shared|<no value>'         '/unrelated|other-project|<no value>'
    else
      printf '%s
' '/unrelated|other-project|<no value>'
    fi
    exit 0
    ;;
  "network inspect")
    if [ ! -f "$STATE_DIR/network-removed" ]; then
      printf '%s
'         'shared-net|bh-local-shared|<no value>'         'unrelated-net|other-project|<no value>'
    else
      printf '%s
' 'unrelated-net|other-project|<no value>'
    fi
    exit 0
    ;;
  "volume inspect")
    if [ ! -f "$STATE_DIR/volume-removed" ]; then
      printf '%s
' 'shared-vol|bh-local-shared|<no value>'
    fi
    exit 0
    ;;
esac

if [ "$1 $2" = "container rm" ]; then
  touch "$STATE_DIR/containers-removed"
  exit 0
fi
if [ "$1 $2" = "network rm" ]; then
  touch "$STATE_DIR/network-removed"
  exit 0
fi
if [ "$1 $2" = "volume rm" ]; then
  touch "$STATE_DIR/volume-removed"
  exit 0
fi

printf 'unexpected arguments: %s
' "$*" >&2
exit 2
`
	if err := os.WriteFile(runtimePath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STATE_DIR", stateDir)
	t.Setenv("CALL_LOG", logPath)

	compose := Compose{command: runtimePath}
	if err := compose.DestroyProjectRemoveOrphans(context.Background(), "bh-local-shared", "", ""); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(raw)
	for _, want := range []string{
		"container rm -f current",
		"container rm -f orphan",
		"network rm shared-net",
		"volume rm -f shared-vol",
	} {
		if !strings.Contains(calls, want) {
			t.Fatalf("full owned-project destroy missing %q:
%s", want, calls)
		}
	}
	for _, forbidden := range []string{
		"container rm -f unrelated",
		"network rm unrelated-net",
	} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("full owned-project destroy touched unrelated resource %q:
%s", forbidden, calls)
		}
	}
}
