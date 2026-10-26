package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveNewApplicationRoot(name, parent string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("application name is required")
	}
	if strings.TrimSpace(parent) != "" {
		parent, err := expandUserPath(parent)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(parent) {
			abs, err := filepath.Abs(parent)
			if err != nil {
				return "", err
			}
			parent = abs
		}
		return filepath.Join(parent, name), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if filepath.Base(filepath.Clean(cwd)) != name {
		return "", usageError("project location is ambiguous", "Use --directory <parent> so BaseHarbor can create <parent>/"+name+".")
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return "", err
	}
	if len(entries) != 0 {
		return "", usageError("current directory is not empty", "Use --directory <parent> to choose a new project location.")
	}
	return cwd, nil
}

func expandUserPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if value == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(value, "~/")), nil
	}
	return value, nil
}

func displayUserPath(value string) string {
	if value == "" {
		return value
	}
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	return shellDisplayPath(value)
}
