package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ControlPlaneRuntimeInventory observes the existing native owner, including
// its bootstrap or secured renderer shape. It does not initialize credentials,
// issue certificates, change a profile or mutate runtime state.
type ControlPlaneRuntimeInventory struct {
	Files            Files
	SQLServices      []string
	SecretsServices  []string
	Resources        []ProjectResource
	ProfiledServices []string
	OneShotServices  []string
}

// ContinuousServices selects normal availability requirements without changing
// the complete module inventory used by stop, retirement and recovery owners.
func (inventory ControlPlaneRuntimeInventory) ContinuousServices(services []string) []string {
	selected := []string{}
	for _, service := range services {
		if !slices.Contains(inventory.ProfiledServices, service) && !slices.Contains(inventory.OneShotServices, service) {
			selected = append(selected, service)
		}
	}
	return selected
}

func ExistingControlPlaneRuntimeInventory(stateDir, project string) (ControlPlaneRuntimeInventory, error) {
	inventory := ControlPlaneRuntimeInventory{SQLServices: []string{}, SecretsServices: []string{}, Resources: []ProjectResource{}}
	for _, path := range []string{stateDir, filepath.Join(stateDir, composeName), filepath.Join(stateDir, envName)} {
		info, err := os.Lstat(path)
		if err != nil {
			return inventory, err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || (path == stateDir && !info.IsDir()) || (path != stateDir && !info.Mode().IsRegular()) {
			return inventory, errors.New("native control-plane inventory requires protected canonical owner files")
		}
	}
	files, err := ExistingFilesForProject(stateDir, project)
	if err != nil {
		return inventory, err
	}
	inventory.Files = files
	actual, err := readCanonicalOwnerFile(files.Compose)
	if err != nil {
		return inventory, err
	}
	bootstrap := renderControlPlaneForFiles(files)
	secured, err := renderSecureControlPlanePostgres(bootstrap)
	if err != nil {
		return inventory, err
	}
	secured, err = renderSecureControlPlaneOpenBao(secured)
	if err != nil {
		return inventory, err
	}
	if !bytes.Equal(actual, []byte(bootstrap)) && !bytes.Equal(actual, []byte(secured)) {
		return inventory, errors.New("native control-plane definition differs from its canonical owner shape")
	}
	var model struct {
		Services map[string]struct {
			Profiles  []string `yaml:"profiles"`
			Restart   string   `yaml:"restart"`
			DependsOn map[string]struct {
				Condition string `yaml:"condition"`
			} `yaml:"depends_on"`
		} `yaml:"services"`
		Volumes map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"volumes"`
		Networks map[string]struct {
			Name     string `yaml:"name"`
			External bool   `yaml:"external"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal(actual, &model); err != nil {
		return inventory, err
	}
	for _, definition := range model.Services {
		for dependency, requirement := range definition.DependsOn {
			if requirement.Condition == "service_completed_successfully" && model.Services[dependency].Restart == "no" && !slices.Contains(inventory.OneShotServices, dependency) {
				inventory.OneShotServices = append(inventory.OneShotServices, dependency)
			}
		}
	}
	for service, definition := range model.Services {
		if len(definition.Profiles) > 0 {
			inventory.ProfiledServices = append(inventory.ProfiledServices, service)
		}
		if strings.HasPrefix(service, "postgres") {
			inventory.SQLServices = append(inventory.SQLServices, service)
		} else if strings.HasPrefix(service, "openbao") {
			inventory.SecretsServices = append(inventory.SecretsServices, service)
		} else {
			return inventory, fmt.Errorf("native control-plane service %s has no canonical module owner", service)
		}
	}
	// Use the shared protected-owner authority for resources as well as exact
	// renderer matching. UID, hardlinks and every parent are revalidated; a
	// matching document through a foreign link cannot authorize Core mutation.
	inventory.Resources, err = ExistingCanonicalProjectInventory(project, files.Compose, string(actual), files.Env)
	if err != nil {
		return inventory, err
	}
	sort.Strings(inventory.SQLServices)
	sort.Strings(inventory.SecretsServices)
	sort.Strings(inventory.ProfiledServices)
	sort.Strings(inventory.OneShotServices)
	sort.Slice(inventory.Resources, func(i, j int) bool {
		return inventory.Resources[i].Kind+inventory.Resources[i].Name < inventory.Resources[j].Kind+inventory.Resources[j].Name
	})
	return inventory, nil
}
