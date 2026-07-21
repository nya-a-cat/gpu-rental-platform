package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

var (
	ErrInvalid       = errors.New("invalid workspace request")
	ErrNotFound      = errors.New("workspace not found")
	ErrConflict      = errors.New("workspace conflict")
	ErrQuotaExceeded = errors.New("workspace GPU quota exceeded")
)

type DesiredState string

const (
	DesiredRunning    DesiredState = "running"
	DesiredStopped    DesiredState = "stopped"
	DesiredTerminated DesiredState = "terminated"
)

type AccessType string

const (
	AccessSSH         AccessType = "ssh"
	AccessWebTerminal AccessType = "web-terminal"
	AccessJupyter     AccessType = "jupyter"
)

type Workspace struct {
	ID                   string              `json:"id"`
	ProjectID            string              `json:"projectId"`
	ClusterID            string              `json:"clusterId"`
	AcceleratorProfileID string              `json:"acceleratorProfileId"`
	Name                 string              `json:"name"`
	GPUCount             int                 `json:"gpuCount"`
	StorageGiB           int                 `json:"storageGiB"`
	NamespaceName        string              `json:"namespaceName"`
	DesiredState         DesiredState        `json:"desiredState"`
	ObservedState        string              `json:"observedState"`
	ProvisioningState    string              `json:"provisioningState"`
	Conditions           []tenancy.Condition `json:"conditions"`
	Generation           int64               `json:"generation"`
	ObservedGeneration   int64               `json:"observedGeneration"`
	ManifestWorkName     string              `json:"manifestWorkName"`
	CreatedAt            time.Time           `json:"createdAt"`
	UpdatedAt            time.Time           `json:"updatedAt"`
}

type CreateParams struct {
	Mutation             tenancy.MutationContext
	ProjectID            string
	ClusterID            string
	AcceleratorProfileID string
	Name                 string
	StorageGiB           int
}

type SetDesiredStateParams struct {
	Mutation     tenancy.MutationContext
	WorkspaceID  string
	DesiredState DesiredState
}

type AccessToken struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspaceId"`
	AccessType  AccessType `json:"accessType"`
	Token       string     `json:"token"`
	ExpiresAt   time.Time  `json:"expiresAt"`
}

type CreateAccessTokenParams struct {
	Mutation    tenancy.MutationContext
	WorkspaceID string
	AccessType  AccessType
	TTL         time.Duration
}

type RevokeAccessTokenParams struct {
	Mutation    tenancy.MutationContext
	WorkspaceID string
	TokenID     string
}

type Repository interface {
	CreateWorkspace(context.Context, CreateParams) (tenancy.Acceptance, error)
	GetWorkspace(context.Context, string) (Workspace, error)
	SetWorkspaceDesiredState(context.Context, SetDesiredStateParams) (tenancy.Acceptance, error)
	CreateAccessToken(context.Context, CreateAccessTokenParams) (AccessToken, error)
	RevokeAccessToken(context.Context, RevokeAccessTokenParams) (tenancy.Acceptance, error)
}
