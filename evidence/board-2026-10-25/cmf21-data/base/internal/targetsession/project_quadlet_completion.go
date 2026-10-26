package targetsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
)

// VerifyQuadletCompletion asks the authenticated Node to verify successful
// execution after activation of this exact immutable source on its current boot.
// Missing containers or an earlier unit's success cannot substitute for proof.
// This observation never republishes, starts or retries a runtime mutation.
func (r *ProjectRuntime) VerifyQuadletCompletion(ctx context.Context, project *StagedProject, file string) error {
	if r == nil || r.scope.Runtime != "podman" || project == nil || project.scope != r.scope ||
		path.Base(file) != file || path.Ext(file) != ".container" ||
		!projectBundleID.MatchString(file[:len(file)-len(".container")]) {
		return ErrUnavailable
	}
	content, ok := project.files[file]
	if !ok {
		return errors.New("unstaged Quadlet completion source")
	}
	payload := struct {
		Name             string `json:"name"`
		Content          string `json:"content"`
		ProjectDirectory string `json:"project_directory"`
	}{file, string(content.data), project.directory}
	var result struct {
		Name             string `json:"name"`
		ProjectDirectory string `json:"project_directory"`
		ContentSHA256    string `json:"content_sha256"`
		Completed        bool   `json:"completed"`
	}
	if err := r.invoke(ctx, "runtime.quadlet.verify-completion", payload, &result); err != nil {
		return err
	}
	digest := sha256.Sum256(content.data)
	if !result.Completed || result.Name != file || result.ProjectDirectory != project.directory ||
		result.ContentSHA256 != hex.EncodeToString(digest[:]) {
		return errors.New("remote completion does not bind the selected immutable source")
	}
	return nil
}
