package application

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	if err != nil || !sharedBackendComposeMatchesCanonical(actual, []byte(canonical)) {
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

// sharedBackendComposeMatchesCanonical accepts the exact previous renderer
// shape only when its sole differences are the fixed Compose user overrides
// removed for arbitrary-UID platform compatibility. Any other drift remains
// fail-closed. The next reconciliation writes the current canonical document.
func sharedBackendComposeMatchesCanonical(actual, canonical []byte) bool {
	if bytes.Equal(actual, canonical) {
		return true
	}
	var observed map[string]any
	if yaml.Unmarshal(actual, &observed) != nil {
		return false
	}
	services, ok := observed["services"].(map[string]any)
	if !ok {
		return false
	}
	changed := false
	legacyValkey := map[string]bool{}
	for name, rawService := range services {
		service, ok := rawService.(map[string]any)
		if !ok {
			return false
		}
		image, _ := service["image"].(string)
		if image == "docker.io/valkey/valkey:9.1.2-alpine" {
			service["image"] = valkeyProviderImage
			service["build"] = map[string]any{"context": "./" + valkeyProviderBuildDir, "dockerfile": "Dockerfile"}
			legacyValkey[name] = true
			changed = true
		}
		if legacyUser, exists := service["user"]; exists {
			want, supported := legacySharedBackendUserForImage(image)
			if !supported || fmt.Sprint(legacyUser) != want {
				return false
			}
			delete(service, "user")
			changed = true
		}
	}
	if !changed {
		return false
	}
	var expected map[string]any
	if yaml.Unmarshal(canonical, &expected) != nil {
		return false
	}
	// Earlier Compose renderers used the same Valkey command without the
	// now-required arbitrary-UID data-permission prelude. Accept that exact
	// historical command only alongside the known upstream image migration.
	expectedServices, ok := expected["services"].(map[string]any)
	if !ok {
		return false
	}
	for name := range legacyValkey {
		observedService, ok := services[name].(map[string]any)
		if !ok {
			return false
		}
		expectedService, ok := expectedServices[name].(map[string]any)
		if !ok {
			return false
		}
		if !reflect.DeepEqual(observedService["command"], expectedService["command"]) {
			legacyCommand, ok := sharedValkeyCommandWithoutPermissionPrelude(expectedService["command"])
			if !ok || !reflect.DeepEqual(observedService["command"], legacyCommand) {
				return false
			}
			observedService["command"] = expectedService["command"]
		}
	}
	return reflect.DeepEqual(observed, expected)
}

func sharedValkeyCommandWithoutPermissionPrelude(command any) (any, bool) {
	parts, ok := command.([]any)
	if !ok || len(parts) != 3 {
		return nil, false
	}
	script, ok := parts[2].(string)
	if !ok {
		return nil, false
	}
	prelude := valkeyDataPermissionPrelude
	if !strings.HasPrefix(script, prelude) {
		return nil, false
	}
	legacy := append([]any(nil), parts...)
	legacyScript := strings.TrimPrefix(script, prelude)
	legacyScript = strings.TrimPrefix(legacyScript, "\n")
	legacy[2] = legacyScript
	return legacy, true
}

func legacySharedBackendUserForImage(image string) (string, bool) {
	switch image {
	case PostgresUIImage:
		return "5050:5050", true
	case CacheUIImage:
		return "redis", true
	case UIProxyImage:
		return "65532:65532", true
	case "docker.io/library/haproxy:3.2.23-alpine":
		return "99:99", true
	case "docker.io/valkey/valkey:9.1.2-alpine":
		return "999:1000", true
	case valkeyProviderImage:
		return "999:1000", true
	default:
		return "", false
	}
}
