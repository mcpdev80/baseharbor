package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	controlplaneassets "github.com/mcpdev80/baseharbor/deploy/control-plane"
	"github.com/mcpdev80/baseharbor/internal/cli"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var developmentRuntimeExecutable = os.Executable

// Development CLIs cannot consume an unrelated moving edge build. Package the
// installed binary with the canonical glibc-compatible runtime recipe on the
// owning runtime; local Go builds may use CGO. Explicit image overrides and
// release images retain their existing behavior.
func ensureMatchingDevelopmentRuntimeImage(ctx context.Context, provider bhruntime.RuntimeProvider, out io.Writer) (bool, error) {
	requested := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"))
	managed := regexp.MustCompile(`^localhost/baseharbor-runtime:dev-[0-9a-f]{64}$`).MatchString(requested)
	if defaultRuntimeImage(version) != "ghcr.io/mcpdev80/baseharbor-runtime:edge" ||
		!regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(commit) ||
		(requested != "" && requested != defaultRuntimeImage(version) && !managed) {
		return false, nil
	}
	if runtime.GOOS != "linux" {
		return true, errors.New("a development Core requires a matching Linux runtime image; set BASEHARBOR_RUNTIME_IMAGE to the exact candidate image")
	}
	path, err := developmentRuntimeExecutable()
	if err != nil {
		return true, err
	}
	binary, err := os.ReadFile(path)
	if err != nil {
		return true, fmt.Errorf("read installed runtime binary: %w", err)
	}
	sum := sha256.Sum256(binary)
	image := "localhost/baseharbor-runtime:dev-" + hex.EncodeToString(sum[:])
	buildDir, err := os.MkdirTemp("", "baseharbor-runtime-image-")
	if err != nil {
		return true, err
	}
	defer os.RemoveAll(buildDir)
	if err := os.WriteFile(filepath.Join(buildDir, "baha"), binary, 0700); err != nil {
		return true, err
	}
	if err := os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(controlplaneassets.BinaryDockerfile), 0600); err != nil {
		return true, err
	}
	compose := filepath.Join(buildDir, "compose.yaml")
	definition := "services:\n  runtime-image:\n    image: " + image + "\n    build:\n      context: .\n      dockerfile: Dockerfile\n"
	if err := os.WriteFile(compose, []byte(definition), 0600); err != nil {
		return true, err
	}
	cli.ReportActivityDetail(out, "Preparing runtime image from the installed Core binary (cached build)")
	if err := provider.BuildProjectFilesSelectedProgress(ctx, "baseharbor-runtime-image", buildDir, nil, []string{"runtime-image"}, func(detail string) { cli.ReportActivityDetail(out, detail) }, compose); err != nil {
		return true, fmt.Errorf("build runtime image matching the installed Core: %w", err)
	}
	// Change selection only after a successful build. No registry tag is rewritten.
	if err := os.Setenv("BASEHARBOR_RUNTIME_IMAGE", image); err != nil {
		return true, err
	}
	return true, nil
}
