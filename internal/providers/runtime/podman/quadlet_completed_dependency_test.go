package podman

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCompletedDependencyChecksActualExitEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, state   string
		failed, valid bool
	}{
		{"active-exited", "ActiveState=active\nSubState=exited\nResult=success\nExecMainCode=1\nExecMainStatus=0\nExecMainExitTimestampMonotonic=100\n", false, true},
		{"inactive-completed", "ActiveState=inactive\nSubState=dead\nResult=success\nExecMainCode=1\nExecMainStatus=0\nExecMainExitTimestampMonotonic=100\n", false, true},
		{"never-started", "ActiveState=inactive\nSubState=dead\nResult=success\nExecMainCode=0\nExecMainStatus=0\nExecMainExitTimestampMonotonic=0\n", false, false},
		{"still-running", "ActiveState=active\nSubState=running\nResult=success\nExecMainCode=0\nExecMainStatus=0\nExecMainExitTimestampMonotonic=0\n", false, false},
		{"failed-exit", "ActiveState=failed\nSubState=failed\nResult=exit-code\nExecMainCode=1\nExecMainStatus=17\nExecMainExitTimestampMonotonic=100\n", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := "#!/bin/sh\ncase \"$*\" in\n*is-failed*) exit "
			if tc.failed {
				script += "0"
			} else {
				script += "1"
			}
			script += " ;;\n*show*) cat <<'STATE'\n" + tc.state + "STATE\n;;\n*) exit 1 ;;\nesac\n"
			if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/sh", "-ec", quadletCompletedDependencyScript("owned-init.service"))
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			if err := cmd.Run(); (err == nil) != tc.valid {
				t.Fatalf("valid=%t: %v", tc.valid, err)
			}
		})
	}
}
