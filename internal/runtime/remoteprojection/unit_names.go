package remoteprojection

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// Only realization unit names are shortened. Authoritative project labels and
// native ContainerName/NetworkName/VolumeName remain unchanged.
func compactGraphUnitNames(graph ProjectedQuadletGraph) (ProjectedQuadletGraph, error) {
	files, services := map[string]string{}, map[string]string{}
	seen := map[string]bool{}
	for _, name := range graph.Units {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		compact := name
		if len(base) > 64 {
			sum := sha256.Sum256([]byte(base))
			compact = "bh-unit-" + hex.EncodeToString(sum[:24]) + ext
		}
		if seen[compact] {
			return ProjectedQuadletGraph{}, errors.New("ambiguous bounded Quadlet unit identity")
		}
		seen[compact] = true
		files[name] = compact
		services[generatedServiceName(name)] = generatedServiceName(compact)
	}
	result := ProjectedQuadletGraph{Files: map[string]string{}}
	for name, content := range graph.Files {
		if filepath.Ext(name) == ".env" {
			result.Files[name] = content
			continue
		}
		lines := strings.Split(content, "\n")
		section := ""
		for i, line := range lines {
			if strings.HasPrefix(line, "[") {
				section = line
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			if section == "[Unit]" {
				switch key {
				case "Requires", "Wants", "After", "Before", "BindsTo", "PartOf":
					refs := strings.Fields(value)
					for j, ref := range refs {
						if bounded, exists := services[ref]; exists {
							refs[j] = bounded
						}
					}
					lines[i] = key + "=" + strings.Join(refs, " ")
				}
			}
			if section == "[Container]" && (key == "Network" || key == "Volume") {
				source, suffix, separator := strings.Cut(value, ":")
				if bounded, exists := files[source]; exists {
					if separator {
						bounded += ":" + suffix
					}
					lines[i] = key + "=" + bounded
				}
			}
		}
		bounded := files[name]
		result.Files[bounded] = strings.Join(lines, "\n")
		result.Units = append(result.Units, bounded)
	}
	sort.Strings(result.Units)
	return result, nil
}

func generatedServiceName(file string) string {
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	if ext != ".container" {
		base += "-" + strings.TrimPrefix(ext, ".")
	}
	return base + ".service"
}
