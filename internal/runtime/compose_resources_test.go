package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectInventoryRejectsIncompleteInspection(t *testing.T) {
	command := filepath.Join(t.TempDir(), "runtime")
	script := `#!/bin/sh
case "$1 $2" in
  "container ls") printf '%s\n' owned other ;;
  "container inspect")
    case "$INVENTORY_FIXTURE" in
      complete) printf '%s\n' '["owned","ci",null]' '["other","foreign|label",null]' ;;
      missing) printf '%s\n' '["owned","ci",null]' ;;
      duplicate) printf '%s\n' '["owned","ci",null]' '["owned","ci",null]' ;;
      malformed) printf '%s\n' '["owned","ci",null]' 'bad-inspection' ;;
      unexpected) printf '%s\n' '["owned","ci",null]' '["other-name","foreign",null]' ;;
      conflicting) printf '%s\n' '["owned","ci",null]' '["other","foreign","ci"]' ;;
    esac ;;
  "network ls"|"volume ls") ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"complete", "missing", "duplicate", "malformed", "unexpected", "conflicting"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("INVENTORY_FIXTURE", mode)
			actual, err := NewCLIBackend(command).ListOwnedProjectResources(context.Background(), "ci")
			if mode == "complete" {
				if err != nil || len(actual) != 1 || actual[0].Name != "owned" {
					t.Fatalf("inventory=%v err=%v", actual, err)
				}
			} else if err == nil {
				t.Fatalf("%s inventory was accepted: %v", mode, actual)
			}
		})
	}
}

func TestNetworkHasActiveConsumers(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "docker active endpoints", err: errors.New("error while removing network: network demo has active endpoints"), want: true},
		{name: "podman in use", err: errors.New("network is being used by container 123"), want: true},
		{name: "connected containers", err: errors.New("network has connected containers"), want: true},
		{name: "unrelated runtime error", err: errors.New("permission denied"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := networkHasActiveConsumers(tc.err); got != tc.want {
				t.Fatalf("networkHasActiveConsumers(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
