package main

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Only the exact generated PostgreSQL projections can be treated as
// non-persistent inventory after independent proof of their origin and safety.
// This never changes a rejected material's Verified or Ownership fields.
func sqlNativeGeneratedProjectionInventory(materials []application.ProviderMaterial, root string) bool {
	kind := map[string]string{
		filepath.Join(root, "runtime", "pg_hba.conf"):                   "hba",
		filepath.Join(root, "runtime", "server-cert.pem"):               "cert",
		filepath.Join(root, "runtime", "server-key.pem"):                "key",
		filepath.Join(root, "service-access", "pki", "ca.pem"):          "ca",
		filepath.Join(root, "service-access", "pki", "server-cert.pem"): "source-cert",
	}
	for _, material := range materials {
		if material.Verified {
			continue
		}
		if material.Location == nil || material.Location.Kind != "path" {
			return false
		}
		path := material.Location.Reference
		classification, ok := kind[path]
		if !ok || material.Kind != "unknown" || !sqlNativeGeneratedProjectionFile(path, root, classification) {
			return false
		}
	}
	return true
}

func sqlNativeGeneratedProjectionFile(path, root, classification string) bool {
	info, err := os.Lstat(path)
	if err != nil || !sqlPublicProjectionOwner(info) {
		return false
	}
	// The public exception applies to file leaf permissions only; all
	// ancestor directories still need the ordinary protected owner proof.
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		state, err := os.Lstat(dir)
		if err != nil || !state.IsDir() || !bhruntime.ProtectedStateOwner(dir, state) {
			return false
		}
		if dir == root {
			break
		}
		if dir == filepath.Dir(dir) {
			return false
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || len(data) > 1<<20 {
		return false
	}
	defer zeroBytes(data)
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, current) || !sqlPublicProjectionOwner(current) {
		return false
	}
	switch classification {
	case "hba":
		return bytes.Equal(data, []byte("local all all trust\nhostssl all all 0.0.0.0/0 scram-sha-256\nhostssl all all ::/0 scram-sha-256\nhostnossl all all 0.0.0.0/0 reject\nhostnossl all all ::/0 reject\n"))
	case "ca", "source-cert":
		var public application.ProviderMaterial
		return describeProviderCertificateWindow(data, &public) == nil && public.Kind == "certificate"
	case "cert", "key":
		source := filepath.Join(root, "service-access", "pki", "server-cert.pem")
		if classification == "key" {
			source = filepath.Join(root, "service-access", "pki", "server-key.pem")
		}
		original, err := os.Lstat(source)
		if err != nil || !original.Mode().IsRegular() || original.Size() <= 0 || original.Size() > 1<<20 {
			return false
		}
		sourceProtected := classification == "key"
		if sourceProtected {
			if !bhruntime.ProtectedStateOwner(source, original) {
				return false
			}
		} else if !sqlPublicProjectionOwner(original) {
			return false
		}
		protected, err := os.ReadFile(source)
		if err != nil {
			return false
		}
		defer zeroBytes(protected)
		var verified application.ProviderMaterial
		if sourceProtected {
			if err := inspectProviderCertificateMaterial(source, original, &verified); err != nil || verified.Kind != "key" {
				return false
			}
		} else if describeProviderCertificateWindow(protected, &verified) != nil || verified.Kind != "certificate" {
			return false
		}
		now, err := os.Lstat(source)
		if err != nil || !os.SameFile(original, now) || !bytes.Equal(data, protected) {
			return false
		}
		if sourceProtected {
			return bhruntime.ProtectedStateOwner(source, now)
		}
		return sqlPublicProjectionOwner(now)
	default:
		return false
	}
}
