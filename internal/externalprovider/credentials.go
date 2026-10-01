package externalprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Credentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

func ResolveCredentialReference(reference string) (Credentials, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Credentials{}, nil
	}
	u, err := url.Parse(reference)
	if err != nil {
		return Credentials{}, fmt.Errorf("parse credential reference: %w", err)
	}
	if u.Scheme != "file" {
		return Credentials{}, fmt.Errorf("credential reference scheme %q is not available in this runtime; use an installed secret-provider resolver", u.Scheme)
	}
	path := u.Path
	if path == "" {
		path = u.Opaque
	}
	if strings.TrimSpace(path) == "" {
		return Credentials{}, errors.New("file credential reference requires a path")
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("inspect credential reference: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Credentials{}, errors.New("credential reference must point to a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Credentials{}, fmt.Errorf("credential reference %q must not be group/world accessible", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("read credential reference: %w", err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("decode credential reference: %w", err)
	}
	if creds.Username == "" && creds.Password == "" && creds.Token == "" {
		return Credentials{}, errors.New("credential reference contains no supported credentials")
	}
	return creds, nil
}
