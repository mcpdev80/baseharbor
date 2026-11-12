package repositoryinspect

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	SecretSourceTargetDevelopmentUsername = "target-development-username"
	SecretSourceTargetDevelopmentPassword = "target-development-password"
)

var composeSecretNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

type composeBaseHarborExtensions struct {
	Secrets []struct {
		Name   string `yaml:"name"`
		Source string `yaml:"source"`
	} `yaml:"secrets"`
}

type composeBaseHarborDocument struct {
	BaseHarbor composeBaseHarborExtensions `yaml:"x-baseharbor"`
}

// ReadComposeSecretSourcePolicies reads explicit BaseHarbor secret-source
// declarations from Compose's standard x-* extension field. Compose ignores
// this metadata; baha app init turns it into the portable app contract.
func ReadComposeSecretSourcePolicies(root, composePath string) (map[string]string, error) {
	if strings.TrimSpace(composePath) == "" {
		return nil, nil
	}
	path := composePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Compose secret-source declarations: %w", err)
	}
	var document composeBaseHarborDocument
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("parse Compose BaseHarbor extensions: %w", err)
	}
	policies := make(map[string]string, len(document.BaseHarbor.Secrets))
	for _, declaration := range document.BaseHarbor.Secrets {
		name := strings.TrimSpace(declaration.Name)
		source := strings.TrimSpace(declaration.Source)
		if !composeSecretNamePattern.MatchString(name) {
			return nil, fmt.Errorf("Compose x-baseharbor secret name %q must be an environment-compatible uppercase key", name)
		}
		if source != SecretSourceTargetDevelopmentUsername && source != SecretSourceTargetDevelopmentPassword {
			return nil, fmt.Errorf("Compose x-baseharbor secret %s has unsupported source %q", name, source)
		}
		if _, exists := policies[name]; exists {
			return nil, fmt.Errorf("Compose x-baseharbor declares secret source %s more than once", name)
		}
		policies[name] = source
	}
	return policies, nil
}
