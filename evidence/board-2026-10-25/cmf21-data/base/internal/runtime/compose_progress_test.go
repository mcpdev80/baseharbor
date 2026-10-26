package runtime

import "testing"

func TestClassifyComposeProgressLine(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"Image ghcr.io/example/api:latest Pulling", "pulling image ghcr.io/example/api:latest"},
		{"Image postgres:18 Pulled", "image ready postgres:18"},
		{"Service api Building", "building image for api"},
		{"Container mailflow-api-1 Creating", "creating container mailflow-api-1"},
		{"Container mailflow-api-1 Starting", "starting container mailflow-api-1"},
		{"Container mailflow-api-1 Waiting", "waiting for container mailflow-api-1"},
		{"Container mailflow-api-1 Healthy", "container healthy mailflow-api-1"},
		{"#5 [internal] load build definition from Dockerfile", "loading build definition"},
		{"#7 [internal] load metadata for docker.io/library/node:22", "resolving build image metadata"},
		{"some arbitrary runtime line SECRET=value", ""},
	}
	for _, tt := range tests {
		if got := classifyComposeProgressLine(tt.line); got != tt.want {
			t.Fatalf("classifyComposeProgressLine(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestComposeProgressCaptureSuppressesDuplicateAndRawLines(t *testing.T) {
	var got []string
	capture := newComposeProgressCapture(func(detail string) { got = append(got, detail) })
	_, _ = capture.Write([]byte("Container demo-api-1 Starting\n"))
	_, _ = capture.Write([]byte("Container demo-api-1 Starting\n"))
	_, _ = capture.Write([]byte("SECRET=value\n"))
	_, _ = capture.Write([]byte("Container demo-api-1 Started\n"))
	capture.Flush()
	if len(got) != 2 {
		t.Fatalf("progress events = %#v, want 2 classified state changes", got)
	}
	if got[0] != "starting container demo-api-1" || got[1] != "container started demo-api-1" {
		t.Fatalf("progress events = %#v", got)
	}
}
