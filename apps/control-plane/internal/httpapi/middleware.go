package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
)

const requestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get(requestIDHeader))
		if requestID == "" || len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n") {
			generated, err := identity.NewUUID()
			if err != nil {
				writeProblem(response, request, Problem{
					Title:  "Internal Server Error",
					Status: http.StatusInternalServerError,
					Detail: "Unable to create request context.",
					Code:   "request_id_generation_failed",
				})
				return
			}
			requestID = generated
		}
		response.Header().Set(requestIDHeader, requestID)
		request = request.WithContext(context.WithValue(request.Context(), requestIDContextKey{}, requestID))
		next.ServeHTTP(response, request)
	})
}

func recoveryMiddleware(logger *slog.Logger, metrics *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				metrics.RecordPanic()
				logger.ErrorContext(request.Context(), "recovered HTTP panic",
					"request_id", RequestIDFromContext(request.Context()),
					"method", request.Method,
					"path", request.URL.Path,
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				writeProblem(response, request, Problem{
					Title:  "Internal Server Error",
					Status: http.StatusInternalServerError,
					Detail: "The request could not be completed.",
					Code:   "internal_error",
				})
			}
		}()
		next.ServeHTTP(response, request)
	})
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: response, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		logger.InfoContext(request.Context(), "HTTP request completed",
			"request_id", RequestIDFromContext(request.Context()),
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(contents []byte) (int, error) {
	written, err := recorder.ResponseWriter.Write(contents)
	recorder.bytes += written
	return written, err
}
