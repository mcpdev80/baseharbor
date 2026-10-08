package remoteprojection

import (
	"errors"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/providers/runtime/podman"
)

type ProjectedQuadletGraph struct {
	Files     map[string]string
	Units     []string
	InitUnits []string
}

// ProjectRemoteQuadletGraph realizes an already Core-validated Compose project.
// File references must resolve to approved bundle members, not a Core checkout.
// The node resolves explicit markers beneath its own immutable bundle root.
func ProjectRemoteQuadletGraph(compose, env, project string, members []string) (ProjectedQuadletGraph, error) {
	return projectRemoteQuadletGraph(compose, env, project, members, false)
}

// ProjectRemoteQuadletInitGraph requires Core's completion-aware applier. It
// removes native implicit init activation and delegates completion to the
// authenticated, exact-source observation rather than a shell state heuristic.
func ProjectRemoteQuadletInitGraph(compose, env, project string, members []string) (ProjectedQuadletGraph, error) {
	return projectRemoteQuadletGraph(compose, env, project, members, true)
}

func projectRemoteQuadletGraph(compose, env, project string, members []string, completion bool) (ProjectedQuadletGraph, error) {
	allowed := map[string]bool{}
	for _, member := range members {
		if member == "" || member == "." || member == ".." || path.Clean(member) != member ||
			strings.HasPrefix(member, "/") || strings.HasPrefix(member, "../") || strings.ContainsAny(member, "\\\x00\r\n") {
			return ProjectedQuadletGraph{}, errors.New("invalid approved Quadlet bundle member")
		}
		allowed[member] = true
	}
	graph, err := podman.RenderComposeProjectQuadlets(compose, env, project)
	if err != nil {
		return ProjectedQuadletGraph{}, err
	}
	if len(graph.CompletedServices) != 0 && !completion {
		return ProjectedQuadletGraph{}, errors.New("remote Quadlet completion dependencies require a qualified init-workload adapter")
	}
	initUnits, err := prepareRemoteInitUnits(&graph)
	if err != nil {
		return ProjectedQuadletGraph{}, err
	}
	root, err := filepath.Abs(filepath.Dir(compose))
	if err != nil {
		return ProjectedQuadletGraph{}, err
	}
	result := ProjectedQuadletGraph{Files: map[string]string{}, InitUnits: initUnits}
	for name := range graph.Files {
		if filepath.Ext(name) == ".env" {
			allowed[name] = true
		}
	}
	for name, data := range graph.Files {
		ext := filepath.Ext(name)
		if ext != ".env" && ext != ".container" && ext != ".network" && ext != ".volume" {
			return ProjectedQuadletGraph{}, errors.New("remote Quadlet projection requires prebuilt artifacts")
		}
		lines := strings.Split(data, "\n")
		section := ""
		var output []string
		for _, line := range lines {
			if strings.HasPrefix(line, "[") {
				section = line
			}
			// Native process configuration belongs to the node, not the Core host.
			if section == "[Service]" && strings.HasPrefix(line, "Environment=") {
				continue
			}
			if section == "[Container]" && strings.HasPrefix(line, "EnvironmentFile=./") {
				member := strings.TrimPrefix(line, "EnvironmentFile=./")
				if !allowed[member] {
					return ProjectedQuadletGraph{}, errors.New("unstaged Quadlet environment")
				}
				line = "EnvironmentFile=@BASEHARBOR_BUNDLE@/" + member
			}
			if section == "[Container]" && strings.HasPrefix(line, "Volume=/") {
				parts := strings.Split(strings.TrimPrefix(line, "Volume="), ":")
				if len(parts) != 3 || parts[2] != "ro" {
					return ProjectedQuadletGraph{}, errors.New("remote file bindings must be read-only")
				}
				member, err := filepath.Rel(root, parts[0])
				if err != nil || !allowed[filepath.ToSlash(member)] {
					return ProjectedQuadletGraph{}, errors.New("Quadlet binding is outside approved bundle")
				}
				line = "Volume=@BASEHARBOR_BUNDLE@/" + filepath.ToSlash(member) + ":" + parts[1] + ":ro"
			}
			output = append(output, line)
		}
		result.Files[name] = strings.Join(output, "\n")
		if ext != ".env" {
			result.Units = append(result.Units, name)
		}
	}
	sort.Strings(result.Units)
	return compactGraphUnitNames(result)
}
