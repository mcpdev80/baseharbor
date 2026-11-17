package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

// reconcileTargetDevelopmentCredentialSecrets explicitly projects the
// Target's OpenBao-authoritative development login into app-owned OpenBao keys
// declared with a managed source. This is opt-in at the application's
// contract, dev-only, and reconciled on every apply so credential rotations
// reach the workload on the next start.
func reconcileTargetDevelopmentCredentialSecrets(
	ctx context.Context,
	secrets *applicationsecret.Service,
	target string,
	manifest application.Manifest,
) (int, error) {
	var sourced []application.SecretRequirement
	for _, requirement := range manifest.Secrets.Required {
		if requirement.Source != "" {
			sourced = append(sourced, requirement)
		}
	}
	if len(sourced) == 0 {
		return 0, nil
	}
	if !devaccess.Enabled(manifest.Environment) {
		return 0, errors.New("Target development credentials may be projected only into dev applications")
	}
	if secrets == nil {
		return 0, errors.New("application secret scope is unavailable")
	}
	credentials, err := ensureAuthoritativeDeveloperCredentials(ctx, target, manifest.Environment)
	if err != nil {
		return 0, fmt.Errorf("read Target credentials from OpenBao: %w", err)
	}
	for _, requirement := range sourced {
		value, err := targetDevelopmentCredentialSecretValue(requirement.Source, credentials)
		if err != nil {
			return 0, err
		}
		current, getErr := secrets.Get(ctx, manifest.Name, requirement.Name)
		if getErr == nil && bytes.Equal(current, value) {
			clear(current)
			clear(value)
			continue
		}
		if getErr != nil && !errors.Is(getErr, openbao.ErrApplicationSecretNotFound) {
			clear(value)
			return 0, fmt.Errorf("read application secret %s before synchronization: %w", requirement.Name, getErr)
		}
		if err := secrets.Set(ctx, manifest.Name, requirement.Name, value); err != nil {
			clear(value)
			return 0, fmt.Errorf("synchronize application secret %s: %w", requirement.Name, err)
		}
		clear(current)
		current, err = secrets.Get(ctx, manifest.Name, requirement.Name)
		if err != nil {
			clear(value)
			return 0, fmt.Errorf("verify synchronized application secret %s: %w", requirement.Name, err)
		}
		matches := bytes.Equal(current, value)
		clear(current)
		clear(value)
		if !matches {
			return 0, fmt.Errorf("verify synchronized application secret %s: stored value does not match Target OpenBao", requirement.Name)
		}
	}
	return len(sourced), nil
}

func targetDevelopmentCredentialSecretValue(source application.SecretSource, credentials devaccess.Credentials) ([]byte, error) {
	switch source {
	case application.SecretSourceTargetDevelopmentUsername:
		if credentials.Username == "" {
			return nil, errors.New("Target development username is empty")
		}
		return []byte(credentials.Username), nil
	case application.SecretSourceTargetDevelopmentPassword:
		if credentials.Password == "" {
			return nil, errors.New("Target development password is empty")
		}
		return []byte(credentials.Password), nil
	default:
		return nil, fmt.Errorf("unsupported managed secret source %q", source)
	}
}

func hasManagedDevelopmentSecretSources(manifest application.Manifest) bool {
	for _, requirement := range manifest.Secrets.Required {
		if requirement.Source != "" {
			return true
		}
	}
	return false
}

func manifestWithoutManagedSecretSources(manifest application.Manifest) application.Manifest {
	filtered := manifest
	filtered.Secrets.Required = make([]application.SecretRequirement, 0, len(manifest.Secrets.Required))
	for _, requirement := range manifest.Secrets.Required {
		if requirement.Source == "" {
			filtered.Secrets.Required = append(filtered.Secrets.Required, requirement)
		}
	}
	return filtered
}
