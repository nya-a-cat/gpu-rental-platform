package operation

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("operation not found")

type Reader interface {
	GetByID(context.Context, string) (Operation, error)
}

type Repository interface {
	Reader
	Create(context.Context, CreateParams) (Operation, error)
}
