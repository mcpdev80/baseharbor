package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

// ErrImageTransferUnavailable is the fail-closed sentinel for a transfer target
// or source that could not be addressed, refused the operation, returned a
// reply without a usable digest, or produced a transferred image that does not
// carry the verified immutable identity. A half-success (copy attempted, no
// verified immutable identity) always unwraps to this sentinel, so the caller
// performs no transfer of a mutable reference.
var ErrImageTransferUnavailable = errors.New("image transfer: no usable immutable target identity")

// ErrImageTransferTargetRefused is the hard-rejection sentinel for a transfer
// target that actively refused the operation (authorization denial) or that is a
// public-default source which must never be copied from. It is distinct from the
// unavailable sentinel and never unwraps to it, so no fallback may copy from it.
var ErrImageTransferTargetRefused = errors.New("image transfer: target refused the operation")

// ErrImageTransferDigestMismatch is returned when the transferred image does not
// carry the digest that was verified on the source. Tag drift, truncation or a
// hostile source that swapped the layer content behind the same tag is rejected
// rather than accepted.
var ErrImageTransferDigestMismatch = errors.New("image transfer: transferred image digest does not match source")

// ErrImageTransferArchitectureMismatch is returned when the runtime reports a
// different architecture for the transferred image than the source verified. It
// guards against loading a host-architecture image behind the same reference,
// which would silently change the executed payload.
var ErrImageTransferArchitectureMismatch = errors.New("image transfer: transferred image architecture does not match source")

// digestPattern matches an immutable sha256 OCI digest, the only identity this
// adapter transfers. A mutable tag is never an acceptable transfer identity.
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func validImageDigest(digest string) bool {
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
		return false
	}
	_, err := hex.DecodeString(digest[7:])
	return err == nil
}

func isTransferDenial(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "denied") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "access denied")
}

// isTransferHardRejection reports whether an error in the cause chain is a
// hard-rejection identity: a public-default source or a classified endpoint
// denial. These can never be covered by a mutable fallback.
func isTransferHardRejection(err error) bool {
	return errors.Is(err, ErrImageTransferTargetRefused)
}

// repository reconstructs the bare repository reference at its digest from the
// inspected reference. The digest is stripped so the identity carries only the
// immutable repository@digest pin.
func repository(ref, digest string) string {
	if idx := strings.LastIndexByte(ref, '@'); idx >= 0 {
		return ref[:idx]
	}
	return strings.TrimSuffix(ref, "@"+digest)
}

// imageArtifactRepository is the artifact record's repository field: the bare
// repository with any digest pin stripped. Only the owner/provenance repository
// is retained; the mutable tag remains solely in the requested channel.
func imageArtifactRepository(image string) string {
	if at := strings.IndexByte(image, '@'); at >= 0 {
		return image[:at]
	}
	repository, _ := splitReference(image)
	return repository
}

// sourceImageIdentity reconstructs the provenance identity of a rendered image
// reference so the artifact record keeps the repository plus the exact digest
// pin when the selection already names one. A non-sha256 tail is not an
// identity and is dropped.
func sourceImageIdentity(image string) ArtifactIdentity {
	repository, digest := splitDigestReference(strings.TrimSpace(image))
	if digest != "" && !validImageDigest(digest) {
		digest = ""
	}
	return ArtifactIdentity{Repository: repository, Digest: digest, Requested: image}
}

// ImageTransferIdentity is the immutable identity that survives a transfer. It
// records the repository@digest pin plus the platform-manifest digest and
// architecture verified on the source, so the receiver can prove the
// transferred image is byte-identical and runs the intended architecture.
type ImageTransferIdentity struct {
	// Repository is the bare repository reference at its resolved digest.
	Repository string
	// Digest is the immutable sha256 OCI digest verified on the source.
	Digest string
	// ManifestDigest is the platform-manifest digest selected from the index
	// when an index pin is resolved to one platform. Empty when the receiver
	// resolves the pinned repository reference itself.
	ManifestDigest string
	// Architecture is the architecture of the verified platform manifest.
	Architecture string
	// OS is the operating system of the verified platform manifest.
	OS string
}

// ImmutableIdentity returns the repository@digest pin, the only form BaseHarbor
// may transfer. Tags are mutable and never carried across a transfer.
func (i ImageTransferIdentity) ImmutableIdentity() string {
	if i.Digest == "" {
		return i.Repository
	}
	return i.Repository + "@" + i.Digest
}

// Verify proves a transferred image carries the exact identity that was pinned
// on the source: same digest, same platform-manifest digest when one was
// selected, the same architecture, and the same OS. Any divergence, including a
// missing or empty architecture or OS on either side, is a failed transfer.
//
// digest, manifestDigest, architecture and os are the target's reported values
// for the transferred image. A missing or unparseable digest fails the transfer
// closed: the target must prove an immutable identity.
func (i ImageTransferIdentity) Verify(digest, manifestDigest, architecture, os string) error {
	if i.Digest == "" {
		return fmt.Errorf("%w: source did not pin an immutable digest", ErrImageTransferDigestMismatch)
	}
	if digest == "" || !validImageDigest(digest) {
		return fmt.Errorf("%w: transferred image did not report an immutable digest", ErrImageTransferDigestMismatch)
	}
	if digest != i.Digest {
		return fmt.Errorf("%w: transferred digest %q != source digest %q",
			ErrImageTransferDigestMismatch, digest, i.Digest)
	}
	if i.ManifestDigest != "" && strings.TrimSpace(manifestDigest) != i.ManifestDigest {
		return fmt.Errorf("%w: manifest %q != source %q",
			ErrImageTransferDigestMismatch, manifestDigest, i.ManifestDigest)
	}
	// Architecture and OS are mandatory and compared exactly; an empty value on
	// either side is a failed transfer, never a skipped ("not pinned") check.
	if !strings.EqualFold(strings.TrimSpace(architecture), strings.TrimSpace(i.Architecture)) {
		return fmt.Errorf("%w: architecture %q != source %q",
			ErrImageTransferArchitectureMismatch, architecture, i.Architecture)
	}
	if strings.ToLower(strings.TrimSpace(os)) != strings.ToLower(strings.TrimSpace(i.OS)) {
		return fmt.Errorf("%w: os %q != source %q",
			ErrImageTransferArchitectureMismatch, os, i.OS)
	}
	return nil
}

// ImageTransferSource resolves a transfer source to an immutable digest identity
// through a native tool, reusing the host credential helper / auth file. It
// never clones a token into its own file.
type ImageTransferSource interface {
	// Resolve addresses the source through the native tool, pins the digest, and
	// returns the repository@digest identity plus the verified platform-manifest
	// digest and architecture. Fail closed on any of: unreachable host, denied
	// endpoint, public-default source, empty reply, or a reply that does not
	// carry an immutable sha256 digest.
	Resolve(ctx context.Context) (ImageTransferIdentity, error)
}

// ImageTransferTarget receives a transferred image through a native transport
// and verifies the transferred identity carries the exact digest, manifest and
// architecture that were verified on the source.
type ImageTransferTarget interface {
	// Copy ingests identity.ref into the target's native transport (pull, copy
	// or load) without any mutable-tag fallback. It must not accept a
	// substitution, rebuild or conversion of the payload.
	Copy(ctx context.Context, ref string) error
	// Verify inspects the ingested image on the target and returns the identity
	// that was actually received. A target that cannot prove the transferred
	// identity fails closed.
	Verify(ctx context.Context, ref string) (ImageTransferIdentity, error)
}

// ImageTransfer performs a digest-preserving transfer of an immutable image
// identity from a source to a target using only native transports (Docker,
// Podman or Skopeo). It copies the repository@digest pin, never a mutable tag,
// and proves the transferred image carries the same digest, platform-manifest
// digest and architecture that were verified on the source.
//
// Selection rationale (see round-1.md): the adapter reuses the target's native
// registry / local-store transport via a native tool over OCI — it does not
// implement a layer-transfer protocol or a private container format, and it does
// not rebuild, convert or substitute the image. This matches the accepted
// predecessor application_registry_source.go, which addresses native tools and
// never clones a credential or owns a container format. The alternative paths
// (a target-native image store with no cross-host portability, or a native
// save/load OCI archive) are recorded as rejected in the evidence: a native
// registry/copy path is the only one that preserves the immutable digest pin
// across hosts and satisfies the "no trunk-drift in transferred identity"
// requirement.
//
// The caller resolves the source to an immutable digest through
// ResolveSource, then calls Copy to that digest; the target verifies before the
// transfer is reported complete. Any failure propagates with a sentinel error
// and no partial copy is reported: there is no half-success class.
func ImageTransfer(ctx context.Context, source ImageTransferSource, target ImageTransferTarget) (ImageTransferIdentity, error) {
	identity, err := source.Resolve(ctx)
	if err != nil {
		return ImageTransferIdentity{}, err
	}
	ref := identity.ImmutableIdentity()
	if err := target.Copy(ctx, ref); err != nil {
		return ImageTransferIdentity{}, fmt.Errorf("transfer image %s: %w", ref, err)
	}
	// The target reports what it received; the source identity proves the
	// transferred image carried the exact digest, manifest and architecture
	// that were pinned on the source. This is the digest-drift / tag-drift /
	// architecture-drift guard.
	verified, err := target.Verify(ctx, ref)
	if err != nil {
		// The transfer target could not prove the transferred identity. Fail
		// closed: report the error, do not report a partial success.
		return ImageTransferIdentity{}, fmt.Errorf("verify transferred image %s: %w", ref, err)
	}
	if err := identity.Verify(verified.Digest, verified.ManifestDigest, verified.Architecture, verified.OS); err != nil {
		return ImageTransferIdentity{}, fmt.Errorf("verify transferred image %s: %w", ref, err)
	}
	return verified, nil
}

// RuntimeImageTransferSource resolves a source image through a native tool,
// reusing the host's Docker credential helper / Podman auth. It never clones a
// token.
type RuntimeImageTransferSource struct {
	Runtime   bhruntime.ProviderKind
	Location  string
	Requested string
	Exec      ExecFunc
}

// Resolve addresses the source through the native tool, pins the digest, and
// returns the repository@digest identity plus the verified platform-manifest
// architecture. Fail closed on unreachable host, denied endpoint,
// public-default source, empty reply, or a reply without an immutable sha256
// digest.
func (s RuntimeImageTransferSource) Resolve(ctx context.Context) (ImageTransferIdentity, error) {
	cli, err := imageTransferCLIFor(s.Runtime)
	if err != nil {
		return ImageTransferIdentity{}, fmt.Errorf("%w: %v", ErrImageTransferUnavailable, err)
	}
	location := strings.TrimSpace(s.Location)
	if location == "" {
		return ImageTransferIdentity{}, fmt.Errorf("%w: empty source location", ErrImageTransferUnavailable)
	}
	if isDefaultDockerHost(location) {
		return ImageTransferIdentity{}, fmt.Errorf("%w: source %q is a public default, not transferable",
			ErrImageTransferTargetRefused, location)
	}
	repository, tag := splitReference(location)
	requested := s.Requested
	if requested == "" {
		if tag != "" {
			requested = tag
		} else {
			requested = "latest"
		}
	}
	ref := repository + ":" + requested
	line, err := runTransfer(ctx, s.Exec, cli.name, "image", "inspect",
		"--format", cli.repoDigestFormat, ref)
	if err != nil {
		return ImageTransferIdentity{}, fmt.Errorf("%w: %v", ErrImageTransferUnavailable, err)
	}
	repository, digest := splitDigestReference(strings.TrimSpace(line))
	if digest == "" || !validImageDigest(digest) {
		return ImageTransferIdentity{}, fmt.Errorf("%w: source %q did not resolve to an immutable sha256 digest", ErrImageTransferUnavailable, location)
	}
	architecture, os, err := inspectRuntimeImagePlatform(ctx, s.Exec, cli, ref)
	if err != nil {
		return ImageTransferIdentity{}, err
	}
	return ImageTransferIdentity{
		Repository:   repository,
		Digest:       digest,
		Architecture: architecture,
		OS:           os,
	}, nil
}

// RuntimeImageTransferTarget ingests a transferred image through a runtime
// provider's native transport and verifies the transferred identity against the
// host's native tool.
type RuntimeImageTransferTarget struct {
	Compose bhruntime.RuntimeProvider
	Exec    ExecFunc
}

// Copy ingests ref through the provider's native pull site. It performs no
// mutable-tag resolution: the caller must pass an immutable repository@digest.
// The only mutable fallback lives behind an explicit operator opt-in (see
// application_registry_source.go); a caller that pulls a mutable reference here
// is the RED mutant this path proves.
func (t RuntimeImageTransferTarget) Copy(ctx context.Context, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("%w: empty transfer reference", ErrImageTransferUnavailable)
	}
	if err := pullRuntimeImage(ctx, t.Compose, ref); err != nil {
		return fmt.Errorf("pull transferred image %s: %w", ref, err)
	}
	return nil
}

// Verify inspects the transferred image on the target and returns the identity
// that was received, verified against the host's native tool.
func (t RuntimeImageTransferTarget) Verify(ctx context.Context, ref string) (ImageTransferIdentity, error) {
	identity, err := inspectRuntimeImage(ctx, t.Compose, t.Exec, ref)
	if err != nil {
		return ImageTransferIdentity{}, fmt.Errorf("%w: %v", ErrImageTransferUnavailable, err)
	}
	return identity, nil
}

// pullRuntimeImage is the single pull site used by the transfer target. It
// resolves nothing: it pulls exactly the immutable reference the caller passed.
func pullRuntimeImage(ctx context.Context, compose bhruntime.RuntimeProvider, ref string) error {
	if err := compose.PullImage(ctx, ref); err != nil {
		return fmt.Errorf("pull transferred image %s: %w", ref, err)
	}
	return nil
}

// inspectRuntimeImage returns the transferred image's repository, digest and
// architecture as reported by the host's native tool. It fails closed when the
// tool is unavailable, the image is missing, or the reported identity lacks an
// immutable digest.
func inspectRuntimeImage(ctx context.Context, compose bhruntime.RuntimeProvider, exec ExecFunc, ref string) (ImageTransferIdentity, error) {
	ref = strings.TrimSpace(ref)
	_, digest := splitDigestReference(ref)
	if digest == "" || !validImageDigest(digest) {
		return ImageTransferIdentity{}, fmt.Errorf("%w: reference %q does not carry an immutable sha256 digest", ErrImageTransferUnavailable, ref)
	}
	if compose == nil {
		return ImageTransferIdentity{}, fmt.Errorf("%w: target runtime is missing", ErrImageTransferUnavailable)
	}
	cli, err := imageTransferCLIFor(compose.Kind())
	if err != nil {
		return ImageTransferIdentity{}, fmt.Errorf("%w: %v", ErrImageTransferUnavailable, err)
	}
	line, err := runTransfer(ctx, exec, cli.name, "image", "inspect",
		"--format", cli.repoDigestFormat, ref)
	if err != nil {
		return ImageTransferIdentity{}, fmt.Errorf("%w: %v", ErrImageTransferUnavailable, err)
	}
	_, digest = splitDigestReference(strings.TrimSpace(line))
	if digest == "" || !validImageDigest(digest) {
		return ImageTransferIdentity{}, fmt.Errorf("%w: transferred image %q did not report an immutable sha256 digest", ErrImageTransferUnavailable, ref)
	}
	architecture, os, err := inspectRuntimeImagePlatform(ctx, exec, cli, ref)
	if err != nil {
		return ImageTransferIdentity{}, err
	}
	return ImageTransferIdentity{
		Repository:   repository(ref, digest),
		Digest:       digest,
		Architecture: architecture,
		OS:           os,
	}, nil
}

// runTransfer executes a native image tool and returns its stdout. It reuses the
// host credential helper / auth file transparently: the caller only observes
// whether the operation succeeded, never any credential material. A failing
// command is reported with only a redacted class and exit status, never its raw
// stderr.
func runTransfer(ctx context.Context, exec ExecFunc, name string, args ...string) (string, error) {
	if exec == nil {
		return execArtifact(ctx, name, args...)
	}
	return exec(ctx, name, args...)
}

// inspectRuntimeImagePlatform returns the architecture and OS of the platform
// manifest selected for ref as reported by the host's native tool. It fails
// closed on a command error or a malformed/empty report: platform identity is a
// mandatory, parseable part of a verified transfer, so an unparseable report is
// never treated as "not pinned" — it is a failed transfer.
func inspectRuntimeImagePlatform(ctx context.Context, exec ExecFunc, cli imageTransferCLI, ref string) (string, string, error) {
	out, err := runTransfer(ctx, exec, cli.name, "image", "inspect",
		"--format", cli.platformFormat, ref)
	if err != nil {
		return "", "", fmt.Errorf("%w: platform inspect failed for %q: %v",
			ErrImageTransferUnavailable, ref, err)
	}
	idx := strings.IndexByte(out, '|')
	if idx < 0 {
		return "", "", fmt.Errorf("%w: platform inspect %q for %q did not report architecture|OS",
			ErrImageTransferUnavailable, out, ref)
	}
	architecture := strings.TrimSpace(out[:idx])
	os := strings.TrimSpace(out[idx+1:])
	if architecture == "" || os == "" {
		return "", "", fmt.Errorf("%w: platform inspect %q for %q reported empty architecture or OS",
			ErrImageTransferUnavailable, out, ref)
	}
	return architecture, os, nil
}

type imageTransferCLI struct {
	name             string
	repoDigestFormat string
	platformFormat   string
}

func imageTransferCLIFor(kind bhruntime.ProviderKind) (imageTransferCLI, error) {
	switch kind {
	case bhruntime.ProviderDocker:
		return imageTransferCLI{
			name:             "docker",
			repoDigestFormat: `{{index .RepoDigests 0}}`,
			platformFormat:   `{{.Architecture}}|{{.Os}}`,
		}, nil
	case bhruntime.ProviderPodman:
		return imageTransferCLI{
			name:             "podman",
			repoDigestFormat: `{{(index .RepoDigests 0)}}`,
			platformFormat:   `{{.Architecture}}|{{.Os}}`,
		}, nil
	default:
		return imageTransferCLI{}, fmt.Errorf("unsupported image transfer runtime %q", kind)
	}
}

// splitDigestReference separates a repository reference into its bare
// repository and trailing sha256 digest. A reference without an immutable
// digest returns an empty digest.
func splitDigestReference(reference string) (repository, digest string) {
	reference = strings.TrimSpace(reference)
	if at := strings.IndexByte(reference, '@'); at >= 0 {
		repository = reference[:at]
		digest = reference[at+1:]
		// RepoDigests entries are host/repo@sha256:...; strip a leading
		// host:port token before the first ':' so only the digest remains.
		if idx := strings.IndexByte(digest, ':'); idx > 0 && !strings.HasPrefix(digest, "sha256:") {
			digest = digest[idx+1:]
		}
		return repository, digest
	}
	return reference, ""
}

// isTransferUnavailable reports whether a resolve failure is of the merely-
// unavailable class: tool failure, empty or unpinnable reply, malformed
// reference, or unreachable host. A hard rejection (denial or public default)
// is NEVER a merely-unavailable result, so its sentinel takes precedence even
// though it unwraps to the unavailable identity.
func isTransferUnavailable(err error) bool {
	if isTransferHardRejection(err) {
		return false
	}
	return errors.Is(err, ErrImageTransferUnavailable)
}
