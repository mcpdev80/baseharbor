package buildkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/artifact"
)

// Builder uses an external buildctl client and BuildKit daemon. The deployment
// owns daemon access and registry credentials; the portable application
// contract only contributes repository source/build semantics.
type Builder struct {
	Command string
	Address string
}

type metadata struct {
	Digest string `json:"containerimage.digest"`
}

func (b Builder) Build(ctx context.Context, request artifact.BuildRequest, destination string) (artifact.Artifact, error) {
	command := strings.TrimSpace(b.Command)
	if command == "" {
		command = "buildctl"
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return artifact.Artifact{}, fmt.Errorf("BuildKit client %q is unavailable: %w", command, err)
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return artifact.Artifact{}, errors.New("OCI artifact destination is required")
	}
	if strings.Contains(destination, "@") {
		return artifact.Artifact{}, fmt.Errorf("OCI artifact build destination must be tag/repository based before digest resolution: %s", destination)
	}

	metadataFile, err := os.CreateTemp("", "baseharbor-buildkit-metadata-*.json")
	if err != nil {
		return artifact.Artifact{}, err
	}
	metadataPath := metadataFile.Name()
	if err := metadataFile.Close(); err != nil {
		return artifact.Artifact{}, err
	}
	defer os.Remove(metadataPath)

	args := []string{}
	if address := strings.TrimSpace(b.Address); address != "" {
		args = append(args, "--addr", address)
	}
	args = append(args,
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context="+request.Context,
		"--local", "dockerfile="+request.Context,
		"--opt", "filename="+normalizedDockerfile(request.Dockerfile),
		"--output", "type=image,name="+destination+",push=true,oci-mediatypes=true,name-canonical=true",
		"--metadata-file", metadataPath,
		"--progress", "plain",
	)

	cmd := exec.CommandContext(ctx, path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return artifact.Artifact{}, fmt.Errorf("BuildKit build failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return artifact.Artifact{}, fmt.Errorf("read BuildKit metadata: %w", err)
	}
	var result metadata
	if err := json.Unmarshal(data, &result); err != nil {
		return artifact.Artifact{}, fmt.Errorf("decode BuildKit metadata: %w", err)
	}
	result.Digest = strings.TrimSpace(result.Digest)
	if !strings.HasPrefix(result.Digest, "sha256:") {
		return artifact.Artifact{}, fmt.Errorf("BuildKit did not return an OCI image digest")
	}

	return artifact.Artifact{
		Service:   request.Service,
		Reference: destination + "@" + result.Digest,
		Digest:    result.Digest,
	}, nil
}

func normalizedDockerfile(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Dockerfile"
	}
	return filepath.ToSlash(filepath.Clean(value))
}
