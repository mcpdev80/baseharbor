package remoteprojection

import "github.com/mcpdev80/baseharbor/internal/providers/runtime/podman"

// RenderComposeJSON only resolves Core-owned source. It never invokes a runtime
// executable, builds an image or uses the Core host as the execution node.
func RenderComposeJSON(files []string, environment map[string]string) (string, error) {
	return podman.RenderComposeProjectFilesJSON(files, environment)
}
