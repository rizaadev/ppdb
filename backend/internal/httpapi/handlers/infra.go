package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
)

// InfraHandler handles liveness checks and internal Caddy On-Demand TLS verification.
type InfraHandler struct {
	Pool         *pgxpool.Pool
	TenantLookup middleware.TenantLookup
}

// Healthz verifies API responsiveness and PostgreSQL connectivity if configured.
func (h *InfraHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	dbStatus := "demo_in_memory"
	if h.Pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.Pool.Ping(ctx); err != nil {
			respond.Error(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Koneksi database gagal.", nil)
			return
		}
		dbStatus = "connected"
	}

	respond.JSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"database": dbStatus,
	}, nil)
}

// VerifyCaddyTLS validates whether a custom domain belongs to an active tenant before Caddy issues SSL.
func (h *InfraHandler) VerifyCaddyTLS(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if domain == "" || h.TenantLookup == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	tenant, err := h.TenantLookup.FindByHost(r.Context(), domain)
	if err != nil || tenant == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	w.WriteHeader(http.StatusOK)
}

