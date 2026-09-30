package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

const DeploymentRecordVersion = 2

type DeploymentIdentity struct {
	DeploymentID string `json:"deployment_id"`
	ApplicationID string `json:"application_id"`
	Target        string `json:"target"`
	Application   string `json:"application"`
	Environment   string `json:"environment"`
}

func NewDeploymentIdentity(target, applicationID, application, environment string) (DeploymentIdentity, error) {
	id, err := stableid.NewUUIDv4("deployment")
	if err != nil {
		return DeploymentIdentity{}, err
	}
	identity := DeploymentIdentity{
		DeploymentID: id,
		ApplicationID: strings.TrimSpace(applicationID),
		Target: strings.TrimSpace(target),
		Application: strings.TrimSpace(application),
		Environment: strings.TrimSpace(environment),
	}
	if err := identity.Validate(); err != nil {
		return DeploymentIdentity{}, err
	}
	return identity, nil
}

func ValidateDeploymentID(id string) error {
	return stableid.ValidateUUIDv4("deployment", id)
}

type DeploymentSource struct {
	Kind       string `json:"kind"`
	Repository string `json:"repository,omitempty"`
	Manifest   string `json:"manifest,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

type AppliedDeployment struct {
	Intent          json.RawMessage   `json:"intent,omitempty"`
	RuntimeProvider string            `json:"runtime_provider"`
	GeneratedState  map[string]string `json:"generated_state,omitempty"`
	LastAppliedRef  string            `json:"last_applied_ref,omitempty"`
}

type ObservedDeployment struct {
	State      string    `json:"state,omitempty"`
	Ready      bool      `json:"ready,omitempty"`
	VerifiedAt time.Time `json:"verified_at,omitempty"`
}

type DeploymentRecord struct {
	Version  int                `json:"version"`
	Identity DeploymentIdentity `json:"identity"`
	Source   DeploymentSource   `json:"source"`
	Applied  AppliedDeployment  `json:"applied"`
	Observed ObservedDeployment `json:"observed,omitempty"`
}

type DeploymentRecordStateError struct {
	Identity DeploymentIdentity
	Kind     string
	Err      error
}

func (e *DeploymentRecordStateError) Error() string {
	if e == nil {
		return "deployment record state error"
	}
	return fmt.Sprintf("%s deployment state for %s/%s/%s [%s]: %v",
		e.Kind, e.Identity.Target, e.Identity.Application, e.Identity.Environment, e.Identity.DeploymentID, e.Err)
}

func (e *DeploymentRecordStateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func DeploymentRecordState(err error) (*DeploymentRecordStateError, bool) {
	var stateErr *DeploymentRecordStateError
	if errors.As(err, &stateErr) {
		return stateErr, true
	}
	return nil, false
}

func (id DeploymentIdentity) Validate() error {
	if err := ValidateDeploymentID(id.DeploymentID); err != nil {
		return err
	}
	if err := stableid.ValidateUUIDv4("application", id.ApplicationID); err != nil {
		return err
	}
	if err := ValidateTargetName(id.Target); err != nil {
		return err
	}
	if strings.TrimSpace(id.Application) == "" || strings.ContainsAny(id.Application, `/\`) {
		return fmt.Errorf("invalid application %q", id.Application)
	}
	if strings.TrimSpace(id.Environment) == "" || strings.ContainsAny(id.Environment, `/\`) {
		return fmt.Errorf("invalid environment %q", id.Environment)
	}
	return nil
}

func DeploymentRoot(id DeploymentIdentity) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	root, err := TargetStateRoot(id.Target)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "deployments", id.DeploymentID), nil
}

func SaveDeploymentRecord(record DeploymentRecord) error {
	if record.Version == 0 {
		record.Version = DeploymentRecordVersion
	}
	if record.Version != DeploymentRecordVersion {
		return fmt.Errorf("unsupported deployment record version %d", record.Version)
	}
	root, err := DeploymentRoot(record.Identity)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create deployment state: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode deployment record: %w", err)
	}
	path := filepath.Join(root, "deployment.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write deployment record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace deployment record: %w", err)
	}
	return nil
}

func LoadDeploymentRecord(id DeploymentIdentity) (DeploymentRecord, error) {
	root, err := DeploymentRoot(id)
	if err != nil {
		return DeploymentRecord{}, err
	}
	return loadDeploymentRecordFile(id.Target, id.DeploymentID, filepath.Join(root, "deployment.json"), &id)
}

func loadDeploymentRecordFile(target, deploymentID, path string, expected *DeploymentIdentity) (DeploymentRecord, error) {
	identity := DeploymentIdentity{Target: target, DeploymentID: deploymentID}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DeploymentRecord{}, &DeploymentRecordStateError{Identity: identity, Kind: "incomplete", Err: fmt.Errorf("deployment.json is missing: %w", os.ErrNotExist)}
		}
		return DeploymentRecord{}, err
	}
	var record DeploymentRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: identity, Kind: "corrupt", Err: fmt.Errorf("parse deployment.json: %w", err)}
	}
	if record.Version != DeploymentRecordVersion {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: fmt.Errorf("unsupported deployment record version %d", record.Version)}
	}
	if err := record.Identity.Validate(); err != nil {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: err}
	}
	if record.Identity.Target != target || record.Identity.DeploymentID != deploymentID {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: record.Identity, Kind: "corrupt", Err: errors.New("deployment record identity does not match storage path")}
	}
	if expected != nil && record.Identity != *expected {
		return DeploymentRecord{}, &DeploymentRecordStateError{Identity: *expected, Kind: "corrupt", Err: errors.New("deployment record identity does not match requested identity")}
	}
	return record, nil
}

func DeleteDeploymentRecord(id DeploymentIdentity) error {
	root, err := DeploymentRoot(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func FindDeployment(target, applicationID, environment string) (DeploymentRecord, bool, error) {
	records, err := ListDeployments(target)
	if err != nil {
		return DeploymentRecord{}, false, err
	}
	var match *DeploymentRecord
	for i := range records {
		record := records[i]
		if record.Identity.ApplicationID != applicationID || record.Identity.Environment != environment {
			continue
		}
		if match != nil {
			return DeploymentRecord{}, false, fmt.Errorf(
				"multiple deployments claim application_id %s environment %s on target %s",
				applicationID, environment, target,
			)
		}
		copy := record
		match = &copy
	}
	if match == nil {
		return DeploymentRecord{}, false, nil
	}
	return *match, true, nil
}

func ListDeployments(target string) ([]DeploymentRecord, error) {
	root, err := TargetStateRoot(target)
	if err != nil {
		return nil, err
	}
	records, _, err := listDeploymentRecords(filepath.Join(root, "deployments"), target, false)
	return records, err
}

func ListDeploymentsForDisplay(target string) ([]DeploymentRecord, []error, error) {
	root, err := TargetStateRoot(target)
	if err != nil {
		return nil, nil, err
	}
	return listDeploymentRecords(filepath.Join(root, "deployments"), target, true)
}

func ListAllDeploymentsForDisplay() ([]DeploymentRecord, []error, error) {
	return listAllDeployments(true)
}

func ListAllDeployments() ([]DeploymentRecord, error) {
	records, _, err := listAllDeployments(false)
	return records, err
}

func listAllDeployments(bestEffort bool) ([]DeploymentRecord, []error, error) {
	root, err := DataRoot()
	if err != nil {
		return nil, nil, err
	}
	targetsRoot := filepath.Join(root, "targets")
	entries, err := os.ReadDir(targetsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var records []DeploymentRecord
	var warnings []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		items, itemWarnings, err := listDeploymentRecords(filepath.Join(targetsRoot, entry.Name(), "deployments"), entry.Name(), bestEffort)
		if err != nil {
			if bestEffort {
				warnings = append(warnings, fmt.Errorf("target %s: %w", entry.Name(), err))
				continue
			}
			return nil, nil, err
		}
		records = append(records, items...)
		warnings = append(warnings, itemWarnings...)
	}
	sortDeploymentRecords(records)
	return records, warnings, nil
}

func listDeploymentRecords(root, target string, bestEffort bool) ([]DeploymentRecord, []error, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var records []DeploymentRecord
	var warnings []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deploymentID := entry.Name()
		path := filepath.Join(root, deploymentID, "deployment.json")
		record, err := loadDeploymentRecordFile(target, deploymentID, path, nil)
		if err != nil {
			if bestEffort {
				warnings = append(warnings, err)
				continue
			}
			return nil, nil, err
		}
		records = append(records, record)
	}
	sortDeploymentRecords(records)
	return records, warnings, nil
}

func sortDeploymentRecords(records []DeploymentRecord) {
	sort.Slice(records, func(i, j int) bool {
		a, b := records[i].Identity, records[j].Identity
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.Application != b.Application {
			return a.Application < b.Application
		}
		if a.Environment != b.Environment {
			return a.Environment < b.Environment
		}
		return a.DeploymentID < b.DeploymentID
	})
}

func SourceAvailable(source DeploymentSource) bool {
	if strings.TrimSpace(source.Repository) == "" {
		return false
	}
	info, err := os.Stat(source.Repository)
	return err == nil && info.IsDir()
}
