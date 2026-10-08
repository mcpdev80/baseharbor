package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func defaultTargetRecoveryFile(target string) (string, error) {
	if err := deployment.ValidateTargetName(target); err != nil {
		return "", err
	}
	root := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return "", errors.New("cannot determine OpenBao recovery directory")
		}
		root = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(root, "baseharbor-recovery", target, "openbao-recovery.json"), nil
}

func preflightNewTargetRecoveryFile(ctx context.Context, explicit string) (string, string, error) {
	path, source, err := resolveTargetRecoveryFile(ctx, explicit)
	if err != nil {
		return "", "", err
	}
	info, statErr := os.Stat(path)
	if statErr == nil {
		if source == "target default" && !info.IsDir() {
			// A destroyed installation's recovery material remains operator-owned.
			// Allocate a new output name, never delete or overwrite the old file.
			var suffix [16]byte
			if _, err := rand.Read(suffix[:]); err != nil {
				return "", "", fmt.Errorf("allocate fresh recovery output name: %w", err)
			}
			fresh := filepath.Join(filepath.Dir(path), "openbao-recovery-"+hex.EncodeToString(suffix[:])+".json")
			if _, err := os.Lstat(fresh); !errors.Is(err, os.ErrNotExist) {
				return "", "", errors.New("fresh recovery output path is unavailable")
			}
			return fresh, "target default (fresh installation)", nil
		}
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		return "", "", usageError(
			fmt.Sprintf("OpenBao recovery output %s already exists at %s", kind, path),
			fmt.Sprintf("Use a fresh path with 'baha up --recovery-file PATH'. BaseHarbor never overwrites recovery material. No control-plane resources were changed. Existing recovery material at %s was preserved.", path),
		)
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect OpenBao recovery output path %s: %w", path, statErr)
	}
	return path, source, nil
}

func resolveTargetRecoveryFile(ctx context.Context, explicit string) (string, string, error) {
	if path := strings.TrimSpace(explicit); path != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", "", fmt.Errorf("resolve explicit OpenBao recovery file: %w", err)
		}
		return abs, "explicit", nil
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return "", "", err
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return "", "", err
	}
	if def, ok := cfg.Targets[target.Name]; ok {
		if path := strings.TrimSpace(def.OpenBao.RecoveryFile); path != "" {
			abs, err := filepath.Abs(path)
			if err != nil {
				return "", "", fmt.Errorf("resolve persisted OpenBao recovery file for target %s: %w", target.Name, err)
			}
			return abs, "persisted target", nil
		}
	}
	path, err := defaultTargetRecoveryFile(target.Name)
	if err != nil {
		return "", "", err
	}
	return path, "target default", nil
}

func persistTargetRecoveryFileReference(ctx context.Context, recoveryFile string) error {
	path := strings.TrimSpace(recoveryFile)
	if path == "" {
		return errors.New("OpenBao recovery file path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	def, ok := cfg.Targets[target.Name]
	if !ok {
		accessRef := strings.TrimSpace(target.AccessReference)
		if accessRef == "" {
			accessRef = target.Name
		}
		if _, exists := cfg.Access[accessRef]; !exists {
			accessProvider := strings.TrimSpace(target.AccessProvider)
			if accessProvider == "" {
				accessProvider = "local"
			}
			cfg.Access[accessRef] = deployment.AccessDefinition{
				Provider:  accessProvider,
				Reference: target.AccessReference,
			}
			if strings.TrimSpace(cfg.Access[accessRef].Reference) == "" {
				access := cfg.Access[accessRef]
				access.Reference = accessRef
				cfg.Access[accessRef] = access
			}
		}
		def = deployment.TargetDefinition{
			Runtime: deployment.RuntimeDefinition{Provider: target.RuntimeProvider},
			Access:  deployment.TargetAccess{Reference: accessRef},
			Scope:   target.Scope,
		}
	}
	def.OpenBao.RecoveryFile = abs
	cfg.Targets[target.Name] = def
	return cfg.Save()
}
