package main

import "testing"

func TestLooksLikeRemoteGitSource(t *testing.T) {
	for _, source := range []string{
		"https://github.com/example/app.git",
		"ssh://git@example.com/example/app.git",
		"git@example.com:example/app.git",
	} {
		if !looksLikeRemoteGitSource(source) {
			t.Fatalf("%q should be remote", source)
		}
	}
	if looksLikeRemoteGitSource("./local") {
		t.Fatal("local path classified as remote")
	}
}

func TestValidateRemoteGitSourceRejectsEmbeddedCredentials(t *testing.T) {
	if err := validateRemoteGitSource("https://token@example.com/repo.git"); err == nil {
		t.Fatal("expected embedded userinfo to be rejected")
	}
	if err := validateRemoteGitSource("https://example.com/repo.git"); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteGitSource("git@example.com:example/repo.git"); err != nil {
		t.Fatal(err)
	}
}
