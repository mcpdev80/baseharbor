package podman

import "go.yaml.in/yaml/v3"

// Compose merges service network attachments by name, including its short-list
// and long-map forms. Adding a managed transport must preserve authored access
// to independent receivers. Explicit !override is handled by the caller first.
func quadletNetworkAttachmentMap(node *yaml.Node) *yaml.Node {
	if node.Tag == "!reset" {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		return node
	}
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	result := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	seen := map[string]bool{}
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			return nil // Keep invalid attachments subject to normal validation.
		}
		if seen[item.Value] {
			continue
		}
		seen[item.Value] = true
		result.Content = append(result.Content, cloneQuadletYAMLNode(item), &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
	}
	return result
}
