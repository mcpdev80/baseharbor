package development

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type BackstageCatalogOptions struct {
	Owner     string `json:"owner"`
	Lifecycle string `json:"lifecycle,omitempty"`
}

func (o BackstageCatalogOptions) Validate() error {
	if strings.TrimSpace(o.Owner) == "" {
		return fmt.Errorf("Backstage owner is required when catalog metadata emission is enabled")
	}
	switch strings.TrimSpace(o.Lifecycle) {
	case "", "experimental", "production", "deprecated":
		return nil
	default:
		return fmt.Errorf("unsupported Backstage lifecycle %q", o.Lifecycle)
	}
}

func RenderBackstageCatalog(manifest application.Manifest, options BackstageCatalogOptions) (string, error) {
	if err := options.Validate(); err != nil {
		return "", err
	}
	lifecycle := strings.TrimSpace(options.Lifecycle)
	if lifecycle == "" {
		lifecycle = "experimental"
	}
	var b strings.Builder
	b.WriteString("apiVersion: backstage.io/v1alpha1\n")
	b.WriteString("kind: Component\n")
	b.WriteString("metadata:\n")
	fmt.Fprintf(&b, "  name: %s\n", sanitizeCatalogName(manifest.Name))
	b.WriteString("  annotations:\n")
	fmt.Fprintf(&b, "    baseharbor.dev/application: %s\n", sanitizeCatalogName(manifest.Name))
	b.WriteString("spec:\n")
	b.WriteString("  type: service\n")
	fmt.Fprintf(&b, "  lifecycle: %s\n", lifecycle)
	fmt.Fprintf(&b, "  owner: %s\n", strings.TrimSpace(options.Owner))
	return b.String(), nil
}

func sanitizeCatalogName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "application"
	}
	return result
}
