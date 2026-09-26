package main

import (
	"context"
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
		if path := strings.TrimSpace(def.OpenBaoRecoveryFile); path != "" {
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
			cfg.Access[accessRef] = deployment.AccessDefinition{
				Provider:  target.RuntimeProvider,
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
	def.OpenBaoRecoveryFile = abs
	cfg.Targets[target.Name] = def
	return cfg.Save()
}
