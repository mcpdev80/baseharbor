package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServiceBinding is the provider-neutral information needed by developer
// access actions. Callers should prefer these fields over runtime/container
// names so a later provider can satisfy the same action model.
type ServiceBinding struct {
	Kind     string
	Instance string
	Host     string
	Port     string
	Database string
	Username string
	Password string
	URI      string
}

// ResolveServiceBinding loads an already-materialized owner-only service
// binding. It never creates or rotates credentials.
func ResolveServiceBinding(files RuntimeFiles, kind, instance string) (ServiceBinding, error) {
	kind = strings.TrimSpace(kind)
	instance = strings.TrimSpace(instance)
	if kind != "postgres" && kind != "valkey" {
		return ServiceBinding{}, fmt.Errorf("unsupported service binding kind %q", kind)
	}
	if instance == "" {
		return ServiceBinding{}, fmt.Errorf("service instance is required")
	}
	if strings.TrimSpace(files.Bindings) == "" {
		return ServiceBinding{}, fmt.Errorf("application bindings are not materialized")
	}

	dir := filepath.Join(files.Bindings, kind)
	if instance != "default" {
		dir = filepath.Join(dir, instance)
	}
	read := func(name string, required bool) (string, error) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			if !required && os.IsNotExist(err) {
				return "", nil
			}
			if os.IsNotExist(err) {
				return "", fmt.Errorf("%s binding for %s/%s is not materialized; run 'baha app apply'", name, kind, instance)
			}
			return "", fmt.Errorf("read %s binding for %s/%s: %w", name, kind, instance, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	host, err := read("host", true)
	if err != nil { return ServiceBinding{}, err }
	port, err := read("port", true)
	if err != nil { return ServiceBinding{}, err }
	password, err := read("password", true)
	if err != nil { return ServiceBinding{}, err }
	uri, err := read("uri", true)
	if err != nil { return ServiceBinding{}, err }
	username, err := read("username", false)
	if err != nil { return ServiceBinding{}, err }
	database, err := read("database", false)
	if err != nil { return ServiceBinding{}, err }

	return ServiceBinding{
		Kind: kind, Instance: instance, Host: host, Port: port,
		Database: database, Username: username, Password: password, URI: uri,
	}, nil
}
