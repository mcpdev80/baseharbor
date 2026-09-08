package applicationruntimeauth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

var ErrUnauthorized = errors.New("application runtime credential is invalid")

type Verifier struct {
	store application.Store
}

func New(store application.Store) *Verifier {
	return &Verifier{store: store}
}

// Verify proves that token belongs to exactly the requested application. The
// credential is not an operator identity and conveys no permissions outside
// the dedicated runtime API surface.
func (v *Verifier) Verify(_ context.Context, app, token string) error {
	if strings.TrimSpace(app) == "" || strings.TrimSpace(token) == "" {
		return ErrUnauthorized
	}
	m, _, err := v.store.Load(app)
	if err != nil || !m.Services.Secrets {
		return ErrUnauthorized
	}
	files, err := application.ExistingRuntimeFiles(v.store, m)
	if err != nil {
		return ErrUnauthorized
	}
	path := application.RuntimeIdentityTokenPath(files)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return ErrUnauthorized
	}
	stored, err := os.ReadFile(path)
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

func (v *Verifier) CredentialPath(app string) (string, error) {
	m, _, err := v.store.Load(app)
	if err != nil {
		return "", err
	}
	if !m.Services.Secrets {
		return "", fmt.Errorf("application %q does not enable managed secrets", app)
	}
	files, err := application.ExistingRuntimeFiles(v.store, m)
	if err != nil {
		return "", err
	}
	return application.RuntimeIdentityTokenPath(files), nil
}
