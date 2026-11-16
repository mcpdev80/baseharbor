package application

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type RuntimeProjectionFile struct {
	Path string
	Data []byte
	Mode uint32
}

// ManagedRuntimeProjection contains only the Core-generated backend project
// and its referenced protected files. Source builds, workload repositories and
// the installation's manager credentials are outside this projection.
type ManagedRuntimeProjection struct {
	Project string
	Compose string
	Env     string
	Files   []RuntimeProjectionFile
}

var remoteRuntimeProjectName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

func ProjectManagedRuntime(files RuntimeFiles, m Manifest) (ManagedRuntimeProjection, error) {
	if err := m.Validate(); err != nil {
		return ManagedRuntimeProjection{}, err
	}
	if !remoteRuntimeProjectName.MatchString(files.Project) || !filepath.IsAbs(files.Dir) ||
		filepath.Clean(files.Compose) != filepath.Join(files.Dir, "compose.yaml") ||
		filepath.Clean(files.Env) != filepath.Join(files.Dir, "runtime.env") {
		return ManagedRuntimeProjection{}, errors.New("invalid Core runtime projection selection")
	}
	info, err := os.Lstat(files.Dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return ManagedRuntimeProjection{}, errors.New("Core runtime projection directory must be protected")
	}
	root, err := os.OpenRoot(files.Dir)
	if err != nil {
		return ManagedRuntimeProjection{}, errors.New("protected Core runtime directory is unavailable")
	}
	defer root.Close()
	compose, composeMode, err := readRuntimeProjectionFile(root, "compose.yaml")
	if err != nil {
		return ManagedRuntimeProjection{}, err
	}
	resourceProject := files.ResourceProject
	if resourceProject == "" {
		resourceProject = RuntimeProjectName(m)
	}
	if !remoteRuntimeProjectName.MatchString(resourceProject) {
		return ManagedRuntimeProjection{}, errors.New("invalid Core resource project identity")
	}
	expected, err := RuntimeComposeYAMLForProject(m, resourceProject)
	if err != nil {
		return ManagedRuntimeProjection{}, err
	}
	if composeMode != 0600 || !bytes.Equal(compose, []byte(expected)) {
		return ManagedRuntimeProjection{}, ErrRuntimeDefinitionChanged
	}
	var document struct {
		Services map[string]struct {
			Volumes []string `yaml:"volumes"`
		} `yaml:"services"`
		Secrets map[string]struct {
			File string `yaml:"file"`
		} `yaml:"secrets"`
	}
	if yaml.Unmarshal(compose, &document) != nil {
		return ManagedRuntimeProjection{}, errors.New("generated runtime projection is invalid")
	}
	references := map[string]bool{"runtime.env": true}
	for _, service := range document.Services {
		for _, mount := range service.Volumes {
			source, _, found := strings.Cut(mount, ":")
			if !found || !strings.Contains(source, "/") {
				continue // Named volumes contain no transferred file material.
			}
			references[strings.TrimPrefix(source, "./")] = true
		}
	}
	for _, secret := range document.Secrets {
		if secret.File != "" {
			references[strings.TrimPrefix(secret.File, "./")] = true
		}
	}
	if len(references)+1 > 128 {
		return ManagedRuntimeProjection{}, errors.New("runtime projection exceeds file limit")
	}
	projection := ManagedRuntimeProjection{Project: files.Project, Compose: "compose.yaml", Env: "runtime.env"}
	// Native Compose receives the authoritative project identity in generated
	// data. Its project name must never derive from a random staging directory.
	compiled := append([]byte("name: "+files.Project+"\n"), compose...)
	projection.Files = append(projection.Files, RuntimeProjectionFile{Path: "compose.yaml", Data: compiled, Mode: 0600})
	total := len(compiled)
	names := make([]string, 0, len(references))
	for name := range references {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data, mode, err := readRuntimeProjectionFile(root, name)
		if err != nil {
			return ManagedRuntimeProjection{}, err
		}
		if name == "runtime.env" && mode != 0600 {
			return ManagedRuntimeProjection{}, errors.New("runtime environment must remain owner-only")
		}
		total += len(data)
		if total > 4<<20 {
			return ManagedRuntimeProjection{}, errors.New("runtime projection exceeds byte limit")
		}
		projection.Files = append(projection.Files, RuntimeProjectionFile{Path: name, Data: data, Mode: mode})
	}
	return projection, nil
}

func readRuntimeProjectionFile(root *os.Root, name string) ([]byte, uint32, error) {
	return readRuntimeProjectionFileMode(root, name, false)
}

func readRuntimeProjectionFileMode(root *os.Root, name string, readonlyBinding bool) ([]byte, uint32, error) {
	if name == "" || name == "." || path.Clean(name) != name || strings.HasPrefix(name, "/") ||
		name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00\r\n") {
		return nil, 0, errors.New("runtime projection requires confined relative files")
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) ||
			(i == len(parts)-1 && !info.Mode().IsRegular()) {
			return nil, 0, errors.New("runtime projection member is not a protected regular file")
		}
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, 0, errors.New("runtime projection member is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || (info.Mode().Perm() != 0600 && info.Mode().Perm() != 0644 && !(readonlyBinding && (info.Mode().Perm() == 0400 || info.Mode().Perm() == 0444))) || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, 0, errors.New("runtime projection file permissions are unsupported")
	}
	data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, 0, errors.New("runtime projection member exceeds byte limit")
	}
	mode := uint32(info.Mode().Perm())
	// Bundle wire modes stay canonical; all projected binding mounts are read-only.
	if readonlyBinding && mode == 0400 {
		mode = 0600
	}
	if readonlyBinding && mode == 0444 {
		mode = 0644
	}
	return data, mode, nil
}
