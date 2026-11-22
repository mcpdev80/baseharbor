package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// targetSelectionPath is the one seam where the persisted Target selection
// resolves. BASEHARBOR_STATE_DIR is the authoritative isolated installation
// root, so with it set the selection lives below that root — beside the
// Target configuration it chooses between (BF-01, h-d28 akte;
// docs/explanation/targets.md: an isolated installation cannot discover or
// rewrite the normal user's Target selection). Without the override the
// selection is the user-wide activation it has always been, resolving
// through the user configuration locations.
func targetSelectionPath() (string, error) {
	selectionRoot, err := deployment.TargetSelectionRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(selectionRoot, "active-target"), nil
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
	if len(cfg.Targets) == 1 && !deployment.StateRootSelected() {
		// A single registration implies its own selection — the first-run
		// convenience of the user installation. Under a dedicated state
		// root BF-01 forbids the shortcut: an implicit choice would let a
		// lower layer (the builtin fallback, or a foreign installation's
		// organization default) decide what this root acts on; the
		// root-local organization state answers explicitly instead (see
		// resolveTargetWithOrganization).
		for name := range cfg.Targets {
			return name, nil
		}
	}
	if len(cfg.Targets) > 1 {
		choices := cfg.TargetNames()
		sort.Strings(choices)
		return "", machine.NewError(machine.ErrorConflict,
			fmt.Sprintf("multiple deployment targets configured; choose one of: %s", strings.Join(choices, ", ")),
			"Run baha target activate NAME, or set BASEHARBOR_TARGET=NAME for this command. Use baha target list to inspect available targets.", false)
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
