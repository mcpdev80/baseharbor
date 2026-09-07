package auth

import (
	"errors"
	"strings"
)

var (
	ErrMissingAuthorization = errors.New("authorization header required")
	ErrInvalidBearerFormat  = errors.New("authorization must use bearer scheme")
)

// BearerToken parses a bearer token from an Authorization header.
// It returns only public, stable errors and never includes token material.
func BearerToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", ErrMissingAuthorization
	}

	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", ErrInvalidBearerFormat
	}

	return parts[1], nil
}
