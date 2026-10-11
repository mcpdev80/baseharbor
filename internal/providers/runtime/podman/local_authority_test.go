package podman

import "testing"

func TestLocalAuthorityRequiresActualCompleteNativeInfo(t *testing.T) {
	for _, input := range []struct { info string; allowed bool }{
		{`{"host":{"os":"linux","serviceIsRemote":false,"security":{"rootless":true}}}`, true},
		{`{"host":{"os":"linux","serviceIsRemote":true,"security":{"rootless":true}}}`, false},
		{`{"host":{"os":"linux","security":{"rootless":true}}}`, false},
		{`{"host":{"os":"linux","serviceIsRemote":false,"security":{}}}`, false},
		{`{"host":{"os":"linux","serviceIsRemote":false,"security":{"rootless":false}}}`, false},
		{`{"host":{"os":"darwin","serviceIsRemote":false,"security":{"rootless":true}}}`, false},
		{`{}`, false},
		{`invalid`, false},
	} {
		if (verifyLocalAuthorityInfo([]byte(input.info)) == nil) != input.allowed { t.Fatalf("authority did not match native observation: %s", input.info) }
	}
}
