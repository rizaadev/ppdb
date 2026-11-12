package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/respond"
)

type contextKey string

const tenantContextKey contextKey = "resolved_tenant"

// ResolvedTenant represents lightweight tenant metadata attached to every incoming request.
type ResolvedTenant struct {
	ID                uuid.UUID                `json:"id"`
	Slug              string                   `json:"slug"`
	Name              string                   `json:"name"`
	InstitutionType   domain.InstitutionType   `json:"institution_type"`
	TerminologyPreset domain.TerminologyPreset `json:"terminology_preset"`
	PrimaryColor      string                   `json:"primary_color"`
	Tagline           string                   `json:"tagline"`
	CustomDomain      string                   `json:"-"`
}

// TenantLookup resolves a tenant from either a verified custom domain or a platform subdomain slug.
type TenantLookup interface {
	FindByHost(ctx context.Context, host string) (*ResolvedTenant, error)
	FindBySlug(ctx context.Context, slug string) (*ResolvedTenant, error)
}

// TenantFromContext retrieves the resolved tenant from request context if present.
func TenantFromContext(ctx context.Context) (*ResolvedTenant, bool) {
	t, ok := ctx.Value(tenantContextKey).(*ResolvedTenant)
	return t, ok && t != nil
}

// TenantResolver resolves the active tenant from Host (custom domain or subdomain) or X-Tenant-Slug in dev mode.
func TenantResolver(lookup TenantLookup, baseDomain, appEnv, devFallbackSlug string) func(http.Handler) http.Handler {
	return tenantResolverWithMode(lookup, baseDomain, appEnv, devFallbackSlug, true)
}

// OptionalTenantResolver attempts to attach the resolved tenant to context when matched by Host or dev X-Tenant-Slug,
// allowing central endpoints (such as payment gateway webhooks without tenant subdomains) to fall through without 404.
func OptionalTenantResolver(lookup TenantLookup, baseDomain, appEnv string) func(http.Handler) http.Handler {
	return tenantResolverWithMode(lookup, baseDomain, appEnv, "", false)
}

func tenantResolverWithMode(lookup TenantLookup, baseDomain, appEnv, devFallbackSlug string, required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if lookup == nil {
				next.ServeHTTP(w, r)
				return
			}

			host := normalizeHost(r.Host)
			tenant, err := resolveTenantForRequest(r.Context(), lookup, host, r.Header.Get("X-Tenant-Slug"), baseDomain, appEnv, devFallbackSlug)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "TENANT_RESOLUTION_ERROR", "Gagal memproses identitas instansi.", nil)
				return
			}
			if tenant == nil {
				if !required {
					next.ServeHTTP(w, r)
					return
				}
				respond.Error(w, http.StatusNotFound, "TENANT_NOT_FOUND", "Portal sekolah atau madrasah tidak ditemukan.", nil)
				return
			}

			ctx := context.WithValue(r.Context(), tenantContextKey, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveTenantForRequest(
	ctx context.Context,
	lookup TenantLookup,
	host, slugHeader, baseDomain, appEnv, devFallbackSlug string,
) (*ResolvedTenant, error) {
	if appEnv == "development" {
		if slug := strings.ToLower(strings.TrimSpace(slugHeader)); slug != "" {
			return lookup.FindBySlug(ctx, slug)
		}
	}

	if host != "" && !isLoopbackHost(host) {
		if host != baseDomain {
			if slug, ok := extractSubdomainSlug(host, baseDomain); ok {
				if t, err := lookup.FindBySlug(ctx, slug); err != nil || t != nil {
					return t, err
				}
			}
		}
		if t, err := lookup.FindByHost(ctx, host); err != nil || t != nil {
			return t, err
		}
	}

	if appEnv == "development" && devFallbackSlug != "" {
		return lookup.FindBySlug(ctx, devFallbackSlug)
	}

	return nil, nil
}

func normalizeHost(rawHost string) string {
	host := strings.ToLower(strings.TrimSpace(rawHost))
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return parsedHost
	}
	return host
}

func extractSubdomainSlug(host, baseDomain string) (string, bool) {
	if baseDomain == "" || !strings.HasSuffix(host, "."+baseDomain) {
		return "", false
	}
	sub := strings.TrimSuffix(host, "."+baseDomain)
	if sub == "" || strings.Contains(sub, ".") || sub == "www" {
		return "", false
	}
	return sub, true
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

