package openbao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// ApplicationRuntimeClient is the narrow OpenBao data-plane client used by the
// managed runtime API. It authenticates only with each application's AppRole;
// it never receives the BaseHarbor manager identity or a root token.
type ApplicationRuntimeClient struct {
	baseURL *url.URL
	client  *http.Client
}

func NewApplicationRuntimeClient(rawURL string) (*ApplicationRuntimeClient, error) {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid OpenBao runtime URL")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, errors.New("OpenBao runtime URL must not contain a path")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && parsed.Host == "openbao:8200") {
		return nil, errors.New("OpenBao runtime URL must use HTTPS; only the bundled in-stack http://openbao:8200 endpoint is allowed without TLS")
	}
	parsed.Path = ""
	return &ApplicationRuntimeClient{
		baseURL: parsed,
		client:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *ApplicationRuntimeClient) Check(ctx context.Context, credentialsPath string) error {
	_, err := c.login(ctx, credentialsPath)
	return err
}

func (c *ApplicationRuntimeClient) GetApplicationSecret(ctx context.Context, identity ApplicationIdentity, credentialsPath, key string) ([]byte, error) {
	if err := validateRuntimeSecretRequest(identity, key); err != nil {
		return nil, err
	}
	token, err := c.login(ctx, credentialsPath)
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
	status, err := c.requestJSON(ctx, http.MethodGet, runtimeDataPath(identity, key), token, nil, &reply)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrApplicationSecretNotFound
	}
	if status != http.StatusOK {
		return nil, errors.New("read application secret from OpenBao failed")
	}
	return []byte(reply.Data.Data.Value), nil
}

func (c *ApplicationRuntimeClient) SetApplicationSecret(ctx context.Context, identity ApplicationIdentity, credentialsPath, key string, value []byte) error {
	if err := validateRuntimeSecretRequest(identity, key); err != nil {
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
	token, err := c.login(ctx, credentialsPath)
	if err != nil {
		return err
	}
	payload := map[string]any{"data": map[string]string{"value": string(value)}}
	status, err := c.requestJSON(ctx, http.MethodPost, runtimeDataPath(identity, key), token, payload, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return errors.New("write application secret to OpenBao failed")
	}
	stored, err := c.GetApplicationSecret(ctx, identity, credentialsPath, key)
	if err != nil || !bytes.Equal(stored, value) {
		return errors.New("verify application secret after write failed")
	}
	return nil
}

func (c *ApplicationRuntimeClient) DeleteApplicationSecret(ctx context.Context, identity ApplicationIdentity, credentialsPath, key string) error {
	if err := validateRuntimeSecretRequest(identity, key); err != nil {
		return err
	}
	if _, err := c.GetApplicationSecret(ctx, identity, credentialsPath, key); err != nil {
		return err
	}
	token, err := c.login(ctx, credentialsPath)
	if err != nil {
		return err
	}
	status, err := c.requestJSON(ctx, http.MethodDelete, runtimeMetadataPath(identity, key), token, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return errors.New("delete application secret from OpenBao failed")
	}
	if _, err := c.GetApplicationSecret(ctx, identity, credentialsPath, key); !errors.Is(err, ErrApplicationSecretNotFound) {
		return errors.New("verify application secret deletion failed")
	}
	return nil
}

func (c *ApplicationRuntimeClient) login(ctx context.Context, credentialsPath string) (string, error) {
	credentials, err := loadRuntimeApplicationCredentials(credentialsPath)
	if err != nil {
		return "", err
	}
	payload := map[string]string{"role_id": credentials.RoleID, "secret_id": credentials.SecretID}
	var reply struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	status, err := c.requestJSON(ctx, http.MethodPost, "/v1/auth/approle/login", "", payload, &reply)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK || strings.TrimSpace(reply.Auth.ClientToken) == "" {
		return "", errors.New("OpenBao application AppRole login failed")
	}
	return reply.Auth.ClientToken, nil
}

// loadRuntimeApplicationCredentials intentionally differs from the host-side
// loader. The canonical host state remains owner-only (0600). A container
// runtime may project that file as a read-only Compose secret with broader read
// bits, so the broker accepts readability but still rejects symlinks,
// non-regular files and any group/world writable projection.
func loadRuntimeApplicationCredentials(path string) (ApplicationCredentials, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return ApplicationCredentials{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ApplicationCredentials{}, errors.New("OpenBao runtime credential projection must be a regular file")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return ApplicationCredentials{}, fmt.Errorf("OpenBao runtime credential projection is writable by group or others (%o)", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ApplicationCredentials{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return ApplicationCredentials{}, errors.New("invalid OpenBao runtime credential projection")
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	credentials := ApplicationCredentials{RoleID: values["OPENBAO_ROLE_ID"], SecretID: values["OPENBAO_SECRET_ID"]}
	if credentials.RoleID == "" || credentials.SecretID == "" {
		return ApplicationCredentials{}, errors.New("invalid OpenBao runtime credential projection")
	}
	return credentials, nil
}

func (c *ApplicationRuntimeClient) requestJSON(ctx context.Context, method, path, token string, payload any, reply any) (int, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, errors.New("encode OpenBao runtime request failed")
		}
		body = bytes.NewReader(encoded)
	}
	endpoint := *c.baseURL
	endpoint.Path = path
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return 0, errors.New("create OpenBao runtime request failed")
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return 0, errors.New("OpenBao runtime request failed")
	}
	defer res.Body.Close()
	if reply != nil && res.StatusCode >= 200 && res.StatusCode < 300 {
		decoder := json.NewDecoder(io.LimitReader(res.Body, 1<<20))
		if err := decoder.Decode(reply); err != nil {
			return 0, errors.New("OpenBao runtime response was invalid")
		}
	}
	return res.StatusCode, nil
}

func validateRuntimeSecretRequest(identity ApplicationIdentity, key string) error {
	if err := validateApplicationIdentity(identity); err != nil {
		return err
	}
	return validateApplicationSecretKey(key)
}

func runtimeDataPath(identity ApplicationIdentity, key string) string {
	return "/v1/baseharbor/data/" + applicationSecretKeyPath(identity, key)
}

func runtimeMetadataPath(identity ApplicationIdentity, key string) string {
	return "/v1/baseharbor/metadata/" + applicationSecretKeyPath(identity, key)
}
