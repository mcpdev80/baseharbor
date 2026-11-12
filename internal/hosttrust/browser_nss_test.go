package hosttrust

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func browserTestDB(t *testing.T) string {
	t.Helper()
	db := t.TempDir()
	if err := os.WriteFile(filepath.Join(db, "cert9.db"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return db
}

func fakeNSS(t *testing.T) *map[string][]byte {
	t.Helper()
	saved := runNSSCommand
	t.Cleanup(func() { runNSSCommand = saved })
	certificates := map[string][]byte{}
	runNSSCommand = func(_ context.Context, args ...string) ([]byte, error) {
		option := func(key string) string {
			for i, v := range args {
				if v == key && i+1 < len(args) {
					return args[i+1]
				}
			}
			return ""
		}
		nickname := option("-n")
		switch args[0] {
		case "-L":
			if nickname == "" {
				var names []string
				for name := range certificates {
					names = append(names, name)
				}
				return []byte(strings.Join(names, "\n")), nil
			}
			data, ok := certificates[nickname]
			if !ok {
				return nil, errors.New("not found")
			}
			return data, nil
		case "-A":
			if option("-t") != "C,," {
				t.Fatal("browser trust must be limited to TLS CA trust")
			}
			data, err := os.ReadFile(option("-i"))
			if err == nil {
				certificates[nickname] = data
			}
			return nil, err
		case "-D":
			delete(certificates, nickname)
			return nil, nil
		case "-V":
			if _, ok := certificates[nickname]; !ok {
				return nil, errors.New("not trusted")
			}
			return nil, nil
		}
		t.Fatalf("unexpected certutil command: %v", args)
		return nil, nil
	}
	return &certificates
}

func TestBrowserCAOwnershipIdempotenceAndRemoval(t *testing.T) {
	certificates := fakeNSS(t)
	state := t.TempDir()
	db := browserTestDB(t)
	ca := testCA(t, "browser CA")
	for i := 0; i < 2; i++ {
		if err := installBrowserCA(context.Background(), state, ca, db); err != nil {
			t.Fatal(err)
		}
	}
	records, err := StateRecords(state)
	if err != nil || len(records) != 1 {
		t.Fatalf("ownership: %v %v", records, err)
	}
	if records[0].Backend != nssBackendName {
		t.Fatal("browser ownership backend missing")
	}
	n, err := RemoveOwned(context.Background(), state)
	if err != nil || n != 1 || len(*certificates) != 0 {
		t.Fatalf("removal: %d %v", n, err)
	}
}

func TestBrowserCAPreservesOperatorAndChangedCertificates(t *testing.T) {
	certificates := fakeNSS(t)
	state := t.TempDir()
	db := browserTestDB(t)
	ca := testCA(t, "owned browser CA")
	_, fp, _ := ParseCA(ca)
	nickname := "BaseHarbor " + fp
	(*certificates)[nickname] = ca
	if err := installBrowserCA(context.Background(), state, ca, db); err != nil {
		t.Fatal(err)
	}
	records, _ := StateRecords(state)
	if len(records) != 0 {
		t.Fatal("claimed external browser certificate")
	}
	delete(*certificates, nickname)
	if err := installBrowserCA(context.Background(), state, ca, db); err != nil {
		t.Fatal(err)
	}
	foreign := testCA(t, "foreign browser CA")
	(*certificates)[nickname] = foreign
	if err := installBrowserCA(context.Background(), state, ca, db); err == nil {
		t.Fatal("conflicting certificate overwritten")
	}
	result, err := RemoveOwnedDetailed(context.Background(), state)
	if err == nil || len(result.Preserved) != 1 || string((*certificates)[nickname]) != string(foreign) {
		t.Fatal("changed browser certificate was not preserved")
	}
}

func TestBrowserCARollsBackWhenOwnershipCannotPersist(t *testing.T) {
	certificates := fakeNSS(t)
	runner := runNSSCommand
	state := t.TempDir()
	db := browserTestDB(t)
	ca := testCA(t, "rollback browser CA")
	runNSSCommand = func(ctx context.Context, args ...string) ([]byte, error) {
		output, err := runner(ctx, args...)
		if args[0] == "-A" && err == nil {
			if err := os.Mkdir(filepath.Join(state, "host-trust.json.tmp"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		return output, err
	}
	if err := installBrowserCA(context.Background(), state, ca, db); err == nil {
		t.Fatal("ownership persistence failure ignored")
	}
	if len(*certificates) != 0 {
		t.Fatal("untracked browser trust left installed")
	}
}

func TestBrowserDiscoveryRejectsSymlinkedProfilesAndFindsSupportedStores(t *testing.T) {
	home := t.TempDir()
	paths := []string{".pki/nssdb", ".local/share/pki/nssdb", ".mozilla/firefox/test.default", "snap/firefox/common/.mozilla/firefox/snap.default", ".var/app/org.mozilla.firefox/.mozilla/firefox/flatpak.default"}
	for _, p := range paths {
		db := filepath.Join(home, p)
		if err := os.MkdirAll(db, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(db, "cert9.db"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := browserNSSDatabases(home); len(got) != len(paths) {
		t.Fatalf("missed stores: %v", got)
	}
	link := filepath.Join(home, ".mozilla/firefox/symlink.default")
	if err := os.Symlink(browserTestDB(t), link); err != nil {
		t.Fatal(err)
	}
	if got := browserNSSDatabases(home); len(got) != len(paths) {
		t.Fatal("followed browser profile symlink")
	}
}

func TestBrowserNSSActualCertutilRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("certutil"); err != nil {
		t.Skip("NSS certutil is unavailable")
	}
	db := t.TempDir()
	state := t.TempDir()
	if output, err := exec.Command("certutil", "-N", "--empty-password", "-d", "sql:"+db).CombinedOutput(); err != nil {
		t.Fatalf("create isolated NSS database: %v %s", err, output)
	}
	ca := testCA(t, "actual NSS round trip")
	if err := installBrowserCA(context.Background(), state, ca, db); err != nil {
		t.Fatal(err)
	}
	records, err := InspectRecorded(state)
	if err != nil || len(records) != 1 || !records[0].Trusted {
		t.Fatalf("browser trust verification: %v %v", records, err)
	}
	if n, err := RemoveOwned(context.Background(), state); err != nil || n != 1 {
		t.Fatalf("browser trust removal: %d %v", n, err)
	}
}

func TestRemoveOwnedFingerprintPreservesOtherCAAndManualChanges(t *testing.T) {
	certificates := fakeNSS(t)
	state := t.TempDir()
	db := browserTestDB(t)
	first, second := testCA(t, "first target"), testCA(t, "second target")
	for _, ca := range [][]byte{first, second} {
		if err := installBrowserCA(context.Background(), state, ca, db); err != nil {
			t.Fatal(err)
		}
	}
	_, fp, _ := ParseCA(first)
	removed, err := RemoveOwnedFingerprint(context.Background(), state, fp)
	if err != nil || len(removed.Removed) != 1 || len(*certificates) != 1 {
		t.Fatalf("scoped CA removal: %v %v", removed, err)
	}
	records, err := StateRecords(state)
	if err != nil || len(records) != 1 || records[0].Fingerprint == fp {
		t.Fatalf("other CA not retained: %v", err)
	}
	_, secondFP, _ := ParseCA(second)
	(*certificates)["BaseHarbor "+secondFP] = testCA(t, "operator replacement")
	if _, err := RemoveOwnedFingerprint(context.Background(), state, secondFP); err == nil {
		t.Fatal("manually changed CA was not preserved")
	}
	records, _ = StateRecords(state)
	if len(records) != 1 {
		t.Fatal("ownership evidence lost after preservation")
	}
}
