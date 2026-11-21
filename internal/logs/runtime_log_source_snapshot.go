package logs

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

// runtimeLogSourceSnapshotVersion tags the retained, owner-protected log-source
// snapshot so a foreign or tampered file cannot be read as a Loki provider
// snapshot. It mirrors the metrics package's runtime-sources contract.
const runtimeLogSourceSnapshotVersion = "baseharbor.loki-runtime-sources/v1"

type runtimeLogSourceSnapshot struct {
	Version string                       `json:"version"`
	Project string                       `json:"project"`
	Scope   capability.ProviderScope     `json:"scope"`
	Sources []observability.SignalSource `json:"sources"`
}

// readRuntimeLogSourceSnapshot reads the retained, owner-protected Loki
// log-source snapshot of the materialized provider and validates that it still
// matches its runtime owner (project, scope, protected owner). A missing
// snapshot returns found=false; a damaged, foreign-owned, or owner-mismatched
// snapshot returns an error so the datasource can fail closed rather than bind
// a source set that does not belong to this provider.
func readRuntimeLogSourceSnapshot(files ProviderFiles, placement Placement) ([]observability.SignalSource, bool, error) {
	path := filepath.Join(files.Dir, "runtime-sources.json")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	data, err := externalprovider.ReadMaterialReference((&url.URL{Scheme: "file", Path: path}).String(), true)
	if err != nil {
		return nil, true, errors.New("retained Loki source inputs have no protected owner")
	}
	var snapshot runtimeLogSourceSnapshot
	if json.Unmarshal(data, &snapshot) != nil || snapshot.Version != runtimeLogSourceSnapshotVersion || snapshot.Project != placement.Project || snapshot.Scope != placement.Scope || snapshot.Sources == nil {
		return nil, true, errors.New("retained Loki source inputs differ from their runtime owner")
	}
	seen := map[string]bool{}
	for _, source := range snapshot.Sources {
		if source.Validate() != nil || seen[source.ID] {
			return nil, true, errors.New("retained Loki source inputs are invalid or ambiguous")
		}
		seen[source.ID] = true
	}
	return snapshot.Sources, true, nil
}
