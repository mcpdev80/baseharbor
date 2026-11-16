package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// remoteCreateIntentFromRepository uses the authored contract as the remote
// registration intent when create is run from that application's repository.
// Service flags remain available for target-managed apps created elsewhere.
func remoteCreateIntentFromRepository(args []string, fallback application.Manifest) (application.Manifest, bool, error) {
	path, err := application.FindRepositoryManifest(".")
	if errors.Is(err, application.ErrRepositoryManifestNotFound) {
		return fallback, false, nil
	}
	if err != nil {
		return application.Manifest{}, false, err
	}
	manifest, err := application.LoadManifestFile(path)
	if err != nil {
		return application.Manifest{}, false, err
	}
	if manifest.Name != fallback.Name {
		return fallback, false, nil
	}
	if manifest.Environment != fallback.Environment {
		return application.Manifest{}, false, errors.New("remote app create environment does not match the repository manifest")
	}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		var requestedEnvironment string
		switch {
		case arg == "--environment":
			if i+1 >= len(args) {
				return application.Manifest{}, false, usageError("--environment requires a value", "Pass the same environment as the repository manifest.")
			}
			i++
			requestedEnvironment = args[i]
		case strings.HasPrefix(arg, "--environment="):
			requestedEnvironment = strings.TrimPrefix(arg, "--environment=")
		default:
			return application.Manifest{}, false, usageError(
				"service options cannot override a matching repository manifest",
				"Run baha app create NAME from the repository to register its full baseharbor.yaml contract, or create a target-managed application outside that repository with explicit service options.",
			)
		}
		if requestedEnvironment != "" && requestedEnvironment != manifest.Environment {
			return application.Manifest{}, false, errors.New("remote app create environment does not match the repository manifest")
		}
	}
	manifest.ApplicationID = ""
	return manifest, true, nil
}

// persistRemoteRepositoryIdentity records only the identity returned by the
// verified Core. The authored repository manifest remains free of system IDs.
func persistRemoteRepositoryIdentity(root string, intent application.Manifest, applicationID string) (string, error) {
	if strings.TrimSpace(applicationID) == "" {
		return "", errors.New("remote Core returned an empty application identity")
	}
	owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	items, err := owner.List()
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Name == intent.Name && item.ApplicationID != applicationID {
			return "", fmt.Errorf("repository already owns application ID %s; refusing to replace it with Core ID %s", item.ApplicationID, applicationID)
		}
		if item.Name != intent.Name && item.ApplicationID == applicationID {
			return "", fmt.Errorf("Core application ID is already owned by repository app %q", item.Name)
		}
	}
	intent.ApplicationID = applicationID
	return owner.Sync(intent)
}
