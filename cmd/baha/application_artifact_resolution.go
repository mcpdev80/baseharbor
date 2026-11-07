package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Artifact resolution turns a source/version selection into one concrete,
// immutable artifact before the workload lifecycle touches Docker/Compose or
// Podman. It reuses the accepted predecessor authorities: the registry endpoint
// resolver (application_registry_source.go) for remote digests, the native image
// tooling (application_image_transfer.go) for local-store availability, and the
// rendered Compose model (app_workload_build.go) for build definitions. It
// introduces no new registry, build service or container format.
//
// Fail-closed rules the adapter proves:
//   - A selected artifact source that is ambiguous — several digests for one
//     reference, contradictory endpoint replies, or a partially configured
//     selection — resolves to nothing and reports the ambiguity.
//   - An existing image/release is used first when it is verifiably available;
//     only current-checkout mode may fall through to the native build machinery.
//   - A release/promote path never builds, replaces an artifact, or accepts a
//     mutable tag as an exact identity.
//   - Only owner/provenance metadata (repository, digest, requested channel,
//     how it was resolved) is retained; endpoints and credentials never are.

// ErrArtifactResolutionAmbiguous is the fail-closed sentinel for a selection
// that admits more than one concrete artifact: several repo digests behind one
// reference, contradictory resolver replies, or a source pinned twice with
// different values. The caller must disambiguate; no arbitrary winner is picked.
var ErrArtifactResolutionAmbiguous = errors.New("artifact resolution: selection is ambiguous")

// ErrArtifactResolutionUnavailable is the fail-closed sentinel for a selection
// whose artifact could not be addressed at all: no usable digest, a missing
// local image that may not be built, or a native tool failure.
var ErrArtifactResolutionUnavailable = errors.New("artifact resolution: no usable resolved artifact")

// ArtifactResolutionMode states what the lifecycle may do to realize the
// selection. The mode is derived from explicit configuration and the card
// rules — never from a guess.
type ArtifactResolutionMode string

const (
	// ArtifactResolutionExisting uses an existing OCI image/release: it resolves
	// the selection to an immutable digest and verifies the artifact is
	// available through the native image tooling. It never builds.
	ArtifactResolutionExisting ArtifactResolutionMode = "existing"

	// ArtifactResolutionCheckoutBuild is the current-checkout mode: when the
	// rendered workload genuinely carries a build definition, the existing
	// native Docker/Compose or Podman build machinery stays in charge. The
	// adapter only proves the build context is real; it owns no builder.
	ArtifactResolutionCheckoutBuild ArtifactResolutionMode = "checkout-build"
)

// ArtifactResolutionAction is the per-service decision the apply path acts on.
type ArtifactResolutionAction string

const (
	// ArtifactActionUseExisting keeps the service on its available image.
	ArtifactActionUseExisting ArtifactResolutionAction = "use-existing"
	// ArtifactActionNativeBuild routes the service to the native build
	// machinery (current-checkout mode only).
	ArtifactActionNativeBuild ArtifactResolutionAction = "native-build"
)

// ArtifactResolution is the resolved artifact for one workload service. It
// retains only the owner/provenance metadata: what was requested, what concrete
// immutable artifact it resolved to, and which action realizes it. Registry
// endpoints, credentials and tool output are deliberately absent.
type ArtifactResolution struct {
	Service   string
	Action    ArtifactResolutionAction
	Identity  ArtifactIdentity
	Requested string
}

// renderedWorkloadArtifactService is the minimal rendered-Compose view the
// resolver reads: the image selection and the presence/shape of a build
// definition per service.
type renderedWorkloadArtifactService struct {
	Image string          `json:"image"`
	Build json.RawMessage `json:"build"`
}

type renderedWorkloadArtifactConfig struct {
	Services map[string]renderedWorkloadArtifactService `json:"services"`
}

// resolveArtifactServiceDecision is the pure selection core: given what the
// rendered model declares, what concrete artifact (if any) was resolved for it,
// and the mode, it returns the action for the service. It never invents an
// artifact: a build-less service without a resolved digest is unavailable, and
// an existing-mode service whose build definition is the only source is a
// current-checkout violation of the mode.
func resolveArtifactServiceDecision(mode ArtifactResolutionMode, declaredBuild bool, resolved ArtifactIdentity) (ArtifactResolutionAction, error) {
	switch mode {
	case ArtifactResolutionExisting:
		if !validImageDigest(resolved.Digest) {
			return "", fmt.Errorf("%w: existing-artifact mode requires an immutable digest identity, got %q", ErrArtifactResolutionUnavailable, resolved.Requested)
		}
		if declaredBuild && resolved.Repository == "" {
			// An existing-artifact selection that still leans on the checkout
			// build has no concrete artifact; fail closed instead of silently
			// building.
			return "", fmt.Errorf("%w: existing-artifact mode may not fall through to a checkout build", ErrArtifactResolutionUnavailable)
		}
		return ArtifactActionUseExisting, nil
	case ArtifactResolutionCheckoutBuild:
		if declaredBuild {
			return ArtifactActionNativeBuild, nil
		}
		if validImageDigest(resolved.Digest) || strings.TrimSpace(resolved.Repository) != "" {
			return ArtifactActionUseExisting, nil
		}
		return "", fmt.Errorf("%w: service declares neither a build nor a resolvable image artifact", ErrArtifactResolutionUnavailable)
	default:
		return "", fmt.Errorf("%w: unsupported artifact resolution mode %q", ErrArtifactResolutionAmbiguous, mode)
	}
}

// artifactSourceLocation returns the bare repository for a rendered image
// reference: any digest pin is stripped, a tag is kept only as the requested
// channel. An image with no tag resolves on the "latest" channel, matching the
// registry-source authority.
func artifactSourceLocation(image string) (location, requested string, digest string) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", "", ""
	}
	if at := strings.IndexByte(image, '@'); at >= 0 {
		repository := image[:at]
		tail := image[at+1:]
		if colon := strings.IndexByte(tail, ':'); colon >= 0 && !strings.HasPrefix(tail, "sha256:") {
			tail = tail[colon+1:]
		}
		repository, tag := splitReference(repository)
		if tag != "" {
			requested = tag
		}
		return repository, requested, tail
	}
	repository, tag := splitReference(image)
	if tag != "" {
		return repository, tag, ""
	}
	return image, "", ""
}

// resolveLocalRegistryArtifact asks the host's native image tool which concrete
// digests the given reference carries locally. A missing image is unavailable;
// several conflicting digests are ambiguous and never collapsed to a winner.
func resolveLocalRegistryArtifact(ctx context.Context, exec ExecFunc, cli imageTransferCLI, ref string) (ArtifactIdentity, error) {
	line, err := runTransfer(ctx, exec, cli.name, "image", "inspect", "--format", cli.repoDigestFormat, ref)
	if err != nil {
		return ArtifactIdentity{}, fmt.Errorf("%w: local inspect for %q: %v", ErrArtifactResolutionUnavailable, ref, err)
	}
	repository, digest := splitDigestReference(strings.TrimSpace(line))
	if digest == "" || !validImageDigest(digest) {
		return ArtifactIdentity{}, fmt.Errorf("%w: local image %q carries no immutable sha256 digest", ErrArtifactResolutionUnavailable, ref)
	}
	return ArtifactIdentity{Repository: repository, Digest: digest, Requested: ref}, nil
}

// localArtifactAvailable probes the host's local image store through the native
// tool. The probe runs two calls: `image exists` for presence and
// `image inspect` over the repo digests for the immutable identity lines. An
// image that the store does not know is absent without an error; a present
// image with one digest is available; several conflicting digests are
// ambiguous, and a present-but-uninspectable reply fails closed with the tool
// error. The strict resolver surfaces the ambiguity when the concrete artifact
// is actually needed.
func localArtifactAvailable(ctx context.Context, exec ExecFunc, cli imageTransferCLI, ref string) (bool, error) {
	if _, err := runTransfer(ctx, exec, cli.name, "image", "exists", ref); err != nil {
		if isTransferMissing(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: local availability probe for %q: %v", ErrArtifactResolutionUnavailable, ref, err)
	}
	out, err := runTransfer(ctx, exec, cli.name, "image", "inspect", "--format", "{{range .RepoDigests}}{{.}}\n{{end}}", ref)
	if err != nil {
		// A tool that reports the image as existing but cannot inspect it at
		// all is treated as absent rather than guessed available.
		return false, nil
	}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(out, "\n") {
		_, digest := splitDigestReference(strings.TrimSpace(line))
		if digest == "" || !validImageDigest(digest) {
			continue
		}
		if _, dup := seen[digest]; dup {
			continue
		}
		seen[digest] = struct{}{}
	}
	switch len(seen) {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		// Several conflicting digests behind one reference: the store is
		// ambiguous, and no arbitrary winner may realize the selection.
		return false, fmt.Errorf("%w: local reference %q carries %d conflicting digests", ErrArtifactResolutionAmbiguous, ref, len(seen))
	}
}

// isTransferMissing reports whether a native `image exists` failure is the
// store's plain "I do not know this image" answer rather than a tool failure.
// Only the plain missing answer is a silent absence; a broken tool still fails
// closed.
func isTransferMissing(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such image") || strings.Contains(msg, "not known")
}

// resolveRenderedWorkloadArtifacts is the bounded artifact-realization adapter:
// for each selected service of a rendered Compose model it turns the source or
// version selection into one concrete artifact decision, fail-closed on
// ambiguity. mode existing uses the registry resolver or the local store and
// never builds; mode checkout-build keeps the native build machinery in charge
// for services that actually declare a build. It reuses only the predecessor
// authorities and owns no builder, registry client, or credential.
func resolveRenderedWorkloadArtifacts(ctx context.Context, exec ExecFunc, cli imageTransferCLI, rendered string, services []string, mode ArtifactResolutionMode) (map[string]ArtifactResolution, error) {
	var config renderedWorkloadArtifactConfig
	if err := json.Unmarshal([]byte(rendered), &config); err != nil {
		return nil, fmt.Errorf("decode workload artifact configuration: %w", err)
	}
	decisions := make(map[string]ArtifactResolution, len(services))
	for _, service := range services {
		declared, ok := config.Services[service]
		if !ok {
			return nil, fmt.Errorf("%w: service %q missing from the rendered workload model", ErrArtifactResolutionAmbiguous, service)
		}
		_, hasBuild, err := parseRepositoryBuildDefinition(declared.Build)
		if err != nil {
			// A malformed build stanza is a contradictory selection: the model
			// claims a build without being able to describe one.
			return nil, fmt.Errorf("%w: service %q: %v", ErrArtifactResolutionAmbiguous, service, err)
		}
		location, requested, pinnedDigest := artifactSourceLocation(declared.Image)
		resolved := ArtifactIdentity{Requested: requested}
		switch {
		case pinnedDigest != "":
			// The selection already names an immutable artifact; nothing to
			// resolve, only to keep as the concrete identity.
			if !validImageDigest(pinnedDigest) {
				return nil, fmt.Errorf("%w: service %q pins a non-sha256 digest %q", ErrArtifactResolutionAmbiguous, service, pinnedDigest)
			}
			resolved.Repository, resolved.Digest = location, pinnedDigest
			hasBuild = false
		case hasBuild && location == "":
			// A pure build service without an image selection resolves to the
			// build action; the mode decides below whether that is legal.
			if mode != ArtifactResolutionCheckoutBuild {
				return nil, fmt.Errorf("%w: service %q: existing-artifact mode may not fall through to a checkout build", ErrArtifactResolutionUnavailable, service)
			}
		case location != "":
			// The public-default guard belongs to the registry authority and it
			// applies to the resolved host: a bare docker.io-style name that
			// names no explicit host is a local/checkout image, not a
			// docker.io endpoint.
			if strings.Contains(location, "/") && isDefaultDockerHost(location) {
				return nil, fmt.Errorf("%w: service %q: %w: %w: configured endpoint %q is a public default, not an enterprise registry",
					ErrArtifactResolutionUnavailable, service, ErrRegistrySourceUnavailable, ErrRegistrySourcePublicDefault, location)
			}
			endpoint, err := resolveRegistrySource(ctx, location, requested)
			if err == nil {
				repository, digest := splitDigestReference(endpoint)
				if !validImageDigest(digest) {
					return nil, fmt.Errorf("%w: service %q: resolver returned %q without an immutable digest", ErrArtifactResolutionAmbiguous, service, endpoint)
				}
				resolved.Repository, resolved.Digest = repository, digest
				// A successfully resolved registry artifact is the existing
				// artifact the card puts first: a declared build must never
				// override it into native-build (act-1406a blocker).
				hasBuild = false
				break
			}
			if mode != ArtifactResolutionCheckoutBuild && !resolveUnavailable(err) {
				// A hard rejection (denial or public default) is never
				// papered over by the local store.
				return nil, fmt.Errorf("resolve artifact for service %q: %w", service, err)
			}
			// Endpoint unavailable: consult the local store, which is the
			// existing-image path the card demands before any build.
			available, probeErr := localArtifactAvailable(ctx, exec, cli, location+":"+requestedAt(requested))
			if probeErr != nil {
				// An ambiguous local store fails closed in every mode: no
				// arbitrary digest may realize the selection and a conflicting
				// store must never be papered over by a build.
				return nil, fmt.Errorf("probe local artifact for service %q: %w", service, probeErr)
			}
			if available {
				local, localErr := resolveLocalRegistryArtifact(ctx, exec, cli, location+":"+requestedAt(requested))
				if localErr != nil {
					return nil, fmt.Errorf("resolve local artifact for service %q: %w", service, localErr)
				}
				resolved = local
				// The available existing artifact wins over the checkout
				// build: the native build machinery must stay untouched.
				hasBuild = false
				break
			}
			if !hasBuild {
				if mode == ArtifactResolutionExisting {
					return nil, fmt.Errorf("%w: service %q: neither the endpoint nor the local store carries %s", ErrArtifactResolutionUnavailable, service, location)
				}
				return nil, fmt.Errorf("%w: service %q: %v", ErrArtifactResolutionUnavailable, service, err)
			}
		default:
			return nil, fmt.Errorf("%w: service %q declares neither an image nor a build", ErrArtifactResolutionAmbiguous, service)
		}
		action, err := resolveArtifactServiceDecision(mode, hasBuild, resolved)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", service, err)
		}
		decisions[service] = ArtifactResolution{Service: service, Action: action, Identity: resolved, Requested: requested}
	}
	return decisions, nil
}

// requestedAt maps a possibly empty channel to an explicit tag for native
// probes; it mirrors the registry-source default.
func requestedAt(requested string) string {
	if strings.TrimSpace(requested) == "" {
		return "latest"
	}
	return requested
}

// ResolveWorkloadArtifact resolves one configured endpoint/version selection to
// its concrete immutable artifact through the registry authority. It is the
// callable seam for callers that already know the exact source (e.g. the
// runtime image path): the result always carries an sha256 digest, never a
// mutable tag.
func ResolveWorkloadArtifact(ctx context.Context, location, requested string) (ArtifactIdentity, error) {
	endpoint, err := resolveRegistrySource(ctx, location, requested)
	if err != nil {
		return ArtifactIdentity{}, err
	}
	repository, digest := splitDigestReference(endpoint)
	if !validImageDigest(digest) {
		return ArtifactIdentity{}, fmt.Errorf("%w: resolver returned %q without an immutable digest", ErrArtifactResolutionAmbiguous, endpoint)
	}
	return ArtifactIdentity{Repository: repository, Digest: digest, Requested: requested}, nil
}

// ResolveAndPullWorkloadArtifact resolves the selection and pulls exactly the
// immutable identity through the target's native transport. It never pulls a
// mutable reference and never builds; a resolution failure means zero pulls.
func ResolveAndPullWorkloadArtifact(ctx context.Context, compose bhruntime.RuntimeProvider, location, requested string) (ArtifactIdentity, error) {
	identity, err := ResolveWorkloadArtifact(ctx, location, requested)
	if err != nil {
		// resolveUnavailable excludes the unverifiable identity
		// (errClassUnverifiable) as a hard rejection, so an OCI source image
		// that resolved but could not be verified to an immutable digest
		// never reaches the mutable fallback, not even with the opt-in on.
		if resolveUnavailable(err) && allowMutableDevelopmentRuntimeImage() {
			// Explicit operator development opt-in shared with the registry
			// authority: class-gated, so a denial or public-default rejection
			// never lands here.
			pinned := ArtifactIdentity{Repository: location, Requested: requested}
			if pullErr := pullRuntimeImage(ctx, compose, location); pullErr != nil {
				return ArtifactIdentity{}, pullErr
			}
			return pinned, nil
		}
		return ArtifactIdentity{}, fmt.Errorf("resolve workload artifact: %w", err)
	}
	if err := pullRuntimeImage(ctx, compose, identity.ImmutableIdentity()); err != nil {
		return ArtifactIdentity{}, err
	}
	return identity, nil
}

// artifactResolutionStateName is the deployment-local record of the concrete
// artifacts the last verified apply resolved to. Like the build/config
// fingerprints it carries only digests and provenance, never endpoints or
// secrets, and it is owner-only.
const artifactResolutionStateName = "workload-artifact-resolutions.json"

type artifactResolutionStateEntry struct {
	Action    string `json:"action"`
	Identity  string `json:"identity"`
	Requested string `json:"requested,omitempty"`
}

type artifactResolutionState struct {
	Version  int                                     `json:"version"`
	Services map[string]artifactResolutionStateEntry `json:"services"`
}

func repositoryWorkloadArtifactStatePath(files application.RuntimeFiles) string {
	return filepath.Join(files.Dir, artifactResolutionStateName)
}

func persistRepositoryWorkloadArtifactState(files application.RuntimeFiles, decisions map[string]ArtifactResolution) error {
	state := artifactResolutionState{Version: 1, Services: map[string]artifactResolutionStateEntry{}}
	for service, decision := range decisions {
		entry := artifactResolutionStateEntry{Action: string(decision.Action), Requested: decision.Requested}
		if decision.Action == ArtifactActionUseExisting {
			entry.Identity = decision.Identity.ImmutableIdentity()
		}
		state.Services[service] = entry
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(repositoryWorkloadArtifactStatePath(files), append(data, '\n'), 0o600)
}

func loadRepositoryWorkloadArtifactState(files application.RuntimeFiles) (artifactResolutionState, error) {
	var state artifactResolutionState
	data, err := os.ReadFile(repositoryWorkloadArtifactStatePath(files))
	if err != nil {
		if os.IsNotExist(err) {
			return artifactResolutionState{Version: 1, Services: map[string]artifactResolutionStateEntry{}}, nil
		}
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode application workload artifact state: %w", err)
	}
	if state.Services == nil {
		state.Services = map[string]artifactResolutionStateEntry{}
	}
	return state, nil
}

// validateWorkloadArtifactBuildContext proves the checkout-build promise is
// real before anything is scheduled for a native build: the rendered build
// definition must address an existing context directory and Dockerfile inside
// the repository. It keeps the native build machinery least-privilege: the
// adapter never builds here, it only refuses to route to a build that cannot
// run.
func validateWorkloadArtifactBuildContext(workload application.WorkloadFiles, service string, raw json.RawMessage) error {
	definition, ok, err := parseRepositoryBuildDefinition(raw)
	if err != nil {
		return fmt.Errorf("service %q: parse build definition: %w", service, err)
	}
	if !ok {
		return fmt.Errorf("%w: service %q: build action without a rendered build definition", ErrArtifactResolutionAmbiguous, service)
	}
	root, err := filepath.Abs(workload.RepositoryRoot)
	if err != nil {
		return err
	}
	contextDir := definition.Context
	if !filepath.IsAbs(contextDir) {
		contextDir = filepath.Join(root, contextDir)
	}
	info, err := os.Stat(contextDir)
	if err != nil {
		return fmt.Errorf("service %q: build context: %w", service, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("service %q: build context %q is not a directory", service, definition.Context)
	}
	dockerfile := definition.Dockerfile
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(contextDir, dockerfile)
	}
	dockerfile = filepath.Clean(dockerfile)
	if !strings.HasPrefix(dockerfile, filepath.Clean(root)+string(os.PathSeparator)) && dockerfile != filepath.Clean(root) {
		return fmt.Errorf("%w: service %q: build context %q escapes the repository root", ErrArtifactResolutionAmbiguous, service, definition.Context)
	}
	if _, err := os.Stat(dockerfile); err != nil {
		return fmt.Errorf("service %q: Dockerfile/Containerfile: %w", service, err)
	}
	return nil
}

// validateWorkloadArtifactComposeFiles keeps the adapter inside the same
// effective file set the rest of the apply path uses: the rendered model it
// resolves must come from the project's current Compose files.
func validateWorkloadArtifactComposeFiles(workload application.WorkloadFiles, composeFiles []string) error {
	if len(composeFiles) == 0 {
		return fmt.Errorf("%w: artifact resolution requires the effective Compose file set", ErrArtifactResolutionAmbiguous)
	}
	for _, file := range composeFiles {
		if _, err := os.Stat(file); err != nil {
			return fmt.Errorf("artifact resolution Compose file %q: %w", file, err)
		}
	}
	if workload.Compose == "" && workload.RepositoryRoot == "" {
		return fmt.Errorf("%w: artifact resolution requires a repository-owned workload", ErrArtifactResolutionUnavailable)
	}
	if workload.Compose != "" && filepath.Base(workload.Compose) != filepath.Base(composeFiles[0]) {
		return fmt.Errorf("%w: effective Compose file set does not start at the workload Compose file %q", ErrArtifactResolutionAmbiguous, workload.Compose)
	}
	return nil
}

// prepareRepositoryWorkloadArtifactResolution renders the effective workload
// model and resolves every selected service's artifact. It returns nil, nil
// when no service carries a build definition and no endpoint configuration
// forces resolution: the pure-image path needs no artifact plan and stays
// byte-identical to the pre-adapter apply.
func prepareRepositoryWorkloadArtifactResolution(ctx context.Context, compose bhruntime.RuntimeProvider, workload application.WorkloadFiles, environment map[string]string, composeFiles []string, services []string) (map[string]ArtifactResolution, error) {
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, composeFiles...)
	if err != nil {
		return nil, fmt.Errorf("render workload for artifact resolution: %w", err)
	}
	var config renderedWorkloadArtifactConfig
	if err := json.Unmarshal([]byte(rendered), &config); err != nil {
		return nil, fmt.Errorf("decode workload artifact configuration: %w", err)
	}
	needsPlan := false
	for _, service := range services {
		declared := config.Services[service]
		if len(strings.TrimSpace(string(declared.Build))) > 0 && string(strings.TrimSpace(string(declared.Build))) != "null" {
			needsPlan = true
			break
		}
	}
	if !needsPlan {
		return nil, nil
	}
	if err := validateWorkloadArtifactComposeFiles(workload, composeFiles); err != nil {
		return nil, err
	}
	cli, err := imageTransferCLIFor(compose.Kind())
	if err != nil {
		return nil, err
	}
	decisions, err := resolveRenderedWorkloadArtifacts(ctx, artifactResolutionExec, cli, rendered, services, ArtifactResolutionCheckoutBuild)
	if err != nil {
		return nil, err
	}
	// Prove the native-build routing before the apply path schedules a build.
	for _, service := range sortedArtifactResolutionServices(decisions) {
		if decisions[service].Action != ArtifactActionNativeBuild {
			continue
		}
		if err := validateWorkloadArtifactBuildContext(workload, service, config.Services[service].Build); err != nil {
			return nil, err
		}
	}
	return decisions, nil
}

func sortedArtifactResolutionServices(decisions map[string]ArtifactResolution) []string {
	names := make([]string, 0, len(decisions))
	for service := range decisions {
		names = append(names, service)
	}
	sort.Strings(names)
	return names
}

// artifactResolutionExec is the native image tool runner used by the local
// probe; it defaults to the real os/exec runner shared with the registry
// authority, and tests install a fake to observe the invocations.
var artifactResolutionExec ExecFunc = execArtifact
