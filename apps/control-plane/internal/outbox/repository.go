package outbox

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("outbox event not found or lease lost")

type Repository interface {
	Claim(context.Context, ClaimParams) ([]Event, error)
	MarkDelivered(ctx context.Context, eventID, workerID string, generation int) error
	MarkFailed(ctx context.Context, eventID, workerID string, generation int, failure string, backoff time.Duration) error
	MarkDeadLettered(ctx context.Context, eventID, workerID string, generation int, failure string) error
}
