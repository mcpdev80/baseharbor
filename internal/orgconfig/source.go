package orgconfig

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type UpdateStatus struct {
	Current   Resolution `json:"current"`
	Available Resolution `json:"available"`
	Changed   bool       `json:"changed"`
}

func Activate(ctx context.Context, source Source) (ActiveState, error) {
	resolution, config, err := Resolve(ctx, source)
	if err != nil {
		return ActiveState{}, err
	}
	state := ActiveState{Resolution: resolution, Config: config}
	if err := SaveActive(state); err != nil {
		return ActiveState{}, err
	}
	return state, nil
}

func Check(ctx context.Context) (UpdateStatus, Config, error) {
	current, err := LoadActive()
	if err != nil {
		return UpdateStatus{}, Config{}, err
	}
	availableResolution, availableConfig, err := Resolve(ctx, current.Resolution.Source)
	if err != nil {
		return UpdateStatus{}, Config{}, err
	}
	changed := immutableIdentity(current.Resolution) != immutableIdentity(availableResolution)
	return UpdateStatus{Current: current.Resolution, Available: availableResolution, Changed: changed}, availableConfig, nil
}

func Refresh(ctx context.Context) (ActiveState, error) {
	current, err := LoadActive()
	if err != nil {
		return ActiveState{}, err
	}
	return Activate(ctx, current.Resolution.Source)
}

func Resolve(ctx context.Context, source Source) (Resolution, Config, error) {
	if source.Kind == SourceSystem && strings.TrimSpace(source.Location) == "" {
		source.Location = "/etc/baseharbor/organization.yaml"
	}
	if err := source.Validate(); err != nil {
		return Resolution{}, Config{}, err
	}
	switch source.Kind {
	case SourceLocal, SourceSystem:
		return resolveLocal(source)
	case SourceGit:
		return resolveGit(ctx, source)
	case SourceOCI:
		return resolveOCI(ctx, source)
	default:
		return Resolution{}, Config{}, fmt.Errorf("organization source kind %q is unsupported", source.Kind)
	}
}

func resolveLocal(source Source) (Resolution, Config, error) {
	path, err := locateOrganizationFile(source.Location)
	if err != nil {
		return Resolution{}, Config{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Resolution{}, Config{}, fmt.Errorf("read organization configuration: %w", err)
	}
	config, err := parseConfig(content)
	if err != nil {
		return Resolution{}, Config{}, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Resolution{}, Config{}, err
	}
	digest := digestBytes(content)
	resolution := Resolution{
		Source:          source,
		ResolvedVersion: config.APIVersion,
		ResolvedDigest:  digest,
		Provenance:      "file://" + abs,
	}
	cachePath, err := writeCache(resolution, config, content, filepath.Dir(path))
	if err != nil {
		return Resolution{}, Config{}, err
	}
	resolution.CachePath = cachePath
	return resolution, config, nil
}

func resolveGit(ctx context.Context, source Source) (Resolution, Config, error) {
	requested := strings.TrimSpace(source.Requested)
	if requested == "" {
		requested = "HEAD"
		source.Requested = requested
	}
	revision, err := gitResolveRevision(ctx, source.Location, requested)
	if err != nil {
		return Resolution{}, Config{}, err
	}
	tmp, err := os.MkdirTemp("", "baseharbor-org-git-*")
	if err != nil {
		return Resolution{}, Config{}, err
	}
	defer os.RemoveAll(tmp)
	if _, err := run(ctx, "git", "clone", "--quiet", "--no-checkout", source.Location, tmp); err != nil {
		return Resolution{}, Config{}, fmt.Errorf("clone organization Git source: %w", err)
	}
	if _, err := run(ctx, "git", "-C", tmp, "checkout", "--quiet", "--detach", revision); err != nil {
		return Resolution{}, Config{}, fmt.Errorf("checkout organization Git revision %s: %w", revision, err)
	}
	path, err := locateOrganizationFile(tmp)
	if err != nil {
		return Resolution{}, Config{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Resolution{}, Config{}, fmt.Errorf("read organization configuration from Git source: %w", err)
	}
	config, err := parseConfig(content)
	if err != nil {
		return Resolution{}, Config{}, err
	}
	resolution := Resolution{
		Source:           source,
		ResolvedVersion:  config.APIVersion,
		ResolvedDigest:   digestBytes(content),
		ResolvedRevision: revision,
		Provenance:       strings.TrimSpace(source.Location) + "@" + revision,
	}
	cachePath, err := writeCache(resolution, config, content, filepath.Dir(path))
	if err != nil {
		return Resolution{}, Config{}, err
	}
	resolution.CachePath = cachePath
	return resolution, config, nil
}

func gitResolveRevision(ctx context.Context, location, requested string) (string, error) {
	out, err := run(ctx, "git", "ls-remote", "--exit-code", strings.TrimSpace(location), strings.TrimSpace(requested))
	if err != nil && requested == "HEAD" {
		out, err = run(ctx, "git", "ls-remote", "--exit-code", strings.TrimSpace(location), "HEAD")
	}
	if err != nil {
		return "", fmt.Errorf("resolve organization Git ref %q: %w", requested, err)
	}
	fields := strings.Fields(out)
	if len(fields) < 2 || len(fields[0]) != 40 {
		return "", fmt.Errorf("organization Git ref %q did not resolve to one immutable revision", requested)
	}
	return fields[0], nil
}

func resolveOCI(ctx context.Context, source Source) (Resolution, Config, error) {
	requested := strings.TrimSpace(source.Requested)
	if requested == "" {
		requested = "latest"
		source.Requested = requested
	}
	ref := strings.TrimSpace(source.Location)
	if strings.HasPrefix(requested, "sha256:") {
		ref += "@" + requested
	} else {
		ref += ":" + requested
	}
	out, err := run(ctx, "oras", "resolve", ref)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Resolution{}, Config{}, fmt.Errorf("resolve organization OCI source: oras is required; install ORAS or use Git/local/system distribution")
		}
		return Resolution{}, Config{}, fmt.Errorf("resolve organization OCI source %q: %w", ref, err)
	}
	digest := firstDigest(out)
	if digest == "" {
		return Resolution{}, Config{}, fmt.Errorf("organization OCI source %q did not resolve to an immutable sha256 digest", ref)
	}
	tmp, err := os.MkdirTemp("", "baseharbor-org-oci-*")
	if err != nil {
		return Resolution{}, Config{}, err
	}
	defer os.RemoveAll(tmp)
	immutableRef := strings.TrimSpace(source.Location) + "@" + digest
	if _, err := run(ctx, "oras", "pull", immutableRef, "--output", tmp); err != nil {
		return Resolution{}, Config{}, fmt.Errorf("pull organization OCI source %q: %w", immutableRef, err)
	}
	path, err := locateOrganizationFile(tmp)
	if err != nil {
		return Resolution{}, Config{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Resolution{}, Config{}, fmt.Errorf("read organization configuration from OCI source: %w", err)
	}
	config, err := parseConfig(content)
	if err != nil {
		return Resolution{}, Config{}, err
	}
	resolution := Resolution{
		Source:          source,
		ResolvedVersion: config.APIVersion,
		ResolvedDigest:  digest,
		Provenance:      immutableRef,
	}
	cachePath, err := writeCache(resolution, config, content, filepath.Dir(path))
	if err != nil {
		return Resolution{}, Config{}, err
	}
	resolution.CachePath = cachePath
	return resolution, config, nil
}

func locateOrganizationFile(location string) (string, error) {
	location = strings.TrimSpace(location)
	info, err := os.Stat(location)
	if err != nil {
		return "", fmt.Errorf("organization source %q: %w", location, err)
	}
	if !info.IsDir() {
		return location, nil
	}
	for _, name := range []string{"organization.yaml", "organization.yml", "baseharbor-organization.yaml"} {
		path := filepath.Join(location, name)
		if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("organization source %q does not contain organization.yaml, organization.yml or baseharbor-organization.yaml", location)
}

func parseConfig(content []byte) (Config, error) {
	var config Config
	if err := yaml.Unmarshal(content, &config); err != nil {
		return Config{}, fmt.Errorf("parse organization configuration: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate organization configuration: %w", err)
	}
	return config, nil
}

func digestBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func immutableIdentity(r Resolution) string {
	if value := strings.TrimSpace(r.ResolvedDigest); value != "" {
		return value
	}
	return strings.TrimSpace(r.ResolvedRevision)
}

func firstDigest(out string) string {
	for _, field := range strings.Fields(out) {
		if digestPattern.MatchString(field) {
			return field
		}
	}
	return ""
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, detail)
	}
	return strings.TrimSpace(stdout.String()), nil
}
