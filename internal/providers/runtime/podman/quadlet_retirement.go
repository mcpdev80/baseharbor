package podman

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type retiredQuadletDefinition struct {
	path   string
	owner  os.FileInfo
	digest [32]byte
}

func plannedQuadletRetirement(dir, project string, resources []ProjectResource) ([]retiredQuadletDefinition, error) {
	definitions := []retiredQuadletDefinition{}
	for _, resource := range resources {
		if resource.Kind != "container" || !strings.HasPrefix(resource.Name, project+"-") || !strings.HasSuffix(resource.Name, "-1") {
			return nil, errors.New("Quadlet retirement requires canonical selected container names")
		}
		service := strings.TrimSuffix(strings.TrimPrefix(resource.Name, project+"-"), "-1")
		if service == "" || sanitizeQuadletName(service) != service || sanitizeQuadletName(project) != project {
			return nil, errors.New("Quadlet retirement has ambiguous native unit identity")
		}
		path := filepath.Join(dir, project+"-"+service+".container")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || info.Size() > 1<<20 {
			return nil, errors.New("Quadlet retirement requires an unmodified regular owner definition")
		}
		parent, err := os.Lstat(dir)
		if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm()&0022 != 0 {
			return nil, errors.New("Quadlet retirement has an ambiguous unit directory owner")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) {
			file.Close()
			return nil, errors.New("Quadlet definition owner changed")
		}
		data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		file.Close()
		if err != nil || len(data) > 1<<20 {
			return nil, errors.New("Quadlet definition cannot be verified")
		}
		content := string(data)
		for _, line := range []string{"Description=BaseHarbor Quadlet service " + project + "/" + service, "ContainerName=" + project + "-" + service, "Label=com.docker.compose.project=" + project, "Label=com.docker.compose.service=" + service, "Label=io.podman.compose.project=" + project, "Label=io.podman.compose.service=" + service} {
			prefix := strings.SplitN(line, "=", 2)[0] + "="
			if strings.HasPrefix(line, "Label=") {
				prefix = strings.Join(strings.SplitN(line, "=", 3)[:2], "=") + "="
			}
			if !strings.Contains("\n"+content, "\n"+line+"\n") || strings.Count("\n"+content, "\n"+prefix) != 1 {
				return nil, errors.New("Quadlet definition differs from selected container owner")
			}
		}
		definitions = append(definitions, retiredQuadletDefinition{path: path, owner: info, digest: sha256.Sum256(data)})
	}
	return definitions, nil
}

func removeRetiredQuadletDefinitions(ctx context.Context, definitions []retiredQuadletDefinition) error {
	if len(definitions) == 0 {
		return nil
	}
	units := []string{}
	for _, definition := range definitions {
		if err := verifyRetiredQuadletDefinition(definition); err != nil {
			return err
		}
		units = append(units, quadletUnitForFile(filepath.Base(definition.path)))
	}
	// Also stop units whose container is already absent after an interrupted
	// earlier apply. Native inspection alone cannot observe those boot owners.
	if _, err := quadletSystemctlBlocking(ctx, nil, append([]string{"stop"}, units...)...); err != nil {
		return err
	}
	for _, definition := range definitions {
		if err := verifyRetiredQuadletDefinition(definition); err != nil {
			return err
		}
		if err := os.Remove(definition.path); err != nil {
			return fmt.Errorf("retire owned Quadlet container definition: %w", err)
		}
	}
	_, err := quadletSystemctlBlocking(ctx, nil, "daemon-reload")
	return err
}

func verifyRetiredQuadletDefinition(definition retiredQuadletDefinition) error {
	changed := errors.New("Quadlet retirement owner changed; retain remaining definitions for retry")
	current, err := os.Lstat(definition.path)
	if err != nil || !current.Mode().IsRegular() || current.Mode().Perm()&0022 != 0 || !os.SameFile(definition.owner, current) || current.Size() > 1<<20 {
		return changed
	}
	file, err := os.Open(definition.path)
	if err != nil {
		return changed
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(definition.owner, opened) {
		return changed
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || sha256.Sum256(data) != definition.digest {
		return changed
	}
	after, err := os.Lstat(definition.path)
	if err != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(definition.owner, after) {
		return changed
	}
	return nil
}
