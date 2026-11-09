package stableid

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed testdata/lock-probe-linux-amd64
var lockProbeBinary []byte

//go:embed lock_probe.go.txt
var lockProbeSource string

// lockProbeFunc performs the embedded cross-process probe. It is a package var
// so the cross-process regression can simulate an unenforcing filesystem
// (FREE) without a second real OS process. Production code uses
// lockProbeRunDefault.
var lockProbeFunc = lockProbeRunDefault

// LockProbeFunc is the exported boundary seam for the cross-process probe so
// that callers in other packages (notably the cmd/baha command-path regression
// for the lifecycle lock) can inject a not-enforcing probe without a second
// real OS process. The CLI package overrides this ONLY in tests to drive the
// real retry-loop decision; production code never sets it, so production runs
// lockProbeRunDefault. It is a test seam, never a production hook.
var LockProbeFunc = lockProbeRunDefault

// errProbeUnavailable is returned when neither the embedded probe binary nor a
// Go toolchain can run the cross-process probe. It is NOT a lock failure: the
// caller cannot verify cross-process enforcement, so AcquireLifecycleLock (and
// the other lifecycle acquirers) fail closed rather than trust an unverifiable
// advisory lock.
var errProbeUnavailable = errors.New("cross-process lock probe cannot run on this platform")

// lockProbeRun invokes the embedded cross-process lock probe as a short-lived
// separate OS process and reports whether the lock file at lockPath was found
// FREE. The child opens the same path and tries LOCK_EX|LOCK_NB with no wait.
// It exits 0 when it acquires the lock (proving the filesystem does NOT enforce
// cross-process mutual exclusion while the caller holds the flock), and 1 when
// the kernel correctly blocks it. It returns "free" only in the first case:
// a child that acquires proves the caller's advisory lock is NOT exclusive
// across processes, so the caller must fail closed rather than trust a lock it
// cannot verify is exclusive.
func lockProbeRun(lockPath string) (free bool, err error) {
	return LockProbeFunc(lockPath)
}

// lockProbeRunDefault implements lockProbeFunc with the real embedded binary
// probe. The probe is a statically-linked Linux/AMD64 helper (see
// internal/stableid/testdata/lock-probe-linux-amd64) that re-implements the
// flock probe with only the Go standard library's syscall package, so it runs
// with NO Go toolchain, PATH, or module cache — the exact conditions that made
// the previous `go run`-based probe fail on CI runners. On other platforms it
// falls back to building the stdlib-only probe from the embedded source with
// the toolchain; if neither works, it reports errProbeUnavailable so the caller
// fails closed instead of trusting an unverifiable lock.
func lockProbeRunDefault(lockPath string) (free bool, err error) {
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		if free, err = runEmbeddedBinaryProbe(lockPath); err == nil {
			return free, nil
		}
		// A runtime failure of the embedded binary (e.g. corrupted embed) falls
		// through to the toolchain build rather than failing the acquire.
	}
	return runToolchainProbe(lockPath)
}

// runEmbeddedBinaryProbe copies the embedded amd64 binary to a private temp
// file and executes it directly.
func runEmbeddedBinaryProbe(lockPath string) (free bool, err error) {
	binPath, cleanup, err := writeProbeBinary()
	defer cleanup()
	if err != nil {
		return false, err
	}
	out, runErr := exec.Command(binPath, lockPath).CombinedOutput()
	if runErr != nil {
		// Exit 1 == child observed the lock HELD (the correct case). Only
		// surface other failures; a transient toolchain hiccup would
		// otherwise mask a genuine HELD observation.
		if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("cross-process lock probe failed: %w: %s", runErr, bytes.TrimSpace(out))
	}
	return true, nil
}

// writeProbeBinary copies the embedded probe binary into a private temporary
// executable file and returns its path plus a cleanup func.
func writeProbeBinary() (path string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "lock-probe-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	path = dir + "/lock-probe"
	if err = os.WriteFile(path, lockProbeBinary, 0700); err != nil {
		return path, cleanup, err
	}
	return path, cleanup, nil
}

// probeEnv returns an environment for the toolchain probe subprocess with PATH
// anchored to the active Go toolchain so `go build` resolves even when the test
// process inherited a PATH without Go (as CI's setup-go can produce).
func probeEnv() []string {
	parts := []string{}
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		parts = append(parts, filepath.Join(goroot, "bin"))
	}
	for _, dir := range []string{"/usr/local/go/bin", "/usr/lib/go-*/bin", "/opt/go/bin"} {
		matches, _ := filepath.Glob(dir)
		parts = append(parts, matches...)
	}
	parts = append(parts, "/usr/bin", "/bin", "/usr/local/bin")
	return append([]string{"PATH=" + strings.Join(parts, ":")}, os.Environ()...)
}

// runToolchainProbe is the cross-platform fallback: it writes the stdlib-only
// probe source to a temp dir and builds + runs it with the Go toolchain. It
// needs a working GOCACHE/GOPATH, which CI's setup-go normally provides on the
// job's PATH.
func runToolchainProbe(lockPath string) (free bool, err error) {
	dir, err := os.MkdirTemp("", "lock-probe-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	helper := filepath.Join(dir, "probe.go")
	if err = os.WriteFile(helper, []byte(lockProbeSource), 0600); err != nil {
		return false, err
	}
	bin := filepath.Join(dir, "probe")
	build := exec.Command("go", "build", "-o", bin, helper)
	build.Env = probeEnv()
	if out, err := build.CombinedOutput(); err != nil {
		return false, fmt.Errorf("build lock probe: %w: %s", err, bytes.TrimSpace(out))
	}
	out, runErr := exec.Command(bin, lockPath).CombinedOutput()
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("cross-process lock probe failed: %w: %s", runErr, bytes.TrimSpace(out))
	}
	return true, nil
}
