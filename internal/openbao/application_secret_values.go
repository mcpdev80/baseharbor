package openbao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	applicationSecretSentinel = "_baseharbor"
	maxApplicationSecretBytes = 1 << 20
)

var ErrApplicationSecretNotFound = errors.New("application secret does not exist")

func EnsureApplicationSecretNamespace(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string) error {
	if err := InspectApplicationScope(ctx, executor, files, identity, credentialsPath); err != nil {
		return err
	}
	credentials, err := loadApplicationCredentials(credentialsPath)
	if err != nil {
		return err
	}
	token, err := loginApplication(ctx, executor, files, credentials)
	if err != nil {
		return err
	}

	path := applicationSecretKeyPath(identity, applicationSecretSentinel)
	out, readErr := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv get -field=value -mount=baseharbor %s`, path))
	if readErr == nil {
		if strings.TrimSpace(out) != "managed" {
			return errors.New("OpenBao application secret namespace marker has an unexpected value")
		}
		return nil
	}

	if _, err := execWithTokenInput(ctx, executor, files, token, fmt.Sprintf(`exec bao kv put -mount=baseharbor %s value=-`, path), []byte("managed")); err != nil {
		return errors.New("OpenBao application secret namespace could not be initialized")
	}
	out, err = execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv get -field=value -mount=baseharbor %s`, path))
	if err != nil || strings.TrimSpace(out) != "managed" {
		return errors.New("OpenBao application secret namespace verification failed")
	}
	return nil
}

func SetApplicationSecret(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath, key string, value []byte) error {
	if err := validateApplicationSecretKey(key); err != nil {
		return err
	}
	if len(value) == 0 {
		return errors.New("application secret value must not be empty")
	}
	if len(value) > maxApplicationSecretBytes {
		return fmt.Errorf("application secret value exceeds the %d-byte limit", maxApplicationSecretBytes)
	}
	if !utf8.Valid(value) {
		return errors.New("application secret value must be valid UTF-8 text")
	}
	if err := InspectApplicationScope(ctx, executor, files, identity, credentialsPath); err != nil {
		return err
	}
	credentials, err := loadApplicationCredentials(credentialsPath)
	if err != nil {
		return err
	}
	token, err := loginApplication(ctx, executor, files, credentials)
	if err != nil {
		return err
	}
	path := applicationSecretKeyPath(identity, key)
	if _, err := execWithTokenInput(ctx, executor, files, token, fmt.Sprintf(`exec bao kv put -mount=baseharbor %s value=-`, path), value); err != nil {
		return errors.New("write application secret to OpenBao failed")
	}
	stored, err := readApplicationSecretValue(ctx, executor, files, token, path)
	if err != nil {
		return errors.New("verify application secret after write failed")
	}
	if !bytes.Equal(stored, value) {
		return errors.New("verify application secret after write failed: stored value differs from stdin payload")
	}
	return nil
}

func ListApplicationSecretKeys(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string) ([]string, error) {
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
	return listApplicationSecretKeysWithToken(ctx, executor, files, token, identity)
}

func DeleteApplicationSecret(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath, key string) error {
	if err := validateApplicationSecretKey(key); err != nil {
		return err
	}
	keys, err := ListApplicationSecretKeys(ctx, executor, files, identity, credentialsPath)
	if err != nil {
		return err
	}
	if !containsApplicationSecretKey(keys, key) {
		return ErrApplicationSecretNotFound
	}

	credentials, err := loadApplicationCredentials(credentialsPath)
	if err != nil {
		return err
	}
	token, err := loginApplication(ctx, executor, files, credentials)
	if err != nil {
		return err
	}
	path := applicationSecretKeyPath(identity, key)
	if _, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv metadata delete -mount=baseharbor %s`, path)); err != nil {
		return errors.New("delete application secret from OpenBao failed")
	}
	keys, err = listApplicationSecretKeysWithToken(ctx, executor, files, token, identity)
	if err != nil {
		return errors.New("verify application secret deletion failed")
	}
	if containsApplicationSecretKey(keys, key) {
		return errors.New("verify application secret deletion failed: key is still present")
	}
	return nil
}

func readApplicationSecretValue(ctx context.Context, executor Executor, files bhruntime.Files, token, path string) ([]byte, error) {
	out, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv get -format=json -mount=baseharbor %s`, path))
	if err != nil {
		return nil, err
	}
	var reply struct {
		Data struct {
			Data struct {
				Value string `json:"value"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		return nil, errors.New("application secret read returned an invalid response")
	}
	return []byte(reply.Data.Data.Value), nil
}

func listApplicationSecretKeysWithToken(ctx context.Context, executor Executor, files bhruntime.Files, token string, identity ApplicationIdentity) ([]string, error) {
	out, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv list -format=json -mount=baseharbor %s`, applicationSecretPath(identity)))
	if err != nil {
		return nil, errors.New("list application secrets failed; run 'baha app apply NAME' to reconcile the managed secret namespace")
	}
	return parseApplicationSecretKeyList(out)
}

func listApplicationSecretDocuments(ctx context.Context, executor Executor, files bhruntime.Files, token string, identity ApplicationIdentity) ([]string, error) {
	out, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao kv list -format=json -mount=baseharbor %s`, applicationSecretPath(identity)))
	if err != nil {
		return nil, errors.New("list managed OpenBao application secret documents failed")
	}
	var raw []string
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, errors.New("list managed OpenBao application secret documents returned an invalid response")
	}
	for _, key := range raw {
		if key == applicationSecretSentinel {
			continue
		}
		if err := validateApplicationSecretKey(key); err != nil {
			return nil, errors.New("managed OpenBao application secret namespace contains an unexpected entry")
		}
	}
	return raw, nil
}

func parseApplicationSecretKeyList(out string) ([]string, error) {
	var raw []string
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, errors.New("list application secrets returned an invalid response")
	}
	keys := make([]string, 0, len(raw))
	for _, key := range raw {
		if key == applicationSecretSentinel {
			continue
		}
		if err := validateApplicationSecretKey(key); err != nil {
			return nil, errors.New("application secret namespace contains an unexpected entry")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func containsApplicationSecretKey(keys []string, wanted string) bool {
	for _, key := range keys {
		if key == wanted {
			return true
		}
	}
	return false
}

func validateApplicationSecretKey(key string) error {
	if key == "" {
		return errors.New("application secret key is required")
	}
	if len(key) > 128 {
		return errors.New("application secret key must be at most 128 characters")
	}
	if key == applicationSecretSentinel || strings.HasPrefix(key, "__baseharbor_") {
		return errors.New("application secret key uses a reserved BaseHarbor name")
	}
	for i, r := range key {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return fmt.Errorf("invalid application secret key %q", key)
		}
		if i == 0 && (r == '-' || r == '.') {
			return fmt.Errorf("invalid application secret key %q", key)
		}
	}
	return nil
}

func applicationSecretKeyPath(identity ApplicationIdentity, key string) string {
	return applicationSecretPath(identity) + "/" + key
}

func execWithTokenInput(ctx context.Context, executor Executor, files bhruntime.Files, token, command string, payload []byte) (string, error) {
	const prefix = `IFS= read -r BAO_TOKEN
export BAO_TOKEN
`
	input := make([]byte, 0, len(token)+1+len(payload))
	input = append(input, token...)
	input = append(input, '\n')
	input = append(input, payload...)
	return executor.ExecProjectInput(ctx, projectName, files.Compose, files.Env, input, serviceName, "sh", "-ceu", prefix+command)
}
