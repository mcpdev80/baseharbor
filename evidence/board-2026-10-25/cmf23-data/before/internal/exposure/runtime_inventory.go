package exposure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RuntimeInventory observes the existing renderer owner. It does not allocate
// ports, rewrite routes, issue certificates or introduce an ownership record.
type RuntimeInventory struct {
	State     State
	Files     Files
	Resources []bhruntime.ProjectResource
}

func ExistingRuntimeInventory(m application.Manifest, runtime application.RuntimeFiles) (RuntimeInventory, error) {
	result := RuntimeInventory{Files: FilesFor(runtime), Resources: []bhruntime.ProjectResource{}}
	for _, path := range []string{runtime.Dir, filepath.Join(runtime.Dir, "providers"), result.Files.Dir} {
		info, err := os.Lstat(path)
		if err != nil {
			return result, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return result, errors.New("exposure inventory requires protected canonical owner directories")
		}
	}
	data, err := readProtectedInventoryFile(result.Files.State)
	if err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result.State) != nil || result.State.Version != stateVersion || result.State.Project != ProjectNameForRuntime(m, runtime) || result.State.Network != application.ApplicationExposureNetworkNameForProject(runtime.ResourceProject) || len(result.State.Routes) == 0 {
		return result, errors.New("exposure state does not match its existing application runtime owner")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return result, errors.New("exposure state has trailing data")
	}
	seen := map[string]bool{}
	for _, route := range result.State.Routes {
		if !regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`).MatchString(route.Name) || len(route.Name) > 63 || seen[route.Name] || route.Service == "" || route.TargetPort < 1 || route.TargetPort > 65535 || route.PublishedPort < 1 || route.PublishedPort > 65535 || (route.Protocol != "http" && route.Protocol != "https") || (route.Visibility != "public" && route.Visibility != "internal") {
			return result, errors.New("exposure runtime route ownership is invalid or duplicated")
		}
		seen[route.Name] = true
		result.Resources = append(result.Resources, bhruntime.ProjectResource{Kind: "container", Name: result.State.Project + "-baseharbor-internal-exposure-" + route.Name + "-1"})
	}
	compose, err := readProtectedInventoryFile(result.Files.Compose)
	if err != nil {
		return result, err
	}
	if !bytes.Equal(compose, []byte(composeYAML(result.State, result.Files))) {
		return result, errors.New("exposure runtime definition differs from its canonical owner shape")
	}
	env, err := readProtectedInventoryFile(result.Files.Env)
	if err != nil {
		return result, err
	}
	if !bytes.Equal(env, []byte("# BaseHarbor managed Caddy exposure provider\n")) {
		return result, errors.New("exposure runtime environment differs from its canonical owner shape")
	}
	return result, nil
}

func readProtectedInventoryFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return nil, errors.New("exposure inventory file is unprotected, noncanonical or oversized")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("exposure inventory file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	fresh, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, fresh) || fresh.Size() != info.Size() || !fresh.ModTime().Equal(info.ModTime()) || len(data) > 1<<20 {
		return nil, errors.New("exposure inventory file changed while reading")
	}
	return data, nil
}

// RetireRuntime preserves the canonical route/configuration owner for retry
// and re-add. The application exposure network belongs to the workload owner.
func RetireRuntime(ctx context.Context, provider bhruntime.RuntimeProvider, m application.Manifest, runtime application.RuntimeFiles) (bhruntime.RuntimeRetirement, error) {
	result := bhruntime.RuntimeRetirement{Removed: []bhruntime.ProjectResource{}, Retained: []bhruntime.RetainedRuntimeResource{}}
	inventory, err := ExistingRuntimeInventory(m, runtime)
	if err != nil {
		return result, err
	}
	native, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
	if !ok {
		return result, errors.New("runtime has no data-preserving exposure retirement adapter")
	}
	result, err = bhruntime.RetireOwnedServices(ctx, native, inventory.State.Project, inventory.Resources, inventory.Resources)
	result.Retained = append(result.Retained, bhruntime.RetainedRuntimeResource{Resource: bhruntime.ProjectResource{Kind: "file", Name: inventory.Files.Dir}, Reason: "existing exposure route/configuration and certificate material retained for retry and recovery; data deletion requires separate explicit consent"})
	return result, err
}
