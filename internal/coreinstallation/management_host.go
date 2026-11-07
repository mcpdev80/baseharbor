package coreinstallation

import (
	"errors"
	"os"
	"strings"
)

// A ManagementHost runs one durable minimal authority on the selected host: the
// control-plane server is the single active writer of one durable state root and
// nothing else. It binds to exactly one Core installation and proves, on every
// startup, that this process is the current owner rather than a second or
// foreign writer.

// ErrManagementHostRootMissing is returned when the selected host has no durable
// installation state root yet. A missing root is a bootstrap, not an error: the
// caller must provision the installation before this host can own it.
var ErrManagementHostRootMissing = errors.New("management host state root is missing; bootstrap the installation first")

// ErrManagementHostRootForeign is returned when the durable state root holds an
// installation that does not belong to this management host selection. A foreign
// state root selects no authority: the host never adopts a second or rewritten
// installation.
var ErrManagementHostRootForeign = errors.New("management host state root holds a foreign installation; bootstrap a fresh host or attach to the owned one")

// ErrManagementHostRootProtected is returned when the durable state root is not
// owned and protected as a single authoritative writer requires. Any world- or
// group-accessible root is rejected: the durable authority must be private.
var ErrManagementHostRootProtected = errors.New("management host state root must be a protected, owned directory")

// ErrManagementHostAlreadyOwned is returned when a second process already holds
// the active-writer lifecycle lock for the durable state root. Only one durable
// writer exists per host; a live lock means the authority is already running.
var ErrManagementHostAlreadyOwned = errors.New("management host state root is already owned by a running writer")

// ErrManagementHostOwnedFreshRoot is returned when a fresh durable state root
// that does not yet carry a genuine installation is already owned by a second
// concurrent writer. Bootstrap must yield EXCLUSIVE ownership of the newly
// created root to a single process before the host binds: two competing fresh
// bootstraps must never both proceed to write the same durable authority.
var ErrManagementHostOwnedFreshRoot = errors.New("management host bootstrap root is already owned by a concurrent writer")

// ErrManagementHostBootstrapContended is returned when a durable state root
// that already carries a genuine installation is still guarded by the atomic
// bootstrap lock of a concurrent bootstrap server that has not finished
// provisioning. The bootstrap lock is the single continuous ownership primitive
// that covers fresh-root bootstrap through server lifetime: a second writer may
// not acquire the lifecycle lock on a root the bootstrap server still owns, so
// attach fails closed instead of letting two writers coexist.
var ErrManagementHostBootstrapContended = errors.New("management host bootstrap root is still owned by a concurrent bootstrap server")

// Bootstrap describes a management host with no durable installation state yet.
// Bootstrap versus attach is decided by the presence (attach) or absence
// (bootstrap) of a valid installation state root; it is never guessed from an
// empty or foreign root.
type Bootstrap bool

// IsBootstrap reports true when this host must bootstrap a fresh durable
// installation rather than attach to an existing one.
func (b Bootstrap) IsBootstrap() bool { return bool(b) }

// ManagementHost proves that the running process is the single active writer of
// one durable installation state root on the selected host.
//
// It binds to one durable state root (the installation runtime root) and enforces
// three invariants that make the host a durable minimal authority rather than a
// cached or copied copy:
//
//   - one state root: the durable installation identity is loaded from exactly
//     this path and nowhere else;
//   - active-writer ownership: the process holds the kernel lifecycle lock for
//     this root, so a second concurrent writer is rejected and a dead writer does
//     not leave a stale active lock;
//   - protected root: the state root is a private (mode 0700) directory, never
//     world- or group-accessible.
//
// Bootstrap versus attach is decided from the durable state alone: a missing root
// is bootstrap, a valid root is attach, and a foreign or unreadable root fails
// closed with ErrManagementHostRootForeign. No local fallback is selected for a
// missing, foreign or locked root.
type ManagementHost struct {
	stateRoot string
}

// NewManagementHost binds a management host to exactly one durable state root. An
// empty root fails closed: a management host must have exactly one durable state
// root, never an implicit or empty one.
func NewManagementHost(stateRoot string) (*ManagementHost, error) {
	if strings.TrimSpace(stateRoot) == "" {
		return nil, errors.New("management host requires exactly one durable state root")
	}
	return &ManagementHost{stateRoot: stateRoot}, nil
}

// isEmptyRoot reports whether root is an existing directory that is private and
// carries no entries other than the bootstrap lock file. An empty durable state
// root is a bootstrap: it has not yet carried a genuine Core installation, so
// the atomic bootstrap lock can serialise the two competing fresh bootstraps
// that create it.
func isEmptyRoot(root string) bool {
	info, err := os.Lstat(root)
	if err != nil {
		return false
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() != "." && e.Name() != ".." && e.Name() != ".bootstrap.lock" {
			return false
		}
	}
	return true
}

// StateRoot returns the durable state root this host owns.
func (m *ManagementHost) StateRoot() string { return m.stateRoot }

// Verify proves that this process is the single active writer of the durable
// installation state root on the selected host.
//
// It enforces, in order:
//   - the root exists and is a protected (mode 0700) directory;
//   - a valid Core installation identity is loaded from exactly that root;
//   - the process holds the kernel lifecycle lock, so no second writer is active.
//
// A missing root is not an error here: it returns Bootstrap==true and holds an
// atomic bootstrap lock so the caller may bootstrap a fresh durable
// installation. A foreign or unreadable root fails closed with a named sentinel
// error and selects no local fallback.
func (m *ManagementHost) Verify() (Bootstrap, func(), error) {
	info, err := os.Lstat(m.stateRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// A missing durable state root is a bootstrap: nothing is owned yet.
			// Acquire the atomic bootstrap lock so a second competing fresh
			// bootstrap is refused with ErrManagementHostOwnedFreshRoot, and only
			// one process ever owns the new authority. The caller provisions the
			// fresh installation while the lock is held.
			unlock, berr := AcquireBootstrapLock(m.stateRoot)
			if berr != nil {
				return false, nil, ErrManagementHostOwnedFreshRoot
			}
			return true, unlock, nil
		}
		return false, nil, ErrManagementHostRootForeign
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return false, nil, ErrManagementHostRootProtected
	}
	// A durable state root that exists but carries no genuine Core installation
	// yet (a competing bootstrap's MkdirAll created it, leaving only the atomic
	// bootstrap lock) is still a bootstrap: serialise the two competing fresh
	// bootstraps through the atomic bootstrap lock so only one process ever owns
	// the new authority.
	if isEmptyRoot(m.stateRoot) {
		unlock, berr := AcquireBootstrapLock(m.stateRoot)
		if berr != nil {
			return false, nil, ErrManagementHostOwnedFreshRoot
		}
		return true, unlock, nil
	}
	// The durable installation identity is loaded from exactly this root. A root
	// that does not load back as a genuine baseharbor Core installation is
	// foreign: it selects no authority and is never adopted as a local fallback.
	if _, err := Load(m.stateRoot); err != nil {
		return false, nil, ErrManagementHostRootForeign
	}
	// Single continuous ownership across the root's whole life: a durable root
	// that already carries a genuine installation may still be guarded by the
	// atomic bootstrap lock of a concurrent bootstrap server that has not yet
	// finished provisioning (the round-3 gap: two INDEPENDENT kernel locks whose
	// acquisition paths never checked one another, so a second process could take
	// the lifecycle lock while the bootstrap server still held only the
	// bootstrap lock). The bootstrap lock is therefore re-taken here first so the
	// attach path checks it as well as the lifecycle lock; a bootstrap server
	// still holding it makes attach fail closed with ErrManagementHostBootstrapContended
	// instead of letting two writers coexist. It is released below once the
	// lifecycle lock is held, so no lock is leaked and no interval is left
	// unprotected.
	bunlock, berr := AcquireBootstrapLock(m.stateRoot)
	if berr != nil {
		return false, nil, ErrManagementHostBootstrapContended
	}
	// Active-writer ownership: hold the kernel lifecycle lock for this root. A
	// second concurrent writer fails closed; a dead writer has already released
	// the lock on process exit, so no stale active lock survives.
	unlock, lerr := AcquireLifecycleLock(m.stateRoot)
	if lerr != nil {
		if bunlock != nil {
			bunlock()
		}
		if strings.Contains(lerr.Error(), "resource temporarily unavailable") ||
			strings.Contains(lerr.Error(), "busy") ||
			strings.Contains(lerr.Error(), "already") {
			return false, nil, ErrManagementHostAlreadyOwned
		}
		return false, nil, lerr
	}
	// Lifecycle lock held; release the bootstrap lock that only bridged to it.
	// The bootstrap lock and lifecycle lock never overlap as the sole owner:
	// the lifecycle lock is the durable active-writer lock for the rest of the
	// server lifetime.
	if bunlock != nil {
		bunlock()
	}
	return false, unlock, nil
}
