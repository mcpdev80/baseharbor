package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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

// ErrRegistrySourcePublicDefault documents a configured endpoint that is a
// public docker.io default rather than an enterprise registry. It is a hard
// rejection: never the merely-unavailable class the development opt-in
// fallback may apply to. It IS the distinct hard-rejection identity that the
// classifier and the negative cells assert (via the errClassPublicDefault
// alias), unwrapping to the unavailable identity only for one-call error
// classification.
var ErrRegistrySourcePublicDefault = errors.New("registry: configured endpoint is a public default")

// errClassPublicDefault aliases the public-default hard-rejection identity so
// rejection sites and the classifier name the class they check.
var errClassPublicDefault = ErrRegistrySourcePublicDefault

// errRegistrySourceDenied is the hard-rejection class identity for endpoint
// refusals (authorization denied/forbidden/unauthorized replies). Like the
// public-default sentinel it is distinct and never unwraps to the unavailable
// identity; the classifier recognizes denials with it.
var errRegistrySourceDenied = errors.New("registry: endpoint refused the resolve")

// errClassDenied aliases the denial hard-rejection identity.
var errClassDenied = errRegistrySourceDenied

// ErrRegistryUnverifiable is the distinct hard-rejection identity for an OCI
// source image that could not be verified: the native tool ran but the reply
// was empty, carried no immutable sha256 digest, or did not actually pin it.
// An UNVERIFIABLE resolve must never qualify the mutable fallback behind the
// development opt-in: falling back would pull a tag whose content was never
// proven, exactly the mutable-reference risk this adapter exists to close.
// Like the denial identity it also unwraps to the unavailable class so the
// machine-error classification in main.go keeps its semantics; what excludes
// it from the fallback is the identity check in isHardRejection, which is
// listed FIRST so errors.Is can never short-circuit through the alias.
var ErrRegistryUnverifiable = fmt.Errorf("%w: %w: the OCI source image could not be verified to an immutable digest", ErrRegistrySourceUnavailable, errRegistrySourceUnverifiable)

// errRegistrySourceUnverifiable carries the bare, distinct unverifiable
// identity node: it aliases NOTHING, so the node-identity walk in
// isHardRejection can single it out of a chain that also contains the shared
// unavailable alias. The sentinel above unwraps to it (and through it to the
// unavailable class, exactly like the denial identity does).
var errRegistrySourceUnverifiable = errors.New("registry: unverifiable OCI source image")

// errClassUnverifiable aliases the unverifiable hard-rejection identity.
var errClassUnverifiable = errRegistrySourceUnverifiable

// ErrRegistryNativeToolMissing documents that the host lacks the native OCI
// artifact tool (oras) needed to resolve a configured endpoint. It is a
// fail-closed, actionable absence — not the generic "command failed" class:
// the message names the missing binary and the remediation. It unwraps to
// ErrRegistrySourceUnavailable so every existing class gate keeps its exact
// semantics, and the identity itself is what the caller and the tests match
// on. An oras that exists but fails for another reason (network, denial)
// never carries this identity.
var ErrRegistryNativeToolMissing = fmt.Errorf("%w: %w: the native OCI artifact tool 'oras' is not installed; install ORAS or provide the runtime image through the container runtime", ErrRegistrySourceUnavailable, exec.ErrNotFound)

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
		return ArtifactIdentity{}, fmt.Errorf("%w: %w: configured endpoint %q is a public default, not an enterprise registry", ErrRegistrySourceUnavailable, ErrRegistrySourcePublicDefault, location)
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
	// The public-default classification never depends on the native tool: the
	// host check runs first, so no tool reply can downgrade a docker.io
	// endpoint into the unavailable class the development opt-in may cover.
	if isDefaultDockerHost(location) {
		return "", "", fmt.Errorf("%w: %w: configured endpoint %q is a public default, not an enterprise registry", ErrRegistrySourceUnavailable, ErrRegistrySourcePublicDefault, location)
	}
	ref := location + ":" + requested
	out, runErr := s.run(ctx, "resolve", ref)
	if runErr != nil {
		if orgconfig.IsNativeToolNotFound(runErr) {
			// The host simply lacks the oras binary. Report the actionable
			// missing-tool identity instead of the generic redacted
			// "oras: command failed"; it unwraps to the unavailable identity
			// so every class gate behaves exactly as before.
			return "", "", ErrRegistryNativeToolMissing
		}
		if isEndpointDenial(runErr) {
			// Classify an endpoint refusal with both sentinels so the error
			// still unwraps to ErrRegistrySourceUnavailable for callers while
			// the pull-side fallback — which excludes hard-rejection
			// identities in the cause chain — can never apply to a denial.
			return "", "", fmt.Errorf("%w: %w: resolve artifact endpoint %q: %v",
				ErrRegistrySourceUnavailable, errRegistrySourceDenied, location, runErr)
		}
		return "", "", fmt.Errorf("%w: resolve artifact endpoint %q: %v", ErrRegistrySourceUnavailable, location, runErr)
	}
	// The tool reports the resolved reference; extract repository and digest.
	// Every failure below is an UNVERIFIABLE resolution: the image could not
	// be proven to an immutable digest, so it carries the distinct hard-
	// rejection identity and can never qualify the mutable fallback.
	resolvedRef := extractReference(strings.TrimSpace(out))
	if resolvedRef == "" {
		return "", "", fmt.Errorf("%w: resolve returned empty output", ErrRegistryUnverifiable)
	}
	digest = firstDigestToken(resolvedRef)
	if digest == "" || !DigestPattern.MatchString(digest) {
		return "", "", fmt.Errorf("%w: endpoint %q did not resolve to an immutable sha256 digest", ErrRegistryUnverifiable, location)
	}
	repository = canonicalArtifactRepository(repositoryOf(resolvedRef))
	// Defensive: the resolved reference must actually carry the pinned digest.
	if !strings.HasSuffix(resolvedRef, "@"+digest) {
		return "", "", fmt.Errorf("%w: resolved reference %q does not carry the pinned digest %q", ErrRegistryUnverifiable, resolvedRef, digest)
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
		// Preserve the executable-not-found identity through the redaction:
		// only a plain non-zero exit becomes the generic class. Wrapping with
		// %w keeps the raw stderr private while letting callers classify a
		// missing tool (oras) with ErrRegistryNativeToolMissing.
		var execErr *exec.Error
		if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			return "", fmt.Errorf("%w: %s: executable file not found in $PATH", err, name)
		}
		return "", exitStatusError(name, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// resolveRegistrySource drives the configured OCI endpoint through
// RegistrySource.Resolve and returns an image reference that the runtime can
// pull. It is the production seam that connects the application lifecycle to
// RegistrySource: the release/image selection resolves the configured endpoint
// to an immutable sha256 digest identity, and the caller pulls that exact pin.
//
// Resolution is best-effort for a mutable development image. The adapter fails
// closed for a configured enterprise endpoint, but a mutable development image
// is resolved on demand from whatever native tooling is present; when the
// endpoint cannot be resolved (no native tool, unreachable host, no usable
// digest) the original tag reference is returned so the existing mutable-image
// path is preserved. Callers pull exactly what is returned.
//
// It defaults to resolveRegistrySourceImpl; tests install a fake runner via
// registrySourceExec to observe the real application lifecycle's native
// invocation.
var resolveRegistrySource = resolveRegistrySourceImpl

// isEndpointDenial reports whether the native tool's error indicates the
// endpoint refused the request (authorization denial) rather than being merely
// unreachable. The classifier only inspects the message text of the tool
// error; denials are a hard rejection class the opt-in fallback never covers.
//
// NOTE: exitStatusError redacts stderr to a fixed string, so on production
// paths a refusal usually arrives as a plain non-zero exit and lands in the
// unavailable class; the denial marker keeps the classification hook for the
// replies that do carry their refusal text.
func isEndpointDenial(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "denied") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "access denied")
}

// registrySourceExec is the native artifact tool runner used by
// resolveRegistrySourceImpl. It defaults to the real os/exec runner; tests
// install a fake runner that records the invocation and canned resolve output.
var registrySourceExec ExecFunc = execArtifact

// resolveUnavailable reports whether the resolve failure is of the merely-
// unavailable class: the endpoint was unreachable or the tool failed to
// reach/answer it. A hard rejection (denial, public default, or an
// UNVERIFIABLE resolve — empty, unpinnable or digest-mismatched reply) is
// NEVER a merely-unavailable result, so its sentinel takes precedence even
// though it unwraps to the unavailable identity. This is the ONLY class the
// development opt-in fallback may ever pull the mutable reference for; in
// particular, an image that resolved but could not be verified to an
// immutable digest never qualifies: fallback would pull unproven content.
func resolveUnavailable(err error) bool {
	if isHardRejection(err) {
		return false
	}
	return errors.Is(err, ErrRegistrySourceUnavailable)
}

// isHardRejection reports whether an error in the cause chain is one of the
// distinct hard-rejection identities: the public-default sentinel or a
// classified endpoint denial. Unlike the unavailable class these can never be
// covered by the development opt-in fallback.
// The identity tests below are deliberate: isHardRejection and
// resolveUnavailable walk the cause chain node by node, so each node must be
// compared by identity rather than by errors.Is — otherwise a hard sentinel
// that aliases the unavailable identity would hide itself.
// The identity comparison is deliberate: resolveUnavailable walks the cause
// chain node by node, so each node must be compared by identity rather than
// with errors.Is — otherwise an alias of the unavailable identity (mutant or
// misplacement) would hide itself.
func isHardRejection(err error) bool {
	// Node-identity walk: the chain is inspected node by node because the
	// multi-sentinel wrappers put the plain unavailable identity FIRST
	// (errors.Join layout), so errors.Is can never distinguish "carries a
	// hard identity" from "is merely unavailable" — an unavailable-only
	// chain answers true to errors.Is on any class alias it contains.
	// Only the bare identity nodes carry the decision.
	for node := err; node != nil; node = errors.Unwrap(node) {
		switch node {
		case error(errClassUnverifiable), error(errClassPublicDefault), error(errClassDenied):
			return true
		}
		if joined, ok := node.(interface{ Unwrap() []error }); ok {
			if joinHardRejection(joined) {
				return true
			}
		}
	}
	return false
}

// joinHardRejection walks one level of a multi-error (errors.Join layout, as
// produced by the "%w: %w" wrappers) with the same node-identity rule.
func joinHardRejection(join interface{ Unwrap() []error }) bool {
	for _, child := range join.Unwrap() {
		switch child {
		case error(errClassUnverifiable), error(errClassPublicDefault), error(errClassDenied):
			return true
		}
	}
	return false
}

// resolveRegistrySourceImpl is the production resolveRegistrySource.
func resolveRegistrySourceImpl(ctx context.Context, location, requested string) (string, error) {
	// The configured endpoint (e.g. BASEHARBOR_RUNTIME_IMAGE) already carries a
	// tag, so strip it to a bare repository and let the caller's requested
	// channel select the tag; an empty requested keeps the location's own tag.
	repository, tag := splitReference(location)
	if location == "" {
		return "", ErrRegistrySourceEmpty
	}
	// A public-default host must fail closed here, before the runner seam: a
	// CI host that happens to have oras installed could otherwise "resolve"
	// docker.io and pull from the public default. RegistrySource.Resolve
	// applies the same rule; this pre-flight makes the rejection independent
	// of tool availability.
	if isDefaultDockerHost(repository) {
		return "", fmt.Errorf("%w: %w: configured endpoint %q is a public default, not an enterprise registry", ErrRegistrySourceUnavailable, ErrRegistrySourcePublicDefault, location)
	}
	if requested == "" {
		if tag != "" {
			requested = tag
		} else {
			requested = "latest"
		}
	}
	identity, err := (RegistrySource{
		Kind:      SourceOCIAddressed,
		Location:  repository,
		Requested: requested,
		Exec:      registrySourceExec,
	}).Resolve(ctx)
	if err != nil {
		// Fail closed: a rejected endpoint (denied/unreachable, missing
		// digest, public-default docker.io) propagates so the caller performs
		// no pull of a mutable reference. The only mutable fallback lives in
		// pullResolvedRuntimeImage behind an explicit operator opt-in, and the
		// opt-in is class-gated: it never applies to a classified endpoint
		// denial or to a public-default rejection.
		return "", err
	}
	return identity.ImmutableIdentity(), nil
}

// splitReference separates a repository reference into its bare repository and
// trailing tag. A reference without a tag returns an empty tag.
func splitReference(reference string) (repository, tag string) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", ""
	}
	slash := strings.LastIndex(reference, "/")
	colon := strings.LastIndex(reference, ":")
	if colon > slash && !strings.Contains(reference[colon:], "@") {
		return reference[:colon], reference[colon+1:]
	}
	return reference, ""
}

// pullResolvedRuntimeImage is the single pull site (cmd/baha/app_broker.go) that
// connects the runtime lifecycle to RegistrySource. It resolves the configured
// endpoint through resolveRegistrySource and pulls exactly the immutable
// repository@digest identity it returns. Any resolution failure propagates
// fail-closed with zero pulls. The only mutable fallback is the explicit
// operator opt-in BASEHARBOR_ALLOW_MUTABLE_DEVELOPMENT_IMAGE, and it runs only
// when the endpoint was classified UNAVAILABLE — a denied endpoint or a
// public-default docker.io location is rejected before the fallback is
// consulted, so the opt-in cannot bypass the endpoint or public-default
// rejection rules. A caller that pulls a mutable reference on a rejected
// resolve is the RED mutant this path proves.
func pullResolvedRuntimeImage(ctx context.Context, compose bhruntime.RuntimeProvider, location, requested string) error {
	resolvedImage, err := resolveRegistrySource(ctx, location, requested)
	if err != nil {
		if errors.Is(err, ErrRegistryNativeToolMissing) {
			// Fail closed with the actionable cause FIRST: a host without the
			// oras binary gets the install instruction, never an opaque
			// "oras: command failed", even when the mutable development
			// opt-in is on. The opt-in below still governs the pull reference;
			// this branch only names the resolve cause.
			return fmt.Errorf("resolve development runtime image: %w", err)
		}
		if resolveUnavailable(err) && allowMutableDevelopmentRuntimeImage() {
			// Explicit legacy development opt-in: pull the original tag
			// reference. Class-gated: a public-default rejection and an
			// endpoint denial both fail closed above, so this fallback
			// bypasses no rejection rule.
			resolvedImage = location
		} else {
			return fmt.Errorf("resolve development runtime image: %w", err)
		}
	}
	if err := compose.PullImage(ctx, resolvedImage); err != nil {
		// Apply the same native-tool classifier as OCI Resolve, but never
		// expose untrusted runtime/registry stderr through error wrapping.
		classified := orgconfig.ClassifyNativeToolFailure("oras", err)
		if errors.Is(classified, orgconfig.ErrNativeToolNotFound) {
			return fmt.Errorf("pull resolved development runtime image: %w", ErrRegistryNativeToolMissing)
		}
		return fmt.Errorf("pull resolved development runtime image: %w", ErrRegistrySourceUnavailable)
	}
	return nil
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
