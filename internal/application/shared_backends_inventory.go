package application

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

// SharedBackendInventory is an ephemeral projection of the existing owner.
// Retained databases/credentials do not prove an active consumer.
type SharedBackendInventory struct {
	Files           SharedBackendFiles
	Services        map[capability.ProviderKind][]string
	Resources       []bhruntime.ProjectResource
	CoreSQL         *bhruntime.Files
	NetworkExternal bool
}

func ExistingSharedBackendInventoryAt(dataDir, namespace, environment string) (SharedBackendInventory, error) {
	files := SharedBackendFilesAt(dataDir, namespace, environment)
	inventory := SharedBackendInventory{Files: files, Services: map[capability.ProviderKind][]string{}, Resources: []bhruntime.ProjectResource{}}
	for _, path := range []string{files.Dir, files.State, files.Compose, files.Env} {
		info, err := os.Lstat(path)
		if err != nil {
			return inventory, err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || (path == files.Dir && !info.IsDir()) || (path != files.Dir && !info.Mode().IsRegular()) {
			return inventory, errors.New("shared backend owner files are unprotected or noncanonical")
		}
	}
	info, err := os.Lstat(files.State)
	if err != nil {
		return inventory, err
	}
	state, err := readSharedBackendDestroyState(files.State, info)
	if err != nil {
		return inventory, err
	}
	if state.Environment != filepath.Base(files.Dir) {
		return inventory, errors.New("shared backend environment differs from its existing owner directory")
	}
	inventory.CoreSQL = state.CoreSQL
	inventory.NetworkExternal = state.NetworkExternal
	canonical := sharedBackendCompose(files, state)
	actual, err := os.ReadFile(files.Compose)
	if err != nil || !bytes.Equal(actual, []byte(canonical)) {
		return inventory, errors.New("shared backend runtime differs from its canonical owner shape")
	}
	var model struct {
		Services map[string]any `yaml:"services"`
		Volumes  map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"volumes"`
		Networks map[string]struct {
			Name     string `yaml:"name"`
			External bool   `yaml:"external"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal([]byte(canonical), &model); err != nil {
		return inventory, err
	}
	var postgres strings.Builder
	if sharedBackendUsesStandalonePostgres(state) {
		writeSharedPostgresCompose(&postgres, state)
	}
	if sharedBackendPostgresUIRequested(state) {
		writeSharedPostgresUICompose(&postgres)
	}
	var pg struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte("services:\n"+postgres.String()), &pg); err != nil {
		return inventory, err
	}
	for name := range model.Services {
		kind := capability.ProviderValkey
		if _, ok := pg.Services[name]; ok {
			kind = capability.ProviderPostgreSQL
		}
		inventory.Services[kind] = append(inventory.Services[kind], name)
		inventory.Resources = append(inventory.Resources, bhruntime.ProjectResource{Kind: "container", Name: fmt.Sprintf("%s-%s-1", files.Project, name)})
	}
	for name, volume := range model.Volumes {
		if volume.Name != "" {
			name = volume.Name
		} else {
			name = files.Project + "_" + name
		}
		inventory.Resources = append(inventory.Resources, bhruntime.ProjectResource{Kind: "volume", Name: name})
	}
	for name, network := range model.Networks {
		if network.External {
			continue
		}
		if network.Name != "" {
			name = network.Name
		} else {
			name = files.Project + "_" + name
		}
		inventory.Resources = append(inventory.Resources, bhruntime.ProjectResource{Kind: "network", Name: name})
	}
	for kind := range inventory.Services {
		sort.Strings(inventory.Services[kind])
	}
	sort.Slice(inventory.Resources, func(i, j int) bool {
		return inventory.Resources[i].Kind+inventory.Resources[i].Name < inventory.Resources[j].Kind+inventory.Resources[j].Name
	})
	return inventory, nil
}
