package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

func providerLogSources(p Placement, registrations []Registration) ([]observability.SignalSource, error) {
	applications := make([]string, 0, len(registrations))
	seen := map[string]struct{}{}
	for _, registration := range registrations {
		if _, ok := seen[registration.Application]; ok {
			continue
		}
		seen[registration.Application] = struct{}{}
		applications = append(applications, registration.Application)
	}
	return observability.ListLogs(
		capability.ProviderPlacement{
			Scope:           p.Scope,
			SharingBoundary: p.SharingBoundary,
			Ownership:       capability.OwnershipBaseHarbor,
		},
		applications,
		true,
		true,
	)
}

func reconcileRegistration(path string, m application.Manifest, present bool) ([]Registration, error) {
	registrations, err := readRegistrations(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	result := make([]Registration, 0, len(registrations)+1)
	var existing *Registration
	for _, registration := range registrations {
		if registration.Application == m.Name && registration.Environment == m.Environment {
			copy := registration
			existing = &copy
			continue
		}
		result = append(result, registration)
	}
	if present {
		r := Registration{Application: m.Name, Environment: m.Environment}
		if existing != nil {
			r.SyslogPort = existing.SyslogPort
			r.ProviderSyslogPort = existing.ProviderSyslogPort
		}
		if r.SyslogPort == 0 {
			r.SyslogPort, err = allocatePort("udp")
			if err != nil {
				return nil, err
			}
		}
		if r.ProviderSyslogPort == 0 {
			r.ProviderSyslogPort, err = allocatePort("udp")
			if err != nil {
				return nil, err
			}
		}
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Application == result[j].Application {
			return result[i].Environment < result[j].Environment
		}
		return result[i].Application < result[j].Application
	})
	if !present && errors.Is(err, os.ErrNotExist) {
		return result, os.ErrNotExist
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return result, nil
}

func readRegistrations(path string) ([]Registration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var registrations []Registration
	if err := json.Unmarshal(data, &registrations); err != nil {
		return nil, fmt.Errorf("decode Loki registrations: %w", err)
	}
	for _, r := range registrations {
		if strings.TrimSpace(r.Application) == "" || strings.TrimSpace(r.Environment) == "" ||
			r.SyslogPort < 1 || r.SyslogPort > 65535 ||
			r.ProviderSyslogPort < 1 || r.ProviderSyslogPort > 65535 {
			return nil, errors.New("invalid Loki registration state")
		}
	}
	return registrations, nil
}

func hasPlatformProviderLogs(sources []observability.SignalSource) bool {
	for _, source := range sources {
		if source.Kind == observability.SignalLogs && source.Class == observability.SourcePlatformProvider {
			return true
		}
	}
	return false
}

func persistedPort(path, key string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && k == key {
			port, err := strconv.Atoi(strings.TrimSpace(v))
			if err == nil && port > 0 && port <= 65535 {
				return port, nil
			}
			return 0, fmt.Errorf("invalid %s port", key)
		}
	}
	return 0, fmt.Errorf("%s is not materialized", key)
}

func persistedOrAllocatedUDPPort(path, key string) (int, error) {
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && k == key {
				port, err := strconv.Atoi(strings.TrimSpace(v))
				if err == nil && port > 0 && port <= 65535 {
					return port, nil
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return allocatePort("udp")
}

func persistedOrAllocatedPort(path, key string) (int, error) {
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && k == key {
				port, err := strconv.Atoi(strings.TrimSpace(v))
				if err == nil && port > 0 && port <= 65535 {
					return port, nil
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return allocatePort("tcp")
}

func allocatePort(network string) (int, error) {
	if network == "tcp" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		defer listener.Close()
		return listener.Addr().(*net.TCPAddr).Port, nil
	}
	if network == "udp" {
		listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
		if err != nil {
			return 0, err
		}
		defer listener.Close()
		return listener.LocalAddr().(*net.UDPAddr).Port, nil
	}
	return 0, fmt.Errorf("unsupported port allocation network %q", network)
}
