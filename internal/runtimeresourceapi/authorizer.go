package runtimeresourceapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type StaticAuthorizer struct {
	app         string
	permissions map[string]map[string]struct{}
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
	permissions := map[string]map[string]struct{}{}
	for _, permission := range source {
		capability := strings.TrimSpace(permission.Capability)
		if capability == "" {
			return nil, errors.New("runtime permission capability is empty")
		}
		if permissions[capability] == nil {
			permissions[capability] = map[string]struct{}{}
		}
		for _, operation := range permission.Operations {
			operation = strings.TrimSpace(operation)
			if operation == "" {
				return nil, errors.New("runtime permission operation is empty")
			}
			permissions[capability][operation] = struct{}{}
		}
	}
	return &StaticAuthorizer{app: app, permissions: permissions}, nil
}

func (a *StaticAuthorizer) AuthorizeRuntimeOperation(app, capability, operation string) error {
	if strings.TrimSpace(app) != a.app {
		return errors.New("runtime application identity mismatch")
	}
	operations := a.permissions[strings.TrimSpace(capability)]
	if operations == nil {
		return errors.New("runtime capability is not authorized")
	}
	if _, ok := operations[strings.TrimSpace(operation)]; !ok {
		return errors.New("runtime operation is not authorized")
	}
	return nil
}

func (a *StaticAuthorizer) Capabilities() map[string][]string {
	result := map[string][]string{}
	for capability, operations := range a.permissions {
		for operation := range operations {
			result[capability] = append(result[capability], operation)
		}
	}
	return result
}
