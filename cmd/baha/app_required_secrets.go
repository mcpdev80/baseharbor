package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func inspectRequiredSecrets(ctx context.Context, compose bhruntime.Compose, platformFiles bhruntime.Files, m application.Manifest, files application.RuntimeFiles) ([]openbao.RequiredSecretStatus, error) {
	required := application.RequiredSecretNames(m)
	if len(required) == 0 {
		return nil, nil
	}
	identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
	return openbao.InspectRequiredApplicationSecrets(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir), required)
}

func requireReadySecrets(statuses []openbao.RequiredSecretStatus) error {
	return openbao.RequireApplicationSecrets(statuses)
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

func printRequiredSecretUnknown(out io.Writer, m application.Manifest) {
	for _, name := range application.RequiredSecretNames(m) {
		fmt.Fprintf(out, "%s\tunknown\tunknown\n", name)
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
