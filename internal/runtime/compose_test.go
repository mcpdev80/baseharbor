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
	  '/demo-api|baseharbor-demo-dev|<no value>|api|<no value>|true|healthy' \
	  '/demo-worker|<no value>|baseharbor-demo-dev|<no value>|worker|true|' \
	  '/other-api|baseharbor-other-dev|<no value>|api|<no value>|true|healthy' \
	  '/stopped-api|baseharbor-demo-dev|<no value>|stopped|<no value>|false|'
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
