package podman

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

func quadletServiceRegistryImages(project QuadletProject, units []string) []string {
	images := map[string]struct{}{}
	for _, unit := range units {
		content := project.Files[strings.TrimSuffix(unit, ".service")+".container"]
		image := quadletDirectiveValue(content, "Image")
		if image == "" || strings.HasSuffix(image, ".build") || quadletDirectiveValue(content, "Pull") == "never" {
			continue
		}
		images[image] = struct{}{}
	}
	result := make([]string, 0, len(images))
	for image := range images {
		result = append(result, image)
	}
	sort.Strings(result)
	return result
}

func quadletPrepareServiceImages(ctx context.Context, project QuadletProject, units []string) error {
	images := quadletServiceRegistryImages(project, units)
	if len(images) == 0 {
		return nil
	}
	path, err := exec.LookPath("podman")
	if err != nil {
		return err
	}
	for _, image := range images {
		check := exec.CommandContext(ctx, path, "image", "exists", image)
		check.Env = runtimeCommandEnv(path)
		if err := check.Run(); err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				return fmt.Errorf("inspect Quadlet image %s: %w", image, err)
			}
		} else {
			continue
		}
		pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		pull := exec.CommandContext(pullCtx, path, "pull", image)
		pull.Env = runtimeCommandEnv(path)
		output, err := pull.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("prepare Quadlet image %s: %w: %s", image, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}
