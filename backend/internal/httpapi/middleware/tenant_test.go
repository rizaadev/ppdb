package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

type stubTenantLookup struct {
	byHost map[string]*ResolvedTenant
	bySlug map[string]*ResolvedTenant
}

func (s *stubTenantLookup) FindByHost(_ context.Context, host string) (*ResolvedTenant, error) {
	return s.byHost[host], nil
}

func (s *stubTenantLookup) FindBySlug(_ context.Context, slug string) (*ResolvedTenant, error) {
	return s.bySlug[slug], nil
}

func TestTenantResolver(t *testing.T) {
	customTenant := &ResolvedTenant{
		ID:                uuid.New(),
		Slug:              "al-hamid",
		Name:              "Madrasah Al-Hamid",
		TerminologyPreset: domain.PresetPMBM,
	}
	subTenant := &ResolvedTenant{
		ID:                uuid.New(),
		Slug:              "sma-harapan",
		Name:              "SMA Harapan",
		TerminologyPreset: domain.PresetSPMB,
	}

	lookup := &stubTenantLookup{
		byHost: map[string]*ResolvedTenant{"pmbm.alhamid.sch.id": customTenant},
		bySlug: map[string]*ResolvedTenant{"sma-harapan": subTenant, "al-hamid": customTenant},
	}

	handler := TenantResolver(lookup, "spmb-platform.id", "development", "sma-harapan")(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resolved, ok := TenantFromContext(r.Context())
			if !ok {
				t.Fatal("expected resolved tenant in context")
			}
			w.Write([]byte(resolved.Slug))
		}),
	)

	t.Run("resolves verified custom domain", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/public/portal", nil)
		req.Host = "pmbm.alhamid.sch.id:443"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || rec.Body.String() != "al-hamid" {
			t.Fatalf("got code=%d body=%q", rec.Code, rec.Body.String())
		}
	})

	t.Run("resolves subdomain slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/public/portal", nil)
		req.Host = "sma-harapan.spmb-platform.id"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || rec.Body.String() != "sma-harapan" {
			t.Fatalf("got code=%d body=%q", rec.Code, rec.Body.String())
		}
	})

	t.Run("resolves verified custom domain matching baseDomain in production", func(t *testing.T) {
		prodLookup := &stubTenantLookup{
			byHost: map[string]*ResolvedTenant{"alhamidcintamulya.fun": customTenant},
			bySlug: map[string]*ResolvedTenant{},
		}
		prodHandler := TenantResolver(prodLookup, "alhamidcintamulya.fun", "production", "")(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resolved, _ := TenantFromContext(r.Context())
				w.Write([]byte(resolved.Slug))
			}),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/public/portal", nil)
		req.Host = "alhamidcintamulya.fun"
		rec := httptest.NewRecorder()
		prodHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || rec.Body.String() != "al-hamid" {
			t.Fatalf("got code=%d body=%q", rec.Code, rec.Body.String())
		}
	})

	t.Run("resolves X-Tenant-Slug header in development", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/public/portal", nil)
		req.Host = "localhost:8080"
		req.Header.Set("X-Tenant-Slug", "al-hamid")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || rec.Body.String() != "al-hamid" {
			t.Fatalf("got code=%d body=%q", rec.Code, rec.Body.String())
		}
	})
}

