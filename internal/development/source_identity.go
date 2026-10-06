package development

import (
	"fmt"
	"net/url"
	"strings"
)

// Canonical source identities are serialized in shared CLI/JSON/MCP output.
// Authentication belongs in Git/registry credential helpers, not those identities.
func validatePublicSourceIdentity(identity string) error {
	if !strings.Contains(identity, "://") {
		return nil
	}
	parsed, err := url.Parse(identity)
	if err != nil {
		return fmt.Errorf("source identity must be a valid public repository or image reference")
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("source identity must not include query parameters; use a credential helper")
	}
	if parsed.User != nil {
		_, hasPassword := parsed.User.Password()
		if hasPassword || parsed.Scheme != "ssh" {
			return fmt.Errorf("source identity must not include credentials; use a credential helper")
		}
	}
	return nil
}
