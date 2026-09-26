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
)

const DeploymentRecordVersion = 1

type DeploymentIdentity struct {
	Target      string `json:"target"`
	Application string `json:"application"`
	Environment string `json:"environment"`
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

func (id DeploymentIdentity) Validate() error {
	if err := ValidateTargetName(id.Target); err != nil {
		return err
	}
	if strings.TrimSpace(id.Application) == "" || strings.ContainsAny(id.Application, `/\\`) {
		return fmt.Errorf("invalid application %q", id.Application)
	}
	if strings.TrimSpace(id.Environment) == "" || strings.ContainsAny(id.Environment, `/\\`) {
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
	return filepath.Join(root, "deployments", id.Application, id.Environment), nil
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
	path := filepath.Join(root, "deployment.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return DeploymentRecord{}, err
	}
	var record DeploymentRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return DeploymentRecord{}, fmt.Errorf("parse deployment record: %w", err)
	}
	if record.Version != DeploymentRecordVersion {
		return DeploymentRecord{}, fmt.Errorf("unsupported deployment record version %d", record.Version)
	}
	if record.Identity != id {
		return DeploymentRecord{}, errors.New("deployment record identity does not match storage path")
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

func ListDeployments(target string) ([]DeploymentRecord, error) {
	root, err := TargetStateRoot(target)
	if err != nil {
		return nil, err
	}
	return listDeploymentRecords(filepath.Join(root, "deployments"))
}

func ListDeploymentsForDisplay(target string) ([]DeploymentRecord, []error, error) {
	root, err := TargetStateRoot(target)
	if err != nil {
		return nil, nil, err
	}
	return listDeploymentRecordsBestEffort(filepath.Join(root, "deployments"))
}

func ListAllDeploymentsForDisplay() ([]DeploymentRecord, []error, error) {
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
		items, itemWarnings, err := ListDeploymentsForDisplay(entry.Name())
		if err != nil {
			warnings = append(warnings, fmt.Errorf("target %s: %w", entry.Name(), err))
			continue
		}
		records = append(records, items...)
		warnings = append(warnings, itemWarnings...)
	}
	sortDeploymentRecords(records)
	return records, warnings, nil
}

func ListAllDeployments() ([]DeploymentRecord, error) {
	root, err := DataRoot()
	if err != nil {
		return nil, err
	}
	targetsRoot := filepath.Join(root, "targets")
	entries, err := os.ReadDir(targetsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []DeploymentRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		items, err := ListDeployments(entry.Name())
		if err != nil {
			return nil, err
		}
		records = append(records, items...)
	}
	sortDeploymentRecords(records)
	return records, nil
}

func listDeploymentRecords(root string) ([]DeploymentRecord, error) {
	apps, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []DeploymentRecord
	for _, app := range apps {
		if !app.IsDir() {
			continue
		}
		envs, err := os.ReadDir(filepath.Join(root, app.Name()))
		if err != nil {
			return nil, err
		}
		for _, env := range envs {
			if !env.IsDir() {
				continue
			}
			id := DeploymentIdentity{Application: app.Name(), Environment: env.Name()}
			id.Target = filepath.Base(filepath.Dir(root))
			record, err := LoadDeploymentRecord(id)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		}
	}
	sortDeploymentRecords(records)
	return records, nil
}

func listDeploymentRecordsBestEffort(root string) ([]DeploymentRecord, []error, error) {
	apps, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var records []DeploymentRecord
	var warnings []error
	for _, app := range apps {
		if !app.IsDir() {
			continue
		}
		envs, err := os.ReadDir(filepath.Join(root, app.Name()))
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", app.Name(), err))
			continue
		}
		for _, env := range envs {
			if !env.IsDir() {
				continue
			}
			id := DeploymentIdentity{
				Target:      filepath.Base(filepath.Dir(root)),
				Application: app.Name(),
				Environment: env.Name(),
			}
			record, err := LoadDeploymentRecord(id)
			if err != nil {
				warnings = append(warnings, fmt.Errorf("%s/%s/%s: %w", id.Target, id.Application, id.Environment, err))
				continue
			}
			records = append(records, record)
		}
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
		return a.Environment < b.Environment
	})
}

func SourceAvailable(source DeploymentSource) bool {
	if strings.TrimSpace(source.Repository) == "" {
		return false
	}
	info, err := os.Stat(source.Repository)
	return err == nil && info.IsDir()
}
