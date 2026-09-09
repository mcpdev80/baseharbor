package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func inspectRequiredApplicationSecrets(
	ctx context.Context,
	compose bhruntime.Compose,
	platformFiles bhruntime.Files,
	m application.Manifest,
	files application.RuntimeFiles,
) ([]openbao.RequiredSecretStatus, error) {
	required := application.RequiredSecretNames(m)
	if len(required) == 0 {
		return nil, nil
	}
	if !m.Services.Secrets {
		return nil, fmt.Errorf("required secrets are declared but managed secrets are disabled")
	}
	identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
	statuses, err := openbao.InspectRequiredApplicationSecrets(
		ctx,
		compose,
		platformFiles,
		identity,
		openbao.ApplicationCredentialsPath(files.Dir),
		required,
	)
	if err != nil {
		return nil, err
	}
	for i := range statuses {
		requirement, ok := application.SecretRequirementByName(m, statuses[i].Name)
		statuses[i].Generated = ok && requirement.Generate != nil
	}
	return statuses, nil
}

func checkRequiredApplicationSecrets(
	ctx context.Context,
	compose bhruntime.Compose,
	platformFiles bhruntime.Files,
	m application.Manifest,
	files application.RuntimeFiles,
) error {
	statuses, err := inspectRequiredApplicationSecrets(ctx, compose, platformFiles, m, files)
	if err != nil {
		return err
	}
	if err := openbao.RequireApplicationSecrets(statuses); err != nil {
		return fmt.Errorf("%w; run 'baha app preflight' for the exact safe remediation command for each secret", err)
	}
	return nil
}

func printRequiredSecretStatus(out io.Writer, statuses []openbao.RequiredSecretStatus) {
	if len(statuses) == 0 {
		return
	}
	configured := false
	for _, status := range statuses {
		configured = configured || status.Present
	}
	if !configured {
		fmt.Fprintln(out, "No application secrets have been configured yet.")
	}
	fmt.Fprintln(out, "REQUIRED SECRET\tSTATUS\tACTION")
	for _, status := range statuses {
		state := "present"
		action := "-"
		switch {
		case !status.Present && status.Generated:
			state = "missing - will be generated automatically"
			action = "baha app apply"
		case !status.Present:
			state = "missing - user input required"
			action = "baha app secret set " + status.Name + " --stdin"
		case !status.Usable:
			state = "present but unusable"
			action = "baha app secret set " + status.Name + " --stdin"
		}
		fmt.Fprintf(out, "%s\t%s\t%s\n", status.Name, state, action)
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
