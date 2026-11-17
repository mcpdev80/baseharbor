package deployment

import (
	"errors"
	"fmt"
	"strings"
)

// ErrManagedCredentialAlreadyExists documents a create-only (compare-and-swap)
// seed that could not run because a managed credential already exists for the
// reference. It lives in the low-level deployment package so both the OpenBao
// managed-secret authority and the devaccess device-rebind binder can reference
// the same sentinel without an import cycle. The fresh-device reconcile path
// treats this as the winning case: the existing managed authority keeps its
// value and is read back, so a concurrently created managed value is never
// overwritten by the stale local candidate.
var ErrManagedCredentialAlreadyExists = errors.New("managed credential already exists")

func OperatorAuthForTargetEnvironment(target, environment string) (OperatorAuthEnvironmentConfig, bool, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return OperatorAuthEnvironmentConfig{}, false, err
	}
	def, ok := cfg.Targets[strings.TrimSpace(target)]
	if !ok {
		if strings.TrimSpace(target) == "local" {
			return OperatorAuthEnvironmentConfig{}, false, nil
		}
		return OperatorAuthEnvironmentConfig{}, false, fmt.Errorf("target %q is not configured", target)
	}
	auth, ok := def.OperatorAuth[strings.TrimSpace(environment)]
	return auth, ok, nil
}

func SetOperatorAuthForTargetEnvironment(target, environment string, auth OperatorAuthEnvironmentConfig) error {
	target = strings.TrimSpace(target)
	environment = strings.TrimSpace(environment)
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	def, ok := cfg.Targets[target]
	if !ok {
		return fmt.Errorf("target %q is not configured", target)
	}
	if def.OperatorAuth == nil {
		def.OperatorAuth = map[string]OperatorAuthEnvironmentConfig{}
	}
	if len(auth.Scopes) == 0 {
		auth.Scopes = []string{"openid", "profile", "email"}
	}
	def.OperatorAuth[environment] = auth
	cfg.Targets[target] = def
	return cfg.Save()
}

func DeleteOperatorAuthForTargetEnvironment(target, environment string) error {
	target = strings.TrimSpace(target)
	environment = strings.TrimSpace(environment)
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	def, ok := cfg.Targets[target]
	if !ok {
		return fmt.Errorf("target %q is not configured", target)
	}
	if def.OperatorAuth != nil {
		delete(def.OperatorAuth, environment)
		cfg.Targets[target] = def
	}
	return cfg.Save()
}
