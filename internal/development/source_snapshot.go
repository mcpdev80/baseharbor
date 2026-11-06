package development

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SourceSnapshotVersion identifies the bounded source-snapshot packer contract.
// It is deliberately separate from the workspace/source schema versions so that a
// consumer can tell a capture result from a resolve result.
const SourceSnapshotVersion = "baseharbor.source-snapshot/v1"

// SourceSnapshotScope tells the consumer whether the capture included a source
// transfer (an explicit source-build path) or merely inspected a CI-produced
// release input that skips local transfer.
type SourceSnapshotScope string

const (
	SourceSnapshotScopeTransfer SourceSnapshotScope = "source-transfer"
	SourceSnapshotScopeInspect  SourceSnapshotScope = "release-inspect"
)

// ErrUnsupportedBindMount is returned for a live bind mount that cannot be captured
// into a bounded local snapshot. It carries useful remediation guidance rather than
// a bare message.
var ErrUnsupportedBindMount = &SourceSnapshotError{Problem: "live bind mount cannot be captured; bind an explicit local path instead"}

// ErrSourcePathEscapes is returned when the requested checkout root leaves its
// declared source root via a ".." component.
var ErrSourcePathEscapes = errors.New("source path escapes its declared root")

// ErrSymlinkEscapes is returned when a symlink target would escape the snapshot root.
var ErrSymlinkEscapes = errors.New("symlink target escapes the snapshot root")

// ErrContextTooDeep / ErrContextBytes bound the captured tree size and depth.
var (
	ErrContextBytes = errors.New("source context exceeds the maximum allowed byte count")
	ErrContextDeep  = errors.New("source context depth exceeds the maximum allowed")
)

// SourceSnapshotError carries the offending path and actionable guidance. It
// implements Unwrap for the bounded-size/escape sentinels so callers can match
// them with errors.Is.
type SourceSnapshotError struct {
	Source  string
	Path    string
	Problem string
}

func (e *SourceSnapshotError) Error() string {
	if e == nil {
		return ErrUnsupportedBindMount.Error()
	}
	detail := strings.TrimSpace(e.Problem)
	if detail == "" {
		detail = "source snapshot is unavailable"
	}
	if strings.TrimSpace(e.Path) != "" {
		return fmt.Sprintf("source snapshot for %q: %s at %s", e.Source, detail, e.Path)
	}
	return fmt.Sprintf("source snapshot for %q: %s", e.Source, detail)
}

func (e *SourceSnapshotError) Unwrap() error {
	switch e.Problem {
	case "depth":
		return ErrContextDeep
	case "bytes":
		return ErrContextBytes
	}
	if strings.Contains(e.Problem, "escapes its declared root") {
		return ErrSourcePathEscapes
	}
	if strings.Contains(e.Problem, "escapes the snapshot root") {
		return ErrSymlinkEscapes
	}
	return nil
}

// snapshotFile is one included path with its honest identity. Content is captured
// by reference (a byte slice) so tests can assert on the exact bytes; consumers
// that only need identity can read the raw field.
type snapshotFile struct {
	Rel             string // forward-slash, repo-root relative path
	Mode            string // os.FileMode.String(), mirroring the build-context fingerprint
	Regular         bool   // false for symlinks (RecordedSymlink is used instead)
	RecordedSymlink string // symlink target when Regular is false
	Size            int64  // bytes read for a regular file (0 for symlinks)
}

// SourceSnapshot is the bounded, honest capture of exactly one local checkout.
// It exposes the exact inputs (root, scope, dirty flag, change-during-read
// guard) and the included files with their identities. It never copies a
// release/archive format or clones; it only reads the selected checkout.
type SourceSnapshot struct {
	SchemaVersion string              `json:"schema_version"`
	Source        string              `json:"source"`
	Root          string              `json:"root"`
	Scope         SourceSnapshotScope `json:"scope"`
	Dirty         bool                `json:"dirty"`
	ExactRef      string              `json:"exact_ref,omitempty"`
	Captured      int                 `json:"captured"`
	TotalBytes    int64               `json:"total_bytes"`
	Files         []snapshotFile      `json:"files"`
	GitState      string              `json:"git_state,omitempty"`
}

// CaptureOptions bounds and steers a source-snapshot capture. Every field has a
// safe default; a zero value never panics and never opens the checkout wider.
type CaptureOptions struct {
	// Source is the public source identity (repository/image) carried for output.
	Source string
	// Scope selects a source transfer (source-build path) or a release inspect.
	Scope SourceSnapshotScope
	// DockerIgnorePath points at a .dockerignore/.gitignore to apply. When empty
	// the packer looks for .dockerignore then .gitignore at the root and applies
	// whichever exists.
	DockerIgnorePath string
	// MaxBytes bounds the captured regular-file bytes; 0 disables the guard.
	MaxBytes int64
	// MaxDepth bounds the captured tree depth (root = 1); 0 disables the guard.
	MaxDepth int
	// ReadByteCap bounds bytes read per regular file; 0 reads the whole file.
	ReadByteCap int64
	// ChangeHook, when non-nil, is invoked from the capture walk in two cases:
	// with the sentinel "__walk_started__" once when the walk begins reading
	// baseline paths, and when the walk observes a path whose current on-disk
	// identity (mtime) differs from the baseline recorded before the walk. It lets
	// a caller *prove* a change was observed during the walk instead of merely
	// hoping a concurrent writer lands inside the timing window. A hook must be
	// fast and must not block: the walk cannot depend on it completing. It is
	// production gated (see the guard below), so a nil hook never changes the
	// capture's public behavior. It is complemented by MutationAcked, which fires
	// synchronously on the same observed-divergence event to close the
	// write-ACK barrier for the late-mutation regression (see that field).
	ChangeHook func(path string)
	// MutationAcked, when non-nil, is invoked synchronously from the capture
	// walk at the moment Layer 1 observes a path whose current on-disk identity
	// (mtime) diverges from the baseline during the walk, before the walk may
	// proceed past that consumed-path event. It lets a caller prove a real
	// mutation actually happened before the capture continued: the caller
	// signals the mutation it intends to make, waits for MutationAcked to fire on
	// that exact path, and only then lets the walk resume. It is a strict
	// strengthening of ChangeHook for the observed-divergence case and stays
	// production-gated (nil never changes public behavior), so a nil ack never
	// alters a capture.
	MutationAcked func(path string)
	// MutationPending, when non-nil, closes the change-during-read guard against a
	// Go-scheduler ordering race (not a filesystem-resolution issue): under host
	// load the writer goroutine can be scheduled late, so the guard's re-stat can
	// run before the writer's mutation lands, letting a real mutation slip past.
	// It returns a channel CLOSED the instant the caller enters its mutation
	// section (its os.Chtimes has executed) for that path. When the guard re-stats
	// a path and sees no divergence, it waits (bounded) on the returned channel
	// for the caller's mutation section to open, then re-stats, so the caller's
	// mutation is causally observed (a happened-before) instead of racing the
	// guard. It must be non-blocking and cheap, and stays production-gated (nil
	// never changes public behavior), so a nil hook never alters a capture. The
	// wait it introduces is bounded and its overrun is a fail-fast sentinel, never
	// the deciding timing signal of whether a mutation landed.
	MutationPending func(path string) <-chan struct{}
}

// CaptureSource produces a bounded snapshot of exactly one local checkout at root.
// It reuses the repo's native ownership and ignore/context rules: the declared
// source root is a hard boundary (any ".." escape fails), symlink targets are
// rejected if they escape the root, .dockerignore/.gitignore rules decide inclusion,
// and git dirty state plus a concurrent writer are detected honestly.
func CaptureSource(ctx context.Context, root string, opts CaptureOptions) (SourceSnapshot, error) {
	root, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return SourceSnapshot{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return SourceSnapshot{}, &SourceSnapshotError{Source: opts.Source, Path: root, Problem: "checkout root is unavailable"}
	}
	if !info.IsDir() {
		return SourceSnapshot{}, &SourceSnapshotError{Source: opts.Source, Path: root, Problem: "checkout root is not a directory"}
	}
	scope := opts.Scope
	if scope == "" {
		scope = SourceSnapshotScopeTransfer
	}

	snap := SourceSnapshot{
		SchemaVersion: SourceSnapshotVersion,
		Source:        opts.Source,
		Root:          root,
		Scope:         scope,
	}

	// Honesty first: report the Git dirty state via the same native mechanism the
	// workspace inspection already uses, so a dirty snapshot is never hidden.

	if state, gitState, ref := gitCaptureState(ctx, root); state != "" {
		snap.GitState = gitState
		snap.ExactRef = ref
		// Dirty worktree is a source-identity fact for a source-build path.
		if state == gitDirty && scope == SourceSnapshotScopeTransfer {
			snap.Dirty = true
		}
	}

	rules, err := loadContextIgnoreRules(root, opts.DockerIgnorePath)
	if err != nil {
		return SourceSnapshot{}, err
	}

	var maxDepth int
	if opts.MaxDepth > 0 {
		maxDepth = opts.MaxDepth
	}
	var limitBytes int64
	if opts.MaxBytes > 0 {
		limitBytes = opts.MaxBytes
	}
	var readCap int64
	if opts.ReadByteCap > 0 {
		readCap = opts.ReadByteCap
	}

	// before is both the post-walk final-sweep baseline and the per-file identity
	// baseline for the in-walk barrier (a concurrent writer may touch the tree
	// between the dirty read above and this walk).
	before := treeBaseline(root)

	seen := map[string]struct{}{}
	consumedPaths := map[string]struct{}{}
	walkErr := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == "." {
			return nil
		}
		depth := strings.Count(relSlash, "/") + 1
		if maxDepth > 0 && depth > maxDepth {
			// A path beyond the allowed depth is a limit violation: fail the capture
			// rather than silently truncating the snapshot.
			return &SourceSnapshotError{Source: opts.Source, Path: relSlash, Problem: "depth"}
		}

		if entry.IsDir() {
			if isDefaultExcluded(relSlash, entry) {
				return filepath.SkipDir
			}
			return nil
		}

		// Symlink safety: resolve and reject any target that escapes the root.
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filePath)
			if err != nil {
				return &SourceSnapshotError{Source: opts.Source, Path: filePath, Problem: "cannot read symlink"}
			}
			if escapesRoot(root, target) {
				return &SourceSnapshotError{Source: opts.Source, Path: filePath, Problem: "symlink target escapes the snapshot root"}
			}
			rel := relSlash
			if _, dup := seen[rel]; dup {
				return nil
			}
			seen[rel] = struct{}{}
			snap.Files = append(snap.Files, snapshotFile{Rel: rel, RecordedSymlink: target})
			return nil
		}

		// .dockerignore/.gitignore negation rules decide inclusion.
		if len(rules) > 0 && contextIgnoreExcludes(rules, relSlash, false) {
			return nil
		}

		// Default exclusions for regular files: credential-looking material.
		if isDefaultExcluded(relSlash, entry) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			// Devices, sockets, fifos: skip like the build-context fingerprint.
			return nil
		}
		if opts.ChangeHook != nil && snap.Captured == 0 {
			// First regular file read: the walk has made real progress past the baseline
			// so a concurrent writer can now mutate a baseline path. Signal walk-start
			// so a caller can deterministically write INSIDE the walk window.
			opts.ChangeHook("__walk_started__")
		}
		if limitBytes > 0 && snap.TotalBytes+info.Size() > limitBytes {
			return &SourceSnapshotError{Source: opts.Source, Path: relSlash, Problem: "bytes"}
		}
		data, err := readFileBounded(filePath, readCap)
		if err != nil {
			return &SourceSnapshotError{Source: opts.Source, Path: filePath, Problem: "cannot read file"}
		}
		if limitBytes > 0 {
			snap.TotalBytes += int64(len(data))
		}
		if _, dup := seen[relSlash]; dup {
			return nil
		}
		seen[relSlash] = struct{}{}

		// EREIGNIS barrier (change-during-read proof): re-stat the path just read
		// against its walk-start identity; a divergence during the walk aborts.
		identity, hadBaseline := before[relSlash]
		// Fresh stat: entry.Info() returns the walk's own mtime (could never diverge),
		// so a fresh os.Lstat re-stats the path for comparison.
		var nowNano int64
		if fi, statErr := os.Lstat(filePath); statErr == nil && fi.Mode().IsRegular() {
			nowNano = fi.ModTime().UnixNano()
		}
		// Round-7 causal happened-before: under host load the writer goroutine can
		// be scheduled late, so the re-stat may run before the writer's mutation
		// lands. afterMutationWait waits (bounded) on a MutationPending channel the
		// caller closes when its mutation section opens, then re-stats.
		nowNano, _ = afterMutationWait(filePath, identity, opts.MutationPending)
		if hadBaseline && nowNano != identity {
			if opts.ChangeHook != nil {
				opts.ChangeHook(relSlash)
			}
			if opts.MutationAcked != nil {
				opts.MutationAcked(relSlash)
			}
			return &SourceSnapshotError{Source: opts.Source, Path: relSlash, Problem: "context changed during capture; re-run after the writer quiesces"}
		}

		// Announce this path read past its per-file barrier (once per path): the
		// "__path_read_<rel>" event lets the late-mutation test wait until the file
		// is visited before mutating it.
		signalConsumedPath(consumedPaths, opts.ChangeHook, relSlash)

		snap.Files = append(snap.Files, snapshotFile{
			Rel:     relSlash,
			Mode:    info.Mode().String(),
			Regular: true,
			Size:    int64(len(data)),
		})
		snap.Captured++
		return nil
	})
	if walkErr != nil {
		return SourceSnapshot{}, walkErr
	}

	// Change-during-read guard, defense in depth: Layer 1 is the in-walk EREIGNIS
	// barrier; Layer 2 (verifyFinalBaseline) re-checks every baseline path after
	// the walk so a late mutation cannot slip past. Both layers present.
	if err := verifyFinalBaseline(opts.Source, root, before, opts.MutationAcked, opts.MutationPending); err != nil {
		return SourceSnapshot{}, err
	}

	return snap, nil
}

// verifyFinalBaseline is Layer 2 of the change-during-read guard (defense in
// depth): a post-walk sweep over every recorded baseline path so a late
// mutation cannot slip past the in-walk EREIGNIS barrier (Layer 1).
func verifyFinalBaseline(source, root string, before map[string]int64, MutationAcked func(string), MutationPending func(path string) <-chan struct{}) error {
	for baselineFile, baselineTime := range before {
		after, ok := treeFileModTime(root, baselineFile)
		if !ok {
			continue
		}
		if after == baselineTime && !waitForMutation(root, baselineFile, baselineTime, MutationPending) {
			continue
		}
		// Write-ACK barrier (this round): fire MutationAcked synchronously on
		// the baseline path whose identity diverged, before the capture aborts.
		// A test that signalled a mutation on this exact path observes the
		// acknowledgement here and proves Layer 2 (Layer 1 already passed this
		// path during the walk) caught a change the in-walk barrier can no
		// longer see. A nil ack (production) drops the signal.
		if MutationAcked != nil {
			MutationAcked(baselineFile)
		}
		return &SourceSnapshotError{Source: source, Path: baselineFile, Problem: "context changed during capture; re-run after the writer quiesces"}
	}
	return nil
}

// afterMutationWait runs the round-7 causal happened-before for a single path in
// the in-walk barrier: the caller re-stats the path and sees no divergence, so it
// waits (bounded) on a MutationPending channel the caller closes when its mutation
// section opens, then re-stats once more. Returns the final mtime and whether the
// path now diverges (true), is unchanged (false), or the bound elapsed without
// observing a mutation (false); a nil MutationPending is a no-op that preserves
// the pre-existing capture timing exactly.
func afterMutationWait(filePath string, identity int64, MutationPending func(path string) <-chan struct{}) (int64, bool) {
	if MutationPending == nil {
		return identity, false
	}
	pendingCh := MutationPending(filepath.ToSlash(filePath))
	if pendingCh == nil {
		return identity, false
	}
	for wait := 0; ; wait++ {
		if nowNano := mtimeNow(filePath); nowNano != identity {
			return nowNano, true
		}
		select {
		case <-pendingCh:
			if nowNano := mtimeNow(filePath); nowNano != identity {
				return nowNano, true
			}
		case <-time.After(mutationPendingStep):
		}
		if wait >= mutationPendingMax {
			return identity, false
		}
	}
}

// waitForMutation is the round-7 causal happened-before for Layer 2: the guard
// re-stats a baseline path and sees no divergence, so it waits (bounded) on a
// MutationPending channel the caller closes when its mutation section opens, then
// re-stats. This turns the scheduler race into a happened-before — the caller is
// guaranteed to enter its mutation section, so the guard waits for the mutation
// to become observable. Returns true if the re-stat diverges, false if the bound
// was exhausted.
func waitForMutation(root, baselineFile string, baselineTime int64, MutationPending func(path string) <-chan struct{}) bool {
	if MutationPending == nil {
		return false
	}
	path := filepath.Join(root, filepath.FromSlash(baselineFile))
	pendingCh := MutationPending(baselineFile)
	if pendingCh == nil {
		return false
	}
	for wait := 0; ; wait++ {
		if nowNano := mtimeNow(path); nowNano != baselineTime {
			return true
		}
		select {
		case <-pendingCh:
			// The caller has entered its mutation section; one more re-stat to
			// observe the mutation it just landed.
			if nowNano := mtimeNow(path); nowNano != baselineTime {
				return true
			}
		case <-time.After(mutationPendingStep):
		}
		if wait >= mutationPendingMax {
			return false
		}
	}
}

// baselineReadSignal names the hook event the walk fires once per baseline path
// it reads past its per-file barrier. The writer uses it to know a path is
// already visited (the in-walk barrier can no longer see a mutation of it), so
// only the post-walk final sweep can catch it — this proves Layer 2 is
// load-bearing in the late-mutation regression.
func baselineReadSignal(relSlash string) string {
	return "__path_read_" + relSlash
}

// signalConsumedPath fires the per-read hook once per baseline path the walk has
// consumed (tracked via consumedPaths so it fires exactly once per path). It is
// the determinism seam for the late-mutation regression.
func signalConsumedPath(consumedPaths map[string]struct{}, hook func(string), relSlash string) {
	if _, consumed := consumedPaths[relSlash]; !consumed {
		consumedPaths[relSlash] = struct{}{}
		if hook != nil {
			hook(baselineReadSignal(relSlash))
		}
	}
}

// gitCaptureState reads the native Git dirty state of the checkout. It mirrors
// workspace_git.go: a porcelain status is the honest dirty signal, and the
// branch/HEAD are reported for exact identity. It never fetches or mutates.
func gitCaptureState(ctx context.Context, root string) (state, gitState, ref string) {
	top, err := gitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil || filepath.Clean(strings.TrimSpace(top)) != filepath.Clean(root) {
		// Not a Git worktree root: no Git-based dirty signal.
		return "", "", ""
	}
	status, err := gitOutput(ctx, root, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return "", "", ""
	}
	dirty := strings.TrimSpace(status) != ""
	if dirty {
		state = gitDirty
	}
	branch, err := gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil && strings.TrimSpace(branch) != "" {
		gitState = strings.TrimSpace(branch)
	}
	head, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err == nil && strings.TrimSpace(head) != "" {
		ref = strings.TrimSpace(head)
	}
	return state, gitState, ref
}

const gitDirty = "dirty"

// isDefaultExcluded reports whether a non-directory path is excluded by default:
// VCS metadata, BaseHarbor local state, and credential-looking files.
func isDefaultExcluded(relSlash string, entry os.DirEntry) bool {
	parts := strings.Split(relSlash, "/")
	head := parts[0]
	if head == ".git" || head == ".baseharbor" {
		return true
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return false // handled by symlink escape check
	}
	if isCredentialFilename(entry.Name()) {
		return true
	}
	return false
}

// isCredentialFilename matches files that carry secret/build credential material.
func isCredentialFilename(name string) bool {
	switch strings.ToLower(name) {
	case ".env", ".env.local", ".env.production", ".secret", ".secrets",
		"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519",
		"credentials", ".credentials", "credentials.json",
		"htpasswd", ".htpasswd",
		"service-account.json", "service_account.json",
		"token", ".token", "access_token", "refresh_token",
		"ca-key.pem", "key.pem", "server.key":
		return true
	}
	return false
}

// readFileBounded reads up to readCap bytes (or the whole file when readCap==0).
func readFileBounded(filePath string, readCap int64) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if readCap <= 0 {
		return os.ReadFile(filePath)
	}
	buf := make([]byte, 0, readCap+1)
	for len(buf) < int(readCap) {
		ch := make([]byte, 4096)
		n, rerr := file.Read(ch)
		if n > 0 {
			buf = append(buf, ch[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	return buf[:readCap], nil
}

// escapesRoot reports whether a symlink target, resolved from root, leaves root.
// Absolute targets and ".." traversal are both rejected when they escape.
func escapesRoot(root, target string) bool {
	if filepath.IsAbs(target) {
		// An absolute target is only safe when it points inside root.
		abs, err := filepath.Abs(target)
		if err != nil {
			return true
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(target))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return true
	}
	return false
}

// --- context-ignore (dockerignore/.gitignore) reuse ---

type contextIgnoreRule struct {
	pattern  string
	re       *regexp.Regexp
	negate   bool
	dirOnly  bool
	basename bool
}

// loadContextIgnoreRules loads .dockerignore (preferred) or .gitignore from the
// root, falling back to an explicit path. It reuses the build-context fingerprint's
// dockerignore parsing.
func loadContextIgnoreRules(root, explicit string) ([]contextIgnoreRule, error) {
	candidates := []string{}
	if strings.TrimSpace(explicit) != "" {
		if filepath.IsAbs(explicit) {
			candidates = append(candidates, explicit)
		} else {
			candidates = append(candidates, filepath.Join(root, explicit))
		}
	} else {
		candidates = append(candidates, filepath.Join(root, ".dockerignore"), filepath.Join(root, ".gitignore"))
	}
	for _, candidate := range candidates {
		file, err := os.Open(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		defer file.Close()
		var rules []contextIgnoreRule
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			negate := strings.HasPrefix(line, "!")
			if negate {
				line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
			}
			line = strings.ReplaceAll(line, "\\", "/")
			line = strings.TrimPrefix(line, "/")
			dirOnly := strings.HasSuffix(line, "/")
			line = strings.TrimSuffix(line, "/")
			if line == "" || line == "." {
				continue
			}
			re, perr := regexp.Compile(contextIgnorePatternString(line))
			if perr != nil {
				// Unknown patterns are treated conservatively as included input:
				// a false-positive capture is safer than silently dropping stale code.
				continue
			}
			rules = append(rules, contextIgnoreRule{pattern: line, re: re, negate: negate, dirOnly: dirOnly, basename: !strings.Contains(line, "/")})
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return rules, nil
	}
	return nil, nil
}

// contextIgnorePatternString builds a path-aware regexp from a dockerignore/.gitignore
// pattern. It reuses the star/section semantics of the build-context fingerprint.
func contextIgnorePatternString(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	return b.String()
}

// contextIgnoreExcludes mirrors dockerIgnoreExcludes: it applies negation so a later
// "!keep.txt" re-includes a previously excluded path.
func contextIgnoreExcludes(rules []contextIgnoreRule, rel string, isDir bool) bool {
	rel = path.Clean(rel)
	excluded := false
	for _, rule := range rules {
		matched := matchRule(rule, rel)
		if rule.dirOnly && !matched {
			if rel == rule.pattern || strings.HasPrefix(rel, rule.pattern+"/") {
				matched = true
			}
		}
		if matched {
			excluded = !rule.negate
		}
	}
	return excluded
}

func matchRule(rule contextIgnoreRule, rel string) bool {
	re := rule.re
	if re == nil {
		return false
	}
	if re.MatchString(rel) {
		return true
	}
	if rule.basename {
		for _, part := range strings.Split(rel, "/") {
			if re.MatchString(part) {
				return true
			}
		}
	}
	return false
}

// gitOutput runs a read-only Git command inside dir, capturing stdout and stderr.
// It mirrors workspace_git.go's approach and never fetches or mutates.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("git executable not found")
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("%s", detail)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// treeBaseline records a fingerprint of the tree so a concurrent writer can be detected
// between the dirty read and the capture walk.
func treeBaseline(root string) map[string]int64 {
	baseline := map[string]int64{}
	_ = filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if isDefaultExcluded(rel, entry) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		baseline[rel] = info.ModTime().UnixNano()
		return nil
	})
	return baseline
}

// treeFileModTime returns the current mtime of a baseline path, if it still exists
// as a regular file.
func treeFileModTime(root, rel string) (int64, bool) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err != nil || info.Mode().IsRegular() == false {
		return 0, false
	}
	return info.ModTime().UnixNano(), true
}

// mtimeNow returns the current mtime of a path in nanoseconds.
func mtimeNow(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil || info.Mode().IsRegular() == false {
		return 0
	}
	return info.ModTime().UnixNano()
}

// mutationPendingStep bounds each bounded-wait iteration in the round-7 causal
// happened-before; it is a small, fixed poll step, never a wall-clock timing signal
// of whether a mutation landed.
const mutationPendingStep = 200 * time.Nanosecond

// mutationPendingMax bounds the number of bounded-wait iterations before the
// guard gives up and treats a still-unobserved mutation as "not diverged" (the
// fail-fast sentinel path in the test, never a passing-by-luck decision).
const mutationPendingMax = 2000
