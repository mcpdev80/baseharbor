package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/runtime/remoteprojection"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
	"go.yaml.in/yaml/v3"
)

const remoteApplicationPhasesFile = "application-phases.json"

type remoteApplicationPhases struct {
	Version   int      `json:"version"`
	Intent    string   `json:"intent"`
	Providers []string `json:"providers"`
	Workloads []string `json:"workloads"`
}

// NewRemoteApplicationRuntime binds providers and prebuilt repository workloads
// into one immutable publication. No source build can run on a selected node.
func NewRemoteApplicationRuntime(transport targetsession.ProjectTransport, scope targetenrollment.Scope, files RuntimeFiles, manifest Manifest, workload *WorkloadFiles, environment map[string]string) (*RemoteManagedRuntime, error) {
	projection, err := ProjectManagedRuntime(files, manifest)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := yaml.Unmarshal(projection.Files[0].Data, &document); err != nil {
		return nil, err
	}
	services, ok := document["services"].(map[string]any)
	if !ok {
		return nil, errors.New("managed Application services are unavailable")
	}
	phases := remoteApplicationPhases{Version: 1, Intent: remoteApplicationIntent(manifest)}
	for name := range services {
		phases.Providers = append(phases.Providers, name)
	}
	if workload != nil {
		if err := appendRemoteWorkload(&projection, document, services, &phases, files, manifest, *workload, environment); err != nil {
			return nil, err
		}
	}
	sort.Strings(phases.Providers)
	sort.Strings(phases.Workloads)
	compiled, err := yaml.Marshal(document)
	if err != nil {
		return nil, err
	}
	projection.Files[0].Data = compiled
	metadata, err := json.Marshal(phases)
	if err != nil {
		return nil, err
	}
	projection.Files = append(projection.Files, RuntimeProjectionFile{Path: remoteApplicationPhasesFile, Data: metadata, Mode: 0600})
	return newRemoteProjectedRuntime(transport, scope, projection)
}

func remoteApplicationIntent(manifest Manifest) string {
	data, _ := json.Marshal(manifest)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func appendRemoteWorkload(projection *ManagedRuntimeProjection, document, services map[string]any, phases *remoteApplicationPhases, files RuntimeFiles, manifest Manifest, workload WorkloadFiles, environment map[string]string) error {
	if workload.Project != files.Project || !filepath.IsAbs(workload.RepositoryRoot) || len(workload.Services) == 0 {
		return errors.New("remote workload differs from selected Application project")
	}
	paths := []string{workload.Compose}
	if workload.Override != "" {
		paths = append(paths, workload.Override)
	}
	rendered, err := remoteprojection.RenderComposeJSON(paths, environment)
	if err != nil {
		return err
	}
	report, err := AnalyzeRenderedComposeSecurity(manifest, []byte(rendered))
	if err != nil {
		return err
	}
	if err := report.Error(); err != nil {
		return err
	}
	var source map[string]any
	if err := json.Unmarshal([]byte(rendered), &source); err != nil {
		return err
	}
	definitions, ok := source["services"].(map[string]any)
	if !ok {
		return errors.New("remote workload services are unavailable")
	}
	seen := map[string]bool{}
	for _, name := range workload.Services {
		definition, ok := definitions[name].(map[string]any)
		if !ok || services[name] != nil || seen[name] {
			return errors.New("remote workload selection overlaps protected providers")
		}
		seen[name] = true
		if definition["build"] != nil || definition["extends"] != nil || definition["env_file"] != nil || definition["configs"] != nil || definition["secrets"] != nil || definition["container_name"] != nil {
			return errors.New("remote workloads require resolved prebuilt images and confined read-only file bindings")
		}
		image, _ := definition["image"].(string)
		if strings.TrimSpace(image) == "" {
			return errors.New("remote workload has no prebuilt image")
		}
		user, _ := definition["user"].(string)
		uid, _, _ := strings.Cut(user, ":")
		id, err := strconv.ParseUint(uid, 10, 16)
		if err != nil || id == 0 || id > 65000 {
			return errors.New("remote workload requires an explicit non-root UID between 1 and 65000")
		}
		for _, field := range []string{"network_mode", "pid", "ipc"} {
			if definition[field] != nil {
				return errors.New("remote workload namespace overrides are unsupported")
			}
		}
		if definition["privileged"] == true {
			return errors.New("remote privileged workloads are unsupported")
		}
		if definition["labels"] != nil {
			labels, ok := definition["labels"].(map[string]any)
			if !ok {
				return errors.New("remote workload labels require explicit key-value ownership checks")
			}
			for label := range labels {
				if strings.HasPrefix(label, "com.docker.compose.") || strings.HasPrefix(label, "io.podman.compose.") {
					return errors.New("remote workload cannot substitute Core ownership labels")
				}
			}
		}
		if err := projectRemoteWorkloadMounts(projection, definition, files.Dir, workload.RepositoryRoot); err != nil {
			return err
		}
		services[name] = escapeRemoteComposeDollars(definition)
		phases.Workloads = append(phases.Workloads, name)
	}
	// Compose dependencies must not reach unselected repository infrastructure.
	for _, name := range phases.Workloads {
		definition := services[name].(map[string]any)
		switch dependencies := definition["depends_on"].(type) {
		case map[string]any:
			for dependency := range dependencies {
				if services[dependency] == nil {
					return errors.New("remote workload has an unpublished dependency")
				}
			}
		case []any:
			for _, dependency := range dependencies {
				if services[fmt.Sprint(dependency)] == nil {
					return errors.New("remote workload has an unpublished dependency")
				}
			}
		}
	}
	for _, section := range []string{"networks", "volumes"} {
		incoming, _ := source[section].(map[string]any)
		existing, _ := document[section].(map[string]any)
		if existing == nil {
			existing = map[string]any{}
			document[section] = existing
		}
		for name, definition := range incoming {
			if existing[name] != nil {
				return errors.New("remote workload resource overlaps protected provider definition")
			}
			if section == "networks" && name == "baseharbor-backend" {
				config, ok := definition.(map[string]any)
				if !ok || config["external"] != true || config["name"] != ApplicationBackendNetworkNameForProject(files.ResourceProject) {
					return errors.New("remote workload backend network differs")
				}
				if existing["default"] == nil {
					return errors.New("remote provider default network is unavailable")
				}
				for _, service := range phases.Workloads {
					rewriteRemoteBackendNetwork(services[service].(map[string]any))
				}
				continue
			} else if config, ok := definition.(map[string]any); ok {
				if config["external"] == true {
					return errors.New("remote workload external resources are outside the protected Application publication")
				}
				if config["name"] != nil && !(section == "networks" && name == "baseharbor-dev-workload" && config["name"] == DevelopmentWorkloadNetworkNameForProject(files.ResourceProject)) {
					return errors.New("remote workload cannot substitute native resource names")
				}
			}
			existing[name] = definition
		}
	}
	return nil
}

func rewriteRemoteBackendNetwork(service map[string]any) {
	switch networks := service["networks"].(type) {
	case map[string]any:
		if definition, ok := networks["baseharbor-backend"]; ok {
			networks["default"] = definition
			delete(networks, "baseharbor-backend")
		}
	case []any:
		for i, name := range networks {
			if name == "baseharbor-backend" {
				networks[i] = "default"
			}
		}
	}
}

func projectRemoteWorkloadMounts(projection *ManagedRuntimeProjection, definition map[string]any, coreRoot, repositoryRoot string) error {
	mounts, _ := definition["volumes"].([]any)
	var projected []any
	for _, entry := range mounts {
		mount, ok := entry.(string)
		if !ok {
			return errors.New("remote workload requires explicit short-syntax read-only file bindings")
		}
		parts := strings.Split(mount, ":")
		if len(parts) < 2 {
			return errors.New("remote workload anonymous volumes are unsupported")
		}
		if !strings.Contains(parts[0], "/") && !strings.HasPrefix(parts[0], ".") {
			projected = append(projected, entry)
			continue
		}
		if len(parts) != 3 || parts[2] != "ro" {
			return errors.New("remote workload file binding must be read-only")
		}
		path := parts[0]
		if !filepath.IsAbs(path) {
			path = filepath.Join(repositoryRoot, path)
		}
		var relative, selectedRoot string
		for _, root := range []string{coreRoot, repositoryRoot} {
			candidate, err := filepath.Rel(root, path)
			if err == nil && candidate != "." && candidate != ".." && !strings.HasPrefix(candidate, "../") {
				relative, selectedRoot = filepath.ToSlash(candidate), root
				break
			}
		}
		if selectedRoot == coreRoot && !remoteCoreWorkloadBindingAllowed(projection, relative) {
			return errors.New("remote workload binding is not an authorized Core workload projection")
		}
		if selectedRoot == "" {
			return errors.New("remote workload binding escapes protected Application source")
		}
		members, err := readRemoteWorkloadBinding(selectedRoot, relative)
		if err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(path))
		prefix := "workload-files/" + hex.EncodeToString(digest[:])
		for _, member := range members {
			name, destination := prefix, parts[1]
			if member.Path != "" {
				name += "/" + member.Path
				destination += "/" + member.Path
			}
			found := false
			for _, file := range projection.Files {
				if file.Path == name {
					found = true
				}
			}
			if !found {
				projection.Files = append(projection.Files, RuntimeProjectionFile{Path: name, Data: member.Data, Mode: member.Mode})
			}
			projected = append(projected, "./"+name+":"+destination+":ro")
		}
	}
	definition["volumes"] = projected
	return nil
}

func remoteCoreWorkloadBindingAllowed(projection *ManagedRuntimeProjection, relative string) bool {
	if relative == workloadServiceBindingDirName || strings.HasPrefix(relative, workloadServiceBindingDirName+"/") {
		return true
	}
	if !strings.HasPrefix(relative, "bindings/") || !strings.HasSuffix(relative, "/ca.pem") {
		return false
	}
	for _, file := range projection.Files {
		if file.Path == relative {
			return true
		}
	}
	return false
}

// Directory bindings become individual file mounts. This preserves native
// readable file modes without exposing a Node-owned 0700 staging directory to
// the workload UID or broadening its permissions.
func readRemoteWorkloadBinding(root, relative string) ([]RuntimeProjectionFile, error) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	info, err := directory.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		data, mode, err := readRuntimeProjectionFileMode(directory, relative, true)
		if err != nil {
			return nil, err
		}
		return []RuntimeProjectionFile{{Data: data, Mode: mode}}, nil
	}
	var members []RuntimeProjectionFile
	total := 0
	var walk func(string, string, int) error
	walk = func(name, suffix string, depth int) error {
		if depth > 16 {
			return errors.New("remote workload binding exceeds directory depth")
		}
		info, err := directory.Lstat(name)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("remote workload binding contains a symbolic link")
		}
		if !info.IsDir() {
			data, mode, err := readRuntimeProjectionFileMode(directory, name, true)
			if err != nil {
				return err
			}
			total += len(data)
			if len(members) >= 128 || total > 4<<20 {
				return errors.New("remote workload binding exceeds publication limits")
			}
			members = append(members, RuntimeProjectionFile{Path: suffix, Data: data, Mode: mode})
			return nil
		}
		file, err := directory.Open(name)
		if err != nil {
			return err
		}
		entries, err := file.ReadDir(129)
		file.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(entries) > 128 {
			return errors.New("remote workload binding exceeds publication limits")
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			child := entry.Name()
			sub := child
			if suffix != "" {
				sub = suffix + "/" + child
			}
			if err := walk(name+"/"+child, sub, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(relative, "", 0); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, errors.New("remote workload binding directory has no regular files")
	}
	return members, nil
}

func escapeRemoteComposeDollars(value any) any {
	switch item := value.(type) {
	case string:
		return strings.ReplaceAll(item, "$", "$$")
	case map[string]any:
		for key, child := range item {
			item[key] = escapeRemoteComposeDollars(child)
		}
	case []any:
		for index, child := range item {
			item[index] = escapeRemoteComposeDollars(child)
		}
	}
	return value
}
