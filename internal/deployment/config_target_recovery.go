package deployment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const TargetDefinitionRecoveryVersion = "baseharbor.target-definition-recovery/v1"

// TargetDefinitionRecovery is an operator recovery copy of one existing
// configuration entry, not a second Target registry or a live provider owner.
// It contains existing references, never exported service credentials.
type TargetDefinitionRecovery struct {
	Version    string           `json:"version"`
	Name       string           `json:"name"`
	DataRoot   string           `json:"data_root"`
	Definition TargetDefinition `json:"definition"`
	Access     AccessDefinition `json:"access"`
}

func TargetRecoveryFromConfig(expected []byte, name string) (TargetDefinitionRecovery, error) {
	if err := ValidateTargetName(name); err != nil {
		return TargetDefinitionRecovery{}, err
	}
	cfg, err := decodeConfig(expected)
	if err != nil {
		return TargetDefinitionRecovery{}, err
	}
	definition, ok := cfg.Targets[name]
	if !ok {
		return TargetDefinitionRecovery{}, errors.New("Target recovery requires its existing configuration entry")
	}
	root, err := DataRoot()
	if err != nil {
		return TargetDefinitionRecovery{}, err
	}
	root, err = filepath.Abs(root)
	return TargetDefinitionRecovery{Version: TargetDefinitionRecoveryVersion, Name: name, DataRoot: root, Definition: definition, Access: cfg.Access[definition.Access.Reference]}, err
}

func (r TargetDefinitionRecovery) OwnerPath() (string, error) {
	if err := ValidateTargetName(r.Name); err != nil {
		return "", err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	path, err := ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "target-recovery", r.Name+"-"+hex.EncodeToString(sum[:])+".json"), nil
}

// SaveExpectedPreservingTarget persists recovery under the same configuration
// lock, before the acknowledged replacement. Existing copies are never
// overwritten. An interrupted replacement leaves protected recovery available.
func (c Config) SaveExpectedPreservingTarget(expected []byte, name string, prepareSelection func() error) (string, error) {
	recovery, err := TargetRecoveryFromConfig(expected, name)
	if err != nil {
		return "", err
	}
	path, err := recovery.OwnerPath()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(recovery)
	if err != nil {
		return "", err
	}
	if len(raw) > 8<<20 {
		return "", errors.New("Target recovery exceeds its supported owner size")
	}
	err = c.saveWithPreparation(expected, true, func() error {
		if err := writeTargetRecoveryOwner(path, raw); err != nil {
			return err
		}
		if prepareSelection != nil {
			return prepareSelection()
		}
		return nil
	})
	return path, err
}

func writeTargetRecoveryOwner(path string, raw []byte) error {
	root := filepath.Dir(path)
	if err := configurationParents(root, true); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || !bhruntime.ProtectedStateOwner(root, info) {
		return errors.New("Target recovery requires its protected owner directory")
	}
	before, err := readConfigurationOwner(path)
	if err != nil {
		return err
	}
	if before != nil {
		if !bytes.Equal(before, raw) {
			return ErrConfigurationChanged
		}
		return nil
	}
	file, err := os.CreateTemp(root, ".target-recovery-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(raw); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	// Link installs a complete synchronized owner without replacing a path
	// created meanwhile. Removing only our temporary name restores one link.
	if err := os.Link(temporary, path); err != nil {
		return err
	}
	if err := os.Remove(temporary); err != nil {
		return err
	}
	after, err := readConfigurationOwner(path)
	if err != nil || !bytes.Equal(after, raw) {
		return errors.New("Target recovery owner verification failed")
	}
	return nil
}

func LoadTargetDefinitionRecovery(path, name string) (TargetDefinitionRecovery, error) {
	raw, err := readConfigurationOwner(path)
	if err != nil || raw == nil {
		return TargetDefinitionRecovery{}, errors.New("Target recovery owner is unavailable or unverified")
	}
	if machine.ValidateJSONObject(raw, 8<<20) != nil {
		return TargetDefinitionRecovery{}, errors.New("Target recovery owner is ambiguous or invalid")
	}
	var r TargetDefinitionRecovery
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&r); err != nil {
		return r, errors.New("Target recovery owner is invalid")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return r, errors.New("Target recovery owner has trailing data")
	}
	root, err := DataRoot()
	if err != nil {
		return r, err
	}
	root, err = filepath.Abs(root)
	if err != nil || r.Version != TargetDefinitionRecoveryVersion || r.Name != name || r.DataRoot != root {
		return r, errors.New("Target recovery does not match the selected name and existing data owner")
	}
	if err := ValidateTargetName(name); err != nil {
		return r, err
	}
	cfg := Config{Version: ConfigVersion, Targets: map[string]TargetDefinition{name: r.Definition}, Access: map[string]AccessDefinition{r.Definition.Access.Reference: r.Access}}
	return r, cfg.Validate()
}

func (r TargetDefinitionRecovery) RestoreInto(cfg Config) (Config, error) {
	if _, exists := cfg.Targets[r.Name]; exists {
		return cfg, errors.New("Target restoration never overwrites an existing configuration entry")
	}
	if access, exists := cfg.Access[r.Definition.Access.Reference]; exists && access != r.Access {
		return cfg, errors.New("Target recovery access reference differs from its existing owner")
	}
	if cfg.Access == nil {
		cfg.Access = map[string]AccessDefinition{}
	}
	if cfg.Targets == nil {
		cfg.Targets = map[string]TargetDefinition{}
	}
	cfg.Access[r.Definition.Access.Reference], cfg.Targets[r.Name] = r.Access, r.Definition
	return cfg, cfg.Validate()
}
