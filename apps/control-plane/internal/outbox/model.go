package outbox

import (
	"encoding/json"
	"time"
)

type Event struct {
	ID             string          `json:"id"`
	AggregateType  string          `json:"aggregateType"`
	AggregateID    string          `json:"aggregateId"`
	EventType      string          `json:"eventType"`
	Payload        json.RawMessage `json:"payload"`
	OccurredAt     time.Time       `json:"occurredAt"`
	AvailableAt    time.Time       `json:"availableAt"`
	Attempts       int             `json:"attempts"`
	LockedBy       *string         `json:"lockedBy,omitempty"`
	LockedUntil    *time.Time      `json:"lockedUntil,omitempty"`
	DeliveredAt    *time.Time      `json:"deliveredAt,omitempty"`
	DeadLetteredAt *time.Time      `json:"deadLetteredAt,omitempty"`
	LastError      *string         `json:"lastError,omitempty"`
}

type ClaimParams struct {
	WorkerID      string
	EventType     string
	Limit         int
	LeaseDuration time.Duration
}
