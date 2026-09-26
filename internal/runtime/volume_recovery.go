package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const recoveryHelperImage = "docker.io/library/alpine:3.22"

func (c Compose) ExportOwnedVolume(ctx context.Context, project, volume string) ([]byte, error) {
	project = strings.TrimSpace(project)
	volume = strings.TrimSpace(volume)
	if project == "" || volume == "" {
		return nil, errors.New("project and volume are required")
	}
	ok, err := c.InspectProjectResource(ctx, project, ProjectResource{Kind: "volume", Name: volume})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("owned volume %s was not found", volume)
	}
	return c.directBinary(ctx, nil, "run", "--rm", "-v", volume+":/data:ro", recoveryHelperImage, "tar", "-C", "/data", "-cf", "-", ".")
}

func (c Compose) RestoreOwnedVolume(ctx context.Context, project, volume string, archive []byte) error {
	project = strings.TrimSpace(project)
	volume = strings.TrimSpace(volume)
	if project == "" || volume == "" {
		return errors.New("project and volume are required")
	}
	ok, err := c.InspectProjectResource(ctx, project, ProjectResource{Kind: "volume", Name: volume})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("owned volume %s was not found", volume)
	}
	_, err = c.directBinary(ctx, archive, "run", "--rm", "-i", "-v", volume+":/data", recoveryHelperImage, "sh", "-ceu", "find /data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +; tar -C /data -xf -")
	if err != nil {
		return fmt.Errorf("restore owned volume %s: %w", volume, err)
	}
	return nil
}

func (c Compose) directBinary(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	if strings.TrimSpace(c.command) == "" {
		return nil, ErrRuntimeNotFound
	}
	cmd := exec.CommandContext(ctx, c.command, args...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("%s %s failed: %s", filepathBase(c.command), strings.Join(args, " "), detail)
	}
	return stdout.Bytes(), nil
}

func filepathBase(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return value
	}
	return parts[len(parts)-1]
}
