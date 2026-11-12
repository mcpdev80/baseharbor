package identityprovider

import "testing"

func TestValidateManagedOperatorUser(t *testing.T) {
	for _, tc := range []struct {
		name, login, display, email string
		wantErr                     bool
	}{
		{"valid", "marcel", "Marcel Pfingstgraef", "dev@pfingstgraef.eu", false},
		{"unsafe login", "../marcel", "Marcel Pfingstgraef", "dev@pfingstgraef.eu", true},
		{"display name required", "marcel", " ", "dev@pfingstgraef.eu", true},
		{"email address required", "marcel", "Marcel Pfingstgraef", "not-an-email", true},
		{"display form rejected", "marcel", "Marcel Pfingstgraef", "Marcel <dev@pfingstgraef.eu>", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateManagedOperatorUser(tc.login, tc.display, tc.email)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateManagedOperatorUser() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
