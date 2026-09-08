package applicationsecret

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

const dynamicReferencePrefix = "baseharbor://secrets/"
const dynamicKeyPrefix = "dyn-"

// Reference is a stable opaque handle applications may persist instead of a
// secret value. It intentionally exposes no provider path, tenant, app or
// environment details.
type Reference string

func (r Reference) String() string { return string(r) }

func (s *Service) CreateDynamic(ctx context.Context, name string, value []byte) (Reference, error) {
	if err := validateDynamicValue(value); err != nil {
		return "", err
	}
	key, err := newDynamicKey()
	if err != nil {
		return "", err
	}
	mutationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.runtimeClient != nil {
		resolved, err := s.resolveRuntime(name)
		if err != nil {
			return "", err
		}
		if err := s.runtimeClient.SetApplicationSecret(mutationCtx, resolved.identity, resolved.credentialsPath, key, value); err != nil {
			return "", err
		}
	} else {
		resolved, err := s.resolve(ctx, name)
		if err != nil {
			return "", err
		}
		if err := openbao.SetApplicationSecret(mutationCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath, key, value); err != nil {
			return "", err
		}
	}
	return Reference(dynamicReferencePrefix + key), nil
}

func (s *Service) ReadDynamic(ctx context.Context, name string, ref Reference) ([]byte, error) {
	key, err := parseDynamicReference(ref)
	if err != nil {
		return nil, err
	}
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if s.runtimeClient != nil {
		resolved, err := s.resolveRuntime(name)
		if err != nil {
			return nil, err
		}
		return s.runtimeClient.GetApplicationSecret(readCtx, resolved.identity, resolved.credentialsPath, key)
	}
	resolved, err := s.resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	return openbao.GetApplicationSecret(readCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath, key)
}

func (s *Service) RotateDynamic(ctx context.Context, name string, ref Reference, value []byte) error {
	if err := validateDynamicValue(value); err != nil {
		return err
	}
	key, err := parseDynamicReference(ref)
	if err != nil {
		return err
	}
	mutationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.runtimeClient != nil {
		resolved, err := s.resolveRuntime(name)
		if err != nil {
			return err
		}
		if _, err := s.runtimeClient.GetApplicationSecret(mutationCtx, resolved.identity, resolved.credentialsPath, key); err != nil {
			return err
		}
		return s.runtimeClient.SetApplicationSecret(mutationCtx, resolved.identity, resolved.credentialsPath, key, value)
	}
	resolved, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if _, err := openbao.GetApplicationSecret(mutationCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath, key); err != nil {
		return err
	}
	return openbao.SetApplicationSecret(mutationCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath, key, value)
}

func (s *Service) DeleteDynamic(ctx context.Context, name string, ref Reference) error {
	key, err := parseDynamicReference(ref)
	if err != nil {
		return err
	}
	mutationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.runtimeClient != nil {
		resolved, err := s.resolveRuntime(name)
		if err != nil {
			return err
		}
		return s.runtimeClient.DeleteApplicationSecret(mutationCtx, resolved.identity, resolved.credentialsPath, key)
	}
	resolved, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	return openbao.DeleteApplicationSecret(mutationCtx, resolved.compose, resolved.platformFiles, resolved.identity, resolved.credentialsPath, key)
}

func validateDynamicValue(value []byte) error {
	if len(value) == 0 {
		return errors.New("application secret value is empty")
	}
	if len(value) > 1<<20 {
		return errors.New("application secret value exceeds the 1048576-byte limit")
	}
	return nil
}

func newDynamicKey() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate application secret reference: %w", err)
	}
	return dynamicKeyPrefix + hex.EncodeToString(raw[:]), nil
}

func parseDynamicReference(ref Reference) (string, error) {
	raw := string(ref)
	if !strings.HasPrefix(raw, dynamicReferencePrefix) {
		return "", errors.New("invalid BaseHarbor secret reference")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "baseharbor" || u.Host != "secrets" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid BaseHarbor secret reference")
	}
	key := strings.TrimPrefix(u.Path, "/")
	if strings.Contains(key, "/") || !strings.HasPrefix(key, dynamicKeyPrefix) {
		return "", errors.New("invalid BaseHarbor secret reference")
	}
	hexPart := strings.TrimPrefix(key, dynamicKeyPrefix)
	if len(hexPart) != 32 {
		return "", errors.New("invalid BaseHarbor secret reference")
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", errors.New("invalid BaseHarbor secret reference")
	}
	return key, nil
}
