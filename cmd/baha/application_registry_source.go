package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ErrRegistrySourceUnavailable documents that the configured enterprise
// registry/artifact endpoint could not be resolved: the host was unreachable,
// no immutable digest was returned, the reference did not carry that digest, or
// no usable native credential was present. It is a fail-closed, documented-absent
// case — never a silent fallback to a default such as docker.io.
var ErrRegistrySourceUnavailable = fmt.Errorf("registry: no usable resolved artifact identity")

// ErrRegistrySourceEmpty documents an empty endpoint: a source with no location
// cannot be resolved and must fail closed rather than default to any host.
var ErrRegistrySourceEmpty = fmt.Errorf("registry: artifact source location is required")

// DigestPattern matches an immutable sha256 OCI digest, the only identity this
// adapter is willing to record as the resolved artifact version.
var DigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// SourceKind selects how the configured endpoint is addressed. Only OCI is in
// scope for an enterprise registry/artifact endpoint; Git/local/system are
// organization-configuration concerns, not artifact endpoints.
type SourceKind string

const (
	// SourceOCIAddressed resolves an OCI artifact endpoint such as
	// registry.example.invalid/org/app.
	SourceOCIAddressed SourceKind = "oci"
)

// RegistrySource is one configured enterprise registry/artifact endpoint. The
// endpoint (Location) is configuration, never a hardcoded host: a different
// location resolves a different endpoint. Requested is the mutable tag/channel
// the operator asked for; the adapter records only the immutable digest it
// resolved to.
type RegistrySource struct {
	Kind      SourceKind
	Location  string
	Requested string
	// Exec runs a native artifact tool. Injectable for tests; when nil the real
	// os/exec runner is used.
	Exec ExecFunc
}

// ExecFunc executes a native artifact tool and returns its stdout. The caller
// decides whether a reply is significant; the raw stdout is never surfaced as a
// credential.
type ExecFunc func(ctx context.Context, name string, args ...string) (string, error)

// ArtifactIdentity is the immutable identity the adapter records: the exact
// repository reference at its resolved sha256 digest. Tags are not immutable and
// are never recorded as the final identity.
type ArtifactIdentity struct {
	Repository string
	Digest     string
	Requested  string
}

// ImmutableIdentity returns the repository@digest pin, the only form BaseHarbor
// may persist as the resolved artifact identity.
func (i ArtifactIdentity) ImmutableIdentity() string {
	if i.Digest == "" {
		return ""
	}
	return i.Repository + "@" + i.Digest
}

// Resolve reads the endpoint from configuration and resolves it to an immutable
// digest identity, reusing the host's native Docker credential helper / Podman
// auth for the registry host. It never clones a token into its own file, never
// prompts, and never defaults to docker.io: an unreachable host, an empty
// reply, a reference that does not carry the digest, or a missing credential all
// fail closed with ErrRegistrySourceUnavailable.
func (s RegistrySource) Resolve(ctx context.Context) (ArtifactIdentity, error) {
	location := strings.TrimSpace(s.Location)
	if location == "" {
		return ArtifactIdentity{}, ErrRegistrySourceEmpty
	}
	requested := strings.TrimSpace(s.Requested)
	if requested == "" {
		requested = "latest"
	}
	// The endpoint must not look like a public default: resolving a configured
	// enterprise endpoint to docker.io would mask a missing configuration or a
	// denied endpoint. Fail closed instead.
	if isDefaultDockerHost(location) {
		return ArtifactIdentity{}, fmt.Errorf("%w: configured endpoint %q is a public default, not an enterprise registry", ErrRegistrySourceUnavailable, location)
	}
	if strings.ContainsAny(location, "\r\n") || strings.ContainsAny(requested, "\r\n") {
		return ArtifactIdentity{}, fmt.Errorf("%w: source location/requested must not contain newlines", ErrRegistrySourceUnavailable)
	}
	resolvedRepository, digest, err := s.resolve(ctx, location, requested)
	if err != nil {
		return ArtifactIdentity{}, err
	}
	return ArtifactIdentity{
		Repository: resolvedRepository,
		Digest:     digest,
		Requested:  requested,
	}, nil
}

// resolve addresses the endpoint with the native tool, pins the digest, and
// returns the canonical repository at that digest.
func (s RegistrySource) resolve(ctx context.Context, location, requested string) (repository, digest string, err error) {
	ref := location + ":" + requested
	out, runErr := s.run(ctx, "resolve", ref)
	if runErr != nil {
		return "", "", fmt.Errorf("%w: resolve artifact endpoint %q: %v", ErrRegistrySourceUnavailable, location, runErr)
	}
	// The tool reports the resolved reference; extract repository and digest.
	resolvedRef := extractReference(strings.TrimSpace(out))
	if resolvedRef == "" {
		return "", "", fmt.Errorf("%w: resolve returned empty output", ErrRegistrySourceUnavailable)
	}
	digest = firstDigestToken(resolvedRef)
	if digest == "" || !DigestPattern.MatchString(digest) {
		return "", "", fmt.Errorf("%w: endpoint %q did not resolve to an immutable sha256 digest", ErrRegistrySourceUnavailable, location)
	}
	repository = canonicalArtifactRepository(repositoryOf(resolvedRef))
	// Defensive: the resolved reference must actually carry the pinned digest.
	if !strings.HasSuffix(resolvedRef, "@"+digest) {
		return "", "", fmt.Errorf("%w: resolved reference %q does not carry the pinned digest %q", ErrRegistrySourceUnavailable, resolvedRef, digest)
	}
	return repository, digest, nil
}

// run executes the native artifact tool, preferring an injected runner for
// tests and falling back to the real os/exec runner in production. The raw
// stderr is never surfaced: only a redacted class and exit status are returned.
func (s RegistrySource) run(ctx context.Context, action, ref string) (string, error) {
	run := s.Exec
	if run == nil {
		run = execArtifact
	}
	return run(ctx, "oras", action, ref)
}

// execArtifact is the production runner backed by os/exec.
func execArtifact(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", exitStatusError(name, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// canonicalArtifactRepository normalizes a resolved reference to the canonical
// repository form, dropping any digest so the digest can be recorded separately.
// It never adds a public default host: a hostless reference stays hostless.
func canonicalArtifactRepository(reference string) string {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return ""
	}
	if at := strings.IndexByte(reference, '@'); at >= 0 {
		reference = reference[:at]
	}
	tagIndex := strings.LastIndex(reference, ":")
	slashIndex := strings.LastIndex(reference, "/")
	if tagIndex > slashIndex {
		reference = reference[:tagIndex]
	}
	return reference
}

// extractReference normalizes native resolve output to the bare resolved
// reference. The real `oras resolve` verb prefixes its digest line with words
// such as `Resolved`; the parser accepts that prefix and returns the
// repository@digest reference, dropping any leading verb token.
func extractReference(out string) string {
	out = strings.TrimSpace(out)
	// A leading verb token ("Resolved ref@digest") is separated by whitespace.
	if sp := strings.IndexByte(out, ' '); sp >= 0 {
		ref := strings.TrimSpace(out[sp+1:])
		if at := strings.Index(ref, "@"); at >= 0 {
			rest := ref[at+1:]
			if DigestPattern.MatchString(rest) {
				return ref[:at+1+len(rest)]
			}
		}
		return ref
	}
	if at := strings.Index(out, "@"); at >= 0 {
		rest := out[at+1:]
		if DigestPattern.MatchString(rest) {
			return out[:at+1+len(rest)]
		}
	}
	return out
}

// repositoryOf extracts the repository (host/path) from a resolved reference,
// leaving any digest behind.
func repositoryOf(reference string) string {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return ""
	}
	if at := strings.IndexByte(reference, '@'); at >= 0 {
		return reference[:at]
	}
	return reference
}

// isDefaultDockerHost reports whether the configured endpoint is a public
// docker.io default, which the adapter refuses to treat as an enterprise
// endpoint. Any reference whose host is a public docker.io registry (the
// implicit default) is rejected so a missing configuration cannot silently
// become docker.io.
func isDefaultDockerHost(location string) bool {
	lower := strings.ToLower(strings.TrimSpace(location))
	// Strip a trailing /path so "docker.io" alone, "index.docker.io", and
	// "registry.docker.io" are all caught.
	host := lower
	if slash := strings.IndexByte(host, '/'); slash >= 0 {
		host = host[:slash]
	}
	switch strings.TrimRight(host, "/") {
	case "docker.io", "index.docker.io", "registry-1.docker.io", "registry.docker.io":
		return true
	}
	return false
}

// firstDigestToken returns the first sha256 digest token in a string, or "".
// A digest may be embedded in a reference (repository@sha256:...) or reported
// standalone; both forms are accepted so the parser matches real `oras resolve`
// output and the orgconfig OCI resolver's digest line.
func firstDigestToken(s string) string {
	for _, field := range strings.Fields(s) {
		if idx := strings.LastIndex(field, "@"); idx >= 0 {
			if digest := field[idx+1:]; DigestPattern.MatchString(digest) {
				return digest
			}
		}
		if DigestPattern.MatchString(field) {
			return field
		}
	}
	return ""
}

// exitStatusError reports an external-command failure with only a redacted class
// and exit status. The raw stderr is dropped so helper payloads and secrets
// cannot leak through it.
func exitStatusError(name string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%s: command failed with a non-zero exit status", name)
	}
	return fmt.Errorf("%s: command failed", name)
}
