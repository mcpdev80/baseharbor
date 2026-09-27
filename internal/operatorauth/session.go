package operatorauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/identity"
)

type Session struct {
	Issuer    string    `json:"issuer"`
	Subject   string    `json:"subject"`
	ClientID  string    `json:"client_id"`
	IDToken   string    `json:"id_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s Session) ValidAt(now time.Time) bool {
	return strings.TrimSpace(s.Issuer) != "" &&
		strings.TrimSpace(s.Subject) != "" &&
		strings.TrimSpace(s.ClientID) != "" &&
		strings.TrimSpace(s.IDToken) != "" &&
		s.ExpiresAt.After(now.Add(30*time.Second))
}

func LoadSession() (Session, error) {
	path, err := SessionPath()
	if err != nil {
		return Session{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, fmt.Errorf("decode operator session: %w", err)
	}
	return session, nil
}

func SaveSession(session Session) error {
	if !session.ValidAt(time.Now()) {
		return errors.New("refusing to persist invalid or expired operator session")
	}
	path, err := SessionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func ClearSession() error {
	path, err := SessionPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func VerifySession(ctx context.Context, cfg Config) (*identity.Principal, error) {
	session, err := LoadSession()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrAuthenticationRequired
		}
		return nil, err
	}
	if !session.ValidAt(time.Now()) || session.Issuer != cfg.Issuer || session.ClientID != cfg.ClientID {
		return nil, ErrAuthenticationRequired
	}
	verifier, err := auth.NewOIDCVerifier(ctx, auth.Config{Issuer: cfg.Issuer, Audiences: []string{cfg.ClientID}})
	if err != nil {
		return nil, err
	}
	principal, err := verifier.Verify(ctx, session.IDToken)
	if err != nil {
		return nil, ErrAuthenticationRequired
	}
	if principal.Subject != session.Subject {
		return nil, ErrAuthenticationRequired
	}
	return principal, nil
}
