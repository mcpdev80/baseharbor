package targetsession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// ProjectTransport carries already authorized Core realization decisions. It
// neither selects an Application placement nor authorizes an operator.
type ProjectTransport interface {
	LiveCapabilities(targetenrollment.Scope) (Capabilities, error)
	Dispatch(context.Context, targetenrollment.Scope, Request) (Response, error)
}

// ProjectRuntime binds immutable project artifacts to one authenticated node.
// It never resolves a remote selection locally or replays an interrupted call.
type ProjectRuntime struct {
	transport ProjectTransport
	scope     targetenrollment.Scope
}

type ProjectFile struct {
	Path string
	Data []byte
	Mode uint32
}

// StagedProject can only be constructed after validating the remote staging
// receipt. Its scope, paths and bytes cannot be substituted by a caller.
type StagedProject struct {
	scope     targetenrollment.Scope
	bundleID  string
	directory string
	files     map[string]stagedProjectFile
}

type stagedProjectFile struct {
	remotePath string
	data       []byte
	mode       uint32
}

var projectBundleID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var projectObjectDirectory = regexp.MustCompile(`^bundles/\.object-[0-9a-f]{32}$`)

func NewProjectRuntime(transport ProjectTransport, scope targetenrollment.Scope) (*ProjectRuntime, error) {
	if transport == nil || scope.Validate() != nil || (scope.Runtime != "docker" && scope.Runtime != "podman") {
		return nil, ErrUnavailable
	}
	runtime := &ProjectRuntime{transport: transport, scope: scope}
	if err := runtime.requireCapability("artifact.bundle.stage"); err != nil {
		return nil, err
	}
	return runtime, nil
}

func (r *ProjectRuntime) requireCapability(operation string) error {
	if r == nil || r.transport == nil {
		return ErrUnavailable
	}
	capabilities, err := r.transport.LiveCapabilities(r.scope)
	if err != nil || capabilities.ContractVersion != contractVersion || capabilities.ProtocolVersion != protocolVersion ||
		capabilities.Node.Scope() != r.scope || capabilities.Node.Identity != r.scope.Identity() {
		return ErrUnavailable
	}
	for _, capability := range capabilities.Capabilities {
		if capability.Name == operation && capability.Available {
			return nil
		}
	}
	return ErrUnavailable
}

func (r *ProjectRuntime) invoke(ctx context.Context, operation string, payload, destination any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.requireCapability(operation); err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.New("remote project request could not be encoded")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	id := hex.EncodeToString(nonce[:])
	correlation := machine.ExecutionCorrelation(ctx)
	if correlation == "" {
		correlation = id
	}
	now := time.Now().UTC()
	deadline := now.Add(2 * time.Minute)
	if bound, ok := ctx.Deadline(); ok && bound.Before(deadline) {
		deadline = bound.UTC()
	}
	request := Request{ContractVersion: contractVersion, ProtocolVersion: protocolVersion, RequestID: id,
		CorrelationID: correlation, TargetID: r.scope.TargetID, Operation: operation,
		IssuedAt: now, DeadlineAt: deadline, Payload: data}
	response, err := r.transport.Dispatch(ctx, r.scope, request)
	if err != nil {
		return ErrUnavailable
	}
	if response.ContractVersion != contractVersion || response.ProtocolVersion != protocolVersion ||
		response.RequestID != id || response.CorrelationID != correlation {
		return errors.New("remote project response identity differs")
	}
	if !response.Success {
		return errors.New("admitted remote project operation failed; reconcile before retrying")
	}
	if destination != nil && json.Unmarshal(response.Result, destination) != nil {
		return errors.New("remote project response is invalid")
	}
	return nil
}

func (r *ProjectRuntime) Stage(ctx context.Context, id string, files []ProjectFile) (*StagedProject, error) {
	if !projectBundleID.MatchString(id) || len(files) == 0 || len(files) > 128 {
		return nil, errors.New("invalid remote project bundle")
	}
	type wireFile struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Data   []byte `json:"data"`
		Mode   uint32 `json:"mode"`
	}
	payload := struct {
		BundleID string     `json:"bundle_id"`
		Files    []wireFile `json:"files"`
	}{BundleID: id}
	expected := make(map[string]wireFile, len(files))
	total := 0
	for _, file := range files {
		if file.Path == "" || file.Path == "." || file.Path == ".." || file.Path == ".manifest.json" || path.Clean(file.Path) != file.Path ||
			strings.HasPrefix(file.Path, "/") || strings.HasPrefix(file.Path, "../") || strings.ContainsAny(file.Path, "\\\x00\r\n") {
			return nil, errors.New("invalid remote project file path")
		}
		if _, duplicate := expected[file.Path]; duplicate {
			return nil, errors.New("duplicate remote project file")
		}
		total += len(file.Data)
		if total > 4<<20 {
			return nil, errors.New("remote project bundle exceeds byte limit")
		}
		data := append([]byte{}, file.Data...)
		digest := sha256.Sum256(data)
		mode, err := projectFileMode(file.Mode)
		if err != nil {
			return nil, err
		}
		wire := wireFile{Path: file.Path, SHA256: hex.EncodeToString(digest[:]), Data: data, Mode: mode}
		expected[file.Path] = wire
		payload.Files = append(payload.Files, wire)
	}
	var receipt struct {
		BundleID string `json:"bundle_id"`
		Files    []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := r.invoke(ctx, "artifact.bundle.stage", payload, &receipt); err != nil {
		return nil, err
	}
	if receipt.BundleID != id || len(receipt.Files) != len(expected) {
		return nil, errors.New("remote project staging receipt differs")
	}
	staged := &StagedProject{scope: r.scope, bundleID: id, files: make(map[string]stagedProjectFile, len(expected))}
	// Match every exact source-relative suffix under the one confined immutable
	// object directory. A digest alone cannot authorize a foreign path.
	for _, entry := range receipt.Files {
		matched := false
		for relative, file := range expected {
			suffix := "/" + relative
			if !strings.HasSuffix(entry.Path, suffix) {
				continue
			}
			directory := strings.TrimSuffix(entry.Path, suffix)
			if !projectObjectDirectory.MatchString(directory) || (staged.directory != "" && directory != staged.directory) ||
				entry.SHA256 != file.SHA256 {
				continue
			}
			if _, duplicate := staged.files[relative]; duplicate {
				return nil, errors.New("duplicate remote project receipt file")
			}
			staged.directory = directory
			staged.files[relative] = stagedProjectFile{remotePath: entry.Path, data: file.Data, mode: file.Mode}
			matched = true
			break
		}
		if !matched {
			return nil, errors.New("remote project staging path or digest differs")
		}
	}
	return staged, nil
}

func projectFileMode(mode uint32) (uint32, error) {
	if mode == 0 {
		mode = 0600
	}
	if mode != 0600 && mode != 0644 && mode != 0700 {
		return 0, errors.New("invalid Core project file permissions")
	}
	return mode, nil
}

func (r *ProjectRuntime) composeSelection(project *StagedProject, files []string, envFile string) ([]string, string, error) {
	if r == nil || r.scope.Runtime != "docker" || project == nil || project.scope != r.scope || len(files) == 0 {
		return nil, "", ErrUnavailable
	}
	selected := make([]string, 0, len(files))
	seen := map[string]bool{}
	for _, file := range files {
		entry, ok := project.files[file]
		if !ok || seen[file] || (path.Ext(file) != ".yaml" && path.Ext(file) != ".yml") {
			return nil, "", errors.New("invalid staged Compose selection")
		}
		seen[file] = true
		selected = append(selected, entry.remotePath)
	}
	remoteEnv := ""
	if envFile != "" {
		entry, ok := project.files[envFile]
		if !ok {
			return nil, "", errors.New("unstaged Compose environment file")
		}
		remoteEnv = entry.remotePath
	}
	return selected, remoteEnv, nil
}

func (r *ProjectRuntime) ApplyCompose(ctx context.Context, project *StagedProject, files []string, envFile string, repair bool) error {
	selected, env, err := r.composeSelection(project, files, envFile)
	if err != nil {
		return err
	}
	payload := struct {
		ProjectDirectory string   `json:"project_directory"`
		Files            []string `json:"files"`
		EnvFile          string   `json:"env_file,omitempty"`
		ForceRecreate    bool     `json:"force_recreate,omitempty"`
		TimeoutSeconds   int      `json:"timeout_seconds"`
	}{project.directory, selected, env, repair, 90}
	var result struct {
		ExitCode *int `json:"exit_code"`
	}
	if err := r.invoke(ctx, "runtime.compose.apply", payload, &result); err != nil {
		return err
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		return errors.New("remote Compose apply failed")
	}
	return nil
}

func (r *ProjectRuntime) DestroyCompose(ctx context.Context, project *StagedProject, files []string, envFile string) error {
	return r.DestroyComposeOwned(ctx, project, files, envFile, false)
}

// Persistent volumes are removed only for an explicit Core-owned reset.
func (r *ProjectRuntime) DestroyComposeOwned(ctx context.Context, project *StagedProject, files []string, envFile string, volumes bool) error {
	selected, env, err := r.composeSelection(project, files, envFile)
	if err != nil {
		return err
	}
	payload := struct {
		ProjectDirectory string   `json:"project_directory"`
		Files            []string `json:"files"`
		EnvFile          string   `json:"env_file,omitempty"`
		Volumes          bool     `json:"volumes,omitempty"`
		TimeoutSeconds   int      `json:"timeout_seconds"`
	}{project.directory, selected, env, volumes, 90}
	var result struct {
		ExitCode *int `json:"exit_code"`
	}
	if err := r.invoke(ctx, "runtime.compose.destroy", payload, &result); err != nil {
		return err
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		return errors.New("remote Compose destroy failed")
	}
	return nil
}

func (r *ProjectRuntime) ApplyQuadlet(ctx context.Context, project *StagedProject, file string) error {
	if r == nil || r.scope.Runtime != "podman" || project == nil || project.scope != r.scope || path.Base(file) != file || path.Ext(file) != ".container" {
		return ErrUnavailable
	}
	entry, ok := project.files[file]
	if !ok {
		return errors.New("unstaged Quadlet unit")
	}
	return r.invoke(ctx, "runtime.quadlet.apply", struct {
		Name             string `json:"name"`
		Content          string `json:"content"`
		Enable           bool   `json:"enable"`
		ProjectDirectory string `json:"project_directory"`
	}{file, string(entry.data), true, project.directory}, nil)
}

func (r *ProjectRuntime) DestroyQuadlet(ctx context.Context, project *StagedProject, file string) error {
	if r == nil || r.scope.Runtime != "podman" || project == nil || project.scope != r.scope || path.Base(file) != file || path.Ext(file) != ".container" {
		return ErrUnavailable
	}
	if _, ok := project.files[file]; !ok {
		return errors.New("unstaged Quadlet unit")
	}
	return r.invoke(ctx, "runtime.quadlet.remove", struct {
		Name string `json:"name"`
	}{file}, nil)
}
