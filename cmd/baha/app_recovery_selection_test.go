package main

import (
	"encoding/json"
	"testing"
)

func TestRecoveryComposeModelAcceptsNormalizedAndShortVolumeSyntax(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      string
		wantSource string
		wantTarget string
		wantType   string
	}{
		{
			name:       "normalized object",
			input:      `{"services":{"api":{"volumes":[{"type":"volume","source":"data","target":"/data"}]}},"volumes":{"data":{}}}`,
			wantSource: "data",
			wantTarget: "/data",
			wantType:   "volume",
		},
		{
			name:       "compose short syntax",
			input:      `{"services":{"api":{"volumes":["data:/data"]}},"volumes":{"data":{}}}`,
			wantSource: "data",
			wantTarget: "/data",
		},
		{
			name:       "bind short syntax with options",
			input:      `{"services":{"api":{"volumes":["./cache:/cache:ro"]}}}`,
			wantSource: "./cache",
			wantTarget: "/cache",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var model recoveryComposeModel
			if err := json.Unmarshal([]byte(tc.input), &model); err != nil {
				t.Fatal(err)
			}
			mounts := model.Services["api"].Volumes
			if len(mounts) != 1 {
				t.Fatalf("mounts=%d want 1", len(mounts))
			}
			got := mounts[0]
			if got.Source != tc.wantSource || got.Target != tc.wantTarget || got.Type != tc.wantType {
				t.Fatalf("mount=%#v want source=%q target=%q type=%q", got, tc.wantSource, tc.wantTarget, tc.wantType)
			}
		})
	}
}
