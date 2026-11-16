package application

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ServiceNames reads the retained source, never today's checkout or inventory.
func (r *RemoteManagedRuntime) ServiceNames() ([]string, error) {
	if r == nil || r.runtime == nil {
		return nil, errors.New("remote application source is unavailable")
	}
	if r.kind == "docker" {
		for _, file := range r.source {
			if file.Path != r.compose {
				continue
			}
			var document struct {
				Services map[string]any `yaml:"services"`
			}
			if err := yaml.Unmarshal(file.Data, &document); err != nil || len(document.Services) == 0 {
				return nil, errors.New("remote application services are invalid")
			}
			var names []string
			for name := range document.Services {
				names = append(names, name)
			}
			sort.Strings(names)
			return names, nil
		}
		return nil, errors.New("remote application Compose source is absent")
	}
	services, err := r.quadletServiceFiles()
	if err != nil {
		return nil, err
	}
	var names []string
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (r *RemoteManagedRuntime) quadletServiceFiles() (map[string]string, error) {
	services := map[string]string{}
	for _, file := range r.source {
		if filepath.Ext(file.Path) != ".container" {
			continue
		}
		section, service := "", ""
		for _, line := range strings.Split(string(file.Data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") {
				section = line
			}
			const prefix = "Label=com.docker.compose.service="
			if section == "[Container]" && strings.HasPrefix(line, prefix) {
				if service != "" {
					return nil, errors.New("ambiguous remote Quadlet service label")
				}
				service = strings.TrimPrefix(line, prefix)
			}
		}
		if service == "" || services[service] != "" {
			return nil, errors.New("ambiguous remote Quadlet service identity")
		}
		services[service] = file.Path
	}
	if len(services) == 0 {
		return nil, errors.New("remote Quadlet services are absent")
	}
	return services, nil
}

func (r *RemoteManagedRuntime) quadletServices(names []string) ([]string, []string, error) {
	services, err := r.quadletServiceFiles()
	if err != nil {
		return nil, nil, err
	}
	selected := map[string]bool{}
	for _, name := range names {
		file := services[name]
		if file == "" || selected[file] {
			return nil, nil, errors.New("remote application phase differs from retained services")
		}
		selected[file] = true
	}
	var units, init []string
	for _, file := range r.units {
		if filepath.Ext(file) != ".container" || selected[file] {
			units = append(units, file)
		}
	}
	for _, file := range r.initUnits {
		if selected[file] {
			init = append(init, file)
		}
	}
	return units, init, nil
}
