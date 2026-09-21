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
case "$*" in
  "container ls -a --format {{.Names}}")
    printf '%s
' demo-api demo-worker other-api stopped-api
    ;;
  "container inspect --format {{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "com.docker.compose.service" }} demo-api")
    printf '%s
' 'baseharbor-demo-dev|api'
    ;;
  "container inspect --format {{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "com.docker.compose.service" }} demo-worker")
    printf '%s
' 'baseharbor-demo-dev|worker'
    ;;
  "container inspect --format {{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "com.docker.compose.service" }} other-api")
    printf '%s
' 'baseharbor-other-dev|api'
    ;;
  "container inspect --format {{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "com.docker.compose.service" }} stopped-api")
    printf '%s
' 'baseharbor-demo-dev|stopped'
    ;;
  "container inspect --format {{.State.Running}} demo-api")
    printf '%s
' true
    ;;
  "container inspect --format {{.State.Running}} demo-worker")
    printf '%s
' true
    ;;
  "container inspect --format {{.State.Running}} stopped-api")
    printf '%s
' false
    ;;
  *)
    printf 'unexpected arguments: %s
' "$*" >&2
    exit 2
    ;;
esac
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
