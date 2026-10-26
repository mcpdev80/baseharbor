package openbao

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalidAddress = errors.New("invalid OpenBao address")
	ErrMissingToken   = errors.New("OpenBao token is required")
	ErrMissingMount   = errors.New("OpenBao KV mount is required")
)

// Config contains the minimum information required by the OpenBao credential adapter.
// Token is intentionally excluded from JSON and must never be logged.
type Config struct {
	Address   string        `json:"address"`
	Token     string        `json:"-"`
	Namespace string        `json:"namespace,omitempty"`
	Mount     string        `json:"mount"`
	Timeout   time.Duration `json:"timeout,omitempty"`
}

func (c Config) Validate() error {
	parsed, err := url.Parse(c.Address)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ErrInvalidAddress
	}
	if strings.TrimSpace(c.Token) == "" {
		return ErrMissingToken
	}
	if strings.Trim(strings.TrimSpace(c.Mount), "/") == "" {
		return ErrMissingMount
	}
	return nil
}

func (c Config) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 10 * time.Second
	}
	return c.Timeout
}
