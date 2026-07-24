package authz_test

import (
	"errors"
	"testing"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/authz"
)

func TestPolicyRoleAndScopeMatrix(t *testing.T) {
	platformAdmin := authz.Principal{SubjectID: "platform-1", Roles: []authz.Role{authz.RolePlatformAdmin}}
	operator := authz.Principal{SubjectID: "ops-1", OrganizationID: "org-a", Roles: []authz.Role{authz.RoleSchoolOperator}}
	teacher := authz.Principal{
		SubjectID:           "teacher-1",
		OrganizationID:      "org-a",
		Roles:               []authz.Role{authz.RoleTeacher},
		AllowedClassroomIDs: []string{"classroom-1"},
	}
	student := authz.Principal{SubjectID: "student-1", OrganizationID: "org-a", Roles: []authz.Role{authz.RoleStudent}}
	auditor := authz.Principal{SubjectID: "auditor-1", OrganizationID: "org-a", Roles: []authz.Role{authz.RoleAuditor}}

	classroomOne := authz.ResourceScope{OrganizationID: "org-a", ClassroomID: "classroom-1"}
	classroomTwo := authz.ResourceScope{OrganizationID: "org-a", ClassroomID: "classroom-2"}
	otherOrganization := authz.ResourceScope{OrganizationID: "org-b", ClassroomID: "classroom-9"}
	ownAssignment := authz.ResourceScope{
		OrganizationID:    "org-a",
		ClassroomID:       "classroom-1",
		AssignmentID:      "assignment-1",
		AssignedSubjectID: "student-1",
	}
	otherAssignment := authz.ResourceScope{
		OrganizationID:    "org-a",
		ClassroomID:       "classroom-1",
		AssignmentID:      "assignment-2",
		AssignedSubjectID: "student-2",
	}

	tests := []struct {
		name       string
		principal  authz.Principal
		permission authz.Permission
		resource   authz.ResourceScope
		allowed    bool
	}{
		{name: "platform administrator same organization", principal: platformAdmin, permission: authz.PermissionDesktopDelete, resource: classroomOne, allowed: true},
		{name: "platform administrator explicit cross organization exception", principal: platformAdmin, permission: authz.PermissionCredentialManage, resource: otherOrganization, allowed: true},
		{name: "platform administrator unknown permission denied", principal: platformAdmin, permission: authz.Permission("desktop.do_everything"), resource: classroomOne, allowed: false},

		{name: "operator reads infrastructure", principal: operator, permission: authz.PermissionInfrastructureRead, resource: classroomOne, allowed: true},
		{name: "operator manages classroom", principal: operator, permission: authz.PermissionClassroomManage, resource: classroomTwo, allowed: true},
		{name: "operator item retry requires item scope", principal: operator, permission: authz.PermissionOperationRetryItem, resource: classroomOne, allowed: false},
		{name: "operator cannot manage platform credentials", principal: operator, permission: authz.PermissionCredentialManage, resource: classroomOne, allowed: false},
		{name: "operator cross organization denied", principal: operator, permission: authz.PermissionDesktopRead, resource: otherOrganization, allowed: false},

		{name: "teacher reads authorized classroom", principal: teacher, permission: authz.PermissionClassroomRead, resource: classroomOne, allowed: true},
		{name: "teacher prechecks authorized classroom", principal: teacher, permission: authz.PermissionClassroomPrecheck, resource: classroomOne, allowed: true},
		{name: "teacher starts desktop in authorized classroom", principal: teacher, permission: authz.PermissionDesktopStart, resource: classroomOne, allowed: true},
		{name: "teacher shuts down desktop in authorized classroom", principal: teacher, permission: authz.PermissionDesktopShutdown, resource: classroomOne, allowed: true},
		{
			name:       "teacher retries one item in authorized classroom",
			principal:  teacher,
			permission: authz.PermissionOperationRetryItem,
			resource:   authz.ResourceScope{OrganizationID: "org-a", ClassroomID: "classroom-1", OperationItemID: "item-1"},
			allowed:    true,
		},
		{name: "teacher retry requires item scope", principal: teacher, permission: authz.PermissionOperationRetryItem, resource: classroomOne, allowed: false},
		{name: "teacher batch retry denied", principal: teacher, permission: authz.PermissionOperationRetryBatch, resource: classroomOne, allowed: false},
		{name: "teacher unauthorized classroom denied", principal: teacher, permission: authz.PermissionDesktopStart, resource: classroomTwo, allowed: false},
		{name: "teacher cross organization denied", principal: teacher, permission: authz.PermissionDesktopStart, resource: otherOrganization, allowed: false},
		{name: "teacher missing classroom scope denied", principal: teacher, permission: authz.PermissionDesktopStart, resource: authz.ResourceScope{OrganizationID: "org-a"}, allowed: false},

		{name: "student reads own assignment", principal: student, permission: authz.PermissionDesktopRead, resource: ownAssignment, allowed: true},
		{name: "student connects own assignment", principal: student, permission: authz.PermissionDesktopConnect, resource: ownAssignment, allowed: true},
		{name: "student cannot read another assignment", principal: student, permission: authz.PermissionDesktopRead, resource: otherAssignment, allowed: false},
		{name: "student assignment id required", principal: student, permission: authz.PermissionDesktopConnect, resource: authz.ResourceScope{OrganizationID: "org-a", AssignedSubjectID: "student-1"}, allowed: false},
		{name: "student cannot start desktop", principal: student, permission: authz.PermissionDesktopStart, resource: ownAssignment, allowed: false},
		{name: "student cross organization denied", principal: student, permission: authz.PermissionDesktopConnect, resource: authz.ResourceScope{OrganizationID: "org-b", AssignmentID: "assignment-1", AssignedSubjectID: "student-1"}, allowed: false},

		{name: "auditor reads audit", principal: auditor, permission: authz.PermissionAuditRead, resource: classroomOne, allowed: true},
		{name: "auditor cannot read classroom", principal: auditor, permission: authz.PermissionClassroomRead, resource: classroomOne, allowed: false},
		{name: "auditor cannot mutate", principal: auditor, permission: authz.PermissionDesktopShutdown, resource: classroomOne, allowed: false},
		{name: "auditor cross organization denied", principal: auditor, permission: authz.PermissionAuditRead, resource: otherOrganization, allowed: false},

		{name: "unknown role denied", principal: authz.Principal{SubjectID: "mystery", OrganizationID: "org-a", Roles: []authz.Role{"unknown"}}, permission: authz.PermissionClassroomRead, resource: classroomOne, allowed: false},
		{name: "missing subject denied", principal: authz.Principal{OrganizationID: "org-a", Roles: []authz.Role{authz.RoleSchoolOperator}}, permission: authz.PermissionClassroomRead, resource: classroomOne, allowed: false},
		{name: "missing principal organization denied", principal: authz.Principal{SubjectID: "ops-2", Roles: []authz.Role{authz.RoleSchoolOperator}}, permission: authz.PermissionClassroomRead, resource: classroomOne, allowed: false},
		{name: "missing resource organization denied", principal: operator, permission: authz.PermissionClassroomRead, resource: authz.ResourceScope{ClassroomID: "classroom-1"}, allowed: false},
		{name: "unknown permission denied", principal: operator, permission: authz.Permission("unknown.permission"), resource: classroomOne, allowed: false},
	}

	engine := authz.New()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := engine.Allowed(test.principal, test.permission, test.resource); got != test.allowed {
				t.Fatalf("Engine.Allowed() = %v, want %v", got, test.allowed)
			}
			err := engine.Authorize(test.principal, test.permission, test.resource)
			if test.allowed && err != nil {
				t.Fatalf("Engine.Authorize() error = %v", err)
			}
			if !test.allowed && !errors.Is(err, authz.ErrDenied) {
				t.Fatalf("Engine.Authorize() error = %v, want ErrDenied", err)
			}

			if got := authz.Allowed(test.principal, test.permission, test.resource); got != test.allowed {
				t.Fatalf("package Allowed() = %v, want %v", got, test.allowed)
			}
			err = authz.Authorize(test.principal, test.permission, test.resource)
			if test.allowed && err != nil {
				t.Fatalf("package Authorize() error = %v", err)
			}
			if !test.allowed && !errors.Is(err, authz.ErrDenied) {
				t.Fatalf("package Authorize() error = %v, want ErrDenied", err)
			}
		})
	}
}

func TestHighRiskPermissionsAreIndependent(t *testing.T) {
	teacher := authz.Principal{
		SubjectID:           "teacher-1",
		OrganizationID:      "org-a",
		Roles:               []authz.Role{authz.RoleTeacher},
		AllowedClassroomIDs: []string{"classroom-1"},
	}
	operator := authz.Principal{SubjectID: "ops-1", OrganizationID: "org-a", Roles: []authz.Role{authz.RoleSchoolOperator}}
	resource := authz.ResourceScope{OrganizationID: "org-a", ClassroomID: "classroom-1"}

	highRisk := []authz.Permission{
		authz.PermissionDesktopDelete,
		authz.PermissionDesktopSnapshotRollback,
		authz.PermissionDesktopRebuild,
		authz.PermissionTemplatePublish,
		authz.PermissionDesktopForceStop,
		authz.PermissionDesktopRemoteControl,
	}
	seen := make(map[authz.Permission]struct{}, len(highRisk))
	for _, permission := range highRisk {
		if _, duplicate := seen[permission]; duplicate {
			t.Fatalf("duplicate high-risk permission value %q", permission)
		}
		seen[permission] = struct{}{}
		if authz.Allowed(teacher, permission, resource) {
			t.Errorf("teacher unexpectedly received high-risk permission %q", permission)
		}
		if !authz.Allowed(operator, permission, resource) {
			t.Errorf("school operator did not receive high-risk permission %q", permission)
		}
	}
}

func TestMultipleRolesComposeWithoutBypassingRoleScope(t *testing.T) {
	principal := authz.Principal{
		SubjectID:           "teacher-auditor",
		OrganizationID:      "org-a",
		Roles:               []authz.Role{authz.RoleTeacher, authz.RoleAuditor, "unknown"},
		AllowedClassroomIDs: []string{"classroom-1"},
	}

	if !authz.Allowed(principal, authz.PermissionAuditRead, authz.ResourceScope{OrganizationID: "org-a"}) {
		t.Fatal("auditor role did not compose with teacher role")
	}
	if !authz.Allowed(principal, authz.PermissionDesktopStart, authz.ResourceScope{OrganizationID: "org-a", ClassroomID: "classroom-1"}) {
		t.Fatal("teacher role did not compose with auditor role")
	}
	if authz.Allowed(principal, authz.PermissionDesktopStart, authz.ResourceScope{OrganizationID: "org-a", ClassroomID: "classroom-2"}) {
		t.Fatal("additional roles bypassed teacher classroom scope")
	}
}

var _ authz.Authorizer = authz.New()
