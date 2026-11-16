package targetsession

import (
	"errors"
	"path"
	"strings"
)

// orderQuadletContainers starts dependencies before dependents. Otherwise
// systemd could start a dependency implicitly, only for a later explicit
// restart to interrupt the application that has just become ready.
func orderQuadletContainers(project *StagedProject, selected []string) ([]string, error) {
	units := map[string]string{}
	var ordered []string
	for _, file := range selected {
		if path.Ext(file) != ".container" {
			ordered = append(ordered, file)
			continue
		}
		units[strings.TrimSuffix(file, ".container")+".service"] = file
	}
	state := map[string]uint8{}
	var visit func(string) error
	visit = func(file string) error {
		if state[file] == 1 {
			return errors.New("cyclic staged Quadlet container dependencies")
		}
		if state[file] == 2 {
			return nil
		}
		state[file] = 1
		section := ""
		for _, line := range strings.Split(string(project.files[file].data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = line
			}
			key, value, ok := strings.Cut(line, "=")
			if section != "[Unit]" || !ok || (key != "Requires" && key != "After") {
				continue
			}
			for _, unit := range strings.Fields(value) {
				if dependency, exists := units[unit]; exists {
					if err := visit(dependency); err != nil {
						return err
					}
				}
			}
		}
		state[file] = 2
		ordered = append(ordered, file)
		return nil
	}
	for _, file := range selected {
		if path.Ext(file) == ".container" {
			if err := visit(file); err != nil {
				return nil, err
			}
		}
	}
	return ordered, nil
}
