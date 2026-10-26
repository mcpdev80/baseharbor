package devaccess

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalHostReadsNeverInitializeDomainState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	if host, err := SharedHost("isolated", "identity"); err != nil || host != "auth.baha.localhost" {
		t.Fatalf("default shared host: %s %v", host, err)
	}
	if host, err := ApplicationHost("isolated", "demo", "api"); err != nil || host != "demo.baha.localhost" {
		t.Fatalf("default application host: %s %v", host, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("host inspection initialized Target/domain state")
	}
	path, err := domainPath("isolated")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "foreign"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := SharedHost("isolated", "identity"); err == nil {
		t.Fatal("damaged domain was replaced with a default")
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("domain inspection altered unverified state")
	}
}

func TestCanonicalDevelopmentHostRules(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if got, err := ConfigureDomain("local", DefaultDomain); err != nil || got != "baha.localhost" {
		t.Fatalf("ConfigureDomain() = %q, %v", got, err)
	}
	appHost, err := ApplicationHost("local", "Demo App", "pgAdmin")
	if err != nil {
		t.Fatal(err)
	}
	if appHost != "demo-app-pgadmin.baha.localhost" {
		t.Fatalf("ApplicationHost() = %q", appHost)
	}
	sharedHost, err := SharedHost("local", "OpenBao")
	if err != nil {
		t.Fatal(err)
	}
	if sharedHost != "secrets.baha.localhost" {
		t.Fatalf("SharedHost() = %q", sharedHost)
	}
	if got := ExposureService("frontend", 1); got != "api" {
		t.Fatalf("single exposure service = %q", got)
	}
	if got := ExposureService("frontend", 2); got != "frontend" {
		t.Fatalf("multi exposure service = %q", got)
	}
}

func TestDevelopmentDomainRejectsNonDNSInput(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, value := range []string{"localhost", "https://dev.local", "bad domain.localhost", "-bad.localhost"} {
		if _, err := ConfigureDomain("local", value); err == nil {
			t.Fatalf("ConfigureDomain(%q) unexpectedly succeeded", value)
		}
	}
}

func TestDevelopmentAliasesAreDNSLabelSafe(t *testing.T) {
	longOwner := "bh-demo-podman-native-external-alias-shared"
	got := ProviderAlias(longOwner, "identity-admin")
	if len(got) > 63 {
		t.Fatalf("ProviderAlias() length = %d, want <= 63: %q", len(got), got)
	}
	if !domainLabel.MatchString(got) {
		t.Fatalf("ProviderAlias() is not a valid DNS label: %q", got)
	}
	if got != ProviderAlias(longOwner, "identity-admin") {
		t.Fatalf("ProviderAlias() must be deterministic")
	}
	if got == ProviderAlias(longOwner+"-other", "identity-admin") {
		t.Fatalf("ProviderAlias() must retain collision resistance for long owners")
	}
}
