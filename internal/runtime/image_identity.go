package runtime

import (
	"context"
	"encoding/hex"
	"strings"
)

func imageRepository(reference string) string {
	repository := strings.SplitN(reference, "@", 2)[0]
	if colon := strings.LastIndexByte(repository, ':'); colon > strings.LastIndexByte(repository, '/') {
		repository = repository[:colon]
	}
	for _, prefix := range []string{"index.docker.io/", "registry-1.docker.io/"} {
		if strings.HasPrefix(repository, prefix) {
			return "docker.io/" + strings.TrimPrefix(repository, prefix)
		}
	}
	first, _, slash := strings.Cut(repository, "/")
	if !slash {
		return "docker.io/library/" + repository
	}
	if !strings.ContainsAny(first, ".:") && first != "localhost" {
		return "docker.io/" + repository
	}
	return repository
}

func validImageDigest(digest string) bool {
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
		return false
	}
	_, err := hex.DecodeString(digest[7:])
	return err == nil
}

// Preserve an immutable OCI index pin rather than arbitrarily choosing its
// platform manifest. A platform-only Podman store must independently resolve
// the exact pinned repository reference to the running container's image ID.
func (c Compose) verifiedImageRepositoryDigest(ctx context.Context, identity ImageIdentity, digests string) string {
	_, requested, pinned := strings.Cut(identity.Reference, "@")
	first := ""
	for _, line := range strings.Split(digests, "\n") {
		line = strings.TrimSpace(line)
		repository, digest, ok := strings.Cut(line, "@")
		if !ok || !validImageDigest(digest) || imageRepository(repository) != imageRepository(identity.Reference) {
			continue
		}
		if first == "" {
			first = line
		}
		if pinned && digest == requested {
			return line
		}
	}
	if pinned && validImageDigest(requested) && first != "" && identity.ImageID != "" {
		resolved, err := c.directOutput(ctx, "image", "inspect", "--format", "{{.Id}}", identity.Reference)
		if err == nil && strings.TrimPrefix(strings.TrimSpace(resolved), "sha256:") == strings.TrimPrefix(identity.ImageID, "sha256:") {
			return imageRepository(identity.Reference) + "@" + requested
		}
	}
	return first
}
