package runtimeexplorer

import "testing"

func TestResourceOwnershipAndOperationContract(t *testing.T) {
	resource := Resource{
		ContractVersion: ContractVersion,
		Ref: ResourceRef{
			Provider:   "docker",
			Target:     "local",
			Kind:       KindContainer,
			ResourceID: "container-123",
		},
		Ownership: OwnershipManaged,
		Relationship: Relationship{
			ApplicationID: "app-123",
			DeploymentID:  "deployment-123",
			Component:     "api",
		},
		Reconciliation: ReconciliationHint{
			PreferredOperation:            "application.restart-component",
			DirectMutationMayBeReconciled: true,
		},
	}
	if err := resource.Validate(); err != nil {
		t.Fatalf("valid managed resource rejected: %v", err)
	}

	resource.Relationship = Relationship{}
	if err := resource.Validate(); err == nil {
		t.Fatal("managed resource without BaseHarbor relationship unexpectedly accepted")
	}

	exec := OperationRequest{
		Resource:  ResourceRef{Provider: "docker", Target: "local", Kind: KindContainer, ResourceID: "container-123"},
		Operation: OperationExec,
		Command:   []string{"/bin/sh", "-lc", "echo ok"},
	}
	if err := exec.Validate(); err != nil {
		t.Fatalf("valid exec operation rejected: %v", err)
	}
	exec.Command = nil
	if err := exec.Validate(); err == nil {
		t.Fatal("exec without explicit command unexpectedly accepted")
	}
}

func TestFutureResourceKindsRemainAdditive(t *testing.T) {
	resource := Resource{
		ContractVersion: ContractVersion,
		Ref: ResourceRef{
			Provider:   "kubernetes",
			Target:     "cluster-a",
			Kind:       ResourceKind("pod"),
			ResourceID: "namespace/default/pod/api-123",
		},
		Ownership: OwnershipPlatform,
	}
	if err := resource.Validate(); err != nil {
		t.Fatalf("future-compatible resource kind rejected: %v", err)
	}
}
