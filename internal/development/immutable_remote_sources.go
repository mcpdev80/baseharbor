package development

import (
	"encoding/hex"
	"fmt"
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
			ref := strings.TrimSpace(source.Ref)
			if len(ref) != 40 || strings.ToLower(ref) != ref {
				return fmt.Errorf("repository source %q requires a pinned 40-character lowercase Git commit SHA; branch and tag names are not immutable release identities", source.ID)
			}
			if _, err := hex.DecodeString(ref); err != nil {
				return fmt.Errorf("repository source %q has an invalid Git commit SHA", source.ID)
			}
		case SourceOCI:
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
