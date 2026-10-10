package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const recoveryHelperImage = "docker.io/library/alpine:3.22"

// VerifyOwnedVolumeQuiesced checks actual native mounts across all projects.
// Unrelated services may share a project label without sharing this datastore;
// a foreign container mounting the datastore still prevents recovery.
func (c Compose) VerifyOwnedVolumeQuiesced(ctx context.Context, project, volume string) error {
	if project == "" || volume == "" {
		return errors.New("owned volume identity required for quiescence verification")
	}
	owned, err := c.InspectProjectResource(ctx, project, ProjectResource{Kind: "volume", Name: volume})
	if err != nil {
		return err
	}
	if !owned {
		return errors.New("recovery datastore volume is not owned by the selected project")
	}
	listed, err := c.directOutput(ctx, "container", "ls", "-q")
	if err != nil {
		return err
	}
	ids := strings.Fields(listed)
	if len(ids) == 0 {
		return nil
	}
	args := append([]string{"container", "inspect", "--format", `{{json .Mounts}}`}, ids...)
	output, err := c.directOutput(ctx, args...)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != len(ids) {
		return errors.New("incomplete running container mount inventory")
	}
	for _, line := range lines {
		var mounts []struct{ Type, Name string }
		if err := json.Unmarshal([]byte(line), &mounts); err != nil {
			return errors.New("invalid running container mount inventory")
		}
		for _, mount := range mounts {
			if mount.Type == "volume" && mount.Name == "" {
				return errors.New("running volume mount has no native identity")
			}
			if mount.Type == "volume" && mount.Name == volume {
				return fmt.Errorf("owned recovery volume %s still has an active container consumer", volume)
			}
		}
	}
	return nil
}

// SeedOwnedVolume streams a verified native archive into an empty replacement
// volume. It never deletes data and refuses a partial or previously used volume.
// The caller supplies the inventoried, digest-pinned image containing GNU tar.
func (c Compose) SeedOwnedVolume(ctx context.Context, project, volume, image, subdir, owner string, archive io.Reader) error {
	if project == "" || volume == "" || !strings.Contains(image, "@sha256:") || archive == nil ||
		(subdir != "data" && subdir != "pgdata" && subdir != "") {
		return errors.New("invalid owned replacement volume seed")
	}
	for _, part := range strings.Split(owner, ":") {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return errors.New("invalid replacement volume owner")
		}
	}
	owned, err := c.InspectProjectResource(ctx, project, ProjectResource{Kind: "volume", Name: volume})
	if err != nil || !owned {
		return fmt.Errorf("replacement volume ownership unverified: %s: %w", volume, err)
	}
	const script = `test -z "$(find /data -mindepth 1 -maxdepth 1 -print -quit)"
destination=/data
if [ -n "$1" ]; then destination=/data/$1; mkdir "$destination"; fi
tar --no-same-owner -C "$destination" -xf -
chmod 0700 /data "$destination"
chown -R "$2" /data
sync`
	args := []string{"run", "--rm", "-i", "--read-only", "--cap-drop", "ALL",
		"--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--security-opt", "no-new-privileges:true",
		"-v", volume + ":/data", "--entrypoint", "/bin/sh", image, "-ceu", script, "--", subdir, owner}
	cmd := exec.CommandContext(ctx, c.command, append(append([]string{}, c.engineArgs...), args...)...)
	cmd.Env, _ = c.commandEnvironment(nil)
	cmd.Stdin = archive
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("seed empty owned replacement volume %s: %w", volume, err)
	}
	return nil
}

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

func (c Compose) EnsureOwnedVolume(ctx context.Context, project, volume string) error {
	project = strings.TrimSpace(project)
	volume = strings.TrimSpace(volume)
	if project == "" || volume == "" {
		return errors.New("project and volume are required")
	}
	ok, err := c.InspectProjectResource(ctx, project, ProjectResource{Kind: "volume", Name: volume})
	if err == nil && ok {
		return nil
	}
	if err != nil && !errors.Is(err, ErrResourceOwnership) {
		return err
	}
	if errors.Is(err, ErrResourceOwnership) {
		return err
	}
	if _, createErr := c.directBinary(ctx, nil, "volume", "create",
		"--label", "com.docker.compose.project="+project,
		"--label", "io.podman.compose.project="+project,
		volume,
	); createErr != nil {
		return fmt.Errorf("create owned recovery volume %s: %w", volume, createErr)
	}
	ok, err = c.InspectProjectResource(ctx, project, ProjectResource{Kind: "volume", Name: volume})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("created recovery volume %s could not be ownership-verified", volume)
	}
	return nil
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
	cmd := exec.CommandContext(ctx, c.command, append(append([]string{}, c.engineArgs...), args...)...)
	cmd.Env, _ = c.commandEnvironment(nil)
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
