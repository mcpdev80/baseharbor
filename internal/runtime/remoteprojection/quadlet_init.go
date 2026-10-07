package remoteprojection

import (
	"errors"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/providers/runtime/podman"
)

func prepareRemoteInitUnits(graph *podman.QuadletProject) ([]string, error) {
	completed := map[string]bool{}
	var initUnits []string
	for service := range graph.CompletedServices {
		unit, ok := graph.ServiceUnits[service]
		if !ok || !strings.HasSuffix(unit, ".service") {
			return nil, errors.New("init service has no selected unit")
		}
		file := strings.TrimSuffix(unit, ".service") + ".container"
		if _, ok := graph.Files[file]; !ok {
			return nil, errors.New("init source is outside projected graph")
		}
		completed[unit] = true
		initUnits = append(initUnits, file)
	}
	if len(initUnits) == 0 {
		return nil, nil
	}
	for file, content := range graph.Files {
		if !strings.HasSuffix(file, ".container") {
			continue
		}
		section := ""
		var output []string
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "[") {
				section = line
			}
			key, value, ok := strings.Cut(line, "=")
			if ok && section == "[Unit]" && (key == "Wants" || key == "Requires" || key == "BindsTo") {
				var keep []string
				for _, ref := range strings.Fields(value) {
					if !completed[ref] {
						keep = append(keep, ref)
					}
				}
				if len(keep) == 0 {
					continue
				}
				line = key + "=" + strings.Join(keep, " ")
			}
			if ok && section == "[Service]" && key == "ExecStartPre" {
				// The Compose renderer generates these only for completion
				// dependencies. Refuse unexpected hooks instead of forwarding a
				// Core-host shell heuristic into the remote completion boundary.
				if !strings.HasPrefix(value, "/bin/sh -ec ") || !strings.Contains(value, "systemctl --user is-failed") {
					return nil, errors.New("unexpected native init completion hook")
				}
				continue
			}
			output = append(output, line)
		}
		if completed[strings.TrimSuffix(file, ".container")+".service"] {
			// Without a target reference an inactive successful unit may be
			// garbage collected before Core verifies its native exit evidence.
			// Retain its completed state without adding an automatic start edge.
			output = append(output, "[Service]", "RemainAfterExit=yes", "Restart=no")
		}
		graph.Files[file] = strings.Join(output, "\n")
	}
	sort.Strings(initUnits)
	return initUnits, nil
}
