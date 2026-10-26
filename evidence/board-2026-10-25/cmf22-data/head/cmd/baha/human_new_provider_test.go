package main

import (
	"testing"
)

func TestProviderWizardConsentRecognizesExplicitYesOnly(t *testing.T) {
	for _, answer := range []string{"yes", "Y", "ja", "J"} {
		if !isAffirmative(answer) {
			t.Fatalf("explicit confirmation %q rejected", answer)
		}
	}
	for _, answer := range []string{"", "no", "cancel", "maybe", "1"} {
		if isAffirmative(answer) {
			t.Fatalf("unsafe implicit provider creation confirmation %q", answer)
		}
	}
}
