package targetsession

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// ProjectRecord is persisted only in protected, Core-owned deployment state.
// It contains commitments and relative Node paths, never file/secret contents.
// It is not an operator API input or independent authority over a project.
type ProjectRecord struct {
	Version   int                    `json:"version"`
	Scope     targetenrollment.Scope `json:"scope"`
	BundleID  string                 `json:"bundle_id"`
	Directory string                 `json:"directory"`
	Files     []ProjectFileRecord    `json:"files"`
}

type ProjectFileRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode,omitempty"`
}

func (p *StagedProject) Record() ProjectRecord {
	if p == nil {
		return ProjectRecord{}
	}
	record := ProjectRecord{Version: 1, Scope: p.scope, BundleID: p.bundleID, Directory: p.directory}
	for relative, file := range p.files {
		digest := sha256.Sum256(file.data)
		record.Files = append(record.Files, ProjectFileRecord{Path: relative, SHA256: hex.EncodeToString(digest[:]), Mode: file.mode})
	}
	sort.Slice(record.Files, func(i, j int) bool { return record.Files[i].Path < record.Files[j].Path })
	return record
}

// RestoreProject rebinds a durable Core receipt to the current authorized
// Scope and exact protected source bytes. It never stages another bundle or
// retries a mutation. Compose revalidates the immutable Node manifest before
// execution; Quadlet realization uses these exact revalidated source bytes.
func (r *ProjectRuntime) RestoreProject(record ProjectRecord, files []ProjectFile) (*StagedProject, error) {
	if r == nil || record.Validate() != nil || record.Scope != r.scope || len(files) != len(record.Files) {
		return nil, errors.New("invalid persisted Core project binding")
	}
	if err := r.requireCapability("artifact.bundle.stage"); err != nil {
		return nil, err
	}
	expected := make(map[string]ProjectFileRecord, len(record.Files))
	for _, entry := range record.Files {
		expected[entry.Path] = entry
	}
	staged := &StagedProject{scope: r.scope, bundleID: record.BundleID, directory: record.Directory, files: make(map[string]stagedProjectFile, len(files))}
	total := 0
	for _, file := range files {
		commitment, exists := expected[file.Path]
		digest := sha256.Sum256(file.Data)
		mode, err := projectFileMode(file.Mode)
		persistedMode, _ := projectFileMode(commitment.Mode)
		if !exists || err != nil || commitment.SHA256 != hex.EncodeToString(digest[:]) || mode != persistedMode {
			return nil, errors.New("persisted project source differs")
		}
		delete(expected, file.Path)
		total += len(file.Data)
		if total > 4<<20 {
			return nil, errors.New("persisted project exceeds byte limit")
		}
		staged.files[file.Path] = stagedProjectFile{remotePath: path.Join(record.Directory, file.Path), data: append([]byte{}, file.Data...), mode: mode}
	}
	return staged, nil
}

// Validate checks a durable commitment without granting execution authority.
// Execution still requires the current authorized Scope and exact source bytes.
func (record ProjectRecord) Validate() error {
	if record.Version != 1 || record.Scope.Validate() != nil || !projectBundleID.MatchString(record.BundleID) ||
		!projectObjectDirectory.MatchString(record.Directory) || len(record.Files) == 0 || len(record.Files) > 128 {
		return errors.New("invalid persisted Core project binding")
	}
	seen := make(map[string]bool, len(record.Files))
	for _, entry := range record.Files {
		if entry.Path == "" || entry.Path == "." || entry.Path == ".." || entry.Path == ".manifest.json" || path.Clean(entry.Path) != entry.Path ||
			strings.HasPrefix(entry.Path, "/") || strings.HasPrefix(entry.Path, "../") || strings.ContainsAny(entry.Path, "\\\x00\r\n") || seen[entry.Path] {
			return errors.New("invalid persisted project member")
		}
		digest, err := hex.DecodeString(entry.SHA256)
		if err != nil || len(digest) != sha256.Size || hex.EncodeToString(digest) != entry.SHA256 {
			return errors.New("invalid persisted project digest")
		}
		if _, err := projectFileMode(entry.Mode); err != nil {
			return err
		}
		seen[entry.Path] = true
	}
	return nil
}
