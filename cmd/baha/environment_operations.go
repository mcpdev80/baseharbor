package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/application"
	"os"
	"path/filepath"
)

type applicationEnvironmentResult struct {
	Application string            `json:"application"`
	Environment string            `json:"environment"`
	Path        string            `json:"path"`
	Values      map[string]string `json:"values"`
}

func inspectApplicationEnvironment(ctx context.Context, resolved resolvedApplication, reveal bool) (applicationEnvironmentResult, error) {
	if err := authorizeApplicationOperation(ctx, "app.environment", resolved); err != nil {
		return applicationEnvironmentResult{}, err
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return applicationEnvironmentResult{}, err
	}
	if _, err := os.Stat(files.ApplicationEnv); err != nil {
		return applicationEnvironmentResult{}, err
	}
	values, err := loadApplicationEnv(files.ApplicationEnv)
	if err != nil {
		return applicationEnvironmentResult{}, err
	}
	if !reveal {
		maskRuntimeSecrets(values)
	}
	path, err := filepath.Abs(files.ApplicationEnv)
	if err != nil {
		return applicationEnvironmentResult{}, err
	}
	return applicationEnvironmentResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Path: path, Values: values}, nil
}

type applicationConnectionResult struct {
	Resource string `json:"resource"`
	Instance string `json:"instance"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Database string `json:"database,omitempty"`
	Username string `json:"username,omitempty"`
}

func publicApplicationConnection(kind string, binding application.ServiceBinding) applicationConnectionResult {
	return applicationConnectionResult{Resource: kind, Instance: binding.Instance, Host: binding.Host, Port: binding.Port, Database: binding.Database, Username: binding.Username}
}
