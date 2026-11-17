package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// NeverDeliveredProof is a one-shot initialization witness, NOT a runtime
// inventory. It is created only by a new repository adoption and consumed
// before the first deployment record can be persisted.
type NeverDeliveredProof struct {
	Version       int    `json:"version"`
	ApplicationID string `json:"application_id"`
	Application   string `json:"application"`
	Environment   string `json:"environment"`
	Target        string `json:"target"`
}

func neverDeliveredProofPath(target, applicationID, environment string) (string, error) {
	if err := ValidateTargetName(target); err != nil {
		return "", err
	}
	if err := stableid.ValidateUUIDv4("application", applicationID); err != nil {
		return "", err
	}
	if strings.TrimSpace(environment) == "" || strings.ContainsAny(environment, "/\\") {
		return "", fmt.Errorf("invalid environment for initialization proof")
	}
	root, err := TargetStateRoot(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "never-delivered", applicationID, environment+".json"), nil
}

func CreateNeverDeliveredProof(target, applicationID, application, environment string) error {
	path, err := neverDeliveredProofPath(target, applicationID, environment)
	if err != nil {
		return err
	}
	proof := NeverDeliveredProof{Version: 1, ApplicationID: applicationID, Application: application, Environment: environment, Target: target}
	data, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func VerifyNeverDeliveredProof(id DeploymentIdentity) error {
	path, err := neverDeliveredProofPath(id.Target, id.ApplicationID, id.Environment)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var p NeverDeliveredProof
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	if p.Version != 1 || p.ApplicationID != id.ApplicationID || p.Application != id.Application || p.Environment != id.Environment || p.Target != id.Target {
		return errors.New("never-delivered proof does not match selected owner")
	}
	return nil
}

func ConsumeNeverDeliveredProof(id DeploymentIdentity) error {
	path, err := neverDeliveredProofPath(id.Target, id.ApplicationID, id.Environment)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
