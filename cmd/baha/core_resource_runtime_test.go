package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/hostresource"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type coreMemorySample struct {
	At       time.Time                   `json:"at"`
	Phase    string                      `json:"phase"`
	Bytes    map[string]uint64           `json:"capability_bytes"`
	Services map[string]uint64           `json:"service_bytes"`
	Total    uint64                      `json:"total_bytes"`
	Host     hostresource.MemoryEvidence `json:"host"`
}

type coreMemorySampler struct {
	mu       sync.Mutex
	samples  []coreMemorySample
	lastErr  error
	ready    bool
	done     chan struct{}
	cancel   context.CancelFunc
	runtime  bhruntime.RuntimeProvider
	engine   string
	projects map[string]bool
}

func startCoreMemorySampler(ctx context.Context, runtime bhruntime.RuntimeProvider, engine string, projects ...string) *coreMemorySampler {
	ctx, cancel := context.WithCancel(ctx)
	s := &coreMemorySampler{done: make(chan struct{}), cancel: cancel, runtime: runtime, engine: engine, projects: map[string]bool{}}
	for _, project := range projects {
		s.projects[project] = true
	}
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			s.mu.Lock()
			ready := s.ready
			s.mu.Unlock()
			sample, err := s.sample(ctx, ready)
			s.mu.Lock()
			if err != nil {
				s.lastErr = err
			} else if sample.Total > 0 {
				s.samples = append(s.samples, sample)
			}
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return s
}

func (s *coreMemorySampler) stop() { s.cancel(); <-s.done }

func parseNativeMemory(value string) (uint64, error) {
	value = strings.TrimSpace(strings.SplitN(value, "/", 2)[0])
	for _, unit := range []struct {
		name       string
		multiplier float64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}} {
		if strings.HasSuffix(value, unit.name) {
			n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, unit.name)), 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1e15 {
				return 0, errors.New("invalid native memory sample")
			}
			return uint64(n * unit.multiplier), nil
		}
	}
	return 0, errors.New("unsupported native memory unit")
}

func (s *coreMemorySampler) sample(ctx context.Context, ready bool) (coreMemorySample, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	sample := coreMemorySample{At: time.Now().UTC(), Phase: "startup/convergence", Bytes: map[string]uint64{}, Services: map[string]uint64{}}
	if ready {
		sample.Phase = "idle"
	}
	containers, err := s.runtime.ListRuntimeContainers(ctx)
	if err != nil {
		return sample, err
	}
	var ids []string
	services := map[string]string{}
	for _, c := range containers {
		if c.Running && s.projects[c.Project] {
			ids = append(ids, c.ID)
			services[c.ID] = c.Service
		}
	}
	if len(ids) == 0 {
		return sample, nil
	}
	args := append([]string{"stats", "--no-stream", "--format", "{{.ID}}|{{.MemUsage}}"}, ids...)
	output, err := exec.CommandContext(ctx, s.engine, args...).Output()
	if err != nil {
		return sample, errors.New("native Core memory statistics unavailable")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		id, usage, ok := strings.Cut(line, "|")
		if !ok {
			return sample, errors.New("invalid native statistics row")
		}
		service := services[id]
		if service == "" {
			for full, name := range services {
				if strings.HasPrefix(full, id) {
					service = name
					break
				}
			}
		}
		if service == "" {
			return sample, errors.New("memory sample belongs to an unknown resource")
		}
		bytes, err := parseNativeMemory(usage)
		if err != nil {
			return sample, err
		}
		capability := "sql"
		if strings.HasPrefix(service, "keycloak") {
			capability = "identity"
		} else if strings.HasPrefix(service, "openbao") {
			capability = "secrets"
		}
		sample.Services[service] = bytes
		sample.Bytes[capability] += bytes
		sample.Total += bytes
	}
	if len(sample.Services) != len(ids) {
		return sample, errors.New("Core memory sample is incomplete")
	}
	sample.Host, err = hostresource.ReadLinux()
	return sample, err
}

func (s *coreMemorySampler) evidence(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for {
		s.mu.Lock()
		samples := append([]coreMemorySample(nil), s.samples...)
		lastErr := s.lastErr
		s.mu.Unlock()
		var idle []coreMemorySample
		var peak uint64
		for _, sample := range samples {
			if sample.Phase == "idle" {
				idle = append(idle, sample)
			} else if sample.Total > peak {
				peak = sample.Total
			}
		}
		if len(idle) >= 11 && idle[len(idle)-1].At.Sub(idle[len(idle)-11].At) >= 30*time.Second {
			window := idle[len(idle)-11:]
			minimum, maximum := window[0].Total, window[0].Total
			complete := true
			for _, sample := range window {
				if sample.Total < minimum {
					minimum = sample.Total
				}
				if sample.Total > maximum {
					maximum = sample.Total
				}
			}
			for _, sample := range window {
				if sample.Bytes["sql"] == 0 || sample.Bytes["secrets"] == 0 || sample.Bytes["identity"] == 0 || len(sample.Services) != len(window[0].Services) {
					complete = false
				}
				for name := range window[0].Services {
					if sample.Services[name] == 0 {
						complete = false
					}
				}
			}
			if complete && minimum > 0 && maximum-minimum <= minimum/10 && peak > 0 {
				metadata, err := s.metadata(ctx)
				if err != nil {
					return nil, err
				}
				return json.Marshal(map[string]any{"schema": "baseharbor.core-resource-samples/v1", "runtime": s.engine, "placement": "installation-shared", "capabilities": []string{"PostgreSQL", "OpenBao", "Keycloak"}, "containers": metadata, "accounting": "native container memory usage; runtime-specific cache accounting", "sampling": "3 second target interval; exact observation timestamps retained", "startup_convergence_sampled_peak_bytes": peak, "idle_stabilized": window, "samples": samples})
			}
		}
		select {
		case <-deadline.Done():
			return nil, fmt.Errorf("Core memory did not stabilize with complete samples: %w", errors.Join(deadline.Err(), lastErr))
		case <-ticker.C:
		}
	}
}

func (s *coreMemorySampler) metadata(ctx context.Context) ([]map[string]any, error) {
	containers, err := s.runtime.ListRuntimeContainers(ctx)
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for _, c := range containers {
		if !c.Running || !s.projects[c.Project] {
			continue
		}
		commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		output, err := exec.CommandContext(commandCtx, s.engine, "inspect", "--format", "{{json .Config.Image}}|{{json .Image}}|{{json .HostConfig.Memory}}", c.ID).Output()
		cancel()
		if err != nil {
			return nil, errors.New("Core image/limit metadata unavailable")
		}
		parts := strings.Split(strings.TrimSpace(string(output)), "|")
		if len(parts) != 3 {
			return nil, errors.New("invalid Core resource metadata")
		}
		var image, imageID string
		var limit uint64
		if json.Unmarshal([]byte(parts[0]), &image) != nil || json.Unmarshal([]byte(parts[1]), &imageID) != nil || json.Unmarshal([]byte(parts[2]), &limit) != nil {
			return nil, errors.New("invalid Core image/limit metadata")
		}
		result = append(result, map[string]any{"service": c.Service, "image_reference": image, "immutable_image_id": imageID, "memory_limit_bytes": limit})
	}
	return result, nil
}

func TestCoreNativeMemoryRejectsInvalidMeasurements(t *testing.T) {
	for _, value := range []string{"NaNMiB", "+InfGiB", "-1MiB", "unknown", "10000000000000000GiB"} {
		if _, err := parseNativeMemory(value); err == nil {
			t.Fatalf("invalid native measurement accepted: %q", value)
		}
	}
	for value, want := range map[string]uint64{"1MiB / 7GiB": 1 << 20, "1.5GiB / 8GiB": 3 << 29, "10MB / 100MB": 10000000} {
		got, err := parseNativeMemory(value)
		if err != nil || got != want {
			t.Fatalf("native memory units differ: %q got=%d want=%d err=%v", value, got, want, err)
		}
	}
}
