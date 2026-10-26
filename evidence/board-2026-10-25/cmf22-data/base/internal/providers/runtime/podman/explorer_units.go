package podman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type explorerUnit struct {
	project, service, path, id string
	definition                 retiredQuadletDefinition
}

// Existing Quadlet files are the stable native owner. A systemd stop can remove
// a container ID; it must not erase the identity needed for an authorized start.
func explorerUnits() ([]explorerUnit, error) {
	dir, err := quadletUserUnitDir()
	if err != nil {
		return nil, err
	}
	for ancestor := dir; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if errors.Is(err, os.ErrNotExist) && ancestor == dir {
			return nil, nil
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrResourceOwnership
		}
		if ancestor == filepath.Dir(ancestor) {
			break
		}
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	units := []explorerUnit{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".container") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, ErrResourceOwnership
		}
		if !info.Mode().IsRegular() || info.Size() > 1<<20 {
			continue // Unreadable foreign definitions cannot establish an owner.
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		opened, statErr := file.Stat()
		if statErr != nil || !os.SameFile(info, opened) {
			file.Close()
			return nil, ErrResourceOwnership
		}
		data, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		file.Close()
		if readErr != nil || len(data) > 1<<20 {
			return nil, ErrResourceOwnership
		}
		prefix := "Description=BaseHarbor Quadlet service "
		var identity string
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, prefix) {
				identity = strings.TrimPrefix(line, prefix)
			}
		}
		if identity == "" {
			continue
		} // Foreign user units are not adopted.
		owner, ok := opened.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != uint32(os.Getuid()) || owner.Nlink != 1 {
			return nil, ErrResourceOwnership
		}
		parts := strings.Split(identity, "/")
		if len(parts) != 2 {
			return nil, ErrResourceOwnership
		}
		project, service := parts[0], parts[1]
		definitions, err := plannedQuadletRetirement(dir, project, []ProjectResource{{Kind: "container", Name: project + "-" + service + "-1"}})
		if err != nil || len(definitions) != 1 || definitions[0].path != path || definitions[0].digest != sha256.Sum256(data) || !os.SameFile(info, definitions[0].owner) {
			return nil, ErrResourceOwnership
		}
		units = append(units, explorerUnit{project: project, service: service, path: path, id: "quadlet:" + strings.TrimSuffix(entry.Name(), ".container"), definition: definitions[0]})
	}
	return units, nil
}

func (p Provider) ExplorerRuntimeContainers(ctx context.Context) ([]RuntimeContainer, error) {
	actual, err := p.Compose.ListRuntimeContainers(ctx)
	if err != nil {
		return nil, err
	}
	units, err := explorerUnits()
	if err != nil {
		return nil, err
	}
	result := []RuntimeContainer{}
	used := map[string]bool{}
	for _, unit := range units {
		resource := ProjectResource{Kind: "container", Name: unit.project + "-" + unit.service + "-1"}
		var observed *RuntimeContainer
		for index := range actual {
			if bhruntime.ContainerMatchesProjectResource(ProviderPodman, actual[index], unit.project, resource) {
				if observed != nil {
					return nil, ErrResourceOwnership
				}
				observed = &actual[index]
			}
		}
		if observed != nil {
			copy := *observed
			used[copy.ID] = true
			copy.ID = unit.id
			result = append(result, copy)
			continue
		}
		state, err := quadletSystemctlBlocking(ctx, nil, "show", "--property=ActiveState", "--value", quadletUnitForFile(filepath.Base(unit.path)))
		if err != nil {
			return nil, err
		}
		state = strings.TrimSpace(state)
		if state != "inactive" && state != "failed" {
			return nil, errors.New("Quadlet unit has no verifiable matching native container")
		}
		result = append(result, RuntimeContainer{ID: unit.id, Name: unit.project + "-" + unit.service, Project: unit.project, Service: unit.service, State: "stopped"})
	}
	for _, container := range actual {
		if !used[container.ID] {
			result = append(result, container)
		}
	}
	return result, nil
}

func (p Provider) ExplorerContainerNativeID(ctx context.Context, id string) (string, error) {
	if !strings.HasPrefix(id, "quadlet:") {
		return id, nil
	}
	units, err := explorerUnits()
	if err != nil {
		return "", err
	}
	containers, err := p.Compose.ListRuntimeContainers(ctx)
	if err != nil {
		return "", err
	}
	for _, unit := range units {
		if unit.id == id {
			for _, container := range containers {
				if bhruntime.ContainerMatchesProjectResource(ProviderPodman, container, unit.project, ProjectResource{Kind: "container", Name: unit.project + "-" + unit.service + "-1"}) {
					return container.ID, nil
				}
			}
		}
	}
	return "", errors.New("selected Quadlet owner has no current native container")
}

func (p Provider) ExplorerOperateContainer(ctx context.Context, id, operation string) (string, error) {
	if operation != "start" && operation != "stop" && operation != "restart" {
		return "", errors.New("unsupported managed Quadlet operation")
	}
	units, err := explorerUnits()
	if err != nil {
		return "", err
	}
	for _, unit := range units {
		if unit.id == id {
			if err := verifyRetiredQuadletDefinition(unit.definition); err != nil {
				return "", err
			}
			return quadletSystemctlBlocking(ctx, nil, operation, quadletUnitForFile(filepath.Base(unit.path)))
		}
	}
	return "", ErrResourceOwnership
}

func (p Provider) ExplorerContainerEvidence(ctx context.Context, id string) (map[string]string, error) {
	if !strings.HasPrefix(id, "quadlet:") {
		return nil, nil
	}
	units, err := explorerUnits()
	if err != nil {
		return nil, err
	}
	for _, unit := range units {
		if unit.id == id {
			return map[string]string{"runtime.quadlet.owner": unit.path, "runtime.quadlet.revision": hex.EncodeToString(unit.definition.digest[:])}, nil
		}
	}
	return nil, ErrResourceOwnership
}
