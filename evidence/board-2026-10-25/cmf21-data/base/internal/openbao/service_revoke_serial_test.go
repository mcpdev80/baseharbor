package openbao

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestServiceRevocationCanonicalizesX509SerialBeforeIssuerLookup(t *testing.T) {
	for _, serial := range []string{"10203", "01:02:03", "01-02-03", "00010203", "01:02:03"} {
		t.Run(serial, func(t *testing.T) {
			fake := &servicePKIFake{}
			if err := RevokeServiceCertificate(context.Background(), fake, servicePKITestFiles(t), serial); err != nil {
				t.Fatal(err)
			}
			found := false
			for i, args := range fake.args {
				if !strings.Contains(args, "baseharbor-pki/revoke") {
					continue
				}
				parts := strings.SplitN(fake.inputs[i], "\n", 2)
				var payload map[string]string
				if len(parts) != 2 || json.Unmarshal([]byte(parts[1]), &payload) != nil || payload["serial_number"] != "01:02:03" {
					t.Fatal("X509 serial did not use canonical issuer storage lookup", serial)
				}
				found = true
			}
			if !found {
				t.Fatal("revocation not sent to issuer")
			}
		})
	}
}

func TestServiceRevocationRejectsInvalidSerialBeforeAuthentication(t *testing.T) {
	for _, serial := range []string{"", "00", "xyz", "-01", "01:", "01::02", "01-:02", "0x123", "01\n02", strings.Repeat("a", 129)} {
		fake := &servicePKIFake{}
		if err := RevokeServiceCertificate(context.Background(), fake, servicePKITestFiles(t), serial); err == nil || len(fake.args) != 0 {
			t.Fatal("invalid serial reached managed issuer", serial)
		}
	}
}
