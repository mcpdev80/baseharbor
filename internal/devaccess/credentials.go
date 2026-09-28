package devaccess

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

const DefaultUsername = "developer"

type Credentials struct {
	Username string
	Password string
}

func Enabled(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "dev", "development":
		return true
	default:
		return false
	}
}

func Ensure(target, environment string) (Credentials, error) {
	if !Enabled(environment) {
		return Credentials{}, fmt.Errorf("developer access is only available for development environments")
	}
	path, err := credentialsPath(target, environment)
	if err != nil {
		return Credentials{}, err
	}
	if credentials, err := Load(target, environment); err == nil {
		return credentials, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Credentials{}, err
	}
	password, err := randomPassword(24)
	if err != nil {
		return Credentials{}, err
	}
	credentials := Credentials{Username: DefaultUsername, Password: password}
	if err := write(path, credentials); err != nil {
		return Credentials{}, err
	}
	return credentials, nil
}

func Configure(target, environment, username, password string) (Credentials, error) {
	if !Enabled(environment) {
		return Credentials{}, fmt.Errorf("developer access is only available for development environments")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = DefaultUsername
	}
	credentials := Credentials{Username: username, Password: password}
	path, err := credentialsPath(target, environment)
	if err != nil {
		return Credentials{}, err
	}
	if err := write(path, credentials); err != nil {
		return Credentials{}, err
	}
	return credentials, nil
}

func Reset(target, environment, username string) (Credentials, error) {
	if strings.TrimSpace(username) == "" {
		if existing, err := Load(target, environment); err == nil {
			username = existing.Username
		} else if !errors.Is(err, os.ErrNotExist) {
			return Credentials{}, err
		}
	}
	if strings.TrimSpace(username) == "" {
		username = DefaultUsername
	}
	password, err := randomPassword(24)
	if err != nil {
		return Credentials{}, err
	}
	return Configure(target, environment, username, password)
}

func Load(target, environment string) (Credentials, error) {
	path, err := credentialsPath(target, environment)
	if err != nil {
		return Credentials{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Credentials{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Credentials{}, errors.New("developer access credentials must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Credentials{}, fmt.Errorf("developer access credentials are accessible by group or others (%o)", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Credentials{}, errors.New("developer access credentials are invalid")
		}
		values[strings.TrimSpace(key)] = value
	}
	credentials := Credentials{
		Username: strings.TrimSpace(values["USERNAME"]),
		Password: values["PASSWORD"],
	}
	if credentials.Username == "" || credentials.Password == "" {
		return Credentials{}, errors.New("developer access credentials are incomplete")
	}
	return credentials, nil
}

func Path(target, environment string) (string, error) {
	return credentialsPath(target, environment)
}

func credentialsPath(target, environment string) (string, error) {
	if !Enabled(environment) {
		return "", fmt.Errorf("developer access is only available for development environments")
	}
	root, err := deployment.TargetStateRoot(strings.TrimSpace(target))
	if err != nil {
		return "", err
	}
	env := strings.ToLower(strings.TrimSpace(environment))
	if env == "development" {
		env = "dev"
	}
	return filepath.Join(root, "developer-access", env, "credentials.env"), nil
}

func write(path string, credentials Credentials) error {
	if strings.TrimSpace(credentials.Username) == "" || credentials.Password == "" {
		return errors.New("developer access credentials are incomplete")
	}
	if strings.ContainsAny(credentials.Username, "\r\n=") || strings.ContainsAny(credentials.Password, "\r\n") {
		return errors.New("developer access credentials contain invalid control characters")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create developer access state: %w", err)
	}
	data := []byte("USERNAME=" + credentials.Username + "\nPASSWORD=" + credentials.Password + "\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write developer access credentials: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install developer access credentials: %w", err)
	}
	return nil
}

func randomPassword(size int) (string, error) {
	if size < 16 {
		return "", errors.New("developer access password length is too small")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate developer access password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
