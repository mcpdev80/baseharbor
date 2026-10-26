package credential

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PreparedMaterialStore interface {
	Load(key string) ([]byte, error)
	Save(key string, material []byte) error
	Clear(key string) error
}

type FilePreparedMaterialStore struct {
	Dir string
}

func (s FilePreparedMaterialStore) Load(key string) ([]byte, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read prepared credential material: %w", err)
	}
	return append([]byte(nil), data...), nil
}

func (s FilePreparedMaterialStore) Save(key string, material []byte) error {
	if len(material) == 0 {
		return errors.New("prepared credential material is empty")
	}
	path, err := s.path(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create prepared credential material directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect prepared credential material directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".prepared-*")
	if err != nil {
		return fmt.Errorf("create prepared credential material temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(material); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace prepared credential material: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func (s FilePreparedMaterialStore) Clear(key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove prepared credential material: %w", err)
	}
	return nil
}

func (s FilePreparedMaterialStore) path(key string) (string, error) {
	key, err := validateRotationKey(key)
	if err != nil {
		return "", err
	}
	dir := filepath.Clean(strings.TrimSpace(s.Dir))
	if dir == "." || dir == "" {
		return "", errors.New("prepared credential material directory is required")
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".material"), nil
}
