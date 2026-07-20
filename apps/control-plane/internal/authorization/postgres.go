package authorization

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type PostgresEngine struct {
	database *sql.DB
}

func NewPostgresEngine(database *sql.DB) *PostgresEngine {
	return &PostgresEngine{database: database}
}

func (engine *PostgresEngine) Authorize(
	ctx context.Context,
	request ports.AuthorizationRequest,
) (ports.AuthorizationDecision, error) {
	if engine == nil || engine.database == nil {
		return ports.AuthorizationDecision{}, errors.New("authorization database is unavailable")
	}
	request.SubjectID = strings.TrimSpace(request.SubjectID)
	if request.SubjectID == "" || !identity.IsUUID(request.ScopeID) {
		return ports.AuthorizationDecision{Allowed: false, Reason: "invalid authorization subject or scope"}, nil
	}
	if request.ScopeType != string(tenancy.ScopeTenant) && request.ScopeType != string(tenancy.ScopeProject) {
		return ports.AuthorizationDecision{Allowed: false, Reason: "unsupported authorization scope"}, nil
	}
	rows, err := engine.database.QueryContext(ctx, `
SELECT scope_type, role
FROM role_bindings
WHERE subject_type = 'user'
  AND subject_id = $1
  AND (
    (scope_type = $2 AND scope_id = $3)
    OR (
      $2 = 'project'
      AND scope_type = 'tenant'
      AND scope_id = (SELECT tenant_id FROM projects WHERE id = $3)
    )
  )`, request.SubjectID, request.ScopeType, request.ScopeID)
	if err != nil {
		return ports.AuthorizationDecision{}, fmt.Errorf("query role bindings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var scopeType tenancy.ScopeType
		var role tenancy.Role
		if err := rows.Scan(&scopeType, &role); err != nil {
			return ports.AuthorizationDecision{}, fmt.Errorf("scan role binding: %w", err)
		}
		if roleAllows(request.Action, request.ScopeType, scopeType, role) {
			return ports.AuthorizationDecision{Allowed: true, Reason: "role binding allows action"}, nil
		}
	}
	if err := rows.Err(); err != nil {
		return ports.AuthorizationDecision{}, fmt.Errorf("iterate role bindings: %w", err)
	}
	return ports.AuthorizationDecision{Allowed: false, Reason: "no role binding allows action"}, nil
}

func roleAllows(action, requestedScope string, boundScope tenancy.ScopeType, role tenancy.Role) bool {
	if requestedScope == string(tenancy.ScopeProject) && boundScope == tenancy.ScopeTenant {
		return role == tenancy.RoleTenantOwner
	}
	switch action {
	case "tenant.read":
		return role == tenancy.RoleTenantOwner || role == tenancy.RoleBillingAdmin || role == tenancy.RoleAuditor || role == tenancy.RoleViewer
	case "project.create":
		return role == tenancy.RoleTenantOwner
	case "project.read", "quota.read":
		return role == tenancy.RoleProjectAdmin || role == tenancy.RoleOperator || role == tenancy.RoleDeveloper || role == tenancy.RoleViewer || role == tenancy.RoleBillingAdmin || role == tenancy.RoleAuditor || role == tenancy.RoleServiceAccount
	case "role_binding.create":
		if boundScope == tenancy.ScopeTenant {
			return role == tenancy.RoleTenantOwner
		}
		return role == tenancy.RoleProjectAdmin
	case "role_binding.read":
		if boundScope == tenancy.ScopeTenant {
			return role == tenancy.RoleTenantOwner || role == tenancy.RoleAuditor || role == tenancy.RoleViewer
		}
		return role == tenancy.RoleProjectAdmin || role == tenancy.RoleOperator || role == tenancy.RoleDeveloper || role == tenancy.RoleViewer || role == tenancy.RoleAuditor || role == tenancy.RoleServiceAccount
	case "quota.set":
		return role == tenancy.RoleProjectAdmin || role == tenancy.RoleBillingAdmin
	default:
		return false
	}
}

var _ ports.AuthorizationEngine = (*PostgresEngine)(nil)
