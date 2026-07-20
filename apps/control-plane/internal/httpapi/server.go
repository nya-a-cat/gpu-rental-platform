package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

type ReadinessChecker interface {
	PingContext(context.Context) error
}

type AgentHealthPolicy struct {
	HeartbeatIntervalSeconds float64 `json:"heartbeatIntervalSeconds"`
	DegradedAfterSeconds     float64 `json:"degradedAfterSeconds"`
	OfflineAfterSeconds      float64 `json:"offlineAfterSeconds"`
}

type SystemInfo struct {
	Product           string            `json:"product"`
	APIVersion        string            `json:"apiVersion"`
	Version           string            `json:"version"`
	Commit            string            `json:"commit"`
	Stage             string            `json:"stage"`
	Architecture      string            `json:"architecture"`
	Persistence       string            `json:"persistence"`
	AgentHealthPolicy AgentHealthPolicy `json:"agentHealthPolicy"`
	Capabilities      []string          `json:"capabilities"`
}

type Dependencies struct {
	Logger           *slog.Logger
	Readiness        ReadinessChecker
	Operations       operation.Reader
	Tenancy          TenancyStore
	Catalog          CatalogStore
	Authenticator    authn.Authenticator
	Authorization    ports.AuthorizationEngine
	ReadinessTimeout time.Duration
	Info             SystemInfo
}

func NewHandler(dependencies Dependencies) http.Handler {
	logger := dependencies.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if dependencies.ReadinessTimeout <= 0 {
		dependencies.ReadinessTimeout = 2 * time.Second
	}
	metrics := NewMetrics()
	mux := http.NewServeMux()
	registerGET(mux, "/health/live", http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
	}))
	registerGET(mux, "/health/ready", readinessHandler(dependencies.Readiness, dependencies.ReadinessTimeout))
	registerGET(mux, "/metrics", metrics.Handler(dependencies.Info))
	registerGET(mux, "/api/v1/system/info", http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, dependencies.Info)
	}))
	registerGET(mux, "/api/v1/operations/{operationID}", operationHandler(dependencies.Operations))
	registerTenancyRoutes(mux, dependencies)
	registerCatalogRoutes(mux, dependencies)
	mux.HandleFunc("/", routeNotFoundHandler)

	var handler http.Handler = mux
	handler = metrics.Middleware(handler)
	handler = loggingMiddleware(logger, handler)
	handler = recoveryMiddleware(logger, metrics, handler)
	handler = requestIDMiddleware(handler)
	return handler
}

func registerGET(mux *http.ServeMux, pattern string, handler http.Handler) {
	guarded := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowedHandler(response, request)
			return
		}
		handler.ServeHTTP(response, request)
	})
	mux.Handle("GET "+pattern, guarded)
	mux.HandleFunc(pattern, methodNotAllowedHandler)
}

func methodNotAllowedHandler(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Allow", http.MethodGet)
	writeProblem(response, request, Problem{
		Title:  "Method Not Allowed",
		Status: http.StatusMethodNotAllowed,
		Detail: "This resource supports GET requests.",
		Code:   "method_not_allowed",
	})
}

func routeNotFoundHandler(response http.ResponseWriter, request *http.Request) {
	writeProblem(response, request, Problem{
		Title:  "Route not found",
		Status: http.StatusNotFound,
		Detail: "No API route matches the requested path.",
		Code:   "route_not_found",
	})
}

func readinessHandler(checker ReadinessChecker, timeout time.Duration) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if checker == nil {
			writeProblem(response, request, Problem{
				Title:  "Service Unavailable",
				Status: http.StatusServiceUnavailable,
				Detail: "PostgreSQL readiness check is unavailable.",
				Code:   "database_unavailable",
			})
			return
		}
		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(request.Context(), timeout)
		defer cancel()
		if err := checker.PingContext(ctx); err != nil {
			writeProblem(response, request, Problem{
				Title:  "Service Unavailable",
				Status: http.StatusServiceUnavailable,
				Detail: "PostgreSQL is unavailable.",
				Code:   "database_unavailable",
			})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"status": "ready",
			"checks": map[string]any{
				"postgresql": map[string]any{
					"status":    "ready",
					"latencyMs": time.Since(startedAt).Milliseconds(),
				},
			},
		})
	}
}

func operationHandler(repository operation.Reader) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		operationID := request.PathValue("operationID")
		if !identity.IsUUID(operationID) {
			writeProblem(response, request, Problem{
				Title:  "Invalid operation ID",
				Status: http.StatusBadRequest,
				Detail: "The operation ID must be a UUID.",
				Code:   "invalid_operation_id",
			})
			return
		}
		if repository == nil {
			writeProblem(response, request, Problem{
				Title:  "Service Unavailable",
				Status: http.StatusServiceUnavailable,
				Detail: "Operation storage is unavailable.",
				Code:   "operation_storage_unavailable",
			})
			return
		}
		result, err := repository.GetByID(request.Context(), operationID)
		if errors.Is(err, operation.ErrNotFound) {
			writeProblem(response, request, Problem{
				Title:  "Operation not found",
				Status: http.StatusNotFound,
				Detail: "No operation exists for the supplied ID.",
				Code:   "operation_not_found",
			})
			return
		}
		if err != nil {
			writeProblem(response, request, Problem{
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
				Detail: "The operation could not be loaded.",
				Code:   "operation_read_failed",
			})
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}
