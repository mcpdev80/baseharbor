package authorization

// Permission identifies an action that may be granted to a role.
type Permission string

const (
	PermRead   Permission = "read"
	PermCreate Permission = "create"
	PermUpdate Permission = "update"
	PermDelete Permission = "delete"
)

const (
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

var rolePermissions = map[string]map[Permission]struct{}{
	RoleEditor: {
		PermRead:   {},
		PermCreate: {},
		PermUpdate: {},
		PermDelete: {},
	},
	RoleViewer: {
		PermRead: {},
	},
}

// Service evaluates role/permission pairs. Unknown roles and permissions deny by default.
type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) Allowed(roles []string, permission Permission) bool {
	if len(roles) == 0 {
		return false
	}

	for _, role := range roles {
		permissions := rolePermissions[role]
		if permissions == nil {
			continue
		}
		if _, ok := permissions[permission]; ok {
			return true
		}
	}
	return false
}

type PermissionError struct {
	Permission Permission
}

func (e *PermissionError) Error() string { return "forbidden" }

func (s *Service) Authorize(roles []string, permission Permission) error {
	if s.Allowed(roles, permission) {
		return nil
	}
	return &PermissionError{Permission: permission}
}
