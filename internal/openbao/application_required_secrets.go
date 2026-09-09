package openbao

import (
	"context"
	"errors"
	"fmt"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RequiredSecretStatus contains readiness metadata only. It never exposes the
// secret value that was read to prove usability. Generated is caller-supplied
// declaration metadata and is intentionally not inferred from the stored value.
type RequiredSecretStatus struct {
	Name      string
	Present   bool
	Usable    bool
	Generated bool
}

func InspectRequiredApplicationSecrets(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string, required []string) ([]RequiredSecretStatus, error) {
	if len(required) == 0 {
		return nil, nil
	}
	if err := InspectApplicationScope(ctx, executor, files, identity, credentialsPath); err != nil {
		return nil, err
	}
	credentials, err := loadApplicationCredentials(credentialsPath)
	if err != nil {
		return nil, err
	}
	token, err := loginApplication(ctx, executor, files, credentials)
	if err != nil {
		return nil, err
	}
	keys, err := listApplicationSecretKeysWithToken(ctx, executor, files, token, identity)
	if err != nil {
		return nil, err
	}
	present := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		present[key] = struct{}{}
	}

	statuses := make([]RequiredSecretStatus, 0, len(required))
	for _, name := range required {
		status := RequiredSecretStatus{Name: name}
		if _, ok := present[name]; !ok {
			statuses = append(statuses, status)
			continue
		}
		status.Present = true
		path := applicationSecretKeyPath(identity, name)
		if _, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv get -field=value -mount=baseharbor %s`, path)); err == nil {
			status.Usable = true
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func RequireApplicationSecrets(statuses []RequiredSecretStatus) error {
	missing := make([]string, 0)
	unusable := make([]string, 0)
	for _, status := range statuses {
		switch {
		case !status.Present:
			missing = append(missing, status.Name)
		case !status.Usable:
			unusable = append(unusable, status.Name)
		}
	}
	if len(missing) == 0 && len(unusable) == 0 {
		return nil
	}
	if len(missing) > 0 && len(unusable) == 0 {
		return fmt.Errorf("required application secrets are missing: %v", missing)
	}
	if len(missing) == 0 {
		return fmt.Errorf("required application secrets are present but unusable: %v", unusable)
	}
	return errors.New("one or more required application secrets are missing or unusable")
}
