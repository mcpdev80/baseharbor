// Package contracts provides the offline registry of shipped public JSON Schemas.
package contracts

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"strconv"
	"strings"
)

const Namespace = "https://github.com/mcpdev80/baseharbor/blob/HEAD/contracts/"

//go:embed */v1/*.schema.json
var schemas embed.FS

// SchemaRegistry returns canonical schema identifiers and their packaged paths.
// It verifies identifiers and references without making network requests.
func SchemaRegistry() (map[string]string, error) {
	documents := map[string]map[string]any{}
	paths := map[string]string{}
	err := fs.WalkDir(schemas, ".", func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := schemas.ReadFile(file)
		if err != nil {
			return err
		}
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			return fmt.Errorf("%s: invalid schema: %w", file, err)
		}
		id, _ := document["$id"].(string)
		if id != Namespace+file {
			return fmt.Errorf("%s: schema ID must match the canonical packaged path", file)
		}
		if _, duplicate := documents[id]; duplicate {
			return fmt.Errorf("duplicate schema ID %s", id)
		}
		if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			return fmt.Errorf("%s: unsupported JSON Schema dialect", file)
		}
		documents[id], paths[id] = document, file
		return nil
	})
	if err != nil {
		return nil, err
	}
	for id, document := range documents {
		if err := validateReferences(id, document, documents); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// ReadSchema resolves a canonical identifier using only the packaged registry.
func ReadSchema(id string) ([]byte, error) {
	registry, err := SchemaRegistry()
	if err != nil {
		return nil, err
	}
	file, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("schema ID is not in the packaged registry")
	}
	return schemas.ReadFile(file)
}

func validateReferences(base string, node any, documents map[string]map[string]any) error {
	switch value := node.(type) {
	case map[string]any:
		if id, ok := value["$id"]; ok && id != base {
			return fmt.Errorf("%s: nested schema IDs are not registered", base)
		}
		if raw, ok := value["$ref"]; ok {
			ref, ok := raw.(string)
			if !ok {
				return fmt.Errorf("%s: non-string schema reference", base)
			}
			baseURL, _ := url.Parse(base)
			reference, err := url.Parse(ref)
			if err != nil {
				return fmt.Errorf("%s: invalid schema reference", base)
			}
			target := baseURL.ResolveReference(reference)
			fragment := target.Fragment
			target.Fragment = ""
			document, found := documents[target.String()]
			if !found || !jsonPointerExists(document, fragment) {
				return fmt.Errorf("%s: unresolved packaged schema reference %q", base, ref)
			}
		}
		for _, child := range value {
			if err := validateReferences(base, child, documents); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := validateReferences(base, child, documents); err != nil {
				return err
			}
		}
	}
	return nil
}

func jsonPointerExists(node any, pointer string) bool {
	if pointer == "" {
		return true
	}
	if !strings.HasPrefix(pointer, "/") {
		return false
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch value := node.(type) {
		case map[string]any:
			var found bool
			node, found = value[part]
			if !found {
				return false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) || strconv.Itoa(index) != part {
				return false
			}
			node = value[index]
		default:
			return false
		}
	}
	return true
}

// SchemaPath is useful for clients that bind an ID to an immutable source ref.
func SchemaPath(id string) (string, error) {
	registry, err := SchemaRegistry()
	if err != nil {
		return "", err
	}
	file, ok := registry[id]
	if !ok {
		return "", fmt.Errorf("schema ID is not in the packaged registry")
	}
	return path.Join("contracts", file), nil
}
