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
	Target      string    `json:"target"`
	Environment string    `json:"environment"`
	Issuer      string    `json:"issuer"`
	Subject     string    `json:"subject"`
	ClientID    string    `json:"client_id"`
	IDToken     string    `json:"id_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s Session) ValidAt(now time.Time) bool {
	return strings.TrimSpace(s.Target) != "" &&
		strings.TrimSpace(s.Environment) != "" &&
		strings.TrimSpace(s.Issuer) != "" &&
		strings.TrimSpace(s.Subject) != "" &&
		strings.TrimSpace(s.ClientID) != "" &&
		strings.TrimSpace(s.IDToken) != "" &&
		s.ExpiresAt.After(now.Add(30*time.Second))
}

func LoadSession(target, environment string) (Session, error) {
	path, err := SessionPath(target, environment)
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
	if session.Target != strings.TrimSpace(target) || session.Environment != strings.TrimSpace(environment) {
		return Session{}, ErrAuthenticationRequired
	}
	return session, nil
}

func SaveSession(session Session) error {
	if !session.ValidAt(time.Now()) {
		return errors.New("refusing to persist invalid or expired operator session")
	}
	path, err := SessionPath(session.Target, session.Environment)
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

func ClearSession(target, environment string) error {
	path, err := SessionPath(target, environment)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

type SessionObservation struct {
	Present     bool      `json:"present"`
	Valid       bool      `json:"valid"`
	Target      string    `json:"target,omitempty"`
	Environment string    `json:"environment,omitempty"`
	Issuer      string    `json:"issuer,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	ClientID    string    `json:"client_id,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

func ObserveSession(target, environment string, cfg Config) (SessionObservation, error) {
	session, err := LoadSession(target, environment)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrAuthenticationRequired) {
		return SessionObservation{}, nil
	}
	if err != nil {
		return SessionObservation{}, err
	}
	observation := SessionObservation{
		Present: true, Target: session.Target, Environment: session.Environment,
		Issuer: session.Issuer, Subject: session.Subject, ClientID: session.ClientID,
		ExpiresAt: session.ExpiresAt,
	}
	observation.Valid = session.ValidAt(time.Now()) &&
		session.Issuer == strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/") &&
		session.ClientID == strings.TrimSpace(cfg.ClientID)
	return observation, nil
}

func VerifySession(ctx context.Context, target, environment string, cfg Config) (*identity.Principal, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	session, err := LoadSession(target, environment)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrAuthenticationRequired
		}
		return nil, err
	}
	if !session.ValidAt(time.Now()) || session.Issuer != strings.TrimRight(cfg.Issuer, "/") || session.ClientID != cfg.ClientID {
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
