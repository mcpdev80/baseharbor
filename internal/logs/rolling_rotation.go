package logs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type lokiSelectedRuntime interface {
	UpProjectFilesSelectedForceRecreateNoBuild(context.Context, string, string, map[string]string, []string, ...string) error
	ExecProject(context.Context, string, string, string, string, ...string) (string, error)
}

func lokiRotationEnvironment(files ProviderFiles) (map[string]string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, errors.New("invalid Loki rotation environment")
		}
		env[key] = value
	}
	return env, nil
}

// Keep two members alive; a gateway response alone cannot prove the restarted
// member is ready. Probe the exact member from the owned gateway network.
func rollLokiStorageMembers(ctx context.Context, runtime Runtime, placement Placement, files ProviderFiles) error {
	selected, ok := runtime.(lokiSelectedRuntime)
	if !ok {
		return errors.New("Loki HA rotation requires selected runtime reconciliation")
	}
	env, err := lokiRotationEnvironment(files)
	if err != nil {
		return err
	}
	gateway := "loki-access"
	if placement.OwnerApplication != "" {
		gateway = "baseharbor-internal-loki-access"
	}
	for _, member := range []string{"loki-1", "loki-2", "loki-3"} {
		if err := selected.UpProjectFilesSelectedForceRecreateNoBuild(ctx, placement.Project, files.Dir, env, []string{member}, files.Compose); err != nil {
			return fmt.Errorf("reconcile Loki member %s: %w", member, err)
		}
		readyCtx, cancel := context.WithTimeout(ctx, time.Minute)
		err := waitLokiMember(readyCtx, selected, placement.Project, files, gateway, member)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func waitLokiMember(ctx context.Context, runtime lokiSelectedRuntime, project string, files ProviderFiles, gateway, member string) error {
	for {
		_, err := runtime.ExecProject(ctx, project, files.Compose, files.Env, gateway, "wget", "-q", "-T", "2", "-O", "/dev/null", "http://"+member+":3100/ready")
		if err == nil {
			return nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("Loki member %s readiness: %w", member, ctx.Err())
		case <-timer.C:
		}
	}
}

func reconcileLokiAccessRuntime(ctx context.Context, runtime Runtime, placement Placement, files ProviderFiles, gateway string, ha bool) error {
	if !ha {
		return runtime.UpProject(ctx, placement.Project, files.Compose, files.Env)
	}
	selected, ok := runtime.(lokiSelectedRuntime)
	if !ok {
		return errors.New("Loki HA PKI rotation requires selected runtime reconciliation")
	}
	env, err := lokiRotationEnvironment(files)
	if err != nil {
		return err
	}
	alloy := "alloy"
	if placement.OwnerApplication != "" {
		alloy = "baseharbor-internal-alloy"
	}
	return selected.UpProjectFilesSelectedForceRecreateNoBuild(ctx, placement.Project, files.Dir, env, []string{gateway, alloy}, files.Compose)
}
