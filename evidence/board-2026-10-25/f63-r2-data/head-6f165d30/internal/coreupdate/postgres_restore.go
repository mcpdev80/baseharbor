package coreupdate

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RestorePostgresBasebackup materializes the complete immutable physical
// archive in a NEW isolated directory. It never replaces a live PGDATA and
// never claims SQL startup, WAL replay or DCS cutover acceptance. An interrupted
// directory is deliberately preserved and cannot be blindly overwritten.
func RestorePostgresBasebackup(ctx context.Context, point StreamRecoveryPoint, major, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return errors.New("physical PostgreSQL restore requires an absolute isolated destination")
	}
	if err := VerifyPostgresBasebackup(point, major); err != nil {
		return err
	}
	parent, err := os.Lstat(filepath.Dir(destination))
	if err != nil {
		return err
	}
	if !parent.IsDir() || parent.Mode().Perm()&0077 != 0 {
		return errors.New("physical PostgreSQL restore parent is not private")
	}
	// OpenRoot confines writes even if a path component is replaced during
	// extraction. Mkdir is exclusive: foreign and partial destinations remain.
	if err := os.Mkdir(destination, 0700); err != nil {
		return fmt.Errorf("new isolated PostgreSQL directory: %w", err)
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	input, err := point.OpenVerified()
	if err != nil {
		return err
	}
	defer input.Close()
	reader := tar.NewReader(input)
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(header.Name, "./")
		if err := verifyPostgresArchiveEntry(header, name); err != nil {
			return err
		}
		if seen[name] {
			return errors.New("duplicate physical PostgreSQL restore entry")
		}
		seen[name] = true
		name = strings.TrimSuffix(name, "/")
		if err := root.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			if err := root.MkdirAll(name, 0700); err != nil {
				return err
			}
			continue
		}
		file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, reader)
		if copyErr == nil {
			copyErr = file.Sync()
		}
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	// Directory metadata must reach disk before downstream activation can use
	// this isolated tree. No completion marker is substituted for a boot proof.
	err = filepath.WalkDir(destination, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		dir, err := os.Open(name)
		if err != nil {
			return err
		}
		syncErr := dir.Sync()
		closeErr := dir.Close()
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	})
	if err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(destination))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// VerifyPostgresBasebackupRestore permits replay only when every isolated
// file still matches the original physical archive. Partial, changed or extra
// files never authorize activation or another extraction over existing data.
func VerifyPostgresBasebackupRestore(ctx context.Context, point StreamRecoveryPoint, major, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := VerifyPostgresBasebackup(point, major); err != nil {
		return err
	}
	st, err := os.Lstat(destination)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe physical PostgreSQL restore directory")
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	archive, err := point.OpenVerified()
	if err != nil {
		return err
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	expected := map[string]bool{".": true}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(strings.TrimPrefix(h.Name, "./"), "/")
		if err := verifyPostgresArchiveEntry(h, name); err != nil {
			return err
		}
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			expected[parent] = true
		}
		expected[name] = true
		st, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeDir {
			if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
				return errors.New("changed PostgreSQL restore directory")
			}
			continue
		}
		if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() != h.Size {
			return fmt.Errorf("changed PostgreSQL restore file %s", name)
		}
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		actualHash, archiveHash := sha256.New(), sha256.New()
		_, readErr := io.Copy(actualHash, file)
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if _, err := io.Copy(archiveHash, reader); err != nil {
			return err
		}
		if string(actualHash.Sum(nil)) != string(archiveHash.Sum(nil)) {
			return fmt.Errorf("PostgreSQL physical restore content changed: %s", name)
		}
	}
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !expected[name] || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("foreign entry in PostgreSQL physical restore: %s", name)
		}
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode().Perm()&0077 != 0 {
				return errors.New("physical PostgreSQL restore directory lost private permissions")
			}
		}
		return nil
	})
}
