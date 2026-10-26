package externalprovider

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalCredentialResolverRejectsRedirectedAndUnownedMaterial(t *testing.T) {
	for _, scenario := range []string{"symlink", "parent-symlink", "hardlink", "readable", "oversize", "authority", "query", "opaque", "invalid-json"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			original := filepath.Join(root, "original")
			data := []byte(`{"username":"consumer","password":"private-sentinel"}`)
			if err := os.WriteFile(original, data, 0600); err != nil {
				t.Fatal(err)
			}
			selected := original
			reference := ""
			switch scenario {
			case "symlink":
				selected = filepath.Join(root, "redirect")
				if err := os.Symlink(original, selected); err != nil {
					t.Fatal(err)
				}
			case "parent-symlink":
				dir := filepath.Join(root, "redirect")
				if err := os.Symlink(root, dir); err != nil {
					t.Fatal(err)
				}
				selected = filepath.Join(dir, "original")
			case "hardlink":
				selected = filepath.Join(root, "linked")
				if err := os.Link(original, selected); err != nil {
					t.Fatal(err)
				}
			case "readable":
				if err := os.Chmod(original, 0644); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				selected = filepath.Join(root, "oversize")
				if err := os.WriteFile(selected, bytes.Repeat([]byte("x"), maxMaterialBytes+1), 0600); err != nil {
					t.Fatal(err)
				}
			case "authority":
				reference = "file://private-sentinel" + original
			case "query":
				reference = "file://" + original + "?token=private-sentinel"
			case "opaque":
				reference = "file:private-sentinel"
			case "invalid-json":
				selected = filepath.Join(root, "invalid")
				if err := os.WriteFile(selected, []byte(`{"password":private-sentinel}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if reference == "" {
				reference = "file://" + selected
			}
			if _, err := ResolveCredentialReference(reference); err == nil || strings.Contains(err.Error(), "private-sentinel") {
				t.Fatal("unsafe credential source accepted or disclosed")
			}
			after, err := os.ReadFile(original)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("credential rejection changed original owner bytes")
			}
		})
	}
}

func TestExternalPublicTrustRejectsWritableCAWithoutRequiringSecrecy(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(ca, data, 0644); err != nil {
		t.Fatal(err)
	}
	if ok, err := appendCAReference(x509.NewCertPool(), ca); err != nil || !ok {
		t.Fatal("readable public CA was rejected", err)
	}
	for _, mode := range []os.FileMode{0620, 0602, 0666} {
		if err := os.Chmod(ca, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := appendCAReference(x509.NewCertPool(), ca); err == nil {
			t.Fatal("writable trust anchor accepted")
		}
	}
	after, _ := os.ReadFile(ca)
	if !bytes.Equal(data, after) {
		t.Fatal("trust rejection changed CA owner")
	}
}

func TestPublicTrustCannotCarryPrivateKeyOrUnrelatedText(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	for _, extra := range [][]byte{pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("private-sentinel")}), []byte("password=private-sentinel\n")} {
		bundle := append(append([]byte(nil), certificate...), extra...)
		if err := ValidatePublicTrustMaterial(bundle); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatal("private or unrelated material escaped through public trust")
		}
	}
	if err := ValidatePublicTrustMaterial(append(certificate, certificate...)); err != nil {
		t.Fatal("valid public certificate bundle was rejected", err)
	}
}
