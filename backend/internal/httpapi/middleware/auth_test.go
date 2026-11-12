package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/modules/auth"
)

type stubTenantLookup struct {
	tenants map[string]*middleware.ResolvedTenant
}

func (s *stubTenantLookup) FindByHost(_ context.Context, _ string) (*middleware.ResolvedTenant, error) {
	return nil, nil
}

func (s *stubTenantLookup) FindBySlug(_ context.Context, slug string) (*middleware.ResolvedTenant, error) {
	return s.tenants[slug], nil
}

func TestAuthenticateAndRequireRoles(t *testing.T) {
	tenantA := uuid.MustParse("01960000-0000-7000-8000-000000000001")
	tenantB := uuid.MustParse("01960000-0000-7000-8000-000000000002")
	userID := uuid.MustParse("01960000-0000-7000-8001-000000000010")

	lookup := &stubTenantLookup{
		tenants: map[string]*middleware.ResolvedTenant{
			"tenant-a": {ID: tenantA, Slug: "tenant-a", Name: "Tenant A"},
			"tenant-b": {ID: tenantB, Slug: "tenant-b", Name: "Tenant B"},
		},
	}

	jwtMgr := auth.NewJWTManager("middleware-test-secret-key-32-bytes!", 15*time.Minute)

	adminToken, err := jwtMgr.Issue(userID, tenantA, false, []domain.RoleScope{
		{TenantID: tenantA, Role: domain.RoleTenantAdmin},
	})
	if err != nil {
		t.Fatalf("Issue admin token: %v", err)
	}
	verifierToken, err := jwtMgr.Issue(userID, tenantA, false, []domain.RoleScope{
		{TenantID: tenantA, Role: domain.RoleVerifier},
	})
	if err != nil {
		t.Fatalf("Issue verifier token: %v", err)
	}
	platformToken, err := jwtMgr.Issue(userID, tenantA, true, nil)
	if err != nil {
		t.Fatalf("Issue platform admin token: %v", err)
	}

	chain := middleware.TenantResolver(lookup, "pbdb.id", "development", "tenant-a")(
		middleware.Authenticate(jwtMgr)(
			middleware.RequireRoles(domain.RoleTenantAdmin)(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					p, ok := middleware.PrincipalFromContext(r.Context())
					if !ok || p.UserID != userID {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusOK)
				}),
			),
		),
	)

	cases := []struct {
		name       string
		slugHeader string
		authHeader string
		wantStatus int
	}{
		{
			name:       "missing authorization header",
			slugHeader: "tenant-a",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid bearer token",
			slugHeader: "tenant-a",
			authHeader: "Bearer invalid.jwt.token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "cross tenant token rejected",
			slugHeader: "tenant-b",
			authHeader: "Bearer " + adminToken,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong role rejected with forbidden",
			slugHeader: "tenant-a",
			authHeader: "Bearer " + verifierToken,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "matching tenant admin allowed",
			slugHeader: "tenant-a",
			authHeader: "Bearer " + adminToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "platform admin allowed across tenants",
			slugHeader: "tenant-b",
			authHeader: "Bearer " + platformToken,
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tenant/settings", nil)
			if tc.slugHeader != "" {
				req.Header.Set("X-Tenant-Slug", tc.slugHeader)
			}
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("got status %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestHasUnitAccess(t *testing.T) {
	tenantID := uuid.New()
	unitA := uuid.New()
	unitB := uuid.New()

	tenantWideAdmin := &middleware.Principal{
		UserID:   uuid.New(),
		TenantID: tenantID,
		Roles: []domain.RoleScope{
			{TenantID: tenantID, UnitID: nil, Role: domain.RoleTenantAdmin},
		},
	}
	unitAVerifier := &middleware.Principal{
		UserID:   uuid.New(),
		TenantID: tenantID,
		Roles: []domain.RoleScope{
			{TenantID: tenantID, UnitID: &unitA, Role: domain.RoleVerifier},
		},
	}
	platformAdmin := &middleware.Principal{
		UserID:          uuid.New(),
		TenantID:        tenantID,
		IsPlatformAdmin: true,
	}

	if middleware.HasUnitAccess(nil, unitA) {
		t.Fatal("nil principal must not have unit access")
	}
	if !middleware.HasUnitAccess(tenantWideAdmin, unitB, domain.RoleTenantAdmin) {
		t.Fatal("tenant_admin with nil UnitID must have access to any unit in tenant")
	}
	if !middleware.HasUnitAccess(unitAVerifier, unitA, domain.RoleVerifier) {
		t.Fatal("unit verifier must have access to assigned unitA")
	}
	if middleware.HasUnitAccess(unitAVerifier, unitB, domain.RoleVerifier) {
		t.Fatal("unit verifier must not have access to unassigned unitB")
	}
	if middleware.HasUnitAccess(unitAVerifier, unitA, domain.RoleTreasurer) {
		t.Fatal("unit verifier must not satisfy treasurer role requirement")
	}
	if !middleware.HasUnitAccess(platformAdmin, unitB, domain.RoleTreasurer) {
		t.Fatal("platform admin must bypass unit scope restrictions")
	}
}

