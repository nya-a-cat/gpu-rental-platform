package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	authorizationengine "github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/authorization"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/identity"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
	platformpostgres "github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/platform/postgres"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/tenancy"
	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/migrations"
)

func TestTenancyIdempotencyRoleBindingsAndQuotaLifecycle(t *testing.T) {
	database := openTenancyIntegrationDatabase(t)
	repository := NewRepository(database)
	fixedNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	repository.now = func() time.Time { return fixedNow }
	ctx := context.Background()

	tenantMutation := tenancy.MutationContext{
		PrincipalID:    "break-glass-admin",
		RequestID:      "request-tenant-create",
		IdempotencyKey: "tenant-create-key",
		RequestHash:    strings.Repeat("a", 64),
	}
	createdTenant, err := repository.CreateTenant(ctx, tenancy.CreateTenantParams{
		Mutation: tenantMutation,
		Name:     "Example Tenant",
		Slug:     "example-tenant",
	})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	if !identity.IsUUID(createdTenant.ResourceID) || !identity.IsUUID(createdTenant.OperationID) || createdTenant.Replayed {
		t.Fatalf("created tenant acceptance = %#v", createdTenant)
	}
	replayedTenant, err := repository.CreateTenant(ctx, tenancy.CreateTenantParams{
		Mutation: tenantMutation,
		Name:     "Example Tenant",
		Slug:     "example-tenant",
	})
	if err != nil {
		t.Fatalf("CreateTenant() replay error = %v", err)
	}
	if replayedTenant.ResourceID != createdTenant.ResourceID || replayedTenant.OperationID != createdTenant.OperationID || !replayedTenant.Replayed {
		t.Fatalf("replayed tenant acceptance = %#v, want IDs %#v", replayedTenant, createdTenant)
	}
	conflictingMutation := tenantMutation
	conflictingMutation.RequestHash = strings.Repeat("b", 64)
	if _, err := repository.CreateTenant(ctx, tenancy.CreateTenantParams{
		Mutation: conflictingMutation,
		Name:     "Changed Tenant",
		Slug:     "changed-tenant",
	}); !errors.Is(err, tenancy.ErrIdempotencyConflict) {
		t.Fatalf("CreateTenant() conflicting replay error = %v, want ErrIdempotencyConflict", err)
	}
	tenant, err := repository.GetTenant(ctx, createdTenant.ResourceID)
	if err != nil {
		t.Fatalf("GetTenant() error = %v", err)
	}
	if tenant.Name != "Example Tenant" || tenant.Slug != "example-tenant" || tenant.Status != "active" || tenant.Generation != 1 {
		t.Fatalf("tenant = %#v", tenant)
	}

	projectAcceptance, err := repository.CreateProject(ctx, tenancy.CreateProjectParams{
		Mutation: tenancy.MutationContext{
			PrincipalID:    "break-glass-admin",
			RequestID:      "request-project-create",
			IdempotencyKey: "project-create-key",
			RequestHash:    strings.Repeat("c", 64),
		},
		TenantID:       tenant.ID,
		Name:           "Research Project",
		Slug:           "research-project",
		IsolationClass: tenancy.IsolationShared,
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	project, err := repository.GetProject(ctx, projectAcceptance.ResourceID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if project.TenantID != tenant.ID || project.IsolationClass != tenancy.IsolationShared || project.DesiredState != "active" || project.ObservedState != "pending" || project.ProvisioningState != "pending" || len(project.Conditions) != 0 {
		t.Fatalf("project = %#v", project)
	}
	if !strings.HasPrefix(project.NamespaceName, "gpu-p-") {
		t.Fatalf("project namespace = %q", project.NamespaceName)
	}
	if _, err := repository.CreateProject(ctx, tenancy.CreateProjectParams{
		Mutation: tenancy.MutationContext{
			PrincipalID:    "break-glass-admin",
			RequestID:      "request-dedicated-project",
			IdempotencyKey: "dedicated-project-key",
			RequestHash:    strings.Repeat("d", 64),
		},
		TenantID:       tenant.ID,
		Name:           "Dedicated Project",
		Slug:           "dedicated-project",
		IsolationClass: tenancy.IsolationDedicatedNodePool,
	}); err == nil {
		t.Fatal("CreateProject() dedicated isolation error = nil")
	}

	bindingAcceptance, err := repository.CreateRoleBinding(ctx, tenancy.CreateRoleBindingParams{
		Mutation: tenancy.MutationContext{
			PrincipalID:    "break-glass-admin",
			RequestID:      "request-role-binding",
			IdempotencyKey: "role-binding-key",
			RequestHash:    strings.Repeat("e", 64),
		},
		ScopeType:   tenancy.ScopeTenant,
		ScopeID:     tenant.ID,
		SubjectType: tenancy.SubjectUser,
		SubjectID:   "owner@example.test",
		Role:        tenancy.RoleTenantOwner,
	})
	if err != nil {
		t.Fatalf("CreateRoleBinding() error = %v", err)
	}
	binding, err := repository.GetRoleBinding(ctx, bindingAcceptance.ResourceID)
	if err != nil {
		t.Fatalf("GetRoleBinding() error = %v", err)
	}
	if binding.ScopeID != tenant.ID || binding.SubjectID != "owner@example.test" || binding.Role != tenancy.RoleTenantOwner || binding.CreatedBy != "break-glass-admin" {
		t.Fatalf("binding = %#v", binding)
	}
	authorization := authorizationengine.NewPostgresEngine(database)
	decision, err := authorization.Authorize(ctx, ports.AuthorizationRequest{
		SubjectID: "owner@example.test",
		Action:    "project.create",
		ScopeType: string(tenancy.ScopeTenant),
		ScopeID:   tenant.ID,
		Resource:  "project",
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("tenant owner project.create decision = %#v, error = %v", decision, err)
	}
	decision, err = authorization.Authorize(ctx, ports.AuthorizationRequest{
		SubjectID:  "owner@example.test",
		Action:     "project.read",
		ScopeType:  string(tenancy.ScopeProject),
		ScopeID:    project.ID,
		Resource:   "project",
		ResourceID: project.ID,
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("tenant owner inherited project.read decision = %#v, error = %v", decision, err)
	}

	quotaAcceptance, err := repository.SetQuota(ctx, tenancy.SetQuotaParams{
		Mutation: tenancy.MutationContext{
			PrincipalID:    "break-glass-admin",
			RequestID:      "request-quota-set",
			IdempotencyKey: "quota-set-key",
			RequestHash:    strings.Repeat("f", 64),
		},
		ProjectID:     project.ID,
		ResourceClass: "nvidia.com/gpu",
		HardLimit:     2,
	})
	if err == nil {
		t.Fatalf("SetQuota() accepted slash-containing class = %#v", quotaAcceptance)
	}
	quotaAcceptance, err = repository.SetQuota(ctx, tenancy.SetQuotaParams{
		Mutation: tenancy.MutationContext{
			PrincipalID:    "break-glass-admin",
			RequestID:      "request-quota-set",
			IdempotencyKey: "quota-set-key",
			RequestHash:    strings.Repeat("f", 64),
		},
		ProjectID:     project.ID,
		ResourceClass: "gpu.nvidia.full",
		HardLimit:     2,
	})
	if err != nil {
		t.Fatalf("SetQuota() error = %v", err)
	}
	if quotaAcceptance.ResourceID != project.ID+"/gpu.nvidia.full" {
		t.Fatalf("quota acceptance = %#v", quotaAcceptance)
	}
	quota, err := repository.GetQuota(ctx, project.ID, "gpu.nvidia.full")
	if err != nil {
		t.Fatalf("GetQuota() error = %v", err)
	}
	if quota.HardLimit != 2 || quota.Reserved != 0 || quota.Allocated != 0 || quota.Generation != 1 {
		t.Fatalf("initial quota = %#v", quota)
	}

	allocationOperation, err := repository.Create(ctx, operation.CreateParams{
		Kind:      "allocation.create",
		Target:    operation.ResourceRef{Type: "allocation", ID: "allocation-1"},
		Retryable: true,
		RequestID: "request-allocation-1",
	})
	if err != nil {
		t.Fatalf("create allocation operation: %v", err)
	}
	reservation, err := repository.ReserveQuota(ctx, tenancy.ReserveQuotaParams{
		ProjectID:     project.ID,
		ResourceClass: "gpu.nvidia.full",
		Amount:        2,
		OperationID:   allocationOperation.ID,
		ExpiresAt:     fixedNow.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("ReserveQuota() error = %v", err)
	}
	quota, err = repository.GetQuota(ctx, project.ID, "gpu.nvidia.full")
	if err != nil || quota.Reserved != 2 || quota.Allocated != 0 {
		t.Fatalf("reserved quota = %#v, error = %v", quota, err)
	}

	overflowOperation, err := repository.Create(ctx, operation.CreateParams{
		Kind:      "allocation.create",
		Target:    operation.ResourceRef{Type: "allocation", ID: "allocation-overflow"},
		Retryable: true,
		RequestID: "request-allocation-overflow",
	})
	if err != nil {
		t.Fatalf("create overflow operation: %v", err)
	}
	if _, err := repository.ReserveQuota(ctx, tenancy.ReserveQuotaParams{
		ProjectID:     project.ID,
		ResourceClass: "gpu.nvidia.full",
		Amount:        1,
		OperationID:   overflowOperation.ID,
		ExpiresAt:     fixedNow.Add(5 * time.Minute),
	}); !errors.Is(err, tenancy.ErrQuotaExceeded) {
		t.Fatalf("ReserveQuota() overflow error = %v, want ErrQuotaExceeded", err)
	}

	committed, err := repository.CommitQuotaReservation(ctx, reservation.ID)
	if err != nil {
		t.Fatalf("CommitQuotaReservation() error = %v", err)
	}
	if committed.Status != tenancy.ReservationCommitted {
		t.Fatalf("committed reservation = %#v", committed)
	}
	quota, err = repository.GetQuota(ctx, project.ID, "gpu.nvidia.full")
	if err != nil || quota.Reserved != 0 || quota.Allocated != 2 {
		t.Fatalf("committed quota = %#v, error = %v", quota, err)
	}

	if _, err := repository.SetQuota(ctx, tenancy.SetQuotaParams{
		Mutation: tenancy.MutationContext{
			PrincipalID:    "break-glass-admin",
			RequestID:      "request-quota-too-low",
			IdempotencyKey: "quota-too-low-key",
			RequestHash:    strings.Repeat("1", 64),
		},
		ProjectID:     project.ID,
		ResourceClass: "gpu.nvidia.full",
		HardLimit:     1,
	}); !errors.Is(err, tenancy.ErrQuotaBelowUsage) {
		t.Fatalf("SetQuota() below usage error = %v, want ErrQuotaBelowUsage", err)
	}

	released, err := repository.ReleaseQuotaReservation(ctx, reservation.ID, tenancy.ReservationReleased)
	if err != nil {
		t.Fatalf("ReleaseQuotaReservation() error = %v", err)
	}
	if released.Status != tenancy.ReservationReleased {
		t.Fatalf("released reservation = %#v", released)
	}
	replayedRelease, err := repository.ReleaseQuotaReservation(ctx, reservation.ID, tenancy.ReservationReleased)
	if err != nil || replayedRelease.Status != tenancy.ReservationReleased {
		t.Fatalf("ReleaseQuotaReservation() replay = %#v, error = %v", replayedRelease, err)
	}
	quota, err = repository.GetQuota(ctx, project.ID, "gpu.nvidia.full")
	if err != nil || quota.Reserved != 0 || quota.Allocated != 0 {
		t.Fatalf("released quota = %#v, error = %v", quota, err)
	}

	var tenantCount, idempotencyCount, auditCount int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM tenants").Scan(&tenantCount); err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM idempotency_records").Scan(&idempotencyCount); err != nil {
		t.Fatalf("count idempotency records: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE actor_id = 'break-glass-admin'").Scan(&auditCount); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if tenantCount != 1 || idempotencyCount != 4 || auditCount != 4 {
		t.Fatalf("tenant/idempotency/audit counts = %d/%d/%d, want 1/4/4", tenantCount, idempotencyCount, auditCount)
	}
}

func openTenancyIntegrationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	adminDatabase := stdlib.OpenDB(*adminConfig)
	t.Cleanup(func() { adminDatabase.Close() })
	if err := adminDatabase.PingContext(ctx); err != nil {
		t.Fatalf("ping integration PostgreSQL: %v", err)
	}
	randomID, err := identity.NewUUID()
	if err != nil {
		t.Fatalf("generate test schema ID: %v", err)
	}
	schema := "tenancy_test_" + randomID[:8]
	if _, err := adminDatabase.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema)); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = adminDatabase.ExecContext(cleanupContext, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
	})

	testConfig := adminConfig.Copy()
	if testConfig.RuntimeParams == nil {
		testConfig.RuntimeParams = map[string]string{}
	}
	testConfig.RuntimeParams["search_path"] = schema
	database := stdlib.OpenDB(*testConfig)
	t.Cleanup(func() { database.Close() })
	if err := platformpostgres.ApplyMigrations(ctx, database, migrations.Files, platformpostgres.MigrationOptions{
		LockTimeout:      5 * time.Second,
		StatementTimeout: 20 * time.Second,
	}); err != nil {
		t.Fatalf("apply integration migrations: %v", err)
	}
	return database
}
