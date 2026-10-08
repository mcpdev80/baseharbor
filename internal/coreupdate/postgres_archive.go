package coreupdate

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
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
	required := map[string]bool{"PG_VERSION": false, "backup_label": false}
	count := 0
	for {
		h, e := tr.Next()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return fmt.Errorf("invalid native PostgreSQL tar archive: %w", e)
		}
		name := strings.TrimPrefix(h.Name, "./")
		if name == "tablespace_map" {
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
				return errors.New("empty PostgreSQL backup label")
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
