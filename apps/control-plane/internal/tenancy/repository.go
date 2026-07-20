package tenancy

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalid             = errors.New("invalid tenancy request")
	ErrNotFound            = errors.New("tenancy resource not found")
	ErrConflict            = errors.New("tenancy resource conflict")
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	ErrQuotaExceeded       = errors.New("quota exceeded")
	ErrQuotaBelowUsage     = errors.New("quota limit is below current reserved and allocated usage")
	ErrInvalidTransition   = errors.New("invalid quota reservation transition")
)

type MutationContext struct {
	PrincipalID    string
	RequestID      string
	IdempotencyKey string
	RequestHash    string
}

type CreateTenantParams struct {
	Mutation MutationContext
	Name     string
	Slug     string
}

type CreateProjectParams struct {
	Mutation       MutationContext
	TenantID       string
	Name           string
	Slug           string
	IsolationClass IsolationClass
}

type CreateRoleBindingParams struct {
	Mutation    MutationContext
	ScopeType   ScopeType
	ScopeID     string
	SubjectType SubjectType
	SubjectID   string
	Role        Role
}

type SetQuotaParams struct {
	Mutation      MutationContext
	ProjectID     string
	ResourceClass string
	HardLimit     int64
}

type ReserveQuotaParams struct {
	ProjectID     string
	ResourceClass string
	Amount        int64
	OperationID   string
	ExpiresAt     time.Time
}

type Repository interface {
	CreateTenant(context.Context, CreateTenantParams) (Acceptance, error)
	GetTenant(context.Context, string) (Tenant, error)
	CreateProject(context.Context, CreateProjectParams) (Acceptance, error)
	GetProject(context.Context, string) (Project, error)
	CreateRoleBinding(context.Context, CreateRoleBindingParams) (Acceptance, error)
	GetRoleBinding(context.Context, string) (RoleBinding, error)
	SetQuota(context.Context, SetQuotaParams) (Acceptance, error)
	GetQuota(context.Context, string, string) (Quota, error)
	ReserveQuota(context.Context, ReserveQuotaParams) (QuotaReservation, error)
	CommitQuotaReservation(context.Context, string) (QuotaReservation, error)
	ReleaseQuotaReservation(context.Context, string, ReservationStatus) (QuotaReservation, error)
}
