package devgateway

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

func ManagementURLs(target, owner string, desired []Route) (map[string]string, map[string]string, error) {
	files, err := FilesFor(target)
	if err != nil {
		return nil, nil, err
	}
	current, err := loadState(files.State)
	if err != nil {
		return nil, nil, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, nil, errors.New("development management URL owner is required")
	}
	wanted := make(map[string]string, len(desired))
	for _, route := range desired {
		if route.ManagementKey == "" {
			continue
		}
		if previous, duplicate := wanted[route.Key]; duplicate && previous != route.ManagementKey {
			return nil, nil, fmt.Errorf("conflicting development management keys for route %q", route.Key)
		}
		wanted[route.Key] = route.ManagementKey
	}
	selected := map[string]Route{}
	priority := map[string]int{}
	for _, route := range current.Routes {
		if route.ManagementKey == "" {
			continue
		}
		managementKey, required := wanted[route.Key]
		if !required || managementKey != route.ManagementKey {
			continue
		}
		candidatePriority := 0
		switch {
		case route.Owner == owner:
			candidatePriority = 2
		case strings.HasPrefix(route.Owner, "shared/"):
			candidatePriority = 1
		default:
			continue
		}
		if candidatePriority < priority[route.ManagementKey] {
			continue
		}
		if candidatePriority == priority[route.ManagementKey] {
			return nil, nil, fmt.Errorf("duplicate development management key %q for owner %q", route.ManagementKey, route.Owner)
		}
		selected[route.ManagementKey] = route
		priority[route.ManagementKey] = candidatePriority
	}
	canonical := make(map[string]string, len(selected))
	directIP := make(map[string]string, len(selected))
	for key, route := range selected {
		canonical[key] = canonicalURL(route.Host, current.HostPort)
		if current.IPAccessAddress != "" && route.DirectIPPort > 0 {
			directIP[key] = "https://" + net.JoinHostPort(current.IPAccessAddress, strconv.Itoa(route.DirectIPPort))
		}
	}
	return canonical, directIP, nil
}
