package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

const remoteManagedSnapshotFile = "remote-managed-snapshot.json"
const remoteManagedSnapshotLimit = 8 << 20

// The snapshot stays in private Core state. Only the separately persisted
// deployment receipt selects which immutable source may be restored.
type remoteManagedSnapshot struct {
	Version   int                         `json:"version"`
	Record    targetsession.ProjectRecord `json:"record"`
	Files     []targetsession.ProjectFile `json:"files"`
	Units     []string                    `json:"units,omitempty"`
	InitUnits []string                    `json:"init_units,omitempty"`
}

// SaveSnapshot is called by the protected deployment commit callback before
// activation. A changed original checkout or provider credential file cannot
// then prevent owned repair/teardown of the previously published project.
// An existing different publication is never silently replaced.
func (r *RemoteManagedRuntime) SaveSnapshot(stateDir string, record targetsession.ProjectRecord) error {
	if r == nil || r.runtime == nil || record.BundleID != r.name {
		return errors.New("remote managed snapshot selection differs")
	}
	if _, err := r.runtime.RestoreProject(record, r.source); err != nil {
		return err
	}
	snapshot := remoteManagedSnapshot{Version: 1, Record: record, Files: r.source, Units: r.units, InitUnits: r.initUnits}
	data, err := json.Marshal(snapshot)
	if err != nil || len(data) > remoteManagedSnapshotLimit {
		return errors.New("remote managed snapshot exceeds protected state limit")
	}
	root, err := openRemoteSnapshotRoot(stateDir)
	if err != nil {
		return err
	}
	defer root.Close()
	previous, err := readRemoteSnapshot(root)
	if err == nil {
		if !reflect.DeepEqual(previous, snapshot) {
			return errors.New("different remote managed publication already retained")
		}
		return syncRemoteSnapshot(root)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Use exclusive creation: concurrent callers cannot overwrite another
	// publication. Incomplete persistence fails closed and requires inspection.
	file, err := root.OpenFile(remoteManagedSnapshotFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncRemoteSnapshot(root)
}

func syncRemoteSnapshot(root *os.Root) error {
	file, err := root.Open(remoteManagedSnapshotFile)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// RemoveSnapshot is only called after authenticated owned teardown. The exact
// publication must match the retained deployment before removal.
func (r *RemoteManagedRuntime) RemoveSnapshot(stateDir string) error {
	if r == nil || r.project == nil {
		return errors.New("remote publication is unavailable")
	}
	root, err := openRemoteSnapshotRoot(stateDir)
	if err != nil {
		return err
	}
	defer root.Close()
	snapshot, err := readRemoteSnapshot(root)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(snapshot.Record, r.Record()) {
		return errors.New("remote publication changed before snapshot cleanup")
	}
	if err := root.Remove(remoteManagedSnapshotFile); err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// RestoreRemoteManagedSnapshot does not regenerate material, build sources or
// stage another bundle. The caller supplies the protected registry receipt and
// the current authorized Node scope; the snapshot supplies only original bytes.
func RestoreRemoteManagedSnapshot(transport targetsession.ProjectTransport, scope targetenrollment.Scope, stateDir string, record targetsession.ProjectRecord) (*RemoteManagedRuntime, error) {
	if record.Validate() != nil || scope != record.Scope {
		return nil, errors.New("remote managed snapshot scope differs")
	}
	root, err := openRemoteSnapshotRoot(stateDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	snapshot, err := readRemoteSnapshot(root)
	if err != nil {
		return nil, err
	}
	if snapshot.Version != 1 || !reflect.DeepEqual(snapshot.Record, record) {
		return nil, errors.New("remote managed snapshot receipt differs")
	}
	if err := snapshot.validateShape(scope.Runtime); err != nil {
		return nil, err
	}
	runtime, err := targetsession.NewProjectRuntime(transport, scope)
	if err != nil {
		return nil, err
	}
	result := &RemoteManagedRuntime{runtime: runtime, kind: scope.Runtime, name: record.BundleID,
		compose: "compose.yaml", env: "runtime.env", source: snapshot.Files,
		units: snapshot.Units, initUnits: snapshot.InitUnits, published: true}
	if err := result.Restore(record); err != nil {
		return nil, err
	}
	return result, nil
}

func openRemoteSnapshotRoot(stateDir string) (*os.Root, error) {
	if !filepath.IsAbs(stateDir) || filepath.Clean(stateDir) != stateDir {
		return nil, errors.New("remote managed snapshot requires protected absolute Core state")
	}
	info, err := os.Lstat(stateDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, errors.New("remote managed snapshot directory must remain owner-only")
	}
	return os.OpenRoot(stateDir)
}

func readRemoteSnapshot(root *os.Root) (remoteManagedSnapshot, error) {
	info, err := root.Lstat(remoteManagedSnapshotFile)
	if err != nil {
		return remoteManagedSnapshot{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return remoteManagedSnapshot{}, errors.New("remote managed snapshot must be a protected regular file")
	}
	file, err := root.Open(remoteManagedSnapshotFile)
	if err != nil {
		return remoteManagedSnapshot{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, remoteManagedSnapshotLimit+1))
	if err != nil || len(data) > remoteManagedSnapshotLimit {
		return remoteManagedSnapshot{}, errors.New("remote managed snapshot exceeds protected state limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var snapshot remoteManagedSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return remoteManagedSnapshot{}, errors.New("invalid remote managed snapshot")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return remoteManagedSnapshot{}, errors.New("trailing remote managed snapshot data")
	}
	return snapshot, nil
}

func (s remoteManagedSnapshot) validateShape(kind string) error {
	var units, init []string
	compose, env := false, false
	for _, file := range s.Files {
		compose = compose || file.Path == "compose.yaml"
		env = env || file.Path == "runtime.env"
		switch filepath.Ext(file.Path) {
		case ".container", ".network", ".volume":
			units = append(units, file.Path)
			if filepath.Ext(file.Path) == ".container" && strings.Contains(string(file.Data), "\n[Service]\nRemainAfterExit=yes\nRestart=no") {
				init = append(init, file.Path)
			}
		}
	}
	if kind == "docker" {
		if !compose || !env || len(s.Units) != 0 || len(s.InitUnits) != 0 {
			return errors.New("invalid Docker managed snapshot shape")
		}
		return nil
	}
	sort.Strings(units)
	sort.Strings(init)
	selected, completed := append([]string(nil), s.Units...), append([]string(nil), s.InitUnits...)
	sort.Strings(selected)
	sort.Strings(completed)
	if kind != "podman" || len(units) == 0 || !reflect.DeepEqual(units, selected) || !reflect.DeepEqual(init, completed) {
		return errors.New("invalid Podman managed snapshot graph")
	}
	return nil
}
