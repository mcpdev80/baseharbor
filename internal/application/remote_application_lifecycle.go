package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// ApplicationPhases is validated against the retained publication and intent.
// Neither current inventory nor a modified checkout can add an executable phase.
func (r *RemoteManagedRuntime) ApplicationPhases(manifest Manifest) ([]string, []string, error) {
	if r == nil || r.runtime == nil {
		return nil, nil, errors.New("remote Application runtime is unavailable")
	}
	var phases remoteApplicationPhases
	var found bool
	for _, file := range r.source {
		if file.Path != remoteApplicationPhasesFile {
			continue
		}
		if found || json.Unmarshal(file.Data, &phases) != nil {
			return nil, nil, errors.New("invalid retained Application phase metadata")
		}
		found = true
	}
	if !found || phases.Version != 1 || phases.Intent != remoteApplicationIntent(manifest) {
		return nil, nil, errors.New("retained remote Application intent differs")
	}
	names, err := r.ServiceNames()
	if err != nil {
		return nil, nil, err
	}
	declared := map[string]bool{}
	for _, name := range names {
		declared[name] = true
	}
	seen := map[string]bool{}
	for _, group := range [][]string{phases.Providers, phases.Workloads} {
		for _, name := range group {
			if !declared[name] || seen[name] {
				return nil, nil, errors.New("retained Application phase service differs")
			}
			seen[name] = true
		}
	}
	if len(seen) != len(declared) {
		return nil, nil, errors.New("retained Application phases omit services")
	}
	return append([]string(nil), phases.Providers...), append([]string(nil), phases.Workloads...), nil
}

// ApplyApplication verifies provider readiness before starting repository code.
// A repair of a stopped workload leaves healthy provider containers untouched.
func (r *RemoteManagedRuntime) ApplyApplication(ctx context.Context, manifest Manifest, repair bool) error {
	providers, workloads, err := r.ApplicationPhases(manifest)
	if err != nil {
		return err
	}
	if len(providers) > 0 {
		if !repair || r.verifyApplicationProviders(ctx, manifest, providers) != nil {
			if err := r.ApplyServices(ctx, providers, repair); err != nil {
				return err
			}
		}
		if err := waitRemoteApplication(ctx, func(ctx context.Context) error { return r.verifyApplicationProviders(ctx, manifest, providers) }); err != nil {
			return err
		}
	}
	if len(workloads) > 0 {
		if err := r.ApplyServices(ctx, workloads, repair); err != nil {
			return err
		}
	}
	return waitRemoteApplication(ctx, func(ctx context.Context) error { return r.VerifyApplication(ctx, manifest) })
}

func waitRemoteApplication(ctx context.Context, verify func(context.Context) error) error {
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := verify(bounded)
		if err == nil {
			return nil
		}
		if errors.Is(err, targetsession.ErrUnavailable) {
			return err
		}
		select {
		case <-bounded.Done():
			return fmt.Errorf("remote Application readiness failed: %w", errors.Join(bounded.Err(), err))
		case <-ticker.C:
		}
	}
}

func (r *RemoteManagedRuntime) verifyApplicationProviders(ctx context.Context, manifest Manifest, providers []string) error {
	if err := r.verifyServices(ctx, providers); err != nil {
		return err
	}
	probe := NewRemoteBackendProbeExecutor(r, r.name)
	if manifest.Services.SQL {
		if err := VerifyPostgresProvider(ctx, probe, manifest); err != nil {
			return err
		}
	}
	if manifest.Services.Cache || manifest.Services.KeyValue {
		if err := VerifyValkeyProvider(ctx, probe, manifest); err != nil {
			return err
		}
	}
	return nil
}

func (r *RemoteManagedRuntime) VerifyApplication(ctx context.Context, manifest Manifest) error {
	providers, workloads, err := r.ApplicationPhases(manifest)
	if err != nil {
		return err
	}
	if err := r.verifyApplicationProviders(ctx, manifest, providers); err != nil {
		return err
	}
	return r.verifyServices(ctx, workloads)
}

func (r *RemoteManagedRuntime) verifyServices(ctx context.Context, selected []string) error {
	if len(selected) == 0 {
		return nil
	}
	observed, err := r.Observe(ctx)
	if err != nil {
		return err
	}
	byName := map[string]targetsession.ProjectService{}
	for _, service := range observed {
		if _, duplicate := byName[service.Service]; duplicate {
			return errors.New("ambiguous remote Application service")
		}
		byName[service.Service] = service
	}
	for _, name := range selected {
		service, found := byName[name]
		if !found || !service.Running || (service.Health != "" && !strings.EqualFold(service.Health, "healthy")) {
			return fmt.Errorf("remote Application service %s is not ready", name)
		}
	}
	return nil
}

func (r *RemoteManagedRuntime) DestroyApplication(ctx context.Context, manifest Manifest, reset bool) error {
	if _, _, err := r.ApplicationPhases(manifest); err != nil {
		return err
	}
	if err := r.DestroyOwned(ctx, reset); err != nil {
		return err
	}
	remaining, err := r.Observe(ctx)
	if err != nil {
		return err
	}
	if len(remaining) != 0 {
		return errors.New("owned remote Application containers remain after destruction")
	}
	return nil
}
