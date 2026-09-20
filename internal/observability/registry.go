package observability

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type SourceClass string

const (
	SourceApplicationProvider SourceClass = "application-provider"
	SourcePlatformProvider    SourceClass = "platform-provider"
)

type MetricsSource struct {
	ID               string                   `json:"id"`
	Provider         capability.ProviderKind  `json:"provider"`
	Class            SourceClass              `json:"class"`
	Scope            capability.ProviderScope `json:"scope"`
	SharingBoundary  string                   `json:"sharing_boundary,omitempty"`
	OwnerApplication string                   `json:"owner_application,omitempty"`
	Network          string                   `json:"network"`
	Target           string                   `json:"target"`
	Path             string                   `json:"path"`
}

func (s MetricsSource) Validate() error {
	if strings.TrimSpace(s.ID) == "" || s.Provider == "" {
		return errors.New("observability metrics source requires id and provider")
	}
	if s.Class != SourceApplicationProvider && s.Class != SourcePlatformProvider {
		return fmt.Errorf("unsupported observability source class %q", s.Class)
	}
	if s.Scope != capability.ScopeShared && s.Scope != capability.ScopeApplication {
		return fmt.Errorf("unsupported observability source scope %q", s.Scope)
	}
	if s.Scope == capability.ScopeApplication && strings.TrimSpace(s.OwnerApplication) == "" {
		return errors.New("application-scoped observability source requires owner application")
	}
	if strings.TrimSpace(s.Network) == "" || strings.TrimSpace(s.Target) == "" || !strings.HasPrefix(strings.TrimSpace(s.Path), "/") {
		return errors.New("observability metrics source requires network, target and absolute path")
	}
	return nil
}

func registryPath() (string, error) {
	dir, err := bhruntime.DataDir("")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "observability", "metrics-sources.json"), nil
}

func Update(source MetricsSource) error {
	if err := source.Validate(); err != nil {
		return err
	}
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []MetricsSource) ([]MetricsSource, error) {
		out := sources[:0]
		for _, existing := range sources {
			if existing.ID != source.ID {
				out = append(out, existing)
			}
		}
		out = append(out, source)
		return out, nil
	})
}

func Remove(id string) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	return mutate(path, func(sources []MetricsSource) ([]MetricsSource, error) {
		out := sources[:0]
		for _, source := range sources {
			if source.ID != id {
				out = append(out, source)
			}
		}
		return out, nil
	})
}

func ListMetrics(placement capability.ProviderPlacement, applications []string, includeApplicationProviders, includePlatformProviders bool) ([]MetricsSource, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	sources, err := read(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	allowedApplications := map[string]struct{}{}
	for _, application := range applications {
		if application = strings.TrimSpace(application); application != "" {
			allowedApplications[application] = struct{}{}
		}
	}
	var out []MetricsSource
	for _, source := range sources {
		switch source.Class {
		case SourceApplicationProvider:
			if !includeApplicationProviders {
				continue
			}
		case SourcePlatformProvider:
			if !includePlatformProviders {
				continue
			}
		}
		switch placement.Scope {
		case capability.ScopeShared:
			if source.Scope == capability.ScopeShared && strings.TrimSpace(source.SharingBoundary) == strings.TrimSpace(placement.SharingBoundary) {
				out = append(out, source)
			} else if source.Scope == capability.ScopeApplication {
				if _, allowed := allowedApplications[source.OwnerApplication]; allowed {
					out = append(out, source)
				}
			}
		case capability.ScopeApplication:
			if source.Scope == capability.ScopeApplication {
				if _, allowed := allowedApplications[source.OwnerApplication]; allowed {
					out = append(out, source)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func mutate(path string, fn func([]MetricsSource) ([]MetricsSource, error)) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	sources, err := read(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sources, err = fn(sources)
	if err != nil {
		return err
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID < sources[j].ID })
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func read(path string) ([]MetricsSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sources []MetricsSource
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("decode observability metrics source registry: %w", err)
	}
	for _, source := range sources {
		if err := source.Validate(); err != nil {
			return nil, fmt.Errorf("invalid observability metrics source %q: %w", source.ID, err)
		}
	}
	return sources, nil
}
