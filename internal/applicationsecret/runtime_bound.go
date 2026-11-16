package applicationsecret

import (
	"context"
	"errors"
	"time"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

// BoundRuntimeService is the least-privilege secret service used inside one
// per-application broker. It has no application store and cannot resolve or
// cross into another application's identity or state.
type BoundRuntimeService struct {
	app             string
	identity        openbao.ApplicationIdentity
	credentialsPath string
	client          *openbao.ApplicationRuntimeClient
}

func NewBoundRuntimeService(app, environment, credentialsPath string, client *openbao.ApplicationRuntimeClient) (*BoundRuntimeService, error) {
	if app == "" || environment == "" || credentialsPath == "" || client == nil {
		return nil, errors.New("bound runtime secret service configuration is incomplete")
	}
	return &BoundRuntimeService{
		app:             app,
		identity:        openbao.ApplicationIdentity{Name: app, Environment: environment},
		credentialsPath: credentialsPath,
		client:          client,
	}, nil
}

func (s *BoundRuntimeService) checkApp(name string) error {
	if name != s.app {
		return errors.New("application runtime secret scope is unavailable")
	}
	return nil
}

func (s *BoundRuntimeService) CreateDynamic(ctx context.Context, name string, value []byte) (Reference, error) {
	if err := s.checkApp(name); err != nil {
		return "", err
	}
	if err := validateDynamicValue(value); err != nil {
		return "", err
	}
	key, err := newDynamicKey()
	if err != nil {
		return "", err
	}
	mutationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.client.SetApplicationSecret(mutationCtx, s.identity, s.credentialsPath, key, value); err != nil {
		return "", err
	}
	return Reference(dynamicReferencePrefix + key), nil
}

func (s *BoundRuntimeService) ReadDynamic(ctx context.Context, name string, ref Reference) ([]byte, error) {
	if err := s.checkApp(name); err != nil {
		return nil, err
	}
	key, err := parseDynamicReference(ref)
	if err != nil {
		return nil, err
	}
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return s.client.GetApplicationSecret(readCtx, s.identity, s.credentialsPath, key)
}

func (s *BoundRuntimeService) RotateDynamic(ctx context.Context, name string, ref Reference, value []byte) error {
	if err := s.checkApp(name); err != nil {
		return err
	}
	if err := validateDynamicValue(value); err != nil {
		return err
	}
	key, err := parseDynamicReference(ref)
	if err != nil {
		return err
	}
	mutationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := s.client.GetApplicationSecret(mutationCtx, s.identity, s.credentialsPath, key); err != nil {
		return err
	}
	return s.client.SetApplicationSecret(mutationCtx, s.identity, s.credentialsPath, key, value)
}

func (s *BoundRuntimeService) DeleteDynamic(ctx context.Context, name string, ref Reference) error {
	if err := s.checkApp(name); err != nil {
		return err
	}
	key, err := parseDynamicReference(ref)
	if err != nil {
		return err
	}
	mutationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.client.DeleteApplicationSecret(mutationCtx, s.identity, s.credentialsPath, key)
}
