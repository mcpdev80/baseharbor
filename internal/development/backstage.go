package development

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func RenderBackstageCatalog(manifest application.Manifest, contract application.PortableContract) string {
	var b strings.Builder
	b.WriteString("apiVersion: backstage.io/v1alpha1\n")
	b.WriteString("kind: Component\n")
	b.WriteString("metadata:\n")
	fmt.Fprintf(&b, "  name: %s\n", sanitizeCatalogName(manifest.Name))
	b.WriteString("  annotations:\n")
	b.WriteString("    baseharbor.dev/contract-version: v1\n")
	fmt.Fprintf(&b, "    baseharbor.dev/environment: %s\n", manifest.Environment)
	b.WriteString("  tags:\n")
	tags := make([]string, 0, len(contract.Capabilities))
	for _, requirement := range contract.Capabilities {
		tags = append(tags, strings.ReplaceAll(string(requirement.Kind), ".", "-"))
	}
	sort.Strings(tags)
	for _, tag := range tags {
		fmt.Fprintf(&b, "    - %s\n", tag)
	}
	b.WriteString("spec:\n")
	b.WriteString("  type: service\n")
	b.WriteString("  lifecycle: experimental\n")
	return b.String()
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
