package coreupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// HAPostgresComposeCheckpoint manages only the three owned Spilo member image
// declarations. It does not restart services or authorize a DB migration.
type HAPostgresComposeCheckpoint struct {
	Path      string
	Directory string
	Previous  BackingPin
	Desired   BackingPin
}

func (c HAPostgresComposeCheckpoint) checkpointPath() (string, error) {
	if c.Path == "" || c.Directory == "" || c.Previous.Role != "core-ha-postgresql" || c.Desired.Role != "core-ha-postgresql" ||
		!validDigest(c.Previous.Digest) || !validDigest(c.Desired.Digest) || c.Previous.Image == "" || c.Desired.Image == "" {
		return "", errors.New("incomplete HA Spilo checkpoint identity")
	}
	if !sameSpiloMajor(c.Previous, c.Desired) {
		return "", errors.New("UNSUPPORTED: HA PostgreSQL major change")
	}
	h := sha256.Sum256([]byte(c.Previous.Image + "@" + c.Previous.Digest + "\x00" + c.Desired.Image + "@" + c.Desired.Digest))
	return filepath.Join(c.Directory, hex.EncodeToString(h[:])+".spilo-compose"), nil
}
func sameSpiloMajor(a, b BackingPin) bool {
	major := func(s string) string {
		index := strings.Index(s, "-spilo-")
		if index <= 0 {
			return ""
		}
		prefix := s[:index]
		for _, r := range prefix {
			if r < '0' || r > '9' {
				return ""
			}
		}
		return prefix
	}
	return major(a.Version) != "" && major(a.Version) == major(b.Version)
}

// RewriteOwnedSpiloImages applies the same immutable image to exactly three
// known managed PostgreSQL members. Unknown current images fail closed.
func RewriteOwnedSpiloImages(input []byte, previous, desired BackingPin) ([]byte, error) {
	if previous.Role != "core-ha-postgresql" || desired.Role != "core-ha-postgresql" ||
		!validDigest(previous.Digest) || !validDigest(desired.Digest) || !sameSpiloMajor(previous, desired) {
		return nil, errors.New("UNSUPPORTED: invalid Spilo image transition")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(input, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("invalid Core HA Compose")
	}
	var services *yaml.Node
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "services" {
			services = root.Content[i+1]
		}
	}
	if services == nil || services.Kind != yaml.MappingNode {
		return nil, errors.New("missing managed services")
	}
	wanted := map[string]bool{"postgres-member-1": false, "postgres-member-2": false, "postgres-member-3": false}
	for i := 0; i+1 < len(services.Content); i += 2 {
		name := services.Content[i].Value
		if _, ok := wanted[name]; !ok {
			continue
		}
		if wanted[name] {
			return nil, fmt.Errorf("duplicate owned Spilo member %s", name)
		}
		wanted[name] = true
		svc := services.Content[i+1]
		if svc.Kind != yaml.MappingNode {
			return nil, errors.New("invalid member service definition")
		}
		found := false
		for j := 0; j+1 < len(svc.Content); j += 2 {
			if svc.Content[j].Value != "image" {
				continue
			}
			if found {
				return nil, fmt.Errorf("duplicate image declaration for %s", name)
			}
			found = true
			image := svc.Content[j+1]
			if image.Kind != yaml.ScalarNode {
				return nil, errors.New("non-scalar Spilo image")
			}
			original := image.Value
			if original != previous.Image && original != previous.Image+"@"+previous.Digest {
				return nil, fmt.Errorf("owned Spilo member %s no longer matches inventoried image", name)
			}
			image.Tag = "!!str"
			image.Value = desired.Image + "@" + desired.Digest
		}
		if !found {
			return nil, fmt.Errorf("missing Spilo image for %s", name)
		}
	}
	for name, exists := range wanted {
		if !exists {
			return nil, fmt.Errorf("missing owned Spilo member %s", name)
		}
	}
	var out bytes.Buffer
	e := yaml.NewEncoder(&out)
	e.SetIndent(2)
	if err := e.Encode(&doc); err != nil {
		return nil, err
	}
	if err := e.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Stage is crash-safe and idempotent. The recovery point and DCS evidence must
// already be verified by the caller before invoking it.
func (c HAPostgresComposeCheckpoint) Stage() error {
	path, err := c.checkpointPath()
	if err != nil {
		return err
	}
	original, err := readVerifiedCompose(c.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Directory, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(c.Directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe Spilo checkpoint directory")
	}
	saved, err := readVerifiedCompose(path)
	if errors.Is(err, os.ErrNotExist) {
		projected, err := RewriteOwnedSpiloImages(original, c.Previous, c.Desired)
		if err != nil {
			return err
		}
		if err := writeExclusive(path, original); err != nil {
			return err
		}
		return atomicReplaceOwnerFile(c.Path, projected)
	}
	if err != nil {
		return err
	}
	projected, err := RewriteOwnedSpiloImages(saved, c.Previous, c.Desired)
	if err != nil {
		return err
	}
	if bytes.Equal(original, projected) {
		return nil
	}
	if !bytes.Equal(original, saved) {
		return errors.New("Spilo Compose drift; refusing replay")
	}
	return atomicReplaceOwnerFile(c.Path, projected)
}
