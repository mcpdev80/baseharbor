package applicationsecret

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Metadata describes a managed application secret without exposing its value.
type Metadata struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Present  bool   `json:"present"`
	Usable   bool   `json:"usable"`
}

// Service owns application-secret orchestration. Transports such as baha and
// HTTP must use this boundary instead of calling OpenBao directly.
type Service struct {
	store application.Store
}

func New(store application.Store) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context, name string) ([]Metadata, error) {
	resolved, err := s.resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	keys, err := openbao.ListApplicationSecretKeys(checkCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath)
	if err != nil {
		return nil, err
	}

	required := make(map[string]struct{}, len(resolved.manifest.Secrets.Required))
	names := make(map[string]struct{}, len(keys)+len(resolved.manifest.Secrets.Required))
	for _, key := range keys {
		if strings.HasPrefix(key, dynamicKeyPrefix) {
			continue
		}
		names[key] = struct{}{}
	}
	for _, requirement := range resolved.manifest.Secrets.Required {
		required[requirement.Name] = struct{}{}
		names[requirement.Name] = struct{}{}
	}
	ordered := make([]string, 0, len(names))
	for key := range names {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	statuses, err := openbao.InspectRequiredApplicationSecrets(checkCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath, ordered)
	if err != nil && len(ordered) > 0 {
		return nil, err
	}
	statusByName := make(map[string]openbao.RequiredSecretStatus, len(statuses))
	for _, status := range statuses {
		statusByName[status.Name] = status
	}

	result := make([]Metadata, 0, len(ordered))
	for _, key := range ordered {
		_, isRequired := required[key]
		status := statusByName[key]
		result = append(result, Metadata{
			Name:     key,
			Required: isRequired,
			Present:  status.Present,
			Usable:   status.Usable,
		})
	}
	return result, nil
}

func (s *Service) Set(ctx context.Context, name, key string, value []byte) error {
	if strings.HasPrefix(key, dynamicKeyPrefix) {
		return errors.New("dynamic application secrets must be managed through a secret reference")
	}
	if len(value) == 0 {
		return errors.New("application secret value is empty")
	}
	if len(value) > 1<<20 {
		return errors.New("application secret value exceeds the 1048576-byte limit")
	}
	resolved, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	mutationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return openbao.SetApplicationSecret(mutationCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath, key, value)
}

func (s *Service) Delete(ctx context.Context, name, key string) error {
	if strings.HasPrefix(key, dynamicKeyPrefix) {
		return errors.New("dynamic application secrets must be managed through a secret reference")
	}
	resolved, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	mutationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return openbao.DeleteApplicationSecret(mutationCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath, key)
}

type resolvedApplication struct {
	manifest        application.Manifest
	compose         bhruntime.Compose
	platformFiles   bhruntime.Files
	identity        openbao.ApplicationIdentity
	credentialsPath string
}

func (s *Service) resolve(ctx context.Context, name string) (resolvedApplication, error) {
	m, _, err := s.store.Load(name)
	if err != nil {
		return resolvedApplication{}, err
	}
	if !m.Services.Secrets {
		return resolvedApplication{}, errors.New("application does not enable managed secrets")
	}
	if err := application.CheckSupportedRuntimeServices(m); err != nil {
		return resolvedApplication{}, err
	}
	files, err := application.ExistingRuntimeFiles(s.store, m)
	if err != nil {
		return resolvedApplication{}, err
	}
	if err := application.CheckRuntimePermissions(files); err != nil {
		return resolvedApplication{}, err
	}
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return resolvedApplication{}, err
	}
	platformFiles, err := bhruntime.ExistingFiles("")
	if err != nil {
		return resolvedApplication{}, errors.New("BaseHarbor OpenBao runtime is not materialized")
	}
	return resolvedApplication{
		manifest:        m,
		compose:         compose,
		platformFiles:   platformFiles,
		identity:        openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment},
		credentialsPath: openbao.ApplicationCredentialsPath(files.Dir),
	}, nil
}
