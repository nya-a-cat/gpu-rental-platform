package operation

import "time"

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusTimedOut  Status = "timed_out"
)

type ResourceRef struct {
	Type string `json:"resourceType"`
	ID   string `json:"resourceId"`
}

type Step struct {
	Name       string     `json:"name"`
	Status     Status     `json:"status"`
	Progress   int        `json:"progress"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Detail     string     `json:"detail,omitempty"`
}

type StructuredError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Detail  string         `json:"detail,omitempty"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type Operation struct {
	ID                string           `json:"id"`
	Kind              string           `json:"kind"`
	Status            Status           `json:"status"`
	Target            ResourceRef      `json:"target"`
	ParentOperationID *string          `json:"parentOperationId,omitempty"`
	Steps             []Step           `json:"steps"`
	Progress          int              `json:"progress"`
	Deadline          *time.Time       `json:"deadline,omitempty"`
	Retryable         bool             `json:"retryable"`
	RequestID         string           `json:"requestId"`
	Error             *StructuredError `json:"error,omitempty"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
	StartedAt         *time.Time       `json:"startedAt,omitempty"`
	FinishedAt        *time.Time       `json:"finishedAt,omitempty"`
}

type CreateParams struct {
	ID                string
	Kind              string
	Target            ResourceRef
	ParentOperationID *string
	Deadline          *time.Time
	Retryable         bool
	RequestID         string
}
