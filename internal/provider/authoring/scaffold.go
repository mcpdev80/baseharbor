package authoring

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type InitResult struct {
	Root  string   `json:"root"`
	Files []string `json:"files"`
}

func Init(root, id string) (InitResult, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return InitResult{}, err
	}
	entries, err := os.ReadDir(abs)
	if err == nil && len(entries) != 0 {
		return InitResult{}, fmt.Errorf("provider directory %s is not empty", abs)
	}
	if err != nil && !os.IsNotExist(err) {
		return InitResult{}, err
	}
	descriptor := Descriptor{
		ID:               strings.TrimSpace(id),
		Version:          "0.1.0",
		ProviderProtocol: "baseharbor.provider/v1",
		ServiceKinds:     []string{"sql"},
		ServiceContracts: []string{"database.sql/v1"},
		Capabilities:     []string{"database.sql/v1"},
		SupportedScopes:  []string{"application"},
	}
	if err := descriptor.Validate(); err != nil {
		return InitResult{}, err
	}
	data, err := yaml.Marshal(descriptor)
	if err != nil {
		return InitResult{}, err
	}
	files := map[string][]byte{
		"provider.yaml":      data,
		"config.schema.json": []byte("{\n  \"$schema\": \"https://json-schema.org/draft/2020-12/schema\",\n  \"type\": \"object\",\n  \"additionalProperties\": false\n}\n"),
		"README.md":          []byte("# BaseHarbor Capability Provider\n\nImplement the baseharbor.provider/v1 lifecycle behind this descriptor. Run baha provider test . before publishing.\n"),
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return InitResult{}, err
	}
	var written []string
	for _, name := range []string{"provider.yaml", "config.schema.json", "README.md"} {
		if err := os.WriteFile(filepath.Join(abs, name), files[name], 0o644); err != nil {
			_ = os.RemoveAll(abs)
			return InitResult{}, err
		}
		written = append(written, name)
	}
	return InitResult{Root: abs, Files: written}, nil
}

func Load(root string) (Descriptor, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	data, err := os.ReadFile(filepath.Join(root, "provider.yaml"))
	if err != nil {
		return Descriptor{}, fmt.Errorf("read provider.yaml: %w", err)
	}
	var descriptor Descriptor
	if err := yaml.Unmarshal(data, &descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("decode provider.yaml: %w", err)
	}
	return descriptor, descriptor.Validate()
}
