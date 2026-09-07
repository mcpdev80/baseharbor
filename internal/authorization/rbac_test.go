package authorization

import "testing"

func TestViewerIsReadOnly(t *testing.T) {
	service := NewService()
	if !service.Allowed([]string{RoleViewer}, PermRead) {
		t.Fatal("viewer should be allowed to read")
	}
	if service.Allowed([]string{RoleViewer}, PermUpdate) {
		t.Fatal("viewer must not be allowed to update")
	}
}

func TestEditorHasCRUD(t *testing.T) {
	service := NewService()
	for _, permission := range []Permission{PermRead, PermCreate, PermUpdate, PermDelete} {
		if !service.Allowed([]string{RoleEditor}, permission) {
			t.Fatalf("editor should allow %q", permission)
		}
	}
}

func TestUnknownRoleAndPermissionFailClosed(t *testing.T) {
	service := NewService()
	if service.Allowed([]string{"owner"}, PermRead) {
		t.Fatal("unknown role must not grant access")
	}
	if service.Allowed([]string{RoleEditor}, Permission("execute")) {
		t.Fatal("unknown permission must not grant access")
	}
	if service.Allowed(nil, PermRead) {
		t.Fatal("empty roles must not grant access")
	}
}

func TestRoleUnion(t *testing.T) {
	service := NewService()
	if !service.Allowed([]string{"unknown", RoleViewer}, PermRead) {
		t.Fatal("known allowed role should grant access even when unknown roles are present")
	}
}
