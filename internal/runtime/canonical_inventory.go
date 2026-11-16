package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ExistingCanonicalProjectInventory derives native resources only AFTER an
// exact match with the provider's current canonical renderer. Arbitrary Compose
// content, labels or names never establish an independent ownership authority.
func ExistingCanonicalProjectInventory(project, compose, canonical string, protectedFiles ...string) ([]ProjectResource, error) {
	if strings.TrimSpace(project) == "" || canonical == "" {
		return nil, errors.New("canonical inventory requires an existing project and provider renderer")
	}
	actual, err := readCanonicalOwnerFile(compose)
	if err != nil || !bytes.Equal(actual, []byte(canonical)) {
		return nil, errors.New("provider runtime definition differs from its protected canonical owner")
	}
	for _, path := range protectedFiles {
		if _, err := readCanonicalOwnerFile(path); err != nil {
			return nil, errors.New("provider recovery owner cannot be verified")
		}
	}
	var model struct {
		Services map[string]struct{} `yaml:"services"`
		Networks map[string]struct {
			Name     string `yaml:"name"`
			External bool   `yaml:"external"`
		} `yaml:"networks"`
		Volumes map[string]struct {
			Name     string `yaml:"name"`
			External bool   `yaml:"external"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(canonical), &model); err != nil || len(model.Services) == 0 {
		return nil, errors.New("canonical provider renderer has no verifiable services")
	}
	resources := []ProjectResource{}
	for service := range model.Services {
		resources = append(resources, ProjectResource{Kind: "container", Name: fmt.Sprintf("%s-%s-1", project, service)})
	}
	for key, network := range model.Networks {
		if network.External {
			continue
		}
		if network.Name == "" {
			network.Name = project + "_" + key
		}
		resources = append(resources, ProjectResource{Kind: "network", Name: network.Name})
	}
	for key, volume := range model.Volumes {
		if volume.External {
			continue
		}
		if volume.Name == "" {
			volume.Name = project + "_" + key
		}
		resources = append(resources, ProjectResource{Kind: "volume", Name: volume.Name})
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Kind+"/"+resources[i].Name < resources[j].Kind+"/"+resources[j].Name
	})
	return resources, nil
}

func readCanonicalOwnerFile(path string) ([]byte, error) {
	failure := errors.New("canonical provider inventory requires a protected regular owner file")
	for parent := filepath.Dir(path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, failure
		}
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > 8<<20 || !ProtectedStateOwner(path, before) {
		return nil, failure
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, failure
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, failure
	}
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	after, afterErr := os.Lstat(path)
	if err != nil || afterErr != nil || len(data) > 8<<20 || !after.Mode().IsRegular() || !os.SameFile(before, after) || !ProtectedStateOwner(path, after) {
		return nil, failure
	}
	return data, nil
}
