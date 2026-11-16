package application

import (
	"fmt"
	"strings"
)

type ManagementAuthenticationClass string

const (
	ManagementAuthNativeOIDC       ManagementAuthenticationClass = "native-oidc"
	ManagementAuthStandardsAdapter ManagementAuthenticationClass = "standards-auth-adapter"
	ManagementAuthNativeCredential ManagementAuthenticationClass = "native-credential"
	ManagementAuthUnsupported      ManagementAuthenticationClass = "unsupported"
)

type ManagementRole string

const (
	ManagementRoleAdmin ManagementRole = "admin"
	ManagementRoleEdit  ManagementRole = "edit"
	ManagementRoleView  ManagementRole = "view"
	ManagementRoleAudit ManagementRole = "audit"
)

type ManagementRoleMappingLevel string

const (
	ManagementRoleMappingExact       ManagementRoleMappingLevel = "exact"
	ManagementRoleMappingLimited     ManagementRoleMappingLevel = "limited"
	ManagementRoleMappingUnsupported ManagementRoleMappingLevel = "unsupported"
)

type ManagementRoleMapping struct {
	Role   ManagementRole             `json:"role"`
	Level  ManagementRoleMappingLevel `json:"level"`
	Native string                     `json:"native,omitempty"`
}

func ValidateManagementAuthenticationClass(class ManagementAuthenticationClass) error {
	switch class {
	case ManagementAuthNativeOIDC, ManagementAuthStandardsAdapter, ManagementAuthNativeCredential, ManagementAuthUnsupported:
		return nil
	default:
		return fmt.Errorf("unsupported management authentication class %q", class)
	}
}

func ValidateManagementRoleMappings(mappings []ManagementRoleMapping) error {
	seen := map[ManagementRole]struct{}{}
	for _, mapping := range mappings {
		switch mapping.Role {
		case ManagementRoleAdmin, ManagementRoleEdit, ManagementRoleView, ManagementRoleAudit:
		default:
			return fmt.Errorf("unsupported management role %q", mapping.Role)
		}
		switch mapping.Level {
		case ManagementRoleMappingExact, ManagementRoleMappingLimited, ManagementRoleMappingUnsupported:
		default:
			return fmt.Errorf("management role %q has unsupported mapping level %q", mapping.Role, mapping.Level)
		}
		if _, ok := seen[mapping.Role]; ok {
			return fmt.Errorf("duplicate management role mapping %q", mapping.Role)
		}
		seen[mapping.Role] = struct{}{}
	}
	return nil
}

func NativeCredentialRoleMappings(adminNative, userNative string) []ManagementRoleMapping {
	adminNative = strings.TrimSpace(adminNative)
	userNative = strings.TrimSpace(userNative)
	return []ManagementRoleMapping{
		{Role: ManagementRoleAdmin, Level: ManagementRoleMappingExact, Native: adminNative},
		{Role: ManagementRoleEdit, Level: ManagementRoleMappingLimited, Native: userNative},
		{Role: ManagementRoleView, Level: ManagementRoleMappingLimited, Native: userNative},
		{Role: ManagementRoleAudit, Level: ManagementRoleMappingUnsupported},
	}
}

func OIDCManagementRoleMappings() []ManagementRoleMapping {
	return []ManagementRoleMapping{
		{Role: ManagementRoleAdmin, Level: ManagementRoleMappingExact, Native: "admin"},
		{Role: ManagementRoleEdit, Level: ManagementRoleMappingExact, Native: "edit"},
		{Role: ManagementRoleView, Level: ManagementRoleMappingExact, Native: "view"},
		{Role: ManagementRoleAudit, Level: ManagementRoleMappingExact, Native: "audit"},
	}
}
