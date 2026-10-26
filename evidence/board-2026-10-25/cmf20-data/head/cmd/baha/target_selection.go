package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func targetSelectionPath() (string, error) {
	configPath, err := deployment.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "active-target"), nil
}

func readPersistedTarget() (string, error) {
	path, err := targetSelectionPath()
	if err != nil {
		return "", err
	}
	data, err := runtimeMutationOwnerBytes(path, true)
	if err != nil {
		return "", err
	}
	if data == nil {
		return "", nil
	}
	if len(data) > 1024 {
		return "", runtimeMutationOwnerError()
	}
	name := strings.TrimSpace(string(data))
	if detached, ok := targetSelectionDetached(data); ok {
		return "", targetSelectionDetachedError(detached)
	}
	if err := deployment.ValidateTargetName(name); err != nil {
		return "", fmt.Errorf("invalid persisted target: %w", err)
	}
	return name, nil
}

func writePersistedTarget(name string) error {
	if err := deployment.ValidateTargetName(name); err != nil {
		return err
	}
	return mutatePersistedTarget(name, false)
}

func clearPersistedTarget() error {
	return mutatePersistedTarget("", true)
}

func selectedTargetName(explicit, environment string, cfg deployment.Config) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if environment != "" {
		return environment, nil
	}
	persisted, err := readPersistedTarget()
	if err != nil {
		return "", err
	}
	if persisted != "" {
		if _, ok := cfg.Targets[persisted]; !ok && persisted != "local" {
			return "", machine.NewError(machine.ErrorNotFound, fmt.Sprintf("activated target %q no longer exists", persisted), "Run baha target deactivate and choose an existing target.", false)
		}
		return persisted, nil
	}
	if cfg.DefaultTarget != "" {
		return cfg.DefaultTarget, nil
	}
	if len(cfg.Targets) == 1 {
		for name := range cfg.Targets {
			return name, nil
		}
	}
	if len(cfg.Targets) > 1 {
		return "", machine.NewError(machine.ErrorConflict, "multiple deployment targets configured", "Run baha target list, then baha target activate NAME or provide --target NAME.", false)
	}
	return "", nil
}

func targetSelectionOrigin(ctx context.Context) string {
	if targetOverrideFromContext(ctx) != "" {
		return "explicit"
	}
	if strings.TrimSpace(os.Getenv("BASEHARBOR_TARGET")) != "" {
		return "environment"
	}
	persisted, err := readPersistedTarget()
	if err == nil && persisted != "" {
		return "persisted"
	}
	cfg, err := deployment.LoadConfig()
	if err == nil {
		if cfg.DefaultTarget != "" {
			return "configured-default"
		}
		if len(cfg.Targets) == 1 {
			return "unique-configured"
		}
	}
	return "implicit-local"
}

func persistedTargetIsActive() bool {
	name, err := readPersistedTarget()
	return err == nil && name != ""
}
