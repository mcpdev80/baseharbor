package runtime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRunningServicesProjectUsesRuntimeContainerState(t *testing.T) {
	dir := t.TempDir()
	runtimePath := filepath.Join(dir, "runtime")
	script := `#!/bin/sh
set -eu

if [ "$1" = "container" ] && [ "$2" = "ls" ]; then
	printf '%s\n' demo-api demo-worker other-api stopped-api
	exit 0
fi

if [ "$1" = "container" ] && [ "$2" = "inspect" ]; then
	template="$4"
	name="$5"
	if [ "$template" = "{{.State.Running}}" ]; then
		case "$name" in
		  demo-api|demo-worker) printf '%s\n' true ;;
		  stopped-api) printf '%s\n' false ;;
		  *) printf 'unexpected running-state container: %s\n' "$name" >&2; exit 2 ;;
		esac
		exit 0
	fi

	case "$name" in
	  demo-api) printf '%s\n' 'baseharbor-demo-dev|api' ;;
	  demo-worker) printf '%s\n' 'baseharbor-demo-dev|worker' ;;
	  other-api) printf '%s\n' 'baseharbor-other-dev|api' ;;
	  stopped-api) printf '%s\n' 'baseharbor-demo-dev|stopped' ;;
	  *) printf 'unexpected label container: %s\n' "$name" >&2; exit 2 ;;
	esac
	exit 0
fi

printf 'unexpected arguments: %s\n' "$*" >&2
exit 2
`
	if err := os.WriteFile(runtimePath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	compose := Compose{command: runtimePath}
	got, err := compose.RunningServicesProject(context.Background(), "baseharbor-demo-dev", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"api", "worker"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("running services = %#v, want %#v", got, want)
	}
}
