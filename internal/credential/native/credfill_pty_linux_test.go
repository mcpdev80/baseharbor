//go:build linux

package native_test

import (
	"os/exec"
	"testing"

	"github.com/creack/pty"
)

// attachControllingPTY returns a hook that attaches a REAL controlling
// terminal to the command the production ExecRunner builds: the child gets
// its own session with the PTY slave on stdin/stdout/stderr, exactly like a
// developer shell during target bootstrap. It changes nothing else about the
// command — the environment vector stays built entirely by production code
// (appendNonInteractiveEnv), which is what this witness tests.
func attachControllingPTY(t *testing.T) func(*exec.Cmd) {
	t.Helper()
	return func(cmd *exec.Cmd) {
		master, slave, err := pty.Open()
		if err != nil {
			ptyUnavailable.Store(true)
			return
		}
		ttyPromptBranchWasLive = true
		t.Cleanup(func() {
			_ = master.Close()
			_ = slave.Close()
		})
		cmd.Stdin = slave
		cmd.Stdout = slave
		cmd.Stderr = slave
		setControllingTTY(cmd)
		// Drain the master so the child's terminal writes never block on a
		// full line-discipline buffer; the witness needs the terminal live.
		stop := make(chan struct{})
		go func() {
			buf := make([]byte, 4096)
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := master.Read(buf); err != nil {
					return
				}
			}
		}()
		t.Cleanup(func() { close(stop) })
	}
}
