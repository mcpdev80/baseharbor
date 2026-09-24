package deployment

import (
	"fmt"
	"strings"
)

const (
	ArtifactRepositoryEnvKey = "BASEHARBOR_ARTIFACT_REPOSITORY"
	BuildKitAddressEnvKey     = "BASEHARBOR_BUILDKIT_ADDRESS"
)

// ArtifactDistributionState is deployment-owned source-to-OCI distribution
// configuration. It is intentionally outside the portable application
// contract.
type ArtifactDistributionState struct {
	RepositoryPrefix string
	BuildKitAddress   string
}

func ArtifactDistributionStateFromValues(values map[string]string) (ArtifactDistributionState, error) {
	state := ArtifactDistributionState{
		RepositoryPrefix: strings.TrimSpace(values[ArtifactRepositoryEnvKey]),
		BuildKitAddress:   strings.TrimSpace(values[BuildKitAddressEnvKey]),
	}
	if state.RepositoryPrefix != "" {
		normalized, err := NormalizeArtifactRepositoryPrefix(state.RepositoryPrefix)
		if err != nil {
			return ArtifactDistributionState{}, err
		}
		state.RepositoryPrefix = normalized
	}
	return state, nil
}

func ApplyArtifactDistributionState(values map[string]string, state ArtifactDistributionState) error {
	if values == nil {
		return fmt.Errorf("artifact distribution state target is nil")
	}
	if strings.TrimSpace(state.RepositoryPrefix) != "" {
		normalized, err := NormalizeArtifactRepositoryPrefix(state.RepositoryPrefix)
		if err != nil {
			return err
		}
		values[ArtifactRepositoryEnvKey] = normalized
	} else {
		delete(values, ArtifactRepositoryEnvKey)
	}
	if address := strings.TrimSpace(state.BuildKitAddress); address != "" {
		values[BuildKitAddressEnvKey] = address
	} else {
		delete(values, BuildKitAddressEnvKey)
	}
	return nil
}

func NormalizeArtifactRepositoryPrefix(value string) (string, error) {
	value = strings.TrimSpace(strings.TrimSuffix(value, "/"))
	if value == "" {
		return "", fmt.Errorf("artifact repository prefix is empty")
	}
	if strings.Contains(value, "://") {
		return "", fmt.Errorf("artifact repository prefix must be an OCI registry/repository reference without URL scheme: %q", value)
	}
	if strings.ContainsAny(value, " 	
") {
		return "", fmt.Errorf("artifact repository prefix contains whitespace: %q", value)
	}
	if strings.Contains(value, "@") {
		return "", fmt.Errorf("artifact repository prefix must not contain a digest: %q", value)
	}
	if !strings.Contains(value, "/") {
		return "", fmt.Errorf("artifact repository prefix must include registry and repository path: %q", value)
	}
	return value, nil
}


func ArtifactDestination(repositoryPrefix, application, environment, service string) (string, error) {
	prefix, err := NormalizeArtifactRepositoryPrefix(repositoryPrefix)
	if err != nil {
		return "", err
	}
	app := artifactSlug(application)
	svc := artifactSlug(service)
	tag := artifactSlug(environment)
	if app == "" || svc == "" || tag == "" {
		return "", fmt.Errorf("application, environment and service are required for artifact destination")
	}
	return prefix + "/" + app + "-" + svc + ":" + tag, nil
}

func artifactSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
