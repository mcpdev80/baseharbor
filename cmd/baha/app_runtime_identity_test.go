package main

import "testing"

func TestParseRuntimeIdentityMutationArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantName  string
		wantYes   bool
		wantError bool
	}{
		{name: "repository implicit", args: nil},
		{name: "explicit app", args: []string{"mailflow"}, wantName: "mailflow"},
		{name: "confirmed repository", args: []string{"--yes"}, wantYes: true},
		{name: "confirmed explicit", args: []string{"mailflow", "--yes"}, wantName: "mailflow", wantYes: true},
		{name: "confirmed first", args: []string{"--yes", "mailflow"}, wantName: "mailflow", wantYes: true},
		{name: "unknown flag", args: []string{"--force"}, wantError: true},
		{name: "two names", args: []string{"mailflow", "awc"}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, confirmed, err := parseRuntimeIdentityMutationArgs(tt.args)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name != tt.wantName || confirmed != tt.wantYes {
				t.Fatalf("got name=%q confirmed=%v, want name=%q confirmed=%v", name, confirmed, tt.wantName, tt.wantYes)
			}
		})
	}
}
