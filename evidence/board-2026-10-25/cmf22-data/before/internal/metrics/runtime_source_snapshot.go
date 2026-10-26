package metrics

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	"github.com/mcpdev80/baseharbor/internal/observability"
)

const runtimeSourceSnapshotVersion = "baseharbor.prometheus-runtime-sources/v1"

type runtimeSourceSnapshot struct {
	Version string                        `json:"version"`
	Project string                        `json:"project"`
	Scope   capability.ProviderScope      `json:"scope"`
	Sources []observability.MetricsSource `json:"sources"`
}

// These are protected inputs of the already rendered provider, not new offers
// or a live discovery registry. Withdrawal elsewhere cannot erase its owner.
func readRuntimeSourceSnapshot(files ProviderFiles, placement Placement) ([]observability.MetricsSource, bool, error) {
	path := filepath.Join(files.Dir, "runtime-sources.json")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	data, err := externalprovider.ReadMaterialReference((&url.URL{Scheme: "file", Path: path}).String(), true)
	if err != nil {
		return nil, true, errors.New("retained Prometheus source inputs have no protected owner")
	}
	var snapshot runtimeSourceSnapshot
	if json.Unmarshal(data, &snapshot) != nil || snapshot.Version != runtimeSourceSnapshotVersion || snapshot.Project != placement.Project || snapshot.Scope != placement.Scope || snapshot.Sources == nil {
		return nil, true, errors.New("retained Prometheus source inputs differ from their runtime owner")
	}
	seen := map[string]bool{}
	for _, source := range snapshot.Sources {
		if source.Validate() != nil || seen[source.ID] {
			return nil, true, errors.New("retained Prometheus source inputs are invalid or ambiguous")
		}
		seen[source.ID] = true
	}
	return snapshot.Sources, true, nil
}

func writeRuntimeSourceSnapshot(files ProviderFiles, placement Placement, sources []observability.MetricsSource) error {
	if _, _, err := readRuntimeSourceSnapshot(files, placement); err != nil {
		return err
	}
	owner, err := os.Lstat(files.Dir)
	if err != nil || !owner.IsDir() || owner.Mode().Perm()&0022 != 0 {
		return errors.New("Prometheus source input directory has no protected owner")
	}
	// Use the existing private runtime owner to check its parent authority;
	// public provider directories need not become owner-only or change mounts.
	privateOwner, err := externalprovider.ReadMaterialReference((&url.URL{Scheme: "file", Path: files.Env}).String(), true)
	clear(privateOwner)
	if err != nil {
		return errors.New("Prometheus source inputs have no private runtime owner")
	}
	retained := append([]observability.MetricsSource{}, sources...)
	data, err := json.Marshal(runtimeSourceSnapshot{Version: runtimeSourceSnapshotVersion, Project: placement.Project, Scope: placement.Scope, Sources: retained})
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(files.Dir, ".runtime-sources-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(files.Dir, "runtime-sources.json"))
}
