package application

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

// NativeBackendResources uses the existing renderer, including its HA networks,
// to project resources. It creates neither a provider offer nor ownership state.
func NativeBackendResources(m Manifest, files RuntimeFiles) ([]bhruntime.ProjectResource, error) {
	if !HasApplicationScopedRuntimeServices(m) {
		return []bhruntime.ProjectResource{}, nil
	}
	canonical, err := RuntimeComposeYAMLForProject(m, files.ResourceProject)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	resources := []bhruntime.ProjectResource{}
	for service := range model.Services {
		resources = append(resources, bhruntime.ProjectResource{Kind: "container", Name: files.Project + "-" + service + "-1"})
	}
	for name, volume := range model.Volumes {
		if volume.Name != "" {
			name = volume.Name
		} else {
			name = files.Project + "_" + name
		}
		resources = append(resources, bhruntime.ProjectResource{Kind: "volume", Name: name})
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
		resources = append(resources, bhruntime.ProjectResource{Kind: "network", Name: name})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Kind+resources[i].Name < resources[j].Kind+resources[j].Name })
	return resources, nil
}

// CheckExistingNativeBackendInventory accepts only a protected canonical
// definition from this deployment's retained or desired intent. A retry may
// already have written the desired definition before retiring old containers.
func CheckExistingNativeBackendInventory(files RuntimeFiles, candidates ...Manifest) error {
	for _, path := range []string{files.Dir, files.Compose, files.Env} {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0077 != 0 || info.Mode()&os.ModeSymlink != 0 || path == files.Dir && !info.IsDir() || path != files.Dir && !info.Mode().IsRegular() {
			return errors.New("native backend owner files are unprotected or noncanonical")
		}
	}
	actual, err := readProtectedNativeBackendDefinition(files.Compose)
	if err != nil {
		return err
	}
	for _, m := range candidates {
		if err := m.ValidateIntent(); err != nil {
			return err
		}
	}
	for _, m := range candidates {
		// A validated empty withdrawal has no executable definition. Retained
		// applied/previous owners must still prove the actual protected bytes.
		if m.Validate() != nil {
			continue
		}
		canonical, err := RuntimeComposeYAMLForProject(m, files.ResourceProject)
		if err != nil {
			return err
		}
		if bytes.Equal(actual, []byte(canonical)) {
			return nil
		}
	}
	return ErrRuntimeDefinitionChanged
}

func readProtectedNativeBackendDefinition(path string) ([]byte, error) {
	expected, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !expected.Mode().IsRegular() || expected.Mode().Perm()&0077 != 0 || expected.Size() > 4<<20 {
		return nil, errors.New("native backend definition is not a bounded protected owner file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(expected, actual) {
		return nil, errors.New("native backend definition owner changed during inspection")
	}
	current, err := os.Lstat(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(expected, current) {
		return nil, errors.New("native backend definition path changed during inspection")
	}
	data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, errors.New("native backend definition exceeds bounded inspection")
	}
	return data, nil
}

// NativeBackendInstanceResources attributes only renderer-defined services to
// their actual existing provider instance, including named PostgreSQL/Valkey
// realizations. A peer instance never inherits another instance's containers.
func NativeBackendInstanceResources(m Manifest, files RuntimeFiles, instance capability.ProviderInstance) ([]bhruntime.ProjectResource, error) {
	if instance.Scope != capability.ScopeApplication || instance.Ownership != capability.OwnershipBaseHarbor || instance.OwnerApplicationID != m.ApplicationID || instance.OwnerEnvironment != m.Environment {
		return nil, fmt.Errorf("native backend instance differs from deployment owner")
	}
	selected := m
	selected.Services = Services{}
	switch instance.Provider.Kind {
	case capability.ProviderPostgreSQL:
		for index, name := range SQLInstanceNames(m) {
			if applicationProviderInstanceID(instance.Provider.Kind, m, name) == instance.ID {
				selected = WithSQLInstances(selected, name)
				selected.Services.SQLManagementUI = index == 0 && m.Services.SQLManagementUI
			}
		}
	case capability.ProviderValkey:
		for index, name := range ValkeyInstanceNames(m) {
			if applicationProviderInstanceID(instance.Provider.Kind, m, name) == instance.ID {
				selected = WithCacheInstances(selected, name)
				selected.Services.CacheManagementUI = index == 0 && (m.Services.CacheManagementUI || m.Services.KeyValueManagementUI)
			}
		}
	case capability.ProviderRabbitMQ:
		selected.Services.MessagingQueue, selected.Services.MessagingPubSub, selected.Services.MessagingStream = m.Services.MessagingQueue, m.Services.MessagingPubSub, m.Services.MessagingStream
		selected.Services.MessagingQueueInstances, selected.Services.MessagingPubSubInstances, selected.Services.MessagingStreamInstances = m.Services.MessagingQueueInstances, m.Services.MessagingPubSubInstances, m.Services.MessagingStreamInstances
		selected.Services.MessagingManagementUI = m.Services.MessagingManagementUI
	case capability.ProviderMongoDB:
		selected.Services.DocumentDatabase, selected.Services.DocumentDatabaseInstances, selected.Services.DocumentDatabaseManagementUI = m.Services.DocumentDatabase, m.Services.DocumentDatabaseInstances, m.Services.DocumentDatabaseManagementUI
	default:
		return nil, fmt.Errorf("provider %s is not rendered by the native application backend", instance.Provider.Kind)
	}
	return NativeBackendResources(selected, files)
}
