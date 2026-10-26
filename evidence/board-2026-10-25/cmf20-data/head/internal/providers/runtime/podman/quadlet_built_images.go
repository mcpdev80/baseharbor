package podman

import (
	"context"
	"fmt"
	"strings"
)

// A .build Image creates a systemd dependency that can rebuild an inactive
// oneshot build unit. Starting without a build must use its completed local
// image directly, while retaining the build source for explicit rebuilds.
func quadletProjectWithBuiltImages(project QuadletProject) (QuadletProject, error) {
	files := make(map[string]string, len(project.Files))
	for name, content := range project.Files {
		files[name] = content
		if !strings.HasSuffix(name, ".container") {
			continue
		}
		image := quadletDirectiveValue(content, "Image")
		if !strings.HasSuffix(image, ".build") {
			continue
		}
		tag := quadletDirectiveValue(project.Files[image], "ImageTag")
		if tag == "" {
			return QuadletProject{}, fmt.Errorf("Quadlet build %s has no local image tag", image)
		}
		content = strings.Replace(content, "Image="+image+"\n", "Image="+tag+"\nPull=never\n", 1)
		files[name] = content
	}
	project.Files = files
	return project, nil
}

func quadletStartProjectNoBuild(ctx context.Context, project QuadletProject, selected []string) error {
	var err error
	project, err = quadletProjectWithBuiltImages(project)
	if err != nil {
		return err
	}
	return quadletStartProjectMode(ctx, project, selected, false)
}
