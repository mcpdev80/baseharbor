package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func checkRequiredApplicationSecrets(
	ctx context.Context,
	compose bhruntime.Compose,
	platformFiles bhruntime.Files,
	m application.Manifest,
	files application.RuntimeFiles,
) error {
	if len(m.Secrets.Required) == 0 {
		return nil
	}
	if !m.Services.Secrets {
		return fmt.Errorf("required secrets are declared but managed secrets are disabled")
	}
	identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
	keys, err := openbao.ListApplicationSecretKeys(
		ctx,
		compose,
		platformFiles,
		identity,
		openbao.ApplicationCredentialsPath(files.Dir),
	)
	if err != nil {
		return err
	}
	present := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		present[key] = struct{}{}
	}
	missing := make([]string, 0)
	for _, key := range m.Secrets.Required {
		if _, ok := present[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf(
		"missing required application secrets: %s; set each value with: printf '%%s' \"$VALUE\" | baha app secret set %s KEY --stdin",
		strings.Join(missing, ", "),
		m.Name,
	)
}
