package coreupdate

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// RewriteOwnedComposeImages patches only precisely listed, BaseHarbor-owned
// provider services. It leaves every unrelated provider and field untouched.
func RewriteOwnedComposeImages(source []byte, mutations map[string]Delta) ([]byte, error) {
	if len(mutations) == 0 {
		return nil, errors.New("no Core provider image mutation requested")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(source, &doc); err != nil {
		return nil, fmt.Errorf("parse Core provider Compose: %w", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("invalid Compose root")
	}
	root := doc.Content[0]
	var services *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "services" {
			services = root.Content[i+1]
			break
		}
	}
	if services == nil || services.Kind != yaml.MappingNode {
		return nil, errors.New("Compose has no service mapping")
	}
	visited := make(map[string]bool, len(mutations))
	for i := 0; i+1 < len(services.Content); i += 2 {
		name := services.Content[i].Value
		delta, selected := mutations[name]
		if !selected {
			continue
		}
		if visited[name] {
			return nil, fmt.Errorf("duplicate Compose service %s", name)
		}
		visited[name] = true
		if delta.Installed.Owner != "baseharbor" || delta.Installed.Instance != name || delta.Installed.Kind != delta.Desired.Kind ||
            !validDigest(delta.Desired.Digest) || strings.TrimSpace(delta.Desired.Image) == "" ||
            providerDowngrade(delta.Installed.Version,delta.Desired.Version) ||
            (delta.Installed.Kind==SQL && strings.Split(delta.Installed.Version,".")[0]!=strings.Split(delta.Desired.Version,".")[0]) ||
            ((delta.Installed.Kind==SQL||delta.Installed.Kind==Secrets)&&delta.Classification!=BackupRequired) ||
            (delta.Installed.Kind==Identity&&delta.Classification!=MigrationRequired) {
			return nil, fmt.Errorf("unowned, unchanged or unsafe provider image change for %s", name)
		}
		service := services.Content[i+1]
		if service.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("service %s is not a mapping", name)
		}
		found := false
		for j := 0; j+1 < len(service.Content); j += 2 {
			if service.Content[j].Value != "image" {
				continue
			}
			found = true
			img := service.Content[j+1]
			if img.Kind != yaml.ScalarNode || img.Value != delta.Installed.Image {
				return nil, fmt.Errorf("Compose service %s image differs from inventoried provider", name)
			}
			img.Tag = "!!str"
			img.Value = delta.Desired.Image + "@" + delta.Desired.Digest
			break
		}
		if !found {
			return nil, fmt.Errorf("Core service %s has no image field", name)
		}
	}
	for service := range mutations {
		if !visited[service] {
			return nil, fmt.Errorf("requested provider %s missing from Compose", service)
		}
	}
	var buf bytes.Buffer
	e := yaml.NewEncoder(&buf)
	e.SetIndent(2)
	if err := e.Encode(&doc); err != nil {
		return nil, err
	}
	if err := e.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
