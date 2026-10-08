package providerupgrade

import (
	"errors"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want Version
	}{
		{"2.7.0", Version{2, 7, 0}},
		{"v26.8.0", Version{26, 8, 0}},
		{"26.8.0.Final", Version{}},
	}
	for _, tt := range tests {
		got, err := ParseVersion(tt.in)
		if tt.want == (Version{}) {
			if err == nil {
				t.Fatalf("%q unexpectedly parsed as %#v", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Fatalf("%q => %#v, %v", tt.in, got, err)
		}
	}
}

func TestParseVersionAcceptsPrereleaseSuffix(t *testing.T) {
	got, err := ParseVersion("26.8.0-rc1")
	if err != nil {
		t.Fatal(err)
	}
	if got != (Version{26, 8, 0}) {
		t.Fatalf("got %#v", got)
	}
}

func TestVersionCompare(t *testing.T) {
	if (Version{26, 7, 5}).Compare(Version{26, 7, 6}) >= 0 {
		t.Fatal("patch comparison broken")
	}
	if (Version{26, 8, 0}).Compare(Version{26, 7, 99}) <= 0 {
		t.Fatal("minor comparison broken")
	}
	if !(Version{2, 7, 0}).SameMinor(Version{2, 7, 9}) {
		t.Fatal("same minor expected")
	}
}

func TestRequestValidateRequiresImmutableDigest(t *testing.T) {
	req := Request{CurrentVersion: "1.0.0", TargetVersion: "1.0.1", TargetImage: "example/provider:1.0.1", TargetDigest: "latest"}
	if err := req.Validate(); err == nil {
		t.Fatal("mutable digest accepted")
	}
}

func TestBackupValidateRejectsProviderAndVersionMismatch(t *testing.T) {
	b := BackupRef{Provider: ProviderOpenBao, ID: "b1", Version: "2.7.0", Verified: true}
	if err := b.Validate(ProviderKeycloak, "2.7.0"); err == nil {
		t.Fatal("provider mismatch accepted")
	}
	if err := b.Validate(ProviderOpenBao, "2.6.0"); err == nil {
		t.Fatal("version mismatch accepted")
	}
	b.Verified = false
	if err := b.Validate(ProviderOpenBao, "2.7.0"); err == nil {
		t.Fatal("unverified backup accepted")
	}
}

func TestErrorClassification(t *testing.T) {
	base := errors.New("failed")
	err := Wrap(ErrorUnsupportedPath, "compatibility", base)
	if ClassOf(err) != ErrorUnsupportedPath {
		t.Fatalf("class=%q", ClassOf(err))
	}
	if !errors.Is(err, base) {
		t.Fatal("wrapped error does not unwrap")
	}
	if Wrap(ErrorApplyFailed, "apply", err) != err {
		t.Fatal("classified error should not be reclassified")
	}
}
