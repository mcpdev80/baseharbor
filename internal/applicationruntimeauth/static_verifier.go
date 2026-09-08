package applicationruntimeauth

import (
	"context"
	"crypto/subtle"
	"errors"
	"os"
	"strings"
)

type StaticVerifier struct {
	app       string
	tokenFile string
}

func NewStatic(app, tokenFile string) (*StaticVerifier, error) {
	if strings.TrimSpace(app) == "" || strings.TrimSpace(tokenFile) == "" {
		return nil, errors.New("static runtime verifier configuration is incomplete")
	}
	return &StaticVerifier{app: app, tokenFile: tokenFile}, nil
}

func (v *StaticVerifier) Verify(_ context.Context, app, token string) error {
	if app != v.app || strings.TrimSpace(token) == "" {
		return ErrUnauthorized
	}
	info, err := os.Stat(v.tokenFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return ErrUnauthorized
	}
	stored, err := os.ReadFile(v.tokenFile)
	if err != nil {
		return ErrUnauthorized
	}
	want := strings.TrimSpace(string(stored))
	got := strings.TrimSpace(token)
	if want == "" || len(want) != len(got) || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		return ErrUnauthorized
	}
	return nil
}
