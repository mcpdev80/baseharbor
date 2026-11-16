package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Inventory protected existing provider files without reading secret values
// into the result. Unverifiable entries retain their individual location and
// reason. Never follow a symlink or invent ownership for an unprotected entry.
func observeOwnedProviderFiles(owner, target, instance, root string) ([]application.ProviderMaterial, bool, error) {
	materials := []application.ProviderMaterial{}
	complete, count := true, 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		count++
		if count > 10000 {
			return fmt.Errorf("provider material inventory exceeds bounded inspection")
		}
		location := application.ResourceLocation{Owner: owner, Target: target, Kind: "path", Reference: path}
		material := application.ProviderMaterial{ID: instance + "/path/" + path, Kind: "unknown", Location: &location, Recovery: []application.ResourceLocation{}, Evidence: "entry or subtree cannot be verified; no contents inspected, ownership/disposition unknown"}
		if walkErr != nil {
			materials = append(materials, material)
			complete = false
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			materials = append(materials, material)
			complete = false
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !bhruntime.ProtectedStateOwner(path, info) {
			material.Evidence = "linked, foreign or unprotected entry remains at this path; ownership and contents are unverified; no removal authorized"
			materials = append(materials, material)
			complete = false
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() || entry.Name() == ".lifecycle.lock" {
			return nil
		}
		if !info.Mode().IsRegular() {
			materials = append(materials, material)
			complete = false
			return nil
		}
		material.Kind, material.Ownership, material.Verified = "configuration", capability.OwnershipBaseHarbor, true
		material.Evidence = fmt.Sprintf("protected retained file; size=%d; contents not exposed; authoritative content revision bound privately", info.Size())
		if err := inspectProviderCertificateMaterial(path, info, &material); err != nil {
			complete = false
			material.Verified = false
			material.Evidence = "protected material could not be fully inspected; certificate validity and disposition remain unknown"
		}
		materials = append(materials, material)
		return nil
	})
	return materials, complete && err == nil, err
}
