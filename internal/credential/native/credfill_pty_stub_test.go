package native_test

import (
	"os/exec"
	"sync/atomic"
	"syscall"
)

// ptyUnavailable records that this environment cannot allocate a PTY, so the
// controlling-terminal witness skips instead of silently passing.
var ptyUnavailable atomic.Bool

// ttyPromptBranchWasLive records that the attach hook actually ran (the
// witness exercised a real controlling terminal), so a silent no-hook cannot
// make the PTY test pass.
var ttyPromptBranchWasLive bool

// setControllingTTY gives the child its own session and the (already wired as
// stdio) PTY slave as its controlling terminal.
func setControllingTTY(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Ctty: 0}
}
