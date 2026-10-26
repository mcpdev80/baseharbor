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
	"path"
	"strings"
	"unicode"

	"github.com/mcpdev80/baseharbor/internal/credential"
)

var (
	ErrNotFound      = errors.New("credential not found")
	ErrAccessDenied  = errors.New("credential access denied")
	ErrInvalidReply  = errors.New("invalid OpenBao response")
	ErrRequestFailed = errors.New("OpenBao request failed")
	ErrInvalidRef    = errors.New("invalid credential ref")
)

// Client resolves opaque BaseHarbor credential references from an OpenBao KV v2 mount.
// The ref is treated as a relative KV path. Scope is reserved for future policy/routing
// decisions and is deliberately not interpolated into the secret path.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

func New(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.timeout(),
		},
	}, nil
}

// NewWithHTTPClient exists for tests and controlled transports. The provided client
// is used as-is; callers remain responsible for secure TLS configuration.
func NewWithHTTPClient(cfg Config, httpClient *http.Client) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, errors.New("http client is required")
	}
	return &Client{cfg: cfg, httpClient: httpClient}, nil
}

func (c *Client) Resolve(ctx context.Context, ref string, _ string) (credential.Data, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return credential.Data{}, credential.ErrEmptyRef
	}

	requestURL, err := c.secretURL(ref)
	if err != nil {
		return credential.Data{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return credential.Data{}, ErrRequestFailed
	}
	req.Header.Set("X-Vault-Token", c.cfg.Token)
	if ns := strings.TrimSpace(c.cfg.Namespace); ns != "" {
		req.Header.Set("X-Vault-Namespace", ns)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return credential.Data{}, fmt.Errorf("%w", ErrRequestFailed)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden:
		return credential.Data{}, ErrAccessDenied
	case http.StatusNotFound:
		return credential.Data{}, ErrNotFound
	default:
		return credential.Data{}, ErrRequestFailed
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return credential.Data{}, ErrInvalidReply
	}

	var envelope struct {
		Data struct {
			Data map[string]json.RawMessage `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&envelope); err != nil || envelope.Data.Data == nil {
		return credential.Data{}, ErrInvalidReply
	}

	payload := make(map[string][]byte, len(envelope.Data.Data))
	for key, raw := range envelope.Data.Data {
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			payload[key] = []byte(value)
			continue
		}
		payload[key] = append([]byte(nil), raw...)
	}

	return credential.Data{Payload: payload}, nil
}

func validateRef(ref string) error {
	if ref == "" || strings.HasPrefix(ref, "/") || strings.Contains(ref, "\\") {
		return ErrInvalidRef
	}
	if strings.ContainsAny(ref, "?#@") {
		return ErrInvalidRef
	}
	for _, r := range ref {
		if unicode.IsControl(r) {
			return ErrInvalidRef
		}
	}
	parts := strings.Split(ref, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ErrInvalidRef
		}
	}
	return nil
}

func (c *Client) secretURL(ref string) (string, error) {
	if err := validateRef(ref); err != nil {
		return "", err
	}
	base, err := url.Parse(c.cfg.Address)
	if err != nil {
		return "", err
	}
	mount := strings.Trim(strings.TrimSpace(c.cfg.Mount), "/")
	base.Path = path.Join(base.Path, "v1", mount, "data", ref)
	return base.String(), nil
}
