package main

import (
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestEtcdRecoveryUserPreservesPrivateHostBindOwnership(t *testing.T) {
	for _, tc := range []struct {
		kind            bhruntime.ProviderKind
		rootless        bool
		user, namespace string
	}{
		{bhruntime.ProviderDocker, true, "0:0", ""},
		{bhruntime.ProviderPodman, true, "1001:1002", "keep-id"},
		{bhruntime.ProviderDocker, false, "1001:1002", ""},
	} {
		identity, err := etcdRecoveryUser(tc.kind, 1001, 1002, tc.rootless)
		if err != nil || identity.User != tc.user || identity.UserNS != tc.namespace {
			t.Fatalf("recovery identity for %s rootless=%v: %+v, %v", tc.kind, tc.rootless, identity, err)
		}
	}
	if _, err := etcdRecoveryUser(bhruntime.ProviderDocker, 0, 0, true); err == nil {
		t.Fatal("unverified root host actor admitted as rootless recovery")
	}
	if _, err := etcdRecoveryUser(bhruntime.ProviderKubernetes, 1001, 1001, true); err == nil {
		t.Fatal("unsupported namespace mapping admitted")
	}
}
