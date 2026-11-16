package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// ObjectStorageSecureBinding describes one application-owned S3 bucket without
// exposing concrete credentials or provider-specific identity/policy objects.
func ObjectStorageSecureBinding(m Manifest, bucket string) capability.SecureBinding {
	base := fmt.Sprintf("baseharbor://applications/%s/%s/object-storage/%s", m.Name, m.Environment, bucket)
	return capability.SecureBinding{
		Identity: &capability.WorkloadIdentityBinding{
			Subject:   fmt.Sprintf("spiffe://baseharbor/apps/%s/%s", m.Name, m.Environment),
			Reference: base + "/identity",
		},
		Credentials: []capability.CredentialReference{
			{Name: "access-key-id", Reference: base + "/credentials/access-key-id"},
			{Name: "secret-access-key", Reference: base + "/credentials/secret-access-key"},
		},
		Authorization: []capability.AuthorizationMetadata{{
			Audience: "object-storage.s3",
			Scopes:   []string{"s3.read", "s3.write", "s3.list"},
		}},
		Lifecycle: capability.SecurityLifecycleSupport{
			Rotatable: true,
			Revocable: true,
		},
		Diagnostics: []capability.Diagnostic{{
			Code:     "s3-bucket-least-privilege",
			Severity: capability.SeverityInfo,
			Message:  "object-storage credentials are scoped to one logical bucket",
		}},
	}
}
