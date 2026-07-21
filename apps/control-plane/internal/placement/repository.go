package placement

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

var (
	ErrInvalid  = errors.New("invalid placement request")
	ErrNotFound = errors.New("placement decision not found")
	ErrCapacity = errors.New("capacity is unavailable")
)

type Decision struct {
	ID                   string          `json:"id"`
	ProjectID            string          `json:"projectId"`
	CapacityPoolID       string          `json:"capacityPoolId"`
	ClusterID            string          `json:"clusterId"`
	NodePoolID           string          `json:"nodePoolId"`
	AcceleratorProfileID string          `json:"acceleratorProfileId"`
	Quantity             int             `json:"quantity"`
	Traits               json.RawMessage `json:"traits"`
	Status               string          `json:"status"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
}

type CreateParams struct {
	Mutation             tenancy.MutationContext
	ProjectID            string
	AcceleratorProfileID string
	Quantity             int
	Traits               map[string]string
}

type Repository interface {
	CreatePlacement(context.Context, CreateParams) (tenancy.Acceptance, error)
	GetPlacement(context.Context, string) (Decision, error)
}
