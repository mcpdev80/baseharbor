package main

import (
	"context"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func reconcileGeneratedApplicationSecrets(
	ctx context.Context,
	compose bhruntime.Compose,
	platformFiles bhruntime.Files,
	m application.Manifest,
	files application.RuntimeFiles,
) ([]string, error) {
	generated := application.GeneratedSecretRequirements(m)
	if len(generated) == 0 {
		return nil, nil
	}

	statuses, err := inspectRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
	if err != nil {
		return nil, err
	}
	statusByName := make(map[string]openbao.RequiredSecretStatus, len(statuses))
	for _, status := range statuses {
		statusByName[status.Name] = status
	}

	identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
	credentialsPath := openbao.ApplicationCredentialsPath(files.Dir)
	var created []string
	for _, requirement := range generated {
		status := statusByName[requirement.Name]
		if status.Present {
			continue
		}
		value, err := application.GenerateSecretValue(*requirement.Generate)
		if err != nil {
			return nil, fmt.Errorf("generate application secret %s: %w", requirement.Name, err)
		}
		if err := openbao.SetApplicationSecret(ctx, compose, platformFiles, identity, credentialsPath, requirement.Name, value); err != nil {
			clear(value)
			return nil, fmt.Errorf("store generated application secret %s: %w", requirement.Name, err)
		}
		clear(value)
		created = append(created, requirement.Name)
	}
	return created, nil
}
