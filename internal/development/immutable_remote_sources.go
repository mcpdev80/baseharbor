package development

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// ValidateImmutableRemoteSources accepts an existing portable source model for
// remote release selection only when each source has an immutable identity.
// The local worktree mapping is intentionally not inspected or published.
func ValidateImmutableRemoteSources(model SourceModel) error {
	if err := model.Validate(); err != nil {
		return err
	}
	for _, source := range model.Sources {
		switch source.Type {
		case SourceRepository:
			if err := validateRemoteGitLocation(source.Repository); err != nil {
				return fmt.Errorf("repository source %q: %w", source.ID, err)
			}
			ref := strings.TrimSpace(source.Ref)
			if len(ref) != 40 || strings.ToLower(ref) != ref {
				return fmt.Errorf("repository source %q requires a pinned 40-character lowercase Git commit SHA; branch and tag names are not immutable release identities", source.ID)
			}
			if _, err := hex.DecodeString(ref); err != nil {
				return fmt.Errorf("repository source %q has an invalid Git commit SHA", source.ID)
			}
		case SourceOCI:
			if source.Image != strings.TrimSpace(source.Image) || strings.ContainsAny(source.Image, " \t\r\n?#") {
				return fmt.Errorf("OCI source %q has an unsafe image reference", source.ID)
			}
			parts := strings.Split(source.Image, "@")
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || !strings.HasPrefix(parts[1], "sha256:") ||
				len(parts[1]) != len("sha256:")+64 || strings.ToLower(parts[1]) != parts[1] {
				return fmt.Errorf("OCI source %q requires an immutable image reference with @sha256:<64 lowercase hex characters>", source.ID)
			}
			if _, err := hex.DecodeString(strings.TrimPrefix(parts[1], "sha256:")); err != nil {
				return fmt.Errorf("OCI source %q has an invalid SHA-256 manifest digest", source.ID)
			}
		}
	}
	return nil
}

// Remote Git sources are fetched by the owning Core, never from a local
// workstation path or unauthenticated/insecure transport.
func validateRemoteGitLocation(location string) error {
	if location == "" || location != strings.TrimSpace(location) || strings.ContainsAny(location, " \t\r\n?#") {
		return fmt.Errorf("Git repository requires a nonempty HTTPS or SSH remote URL without query, fragment or whitespace")
	}
	if strings.HasPrefix(location, "git@") {
		hostAndPath := strings.TrimPrefix(location, "git@")
		host, path, ok := strings.Cut(hostAndPath, ":")
		if !ok || host == "" || path == "" || strings.ContainsAny(host, "/\\@") {
			return fmt.Errorf("invalid SSH Git repository identity")
		}
		return nil
	}
	parsed, err := url.Parse(location)
	if err != nil || parsed == nil || (parsed.Scheme != "https" && parsed.Scheme != "ssh") ||
		parsed.Host == "" || parsed.Path == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("remote Git repository must use HTTPS or SSH, never a local path, file URL or insecure protocol")
	}
	if parsed.User != nil {
		if parsed.Scheme != "ssh" || parsed.User.Username() == "" {
			return fmt.Errorf("Git HTTPS credentials must be provided by the native credential helper, not the repository URL")
		}
		if _, hasPassword := parsed.User.Password(); hasPassword {
			return fmt.Errorf("Git repository URL must not contain embedded passwords")
		}
	}
	return nil
}
