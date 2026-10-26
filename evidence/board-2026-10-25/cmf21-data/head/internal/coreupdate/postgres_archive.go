package coreupdate

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// VerifyPostgresBasebackup verifies that the immutable native backup is a
// plausible single-tablespace PostgreSQL tar archive for the pinned major.
// A valid tar alone is NOT evidence that a restore has succeeded.
func VerifyPostgresBasebackup(p StreamRecoveryPoint, major string) error {
	if major == "" || strings.ContainsAny(major, "/\\\x00") {
		return errors.New("invalid PostgreSQL major")
	}
	if err := p.Verify(); err != nil {
		return err
	}
	path, _, err := p.paths()
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	required := map[string]bool{"PG_VERSION": false, "backup_label": false, "global/pg_control": false}
	count := 0
	seen := map[string]bool{}
	for {
		h, e := tr.Next()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return fmt.Errorf("invalid native PostgreSQL tar archive: %w", e)
		}
		name := strings.TrimPrefix(h.Name, "./")
		if err := verifyPostgresArchiveEntry(h, name); err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("duplicate PostgreSQL backup entry %q", name)
		}
		seen[name] = true
		// PostgreSQL emits this marker even without additional tablespaces.
		// Only an empty regular map is safe for the single stdout archive.
		if name == "tablespace_map" && (h.Size != 0 || h.Typeflag == tar.TypeDir) {
			return errors.New("additional PostgreSQL tablespaces require a multi-archive backup; streaming stdout backup unsupported")
		}
		if _, ok := required[name]; ok {
			if required[name] || h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
				return errors.New("duplicate or nonregular PostgreSQL backup marker")
			}
			if name == "PG_VERSION" {
				data, e := io.ReadAll(io.LimitReader(tr, 64))
				if e != nil || strings.TrimSpace(string(data)) != major {
					return errors.New("PostgreSQL backup version mismatch")
				}
			} else if h.Size == 0 {
				return errors.New("empty PostgreSQL required backup marker")
			}
			required[name] = true
		}
		count++
		if count > 1000000 {
			return errors.New("excessive native backup entries")
		}
	}
	for name, ok := range required {
		if !ok {
			return fmt.Errorf("PostgreSQL native backup missing %s", name)
		}
	}
	return nil
}

// Native stdout backups admit only directories and regular files. External
// tablespaces, links and devices cannot be recovered into this data directory.
func verifyPostgresArchiveEntry(h *tar.Header, name string) error {
	clean := strings.TrimSuffix(name, "/")
	if clean == "" || clean == "." || path.IsAbs(clean) || path.Clean(clean) != clean ||
		clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsAny(clean, "\\\x00") {
		return fmt.Errorf("unsafe PostgreSQL backup path %q", name)
	}
	if h.Typeflag != tar.TypeDir && h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
		return fmt.Errorf("unsupported PostgreSQL backup entry type for %q", name)
	}
	if h.Size < 0 || h.Mode&07000 != 0 {
		return fmt.Errorf("unsafe PostgreSQL backup entry metadata for %q", name)
	}
	return nil
}
