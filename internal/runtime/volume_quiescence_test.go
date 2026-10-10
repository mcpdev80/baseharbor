package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedVolumeQuiescenceChecksMountsAcrossProjectBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, mounts string
		wantError    bool
	}{
		{"unrelated service in same project", `[{"Type":"volume","Name":"app_pg"}]`, false},
		{"foreign consumer of core datastore", `[{"Type":"volume","Name":"owned_pg"}]`, true},
		{"missing volume identity", `[{"Type":"volume"}]`, true},
		{"malformed native inventory", `not-json`, true},
		{"incomplete native inventory", ``, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			command := filepath.Join(dir, "docker")
			script := `#!/bin/sh
case "$1 $2" in
 "volume ls") echo owned_pg ;;
 "volume inspect") echo 'owned_pg|owned|owned' ;;
 "container ls") echo actual-container ;;
 "container inspect") printf '%s\n' "$TEST_NATIVE_MOUNTS" ;;
esac
`
			if err := os.WriteFile(command, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("TEST_NATIVE_MOUNTS", tc.mounts)
			err := (Compose{command: command}).VerifyOwnedVolumeQuiesced(context.Background(), "owned", "owned_pg")
			if (err != nil) != tc.wantError {
				t.Fatalf("quiescence error=%v want-error=%t", err, tc.wantError)
			}
		})
	}
}
