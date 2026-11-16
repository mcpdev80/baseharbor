package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeContainerRetirementDoesNotDeleteAnonymousVolumes(t *testing.T) {
	root := t.TempDir()
	command := filepath.Join(root, "runtime")
	log := filepath.Join(root, "calls")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$RETIREMENT_CALLS"
case "$1 $2" in
  "container ls") printf '%s\n' owned-db-1 other-db-1 ;;
  "container inspect") printf '%s\n' '/owned-db-1|owned|<no value>' '/other-db-1|other|<no value>' ;;
  "container stop") [ "$*" = "container stop --timeout -1 owned-db-1" ]; [ "${RETIREMENT_STOP_FAIL:-0}" != 1 ] ;;
  "container rm") [ "$*" = "container rm owned-db-1" ] ;;
  *) exit 9 ;;
esac
`
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RETIREMENT_CALLS", log)
	provider := Compose{command: command}
	if err := provider.RetireOwnedProjectContainers(context.Background(), "owned", []ProjectResource{{Kind: "container", Name: "owned-db-1"}}); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "container stop --timeout -1 owned-db-1\ncontainer rm owned-db-1\n") || strings.Contains(string(calls), " -v ") || strings.Contains(string(calls), "volume rm") || strings.Contains(string(calls), "container rm other") {
		t.Fatalf("data or foreign resource deletion: %s", calls)
	}
	before := string(calls)
	if err := provider.RetireOwnedProjectContainers(context.Background(), "owned", []ProjectResource{{Kind: "volume", Name: "data"}}); err == nil {
		t.Fatal("data deletion accepted as runtime retirement")
	}
	calls, err = os.ReadFile(log)
	if err != nil || string(calls) != before {
		t.Fatal("invalid selection reached native adapter")
	}
}

func TestNativeRetirementStopFailurePreservesOwnerForRetry(t *testing.T) {
	root := t.TempDir()
	command, log := filepath.Join(root, "runtime"), filepath.Join(root, "calls")
	if err := os.WriteFile(command, []byte(`#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$RETIREMENT_CALLS"
case "$1 $2" in
  "container ls") echo owned-db-1 ;;
  "container inspect") echo '/owned-db-1|owned|<no value>' ;;
  "container stop") exit 23 ;;
  *) exit 9 ;;
esac
`), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RETIREMENT_CALLS", log)
	provider := Compose{command: command}
	if err := provider.RetireOwnedProjectContainers(t.Context(), "owned", []ProjectResource{{Kind: "container", Name: "owned-db-1"}}); err == nil {
		t.Fatal("failed stop reported success")
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "container rm") || !strings.Contains(string(calls), "container stop --timeout -1") {
		t.Fatal("failed graceful stop did not retain container")
	}
}
