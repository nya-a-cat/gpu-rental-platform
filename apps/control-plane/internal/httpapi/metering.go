package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authn"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/metering"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
)

type MeteringStore interface {
	CreateUsageFact(context.Context, metering.CreateUsageFactParams) (tenancy.Acceptance, error)
	GetUsageFact(context.Context, string) (metering.UsageFact, error)
}

type createUsageFactRequest struct {
	TenantID       string          `json:"tenantId"`
	ProjectID      string          `json:"projectId"`
	ResourceClass  string          `json:"resourceClass"`
	Quantity       string          `json:"quantity"`
	AllocationFrom time.Time       `json:"allocationFrom"`
	AllocationTo   time.Time       `json:"allocationTo"`
	Attributes     json.RawMessage `json:"attributes"`
}

type createInvoiceRequest struct {
	TenantID   string    `json:"tenantId"`
	ProjectID  string    `json:"projectId"`
	PeriodFrom time.Time `json:"periodFrom"`
	PeriodTo   time.Time `json:"periodTo"`
}

type createCreditAdjustmentRequest struct {
	TenantID    string `json:"tenantId"`
	ProjectID   string `json:"projectId"`
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
	ReferenceID string `json:"referenceId"`
	Description string `json:"description"`
}

type setBudgetRequest struct {
	TenantID   string `json:"tenantId"`
	Currency   string `json:"currency"`
	LimitMinor int64  `json:"limitMinor"`
}

func registerMeteringRoutes(mux *http.ServeMux, dependencies Dependencies) {
	registerMethods(mux, "/api/v1/usage-facts", map[string]http.Handler{http.MethodPost: createUsageFactHandler(dependencies)})
	registerMethods(mux, "/api/v1/usage-facts/{usageFactID}", map[string]http.Handler{http.MethodGet: getUsageFactHandler(dependencies)})
	registerMethods(mux, "/api/v1/invoices", map[string]http.Handler{http.MethodPost: createInvoiceHandler(dependencies)})
	registerMethods(mux, "/api/v1/invoices/{invoiceID}", map[string]http.Handler{http.MethodGet: getInvoiceHandler(dependencies)})
	registerMethods(mux, "/api/v1/ledger-adjustments", map[string]http.Handler{http.MethodPost: createCreditAdjustmentHandler(dependencies)})
	registerMethods(mux, "/api/v1/projects/{projectID}/budget", map[string]http.Handler{http.MethodPut: setBudgetHandler(dependencies), http.MethodGet: getBudgetHandler(dependencies)})
}

func meteringPrincipal(response http.ResponseWriter, request *http.Request, dependencies Dependencies) (authn.Principal, bool) {
	principal, ok := authenticateRequest(response, request, dependencies.Authenticator)
	if !ok {
		return authn.Principal{}, false
	}
	if !principal.SystemAdmin {
		writeProblem(response, request, Problem{Title: "Forbidden", Status: http.StatusForbidden, Detail: "System administrator access is required for usage fact ingestion.", Code: "forbidden"})
		return authn.Principal{}, false
	}
	if dependencies.Metering == nil {
		writeProblem(response, request, Problem{Title: "Service Unavailable", Status: http.StatusServiceUnavailable, Detail: "Usage fact storage is unavailable.", Code: "metering_storage_unavailable"})
		return authn.Principal{}, false
	}
	return principal, true
}

func createUsageFactHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := meteringPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input createUsageFactRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Metering.CreateUsageFact(request.Context(), metering.CreateUsageFactParams{Mutation: mutation, TenantID: input.TenantID, ProjectID: input.ProjectID, ResourceClass: input.ResourceClass, Quantity: input.Quantity, AllocationFrom: input.AllocationFrom, AllocationTo: input.AllocationTo, Attributes: input.Attributes})
		if err != nil {
			writeMeteringError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/usage-facts/"+accepted.ResourceID, accepted)
	}
}

func getUsageFactHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := meteringPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Metering.GetUsageFact(request.Context(), request.PathValue("usageFactID"))
		if err != nil {
			writeMeteringError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func createInvoiceHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := meteringPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input createInvoiceRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Metering.CreateInvoice(request.Context(), metering.CreateInvoiceParams{Mutation: mutation, TenantID: input.TenantID, ProjectID: input.ProjectID, PeriodFrom: input.PeriodFrom, PeriodTo: input.PeriodTo})
		if err != nil {
			writeMeteringError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/invoices/"+accepted.ResourceID, accepted)
	}
}

func getInvoiceHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := meteringPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Metering.GetInvoice(request.Context(), request.PathValue("invoiceID"))
		if err != nil {
			writeMeteringError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func createCreditAdjustmentHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := meteringPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input createCreditAdjustmentRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Metering.CreateCreditAdjustment(request.Context(), metering.CreateCreditAdjustmentParams{Mutation: mutation, TenantID: input.TenantID, ProjectID: input.ProjectID, AmountMinor: input.AmountMinor, Currency: input.Currency, ReferenceID: input.ReferenceID, Description: input.Description})
		if err != nil {
			writeMeteringError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/ledger-adjustments/"+accepted.ResourceID, accepted)
	}
}

func setBudgetHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := meteringPrincipal(response, request, dependencies)
		if !ok {
			return
		}
		var input setBudgetRequest
		if !decodeRequestJSON(response, request, &input) {
			return
		}
		mutation, ok := mutationContext(response, request, principal, input)
		if !ok {
			return
		}
		accepted, err := dependencies.Metering.SetBudget(request.Context(), metering.SetBudgetParams{Mutation: mutation, TenantID: input.TenantID, ProjectID: request.PathValue("projectID"), Currency: input.Currency, LimitMinor: input.LimitMinor})
		if err != nil {
			writeMeteringError(response, request, err)
			return
		}
		writeAcceptance(response, http.StatusAccepted, "/api/v1/projects/"+request.PathValue("projectID")+"/budget", accepted)
	}
}

func getBudgetHandler(dependencies Dependencies) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if _, ok := meteringPrincipal(response, request, dependencies); !ok {
			return
		}
		result, err := dependencies.Metering.GetBudget(request.Context(), request.PathValue("projectID"))
		if err != nil {
			writeMeteringError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	}
}

func writeMeteringError(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, metering.ErrInvalid):
		writeProblem(response, request, Problem{Title: "Invalid request", Status: http.StatusBadRequest, Detail: "The usage fact payload is invalid.", Code: "invalid_usage_fact"})
	case errors.Is(err, metering.ErrNotFound):
		writeProblem(response, request, Problem{Title: "Resource not found", Status: http.StatusNotFound, Detail: "The usage fact does not exist.", Code: "usage_fact_not_found"})
	case errors.Is(err, metering.ErrConflict):
		writeProblem(response, request, Problem{Title: "Resource conflict", Status: http.StatusConflict, Detail: "The billing period already has an invoice or contains conflicting billing data.", Code: "billing_conflict"})
	case errors.Is(err, metering.ErrBudgetExceeded):
		writeProblem(response, request, Problem{Title: "Budget exceeded", Status: http.StatusConflict, Detail: "The project budget does not have enough available credit for this allocation.", Code: "budget_exceeded"})
	case errors.Is(err, tenancy.ErrIdempotencyConflict):
		writeProblem(response, request, Problem{Title: "Resource conflict", Status: http.StatusConflict, Detail: "The Idempotency-Key was already used with a different request.", Code: "idempotency_conflict"})
	default:
		writeProblem(response, request, Problem{Title: "Internal Server Error", Status: http.StatusInternalServerError, Detail: "The usage fact request could not be completed.", Code: "usage_fact_request_failed"})
	}
}
