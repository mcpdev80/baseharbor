package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

// Engines may report repository@digest instead of the requested tag@digest.
// Recover the tag only from the owned, protected Compose declaration and only
// when both repository and immutable runtime digest agree.
func ownedTaggedImage(image bhruntime.ImageIdentity, compose, service string) (string, error) {
	ref := strings.TrimSpace(image.Reference)
	base := strings.SplitN(ref, "@", 2)[0]
	if strings.LastIndex(base, ":") > strings.LastIndex(base, "/") {
		return base, nil
	}
	info, err := os.Lstat(compose)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("provider Compose is not protected regular material")
	}
	data, err := os.ReadFile(compose)
	if err != nil {
		return "", err
	}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	declared := doc.Services[service].Image
	parts := strings.SplitN(declared, "@", 2)
	if len(parts) != 2 {
		return "", errors.New("canonical image version requires an owned immutable declaration")
	}
	tag := strings.LastIndex(parts[0], ":")
	digest := image.Digest
	if i := strings.Index(digest, "@"); i >= 0 {
		digest = digest[i+1:]
	}
	if tag <= strings.LastIndex(parts[0], "/") || canonicalImageRepository(base) != canonicalImageRepository(parts[0][:tag]) || ref != base+"@"+digest || parts[1] != digest {
		return "", fmt.Errorf("canonical runtime image differs from owned provider pin: observed=%s digest=%s declared=%s", ref, digest, declared)
	}
	return parts[0], nil
}

func canonicalImageRepository(repository string) string {
	for _, prefix := range []string{"index.docker.io/", "registry-1.docker.io/"} {
		if strings.HasPrefix(repository, prefix) {
			return "docker.io/" + strings.TrimPrefix(repository, prefix)
		}
	}
	first, _, hasSlash := strings.Cut(repository, "/")
	if !hasSlash {
		return "docker.io/library/" + repository
	}
	if !strings.ContainsAny(first, ".:") && first != "localhost" {
		return "docker.io/" + repository
	}
	return repository
}
