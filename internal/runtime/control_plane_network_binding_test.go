package runtime

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCoreExternalDefaultNetworkRetainsCredentialsAndSeparateOwner(t *testing.T) {
	files, err := EnsureFilesForProjectAndResources(filepath.Join(t.TempDir(), "runtime"), "bh-owner-shared", "baseharbor-owner", Ports{Postgres: 15432, OpenBao: 18200}, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := loadRuntimeEnvironment(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	files, err = BindControlPlaneToExistingDefaultNetwork(files)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := ExistingFilesForProject(filepath.Dir(files.Compose), files.Project)
	if err != nil || !reloaded.DefaultNetworkExternal {
		t.Fatalf("protected network reference was not restored: %v", err)
	}
	after, err := loadRuntimeEnvironment(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	delete(after, coreExternalDefaultNetworkKey)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("network reference changed existing credentials or ports")
	}
	inventory, err := ExistingControlPlaneRuntimeInventory(filepath.Dir(files.Compose), files.Project)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range inventory.Resources {
		if resource.Kind == "network" && resource.Name == ControlPlaneNetworkName(files.ResourceProject) {
			t.Fatal("Core claimed the retained shared backend's network")
		}
	}
	environment, _ := os.ReadFile(files.Env)
	compose, _ := os.ReadFile(files.Compose)
	if _, err := BindControlPlaneToExistingDefaultNetwork(reloaded); err != nil {
		t.Fatal(err)
	}
	nextEnvironment, _ := os.ReadFile(files.Env)
	nextCompose, _ := os.ReadFile(files.Compose)
	if !bytes.Equal(environment, nextEnvironment) || !bytes.Equal(compose, nextCompose) {
		t.Fatal("repeated network binding rewrote protected material")
	}
}

func TestCoreExternalNetworkResumesInterruptedReferenceWithoutAdoptingForeignDefinition(t *testing.T) {
	files, err := EnsureFilesForProjectAndResources(filepath.Join(t.TempDir(), "runtime"), "bh-owner-shared", "baseharbor-owner", Ports{Postgres: 15432, OpenBao: 18200}, false)
	if err != nil {
		t.Fatal(err)
	}
	environment, _ := os.ReadFile(files.Env)
	if err := os.WriteFile(files.Env, append(environment, []byte(coreExternalDefaultNetworkKey+"=true\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	files, err = ExistingFilesForProject(filepath.Dir(files.Compose), files.Project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindControlPlaneToExistingDefaultNetwork(files); err != nil {
		t.Fatalf("exact pending reference could not resume: %v", err)
	}
	if err := os.WriteFile(files.Compose, []byte("services:\n  foreign:\n    image: foreign\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(files.Env)
	if _, err := BindControlPlaneToExistingDefaultNetwork(files); err == nil {
		t.Fatal("foreign definition was adopted by network binding")
	}
	after, _ := os.ReadFile(files.Env)
	if !bytes.Equal(before, after) {
		t.Fatal("foreign refusal changed credential owner")
	}
}

func TestCoreExternalNetworkRejectsUnprotectedAndAmbiguousOwners(t *testing.T) {
	for _, mode := range []string{"unprotected", "symlink", "duplicate-binding"} {
		t.Run(mode, func(t *testing.T) {
			files, err := EnsureFilesForProjectAndResources(filepath.Join(t.TempDir(), "runtime"), "bh-owner-shared", "baseharbor-owner", Ports{Postgres: 15432, OpenBao: 18200}, false)
			if err != nil {
				t.Fatal(err)
			}
			original, _ := os.ReadFile(files.Env)
			switch mode {
			case "unprotected":
				err = os.Chmod(files.Env, 0644)
			case "symlink":
				backing := files.Env + ".foreign"
				if err = os.Rename(files.Env, backing); err == nil {
					err = os.Symlink(backing, files.Env)
				}
			case "duplicate-binding":
				original = append(original, []byte(coreExternalDefaultNetworkKey+"=false\n"+coreExternalDefaultNetworkKey+"=true\n")...)
				err = os.WriteFile(files.Env, original, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := BindControlPlaneToExistingDefaultNetwork(files); err == nil {
				t.Fatal("unverifiable owner was accepted")
			}
			after, _ := os.ReadFile(files.Env)
			if !bytes.Equal(original, after) {
				t.Fatal("refusal rewrote protected data")
			}
		})
	}
}
