package runtime

import "testing"

func TestContainerIdentityKeepsNativeNamesAndRequiresExactLabels(t *testing.T) {
	resource := ProjectResource{Kind: "container", Name: "owned-keycloak-db-0-1"}
	for _, tc := range []struct {
		kind      ProviderKind
		container RuntimeContainer
		want      bool
	}{
		{ProviderDocker, RuntimeContainer{Name: resource.Name, Project: "owned", Service: "keycloak-db-0"}, true},
		{ProviderPodman, RuntimeContainer{Name: "owned-keycloak-db-0", Project: "owned", Service: "keycloak-db-0"}, true},
		{ProviderDocker, RuntimeContainer{Name: "owned-keycloak-db-0", Project: "owned", Service: "keycloak-db-0"}, false},
		{ProviderPodman, RuntimeContainer{Name: "owned-keycloak-db-0-2", Project: "owned", Service: "keycloak-db-0"}, false},
		{ProviderPodman, RuntimeContainer{Name: resource.Name, Project: "foreign", Service: "keycloak-db-0"}, false},
		{ProviderPodman, RuntimeContainer{Name: resource.Name, Project: "owned", Service: "other"}, false},
	} {
		if got := ContainerMatchesProjectResource(tc.kind, tc.container, "owned", resource); got != tc.want {
			t.Fatalf("native container identity %#v: got %v want %v", tc, got, tc.want)
		}
	}
}
