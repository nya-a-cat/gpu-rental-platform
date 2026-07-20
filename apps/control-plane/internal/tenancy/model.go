package tenancy

import "time"

type IsolationClass string

const (
	IsolationShared            IsolationClass = "shared"
	IsolationDedicatedNodePool IsolationClass = "dedicated-node-pool"
	IsolationDedicatedCluster  IsolationClass = "dedicated-cluster"
)

type Tenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Status     string    `json:"status"`
	Generation int64     `json:"generation"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

type Project struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenantId"`
	Name              string         `json:"name"`
	Slug              string         `json:"slug"`
	IsolationClass    IsolationClass `json:"isolationClass"`
	NamespaceName     string         `json:"namespaceName"`
	DesiredState      string         `json:"desiredState"`
	ObservedState     string         `json:"observedState"`
	ProvisioningState string         `json:"provisioningState"`
	Conditions        []Condition    `json:"conditions"`
	Generation        int64          `json:"generation"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

type ScopeType string

const (
	ScopeTenant  ScopeType = "tenant"
	ScopeProject ScopeType = "project"
)

type SubjectType string

const (
	SubjectUser           SubjectType = "user"
	SubjectGroup          SubjectType = "group"
	SubjectServiceAccount SubjectType = "service_account"
)

type Role string

const (
	RoleTenantOwner    Role = "tenant_owner"
	RoleProjectAdmin   Role = "project_admin"
	RoleOperator       Role = "operator"
	RoleDeveloper      Role = "developer"
	RoleViewer         Role = "viewer"
	RoleBillingAdmin   Role = "billing_admin"
	RoleAuditor        Role = "auditor"
	RoleServiceAccount Role = "service_account"
)

type RoleBinding struct {
	ID          string      `json:"id"`
	ScopeType   ScopeType   `json:"scopeType"`
	ScopeID     string      `json:"scopeId"`
	SubjectType SubjectType `json:"subjectType"`
	SubjectID   string      `json:"subjectId"`
	Role        Role        `json:"role"`
	CreatedBy   string      `json:"createdBy"`
	CreatedAt   time.Time   `json:"createdAt"`
}

type Quota struct {
	ProjectID     string    `json:"projectId"`
	ResourceClass string    `json:"resourceClass"`
	HardLimit     int64     `json:"hardLimit"`
	Reserved      int64     `json:"reserved"`
	Allocated     int64     `json:"allocated"`
	Generation    int64     `json:"generation"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type ReservationStatus string

const (
	ReservationPending   ReservationStatus = "pending"
	ReservationCommitted ReservationStatus = "committed"
	ReservationReleased  ReservationStatus = "released"
	ReservationExpired   ReservationStatus = "expired"
)

type QuotaReservation struct {
	ID            string            `json:"id"`
	ProjectID     string            `json:"projectId"`
	ResourceClass string            `json:"resourceClass"`
	Amount        int64             `json:"amount"`
	Status        ReservationStatus `json:"status"`
	OperationID   string            `json:"operationId"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

type Acceptance struct {
	ResourceID  string `json:"resourceId"`
	OperationID string `json:"operationId"`
	Replayed    bool   `json:"-"`
}
