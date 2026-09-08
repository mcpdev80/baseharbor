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
	return openbao.InspectRequiredApplicationSecrets(
		ctx,
		compose,
		platformFiles,
		identity,
		openbao.ApplicationCredentialsPath(files.Dir),
		required,
	)
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
		return fmt.Errorf("%w; set missing values with: printf '%%s' \"$VALUE\" | baha app secret set %s KEY --stdin", err, m.Name)
	}
	return nil
}

func printRequiredSecretStatus(out io.Writer, statuses []openbao.RequiredSecretStatus) {
	if len(statuses) == 0 {
		return
	}
	fmt.Fprintln(out, "REQUIRED SECRET\tPRESENT\tUSABLE")
	for _, status := range statuses {
		fmt.Fprintf(out, "%s\t%s\t%s\n", status.Name, yesNo(status.Present), yesNo(status.Usable))
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
