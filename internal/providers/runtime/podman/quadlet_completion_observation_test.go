package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuadletCompletionRequiresCurrentLoadedCanonicalUnit(t *testing.T) {
	dir, runtimeDir, unit := "/home/operator/.config/containers/systemd", "/run/user/1000", "owned-init.service"
	valid := "SourcePath=" + dir + "/owned-init.container\nFragmentPath=" + runtimeDir + "/systemd/generator/" + unit + "\nDropInPaths=\nNeedDaemonReload=no\n"
	if !quadletCompletionUnitOwned(valid, dir, runtimeDir, unit) {
		t.Fatal("canonical loaded unit rejected")
	}
	for _, raw := range []string{
		strings.Replace(valid, "SourcePath="+dir, "SourcePath=/foreign", 1),
		strings.Replace(valid, "/systemd/generator/", "/systemd/user/", 1),
		strings.Replace(valid, "DropInPaths=", "DropInPaths=/foreign/override.conf", 1),
		strings.Replace(valid, "NeedDaemonReload=no", "NeedDaemonReload=yes", 1),
		strings.Replace(valid, "DropInPaths=\n", "", 1),
		valid + "NeedDaemonReload=no\n",
		valid + "unstructured\n",
	} {
		if quadletCompletionUnitOwned(raw, dir, runtimeDir, unit) {
			t.Fatal("ambiguous/stale/foreign loaded completion authority accepted")
		}
	}
}

func TestQuadletCompletionRejectsUnownedOrLinkedInstalledFiles(t *testing.T) {
	for _, mutation := range []string{"none", "missing", "symlink", "hardlink", "writable", "linked-parent", "foreign-owner"} {
		t.Run(mutation, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "units")
			q := emptyQuadletProject("owned")
			q.Files["owned-init.container"] = "canonical unit\n"
			q.Files["owned-init.env"] = "protected material\n"
			if err := WriteQuadletProject(dir, q); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "owned-init.container")
			switch mutation {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+"-foreign"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+"-foreign", path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+"-foreign"); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			case "linked-parent":
				link := dir + "-foreign"
				if err := os.Symlink(dir, link); err != nil {
					t.Fatal(err)
				}
				dir = link
			case "foreign-owner":
				if os.Getuid() != 0 {
					t.Skip("foreign UID fixture requires root")
				}
				if err := os.Chown(path, 65534, -1); err != nil {
					t.Skip("foreign UID unavailable")
				}
			}
			if quadletCompletionFilesOwned(dir, q) != (mutation == "none") {
				t.Fatal("installed completion owner classification differs")
			}
		})
	}
}
