package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestParseSelfUpdateOptionsDefaultsToStable(t *testing.T) {
	opts, err := parseSelfUpdateOptions([]string{"--check"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Check || opts.Channel != "stable" || opts.Version != "" {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if _, err := parseSelfUpdateOptions([]string{"--check", "--version", "0.3.0", "--channel", "rc"}); err == nil {
		t.Fatal("expected exact version and channel to be mutually exclusive")
	}
}

func TestResolveSelfUpdateReleaseStableUsesLatestEndpoint(t *testing.T) {
	server := newReleaseTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeReleaseTestJSON(t, w, testRelease("v0.3.0", false))
	})
	defer server.Close()
	withReleaseTestServer(t, server)

	release, err := resolveSelfUpdateRelease(context.Background(), selfUpdateOptions{Check: true, Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v0.3.0" || release.Prerelease {
		t.Fatalf("unexpected stable release: %#v", release)
	}
}

func TestResolveSelfUpdateReleaseRCRequiresExplicitChannel(t *testing.T) {
	server := newReleaseTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases" || r.URL.Query().Get("per_page") != "30" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		writeReleaseTestJSON(t, w, []baseHarborRelease{
			testRelease("v0.3.0", false),
			testRelease("v0.4.0-rc.2", true),
			testRelease("v0.4.0-rc.1", true),
		})
	})
	defer server.Close()
	withReleaseTestServer(t, server)

	release, err := resolveSelfUpdateRelease(context.Background(), selfUpdateOptions{Check: true, Channel: "rc"})
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v0.4.0-rc.2" || !release.Prerelease {
		t.Fatalf("unexpected rc release: %#v", release)
	}
}

func TestResolveSelfUpdateReleaseExactVersion(t *testing.T) {
	server := newReleaseTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/tags/v0.3.0-rc.1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeReleaseTestJSON(t, w, testRelease("v0.3.0-rc.1", true))
	})
	defer server.Close()
	withReleaseTestServer(t, server)

	release, err := resolveSelfUpdateRelease(context.Background(), selfUpdateOptions{Check: true, Channel: "stable", Version: "0.3.0-rc.1"})
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "v0.3.0-rc.1" || !release.Prerelease {
		t.Fatalf("unexpected exact release: %#v", release)
	}
}

func TestInspectSelfUpdateReportsAssetAndRelation(t *testing.T) {
	server := newReleaseTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeReleaseTestJSON(t, w, testRelease("v0.3.0", false))
	})
	defer server.Close()
	withReleaseTestServer(t, server)

	check, err := inspectSelfUpdate(context.Background(), "0.2.0", selfUpdateOptions{Check: true, Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	wantAsset := fmt.Sprintf("baseharbor_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if check.Target != "0.3.0" || check.Relation != "update-available" || check.AssetName != wantAsset || check.ChecksumsURL == "" {
		t.Fatalf("unexpected self-update check: %#v", check)
	}
}

func TestCompareReleaseVersions(t *testing.T) {
	cases := []struct {
		left  string
		right string
		want  int
	}{
		{"0.2.0", "0.3.0", -1},
		{"0.3.0", "0.3.0", 0},
		{"0.3.1", "0.3.0", 1},
		{"0.3.0-rc.1", "0.3.0", -1},
		{"0.3.0-rc.1", "0.3.0-rc.2", -1},
	}
	for _, tc := range cases {
		got, ok := compareReleaseVersions(tc.left, tc.right)
		if !ok || got != tc.want {
			t.Fatalf("compareReleaseVersions(%q, %q) = %d, %v; want %d, true", tc.left, tc.right, got, ok, tc.want)
		}
	}
}

func TestSelfUpdateCommandIsDiscoverableAndMutationDisabled(t *testing.T) {
	root := rootCommand()
	var updateFound bool
	for _, child := range root.Children {
		if child.Name == "update" {
			updateFound = true
			if child.Usage != "baha update --check [--channel stable|rc | --version VERSION]" {
				t.Fatalf("unexpected update usage: %s", child.Usage)
			}
			var out strings.Builder
			if err := child.Run(context.Background(), nil, &out, &out); err == nil || !strings.Contains(err.Error(), "mutation is not enabled yet") {
				t.Fatalf("expected disabled mutation error, got %v", err)
			}
		}
	}
	if !updateFound {
		t.Fatal("root update command not discoverable")
	}
}

func newReleaseTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func withReleaseTestServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	oldBase := releaseAPIBase
	oldClient := releaseHTTPClient
	releaseAPIBase = server.URL
	releaseHTTPClient = server.Client()
	t.Cleanup(func() {
		releaseAPIBase = oldBase
		releaseHTTPClient = oldClient
	})
}

func writeReleaseTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func testRelease(tag string, prerelease bool) baseHarborRelease {
	assetName := fmt.Sprintf("baseharbor_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	return baseHarborRelease{
		TagName:    tag,
		Name:       "BaseHarbor " + tag,
		Prerelease: prerelease,
		HTMLURL:    "https://example.invalid/releases/" + tag,
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Digest             string `json:"digest"`
		}{
			{Name: assetName, BrowserDownloadURL: "https://example.invalid/" + assetName, Digest: "sha256:abc"},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.invalid/checksums.txt", Digest: "sha256:def"},
		},
	}
}
