package authz

import "errors"

type Role string

const (
	RolePlatformAdmin  Role = "platform_admin"
	RoleSchoolOperator Role = "school_operator"
	RoleTeacher        Role = "teacher"
	RoleStudent        Role = "student"
	RoleAuditor        Role = "auditor"
)

type Permission string

const (
	PermissionInfrastructureRead Permission = "infrastructure.read"
	PermissionClusterManage      Permission = "cluster.manage"
	PermissionCredentialManage   Permission = "credential.manage"
	PermissionIdentityManage     Permission = "identity.manage"

	PermissionClassroomRead     Permission = "classroom.read"
	PermissionClassroomManage   Permission = "classroom.manage"
	PermissionClassroomPrecheck Permission = "classroom.precheck"

	PermissionTemplateRead    Permission = "template.read"
	PermissionTemplateManage  Permission = "template.manage"
	PermissionTemplatePublish Permission = "template.publish"

	PermissionTerminalRead   Permission = "terminal.read"
	PermissionTerminalManage Permission = "terminal.manage"

	PermissionDesktopRead             Permission = "desktop.read"
	PermissionDesktopCreate           Permission = "desktop.create"
	PermissionDesktopStart            Permission = "desktop.start"
	PermissionDesktopShutdown         Permission = "desktop.shutdown"
	PermissionDesktopForceStop        Permission = "desktop.force_stop"
	PermissionDesktopDelete           Permission = "desktop.delete"
	PermissionDesktopSnapshotCreate   Permission = "desktop.snapshot.create"
	PermissionDesktopSnapshotRollback Permission = "desktop.snapshot.rollback"
	PermissionDesktopRebuild          Permission = "desktop.rebuild"
	PermissionDesktopConnect          Permission = "desktop.connect"
	PermissionDesktopRemoteControl    Permission = "desktop.remote_control"

	PermissionOperationRead       Permission = "operation.read"
	PermissionOperationRetryItem  Permission = "operation.retry_item"
	PermissionOperationRetryBatch Permission = "operation.retry_batch"

	PermissionAuditRead Permission = "audit.read"
)

// Principal is a verified actor. Authentication middleware is responsible for
// populating it from a trusted identity source; callers must never copy roles
// or classroom scopes directly from unverified request data.
type Principal struct {
	SubjectID           string
	OrganizationID      string
	Roles               []Role
	AllowedClassroomIDs []string
}

// ResourceScope contains only authorization-relevant facts about a target.
// Service code must populate it from trusted storage, not client claims.
type ResourceScope struct {
	OrganizationID string
	ClassroomID    string

	// AssignmentID and AssignedSubjectID prove ownership for student desktop
	// read/connect checks. Both are required for student access.
	AssignmentID      string
	AssignedSubjectID string

	// OperationItemID is required when a teacher retries one failed item. A
	// classroom-only scope is insufficient for the item-level permission.
	OperationItemID string
}

// ErrDenied intentionally does not disclose whether denial was caused by an
// unknown role, missing scope, ownership, or organization mismatch.
var ErrDenied = errors.New("authorization denied")

// Authorizer is the policy boundary expected by service-layer code.
type Authorizer interface {
	Authorize(Principal, Permission, ResourceScope) error
	Allowed(Principal, Permission, ResourceScope) bool
}

// Engine is stateless and safe for concurrent use.
type Engine struct{}

func New() Engine { return Engine{} }

var defaultEngine = New()

// Authorize applies the default policy engine.
func Authorize(principal Principal, permission Permission, resource ResourceScope) error {
	return defaultEngine.Authorize(principal, permission, resource)
}

// Allowed reports whether the default policy engine authorizes an action.
func Allowed(principal Principal, permission Permission, resource ResourceScope) bool {
	return defaultEngine.Allowed(principal, permission, resource)
}

func (Engine) Authorize(principal Principal, permission Permission, resource ResourceScope) error {
	if !defaultAllowed(principal, permission, resource) {
		return ErrDenied
	}
	return nil
}

func (Engine) Allowed(principal Principal, permission Permission, resource ResourceScope) bool {
	return defaultAllowed(principal, permission, resource)
}

func defaultAllowed(principal Principal, permission Permission, resource ResourceScope) bool {
	if principal.SubjectID == "" || !knownPermission(permission) {
		return false
	}
	if permission == PermissionOperationRetryItem && resource.OperationItemID == "" {
		return false
	}

	// Platform administrators are the only explicit cross-organization
	// exception. Unknown permissions are still rejected above.
	if hasRole(principal.Roles, RolePlatformAdmin) {
		return true
	}

	if principal.OrganizationID == "" || resource.OrganizationID == "" || principal.OrganizationID != resource.OrganizationID {
		return false
	}

	for _, role := range principal.Roles {
		if !roleGrants(role, permission) {
			continue
		}
		switch role {
		case RoleSchoolOperator, RoleAuditor:
			return true
		case RoleTeacher:
			if teacherScopeAllows(principal, permission, resource) {
				return true
			}
		case RoleStudent:
			if studentScopeAllows(principal, permission, resource) {
				return true
			}
		}
	}
	return false
}

func teacherScopeAllows(principal Principal, permission Permission, resource ResourceScope) bool {
	if resource.ClassroomID == "" || !contains(principal.AllowedClassroomIDs, resource.ClassroomID) {
		return false
	}
	return true
}

func studentScopeAllows(principal Principal, permission Permission, resource ResourceScope) bool {
	if permission != PermissionDesktopRead && permission != PermissionDesktopConnect {
		return false
	}
	return resource.AssignmentID != "" &&
		resource.AssignedSubjectID != "" &&
		resource.AssignedSubjectID == principal.SubjectID
}

func roleGrants(role Role, permission Permission) bool {
	switch role {
	case RoleSchoolOperator:
		switch permission {
		case PermissionInfrastructureRead,
			PermissionClassroomRead,
			PermissionClassroomManage,
			PermissionClassroomPrecheck,
			PermissionTemplateRead,
			PermissionTemplateManage,
			PermissionTemplatePublish,
			PermissionTerminalRead,
			PermissionTerminalManage,
			PermissionDesktopRead,
			PermissionDesktopCreate,
			PermissionDesktopStart,
			PermissionDesktopShutdown,
			PermissionDesktopForceStop,
			PermissionDesktopDelete,
			PermissionDesktopSnapshotCreate,
			PermissionDesktopSnapshotRollback,
			PermissionDesktopRebuild,
			PermissionDesktopRemoteControl,
			PermissionOperationRead,
			PermissionOperationRetryItem,
			PermissionOperationRetryBatch:
			return true
		}
	case RoleTeacher:
		switch permission {
		case PermissionClassroomRead,
			PermissionClassroomPrecheck,
			PermissionDesktopRead,
			PermissionDesktopStart,
			PermissionDesktopShutdown,
			PermissionOperationRead,
			PermissionOperationRetryItem:
			return true
		}
	case RoleStudent:
		return permission == PermissionDesktopRead || permission == PermissionDesktopConnect
	case RoleAuditor:
		return permission == PermissionAuditRead
	}
	return false
}

func knownPermission(permission Permission) bool {
	switch permission {
	case PermissionInfrastructureRead,
		PermissionClusterManage,
		PermissionCredentialManage,
		PermissionIdentityManage,
		PermissionClassroomRead,
		PermissionClassroomManage,
		PermissionClassroomPrecheck,
		PermissionTemplateRead,
		PermissionTemplateManage,
		PermissionTemplatePublish,
		PermissionTerminalRead,
		PermissionTerminalManage,
		PermissionDesktopRead,
		PermissionDesktopCreate,
		PermissionDesktopStart,
		PermissionDesktopShutdown,
		PermissionDesktopForceStop,
		PermissionDesktopDelete,
		PermissionDesktopSnapshotCreate,
		PermissionDesktopSnapshotRollback,
		PermissionDesktopRebuild,
		PermissionDesktopConnect,
		PermissionDesktopRemoteControl,
		PermissionOperationRead,
		PermissionOperationRetryItem,
		PermissionOperationRetryBatch,
		PermissionAuditRead:
		return true
	default:
		return false
	}
}

func hasRole(roles []Role, wanted Role) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
