package identityprovider

import (
	"os"
	"testing"
)

func TestExternalPublicTrustRejectsUncontrolledWriters(t *testing.T) {
	for _, mode := range []os.FileMode{0620, 0602, 0666} {
		t.Run(mode.String(), func(t *testing.T) {
			m, files, server, _ := externalCredentialFixture(t)
			if err := os.Chmod(os.Getenv(ExternalCAFileEnv), mode); err != nil {
				t.Fatal(err)
			}
			if _, err := NewExternalDriver(m, files, server.URL); err == nil {
				t.Fatal("external trust source with uncontrolled group/other writers was accepted")
			}
		})
	}
}
