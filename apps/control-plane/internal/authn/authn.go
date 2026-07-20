package authn

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

var (
	ErrUnauthenticated = errors.New("request is unauthenticated")
	ErrUnavailable     = errors.New("authentication is unavailable")
)

type Principal struct {
	ID          string
	SystemAdmin bool
}

type Authenticator interface {
	Authenticate(*http.Request) (Principal, error)
}

type BreakGlassAuthenticator struct {
	subjectID string
	tokenHash [sha256.Size]byte
}

func NewBreakGlassAuthenticator(subjectID, token string) (*BreakGlassAuthenticator, error) {
	subjectID = strings.TrimSpace(subjectID)
	token = strings.TrimSpace(token)
	if subjectID == "" || len(subjectID) > 255 {
		return nil, errors.New("break-glass subject ID must contain between 1 and 255 characters")
	}
	if len(token) < 32 {
		return nil, errors.New("break-glass token must contain at least 32 characters")
	}
	return &BreakGlassAuthenticator{
		subjectID: subjectID,
		tokenHash: sha256.Sum256([]byte(token)),
	}, nil
}

func (authenticator *BreakGlassAuthenticator) Authenticate(request *http.Request) (Principal, error) {
	if authenticator == nil {
		return Principal{}, ErrUnavailable
	}
	authorization := strings.TrimSpace(request.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		return Principal{}, ErrUnauthenticated
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	candidateHash := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(authenticator.tokenHash[:], candidateHash[:]) != 1 {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{ID: authenticator.subjectID, SystemAdmin: true}, nil
}
