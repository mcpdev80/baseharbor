package application

import "testing"

func TestApplicationComposeProjectIsSeparateFromResourceIdentity(t *testing.T) {
	m := New("demo", "dev", true, true, false)
	store := Store{Namespace: "ci"}

	if got, want := RuntimeComposeProjectNameForStore(store, m), "bh-ci-demo-dev"; got != want {
		t.Fatalf("compose project = %q, want %q", got, want)
	}
	if got, want := RuntimeProjectNameForStore(store, m), "baseharbor-ci-demo-dev"; got != want {
		t.Fatalf("resource project = %q, want %q", got, want)
	}
	files := RuntimeFilesFor(store, m)
	if files.Project != "bh-ci-demo-dev" {
		t.Fatalf("files.Project = %q", files.Project)
	}
	if files.ResourceProject != "baseharbor-ci-demo-dev" {
		t.Fatalf("files.ResourceProject = %q", files.ResourceProject)
	}
}
