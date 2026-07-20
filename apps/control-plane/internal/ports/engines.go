package ports

import (
	"context"
	"encoding/json"
	"time"
)

// BillingEngine rates immutable usage facts. Implementations may be the local
// engine or an adapter such as OpenMeter.
type BillingEngine interface {
	RateUsage(context.Context, UsageFact) (RatedUsage, error)
}

type UsageFact struct {
	ID             string
	TenantID       string
	ProjectID      string
	ResourceClass  string
	Quantity       string
	AllocationFrom time.Time
	AllocationTo   time.Time
	Attributes     json.RawMessage
}

type RatedUsage struct {
	UsageFactID  string
	PriceBookID  string
	PriceVersion int
	AmountMinor  int64
	Currency     string
	Calculation  json.RawMessage
	CalculatedAt time.Time
}

// AuthorizationEngine keeps resource handlers independent from the initial
// PostgreSQL RoleBinding implementation and a possible future OpenFGA adapter.
type AuthorizationEngine interface {
	Authorize(context.Context, AuthorizationRequest) (AuthorizationDecision, error)
}

type AuthorizationRequest struct {
	SubjectID  string
	Action     string
	ScopeType  string
	ScopeID    string
	Resource   string
	ResourceID string
}

type AuthorizationDecision struct {
	Allowed bool
	Reason  string
}

// JobEngine dispatches long-running domain work while the public API exposes a
// stable Operation resource.
type JobEngine interface {
	Dispatch(context.Context, JobCommand) error
}

type JobCommand struct {
	OperationID string
	Kind        string
	Payload     json.RawMessage
}

// FleetManager is the product control-plane boundary for OCM. The OCM adapter
// owns placement queries and ManifestWork delivery behind this interface.
type FleetManager interface {
	Place(context.Context, PlacementRequest) (PlacementResult, error)
	ApplyWork(context.Context, WorkRequest) (WorkResult, error)
}

type PlacementRequest struct {
	OperationID          string
	ProjectID            string
	CapacityPoolID       string
	AcceleratorProfileID string
	Quantity             int
	Traits               map[string]string
}

type PlacementResult struct {
	ClusterID  string
	NodePoolID string
	DecisionID string
}

type WorkRequest struct {
	OperationID string
	ClusterID   string
	WorkID      string
	Manifest    json.RawMessage
}

type WorkResult struct {
	WorkID    string
	Applied   bool
	Available bool
}
