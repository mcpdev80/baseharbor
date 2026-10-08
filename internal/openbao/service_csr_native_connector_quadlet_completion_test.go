package openbao

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// This exercises the typed completion observation through the enrolled mTLS
// session. It qualifies the source-bound init primitive, not the Application engine.
func (f *nativeConnectorFixture) quadletCompletion(t *testing.T, ctx context.Context, pool *targetsession.Pool, scope targetenrollment.Scope) {
	t.Helper()
	runtime, err := targetsession.NewProjectRuntime(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "failed", "unstarted"} {
		t.Run("quadlet-completion-"+scenario, func(t *testing.T) {
			name := f.name + "-init-" + scenario
			file := name + ".container"
			exit := "0"
			if scenario == "failed" {
				exit = "17"
			}
			content := "[Container]\nImage=" + f.image + "\nContainerName=" + name + "\nUser=1000:1000\nReadOnly=true\nExec=/bin/sh -ec \"sleep 1; exit " + exit + "\"\nLabel=baseharbor.enrollment-qualification=true\nLabel=com.docker.compose.project=" + name + "\nLabel=com.docker.compose.service=init\n[Service]\nTimeoutStartSec=30\n"
			source := []targetsession.ProjectFile{{Path: file, Data: []byte(content)}}
			staged, err := runtime.Stage(ctx, name, source)
			if err != nil {
				t.Fatal("init source staging failed", err)
			}
			t.Cleanup(func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				// Emergency fixture cleanup does not qualify owned destroy.
				root := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers", "systemd")
				_ = exec.CommandContext(cleanup, "systemctl", "--user", "stop", name+".service").Run()
				_ = os.Remove(filepath.Join(root, file))
				_ = os.RemoveAll(filepath.Join(root, file+".d"))
				_ = exec.CommandContext(cleanup, "podman", "rm", "-f", name).Run()
				_ = exec.CommandContext(cleanup, "systemctl", "--user", "daemon-reload").Run()
			})
			if scenario == "unstarted" {
				err = runtime.PublishQuadletGraph(ctx, staged, []string{file})
			} else {
				err = runtime.ApplyQuadlet(ctx, staged, file)
			}
			if err != nil && scenario != "failed" {
				t.Fatal("init publication or activation failed", err)
			}
			if scenario == "success" {
				deadline := time.Now().Add(15 * time.Second)
				for runtime.VerifyQuadletCompletion(ctx, staged, file) != nil {
					if time.Now().After(deadline) || ctx.Err() != nil {
						t.Fatal("current native init completion did not converge")
					}
					time.Sleep(100 * time.Millisecond)
				}
				// Restoring Core handles must preserve the exact receipt binding.
				staged, err = runtime.RestoreProject(staged.Record(), source)
				if err != nil || runtime.VerifyQuadletCompletion(ctx, staged, file) != nil {
					t.Fatal("restored source lost authenticated completion", err)
				}
				// Republishing changed bytes without execution cannot reuse success.
				changed, err := runtime.Stage(ctx, name+"-changed", []targetsession.ProjectFile{{Path: file, Data: []byte(content + "# new source\n")}})
				if err != nil || runtime.PublishQuadletGraph(ctx, changed, []string{file}) != nil {
					t.Fatal("changed init publication failed", err)
				}
				if runtime.VerifyQuadletCompletion(ctx, changed, file) == nil || runtime.VerifyQuadletCompletion(ctx, staged, file) == nil {
					t.Fatal("changed source borrowed earlier native success")
				}
			} else if runtime.VerifyQuadletCompletion(ctx, staged, file) == nil {
				t.Fatal("failed or unstarted init accepted")
			}
			if err := runtime.DestroyQuadlet(ctx, staged, file); err != nil {
				t.Fatal("owned init destroy failed", err)
			}
			if runtime.VerifyQuadletCompletion(ctx, staged, file) == nil {
				t.Fatal("removed init accepted as completed")
			}
		})
	}
	if f.inventory(t, ctx, pool, scope) == "" {
		t.Fatal("init verification or destruction damaged foreign fixture")
	}
	if !t.Failed() {
		t.Log("actual enrolled rootless Quadlet completion verified current source over mTLS, restored exact commitment, denied changed, failed, unstarted and removed units, destroyed owned artifacts and preserved foreign fixture; full Application engine not qualified")
	}
}
