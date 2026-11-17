//go:build !windows

package stableid

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// lockHolderHeader is the magic prefix written at the head of every lifecycle
// lock dotfile. It lets a second reader tell a genuine holder receipt from any
// stray text a foreign writer left in the same byte slot, and it anchors the
// versioned record layout below. Every receipt the product writes starts with
// this exact line.
const lockHolderHeader = "baseharbor-lock v1 pid="

// lockHolderRecord is the durable receipt a lifecycle lock writes into its
// dotfile. It is line-oriented and human-inspectable:
//
//	baseharbor-lock v1 pid=12345
//	startTime=1731148800
//	bootID=3da1-9f0c
//
// The PID alone (the pre-0427-180 contract) is insufficient to prove a holder
// is live: PIDs are recycled by the kernel, so a dead holder and a live holder
// can report the same pid. startTime (unix seconds of the holder's start) and
// bootID (a machine-wide boot identifier) turn a bare pid into a
// reuse-resistant identity: a receipt is "live" only when the pid is alive AND
// its bootID matches the current boot. On platforms without a boot-id source
// the field is empty and the pid+startTime pair still guards the common SIGKILL
// case; pid reuse is only indistinguishable across a reboot with the same start
// second, which startTime already rules out in that narrow case.
type lockHolderRecord struct {
	pid       int
	startTime int64
	bootID    string
}

// lockHolderBytes renders the receipt in the line-oriented layout above. The
// trailing newline keeps the file greppable.
func (r lockHolderRecord) lockHolderBytes() []byte {
	var b strings.Builder
	b.WriteString(lockHolderHeader)
	b.WriteString(strconv.Itoa(r.pid))
	if r.startTime != 0 {
		b.WriteString("\nstartTime=")
		b.WriteString(strconv.FormatInt(r.startTime, 10))
	}
	if r.bootID != "" {
		b.WriteString("\nbootID=")
		b.WriteString(r.bootID)
	}
	b.WriteByte('\n')
	return []byte(b.String())
}

// parseLockHolderRecord decodes a receipt written by lockHolderBytes. It
// returns ok=false when the bytes carry no recognised baseharbor receipt (a
// foreign writer's text, or a pre-0427-180 bare-pid line). A live decision is
// always made on top of this decode by the caller.
func parseLockHolderRecord(buf []byte) (lockHolderRecord, bool) {
	text := strings.TrimSpace(string(buf))
	if !strings.HasPrefix(text, lockHolderHeader) {
		return lockHolderRecord{}, false
	}
	var rec lockHolderRecord
	// The pid is the bare token after the header on the first line. Trim at the
	// first newline so a trailing startTime/bootID line does not break Atoi.
	if rest := strings.TrimPrefix(text, lockHolderHeader); rest != "" {
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[:nl]
		}
		v, err := strconv.Atoi(strings.TrimSpace(rest))
		if err == nil && v > 0 {
			rec.pid = v
		}
	}
	// Read the remaining key=value lines.
	startTimeSeen := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == lockHolderHeader+strconv.Itoa(rec.pid) {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "startTime":
			if v, err := strconv.ParseInt(val, 10, 64); err == nil {
				rec.startTime = v
				startTimeSeen = true
			}
		case "bootID":
			rec.bootID = val
		}
	}
	// Fail closed: a genuine receipt the product writes always carries a
	// positive pid and a positive startTime. A malformed or truncated line that
	// only matches the magic header (pid==0, unparsable pid, or startTime
	// absent/<=0) is rejected with ok=false so reclaimStaleAppLock leaves it
	// untouched. A bare pid==0 must never be treated as a reclaimable holder —
	// that would let a truncated or foreign line delete a live owner's receipt.
	if rec.pid <= 0 || !startTimeSeen || rec.startTime <= 0 {
		return lockHolderRecord{}, false
	}
	return rec, true
}

// currentProcessStartTime returns the calling process's OS-observed start time
// as whole seconds since the unix epoch, derived from /proc/<pid>/stat field 22
// (starttime, in clock ticks since boot) divided by the tick rate. This is the
// holder's real kernel start instant, not the wall-clock moment the receipt is
// written: reclaim re-reads the same field for the live pid and a mismatch
// proves the pid was recycled. It returns 0 on any platform (or read) that
// cannot supply it, where the caller falls back to boot-id alone.
func currentProcessStartTime() int64 {
	tick, ok := processStartTicks(os.Getpid())
	if !ok {
		return 0
	}
	return tick / clockTickCount()
}

// processStartTicks reads /proc/<pid>/stat field 22 (the holder's start time in
// clock ticks since boot) for the given pid. It returns ok=false when the
// field cannot be read (non-Linux, missing file, malformed line), never an
// error — a caller must treat an unreadable field as "no start-time hint".
func processStartTicks(pid int) (int64, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	// /proc/<pid>/stat renders as "<pid> (<comm>) <state> ..."; the comm field
	// may contain spaces or parentheses, so split on the final ')' to reach the
	// first space-separated field after it, whose 19th following token is field
	// 22 (starttime).
	brace := strings.LastIndex(string(data), ")")
	if brace < 0 {
		return 0, false
	}
	fields := strings.Fields(string(data)[brace+1:])
	// After the closing ')': field 3 (state) is fields[0]; field 22 (starttime)
	// is fields[19].
	if len(fields) < 20 {
		return 0, false
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil || ticks < 0 {
		return 0, false
	}
	return ticks, true
}

// clockTickCount returns the system clock tick rate (ticks per second). Linux
// defines HZ as 100 in this context; we use that well-known default so
// starttime ticks convert to coarse seconds. Precise precision is not required:
// the reuse check compares two readings of the same field, so any consistent
// rate suffices, and the fail-closed dead-holder path never depends on it.
func clockTickCount() int64 {
	return 100
}

// lockHolderBootID returns a machine-wide boot identifier that, combined with
// the process start time, makes a holder identity resistant to PID reuse. It
// never fails a caller: any source that cannot be read yields an empty string
// (the caller then falls back to the pid+startTime pair).
func lockHolderBootID() string {
	// Linux exposes the kernel boot time in /proc/stat's "boot" field (seconds
	// since the unix epoch). A coarse but stable label is enough to distinguish
	// boots; the exact value is an implementation detail, not a contract.
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "boot ") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if secs, err := strconv.ParseInt(fields[1], 10, 64); err == nil && secs > 0 {
						return fmt.Sprintf("boot-%d", secs)
					}
				}
			}
		}
	}
	return ""
}

// lockHolderLive reports whether a decoded receipt still identifies a live,
// in-generation holder. A pid is only a weak signal (it is recycled), so the
// reclaim decision is fail-closed: a pid that is currently alive is treated as
// live and NOT reclaimed, even on the same boot where pid reuse is possible —
// destroying another live process's receipt would be worse than leaving a
// stale one in place. The only reclaimable cases are a dead holder (ESRCH, the
// SIGKILL case this task fixes) and a holder whose boot-id is present on both
// sides but from an EARLIER boot (a dead-or-replaced holder from a previous
// boot whose pid was later reassigned on the current boot). A non-zero
// startTime with no boot-id source is retained as a secondary hint but never
// by itself reclaims a live pid.
func lockHolderLive(rec lockHolderRecord) bool {
	if rec.pid <= 0 {
		return false
	}
	if err := unix.Kill(rec.pid, 0); err != nil {
		if errors.Is(err, unix.ESRCH) {
			// Dead holder: reclaimable.
			return false
		}
		// EPERM: the process is alive but owned by another user. Do not reclaim
		// another user's holder — treat it as live.
		return true
	}
	// pid is alive. Reuse is caught two ways:
	//   - boot-id present on both sides but different: the receipt is from an
	//     earlier boot, the original holder is gone, and the kernel later
	//     reassigned this pid on the current boot -> reclaim.
	//   - boot-id unavailable (both empty) but startTime present on both sides
	//     and different: re-read the *recorded* pid's own kernel start time via
	//     /proc/<pid>/stat and compare it to the receipt's startTime. A
	//     genuinely live holder started at that instant and matches; a different
	//     value means the pid was recycled between the receipt's write and now.
	//     The compare is against the recorded pid, never the caller's own pid,
	//     because the receipt belongs to a possibly different process.
	if rec.bootID != "" && lockHolderBootID() != "" {
		return rec.bootID == lockHolderBootID()
	}
	if rec.startTime > 0 {
		ticks, ok := processStartTicks(rec.pid)
		if !ok {
			return true
		}
		return rec.startTime == ticks/clockTickCount()
	}
	return true
}

// ErrLockFilesystemNotEnforcing is returned when a lock was acquired but the
// underlying filesystem does NOT enforce the advisory flock across processes.
// The returned lock is NOT safe to trust for mutual exclusion on that target,
// so AcquireLifecycleLock / AcquireBootstrapLock / AcquireAppLifecycleLock /
// AcquireTargetWideLifecycleLock fail closed instead of emitting false
// security. Callers must NOT retry this as ordinary contention: it is a
// classifiable remediation condition, so callers that route the acquire through
// a bounded retry loop (e.g. cmd/baha/application_lifecycle_lock.go) must
// detect it with errors.Is and fail closed immediately with the actionable
// filesystem remediation. It is exported (unlike the probe-unavailable
// sentinel) precisely so the coreinstallation boundary and CLI callers can
// classify it.
var ErrLockFilesystemNotEnforcing = errors.New(
	"this filesystem does not enforce the lifecycle lock across processes; " +
		"the per-process flock() is advisory-only here and cannot be trusted for " +
		"mutual exclusion. Move the state root to an enforcing local filesystem " +
		"(e.g. the target's local state directory, not a network or overlay " +
		"filesystem that drops flock cross-process enforcement) and retry; do not " +
		"delete the lock file to work around this")

// lockNamed opens the named kernel lock file under root and takes an exclusive,
// non-blocking advisory lock. The lifecycle lock and the bootstrap lock use
// DIFFERENT files (see lifecycleLockFile / bootstrapLockFile) so that bootstrap
// and provider upgrades of the same root never contend with each other.
func lockNamed(root string, name string) (func(), error) {
	fd, err := unix.Open(filepath.Join(root, name), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			message := lifecycleLockHolderMessage(file, name)
			_ = file.Close()
			return nil, fmt.Errorf("%s: %w", message, err)
		}
		_ = file.Close()
		return nil, err
	}
	// Validity probe: an advisory flock is only safe when the filesystem
	// enforces it across processes. On some runtime filesystems (network,
	// certain overlay stacks) a second OS process can still acquire the same
	// file while we hold the lock, so the mutual-exclusion guarantee would be
	// advisory-only. Prove enforcement with a short-lived child that tries to
	// take the same lock; if it acquires, fail closed.
	if free, perr := lockProbeRun(filepath.Join(root, name)); perr != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, perr
	} else if free {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, ErrLockFilesystemNotEnforcing
	}
	if err := file.Truncate(0); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	if err := file.Truncate(0); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	// 0427-180: write the enriched holder receipt (PID + process start time +
	// boot id) instead of a bare pid line. The acquire path reads this back to
	// prove a reported holder is live or stale and reclaim a confirmed-stale
	// one under the target-wide scope. The 179 validity probe above is unchanged
	// and still runs before this write, so cross-process enforcement is not
	// weakened. The leading magic line lets a reader tell a genuine receipt from
	// foreign text or a pre-0427-180 bare-pid line.
	if _, err := file.Write(lockHolderRecord{
		pid:       os.Getpid(),
		startTime: currentProcessStartTime(),
		bootID:    lockHolderBootID(),
	}.lockHolderBytes()); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	return func() { _ = file.Truncate(0); _ = unix.Flock(fd, unix.LOCK_UN); _ = file.Close() }, nil
}

func lock(root string) (func(), error) {
	return lockNamed(root, lifecycleLockFile)
}

func bootstrapLock(root string) (func(), error) {
	return lockNamed(root, bootstrapLockFile)
}

// This diagnostic reads only the private lock file. A PID is advisory: do not
// signal a process automatically, because PID reuse and exited holders exist.
func lifecycleLockHolderMessage(file *os.File, name string) string {
	base := "lock " + name + " is held; inspect the owner process before retrying; do not force-remove the lock file"
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return base
	}
	buf := make([]byte, 256)
	n, _ := file.Read(buf)
	// 0427-180: the receipt is a versioned line-oriented record; decode it for
	// the holder pid instead of parsing a bare pid line.
	if rec, ok := parseLockHolderRecord(buf[:n]); ok && rec.pid > 0 {
		pid := rec.pid
		state := "unknown"
		switch err := unix.Kill(pid, 0); {
		case err == nil:
			state = "alive"
		case errors.Is(err, unix.ESRCH):
			state = "not-running (possibly stale PID)"
		case errors.Is(err, unix.EPERM):
			state = "running (permission denied)"
		}
		return fmt.Sprintf("lock %s held by reported pid=%d state=%s; inspect with 'ps -p %d -o pid,stat,cmd' and stop it explicitly only after verifying ownership; retry afterward (never delete the lock file while held)", name, pid, state, pid)
	}
	return base
}
