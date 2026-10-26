package runtimeresourceapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type StaticAuthorizer struct {
	app         string
	permissions map[string]map[string]map[string]struct{}
}

func NewStaticAuthorizer(app, path string) (*StaticAuthorizer, error) {
	app = strings.TrimSpace(app)
	path = strings.TrimSpace(path)
	if app == "" || path == "" {
		return nil, errors.New("runtime resource authorizer configuration is incomplete")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect runtime permissions: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("runtime permissions file is not protected")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime permissions: %w", err)
	}
	var source []application.RuntimePermission
	if err := json.Unmarshal(data, &source); err != nil {
		return nil, errors.New("runtime permissions file is invalid")
	}
	permissions := map[string]map[string]map[string]struct{}{}
	for _, permission := range source {
		capability := strings.TrimSpace(permission.Capability)
		if capability == "" {
			return nil, errors.New("runtime permission capability is empty")
		}
		if permissions[capability] == nil {
			permissions[capability] = map[string]map[string]struct{}{}
		}
		for _, service := range permission.Services {
			service = strings.TrimSpace(service)
			if service == "" {
				return nil, errors.New("runtime permission service is empty")
			}
			if permissions[capability][service] == nil {
				permissions[capability][service] = map[string]struct{}{}
			}
			for _, operation := range permission.Operations {
				operation = strings.TrimSpace(operation)
				if operation == "" {
					return nil, errors.New("runtime permission operation is empty")
				}
				permissions[capability][service][operation] = struct{}{}
			}
		}
	}
	return &StaticAuthorizer{app: app, permissions: permissions}, nil
}

func (a *StaticAuthorizer) AuthorizeRuntimeOperation(app, service, capability, operation string) error {
	if strings.TrimSpace(app) != a.app {
		return errors.New("runtime application identity mismatch")
	}
	services := a.permissions[strings.TrimSpace(capability)]
	if services == nil {
		return errors.New("runtime capability is not authorized")
	}
	operations := services[strings.TrimSpace(service)]
	if operations == nil {
		return errors.New("runtime service is not authorized for capability")
	}
	if _, ok := operations[strings.TrimSpace(operation)]; !ok {
		return errors.New("runtime operation is not authorized")
	}
	return nil
}

func (a *StaticAuthorizer) Capabilities() map[string][]string {
	result := map[string][]string{}
	for capability, services := range a.permissions {
		seen := map[string]struct{}{}
		for _, operations := range services {
			for operation := range operations {
				seen[operation] = struct{}{}
			}
		}
		for operation := range seen {
			result[capability] = append(result[capability], operation)
		}
		sort.Strings(result[capability])
	}
	return result
}

func (a *StaticAuthorizer) CapabilitiesForService(service string) map[string][]string {
	service = strings.TrimSpace(service)
	result := map[string][]string{}
	for capability, services := range a.permissions {
		for operation := range services[service] {
			result[capability] = append(result[capability], operation)
		}
		sort.Strings(result[capability])
	}
	return result
}

type ServiceTokenVerifier struct {
	byToken map[string]string
}

func NewServiceTokenVerifier(path string) (*ServiceTokenVerifier, error) {
	info, err := os.Lstat(strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("inspect runtime service identities: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("runtime service identity file is not protected")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime service identities: %w", err)
	}
	var byService map[string]string
	if err := json.Unmarshal(data, &byService); err != nil {
		return nil, errors.New("runtime service identity file is invalid")
	}
	byToken := map[string]string{}
	for service, token := range byService {
		service = strings.TrimSpace(service)
		token = strings.TrimSpace(token)
		if service == "" || token == "" {
			return nil, errors.New("runtime service identity entry is incomplete")
		}
		if _, exists := byToken[token]; exists {
			return nil, errors.New("runtime service identity token is duplicated")
		}
		byToken[token] = service
	}
	return &ServiceTokenVerifier{byToken: byToken}, nil
}

func (v *ServiceTokenVerifier) Verify(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("runtime service identity is missing")
	}
	for expected, service := range v.byToken {
		if len(expected) == len(token) && subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1 {
			return service, nil
		}
	}
	return "", errors.New("runtime service identity is invalid")
}
