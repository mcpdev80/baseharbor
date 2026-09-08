package applicationbackup

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

const (
	FormatMagic   = "BASEHARBOR-APP-BACKUP"
	SchemaVersion = 1

	MaxEntries           = 128
	MaxEntryBytes  int64 = 512 << 20
	MaxPayloadBytes int64 = 2 << 30
)

type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	Application   string    `json:"application"`
	Environment   string    `json:"environment"`
	CreatedAt     time.Time `json:"created_at"`
	Entries       []Entry   `json:"entries"`
}

type Entry struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported backup schema version %d", m.SchemaVersion)
	}
	if strings.TrimSpace(m.Application) == "" {
		return errors.New("backup application is required")
	}
	if strings.TrimSpace(m.Environment) == "" {
		return errors.New("backup environment is required")
	}
	if m.CreatedAt.IsZero() {
		return errors.New("backup creation timestamp is required")
	}
	if len(m.Entries) > MaxEntries {
		return fmt.Errorf("backup contains too many entries: %d > %d", len(m.Entries), MaxEntries)
	}

	seen := make(map[string]struct{}, len(m.Entries))
	var total int64
	for _, entry := range m.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		if _, exists := seen[entry.Name]; exists {
			return fmt.Errorf("duplicate backup entry %q", entry.Name)
		}
		seen[entry.Name] = struct{}{}
		if entry.Size > MaxPayloadBytes-total {
			return errors.New("backup payload exceeds maximum size")
		}
		total += entry.Size
	}
	return nil
}

func (e Entry) Validate() error {
	if !validLogicalName(e.Name) {
		return fmt.Errorf("invalid backup entry name %q", e.Name)
	}
	switch e.Kind {
	case "metadata", "postgres", "secrets":
	default:
		return fmt.Errorf("unsupported backup entry kind %q", e.Kind)
	}
	if e.Size < 0 || e.Size > MaxEntryBytes {
		return fmt.Errorf("backup entry %q has invalid size %d", e.Name, e.Size)
	}
	if len(e.SHA256) != 64 || !isLowerHex(e.SHA256) {
		return fmt.Errorf("backup entry %q has invalid sha256", e.Name)
	}
	return nil
}

func validLogicalName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if path.Clean(name) != name || strings.HasPrefix(name, "../") || name == ".." {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return strings.HasPrefix(name, "metadata/") || strings.HasPrefix(name, "postgres/") || strings.HasPrefix(name, "secrets/")
}

func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
