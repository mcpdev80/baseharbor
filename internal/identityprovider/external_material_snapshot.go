package identityprovider

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// Private input snapshots bind discovery and delivered material to the same
// owned files. No fingerprint or confidential value is emitted in results.
type externalMaterialSnapshot struct {
	trust   []byte
	sources []externalMaterialSource
}

type externalMaterialSource struct {
	path         string
	confidential bool
	owner        os.FileInfo
	digest       [32]byte
}

func captureExternalMaterial(files application.RuntimeFiles, secretPath, caPath, secret string) (*externalMaterialSnapshot, error) {
	snapshot := &externalMaterialSnapshot{}
	if secretPath == "" && secret != "" {
		secretPath = filepath.Join(files.Bindings, application.IdentityBindingName, "client-secret")
	}
	for _, source := range []externalMaterialSource{{path: secretPath, confidential: true}, {path: caPath}} {
		if source.path == "" {
			continue
		}
		data, err := readExternalMaterial(source.path, source.confidential)
		if err != nil {
			return nil, errors.New("external identity input material is unavailable or unprotected")
		}
		if source.confidential && strings.TrimSpace(string(data)) != secret {
			zeroExternalMaterial(data)
			return nil, errors.New("external identity credential changed during preparation; replan before rebinding")
		}
		source.owner, err = os.Lstat(source.path)
		if err != nil {
			zeroExternalMaterial(data)
			return nil, errors.New("external identity input owner changed during preparation")
		}
		source.digest = sha256.Sum256(data)
		snapshot.sources = append(snapshot.sources, source)
		if !source.confidential {
			snapshot.trust = append([]byte(nil), data...)
		}
		zeroExternalMaterial(data)
	}
	if err := snapshot.revalidate(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *externalMaterialSnapshot) revalidate() error {
	if s == nil {
		return errors.New("external identity material preparation is missing")
	}
	for _, source := range s.sources {
		info, err := os.Lstat(source.path)
		if err != nil || !os.SameFile(source.owner, info) {
			return errors.New("external identity input owner changed; replan before rebinding")
		}
		data, err := readExternalMaterial(source.path, source.confidential)
		if err != nil {
			return errors.New("external identity input material changed; replan before rebinding")
		}
		after, statErr := os.Lstat(source.path)
		matched := statErr == nil && os.SameFile(source.owner, after) && sha256.Sum256(data) == source.digest
		zeroExternalMaterial(data)
		if !matched {
			return errors.New("external identity input material changed; replan before rebinding")
		}
	}
	return nil
}

func zeroExternalMaterial(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
