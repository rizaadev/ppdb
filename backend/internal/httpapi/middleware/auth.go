package middleware

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/auth"
)

const principalContextKey contextKey = "auth_principal"

// Principal represents an authenticated caller extracted from a verified JWT access token.
type Principal struct {
	UserID          uuid.UUID
	TenantID        uuid.UUID
	IsPlatformAdmin bool
	Roles           []domain.RoleScope
}

// PrincipalFromContext retrieves the authenticated principal from request context if present.
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalContextKey).(*Principal)
	return p, ok && p != nil
}

// ContextWithPrincipal attaches a Principal to ctx (primarily useful in unit tests).
func ContextWithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, p)
}

// Authenticate validates Bearer JWT tokens and enforces tenant binding against ResolvedTenant.
func Authenticate(jwtMgr *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken, ok := extractBearerToken(r.Header.Get("Authorization"))
			if !ok || jwtMgr == nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Autentikasi Bearer token diperlukan.", nil)
				return
			}

			claims, err := jwtMgr.Verify(rawToken)
			if err != nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token akses tidak valid atau sudah kedaluwarsa.", nil)
				return
			}

			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Identitas pengguna pada token tidak valid.", nil)
				return
			}

			if tenant, hasTenant := TenantFromContext(r.Context()); hasTenant && !claims.IsPlatformAdmin && claims.TenantID != tenant.ID {
				respond.Error(w, http.StatusUnauthorized, "TOKEN_TENANT_MISMATCH", "Token akses tidak berlaku untuk portal instansi ini.", nil)
				return
			}

			principal := &Principal{
				UserID:          userID,
				TenantID:        claims.TenantID,
				IsPlatformAdmin: claims.IsPlatformAdmin,
				Roles:           claims.Roles,
			}
			ctx := context.WithValue(r.Context(), principalContextKey, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRoles restricts route access to platform admins or callers holding at least one allowed role in the tenant.
func RequireRoles(allowed ...domain.TenantRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Autentikasi diperlukan.", nil)
				return
			}

			if principal.IsPlatformAdmin {
				next.ServeHTTP(w, r)
				return
			}

			targetTenantID := principal.TenantID
			if tenant, hasTenant := TenantFromContext(r.Context()); hasTenant {
				targetTenantID = tenant.ID
			}

			if !hasTenantRole(principal.Roles, targetTenantID, allowed) {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki hak akses untuk operasi ini.", nil)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePlatformAdmin restricts access exclusively to authenticated SaaS platform superadmin users.
func RequirePlatformAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				respond.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Autentikasi diperlukan.", nil)
				return
			}
			if !principal.IsPlatformAdmin {
				respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Hanya Superadmin Platform yang diizinkan mengakses resource ini.", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HasUnitAccess reports whether principal can access unitID with any of the allowed roles (tenant-wide or matching unit).
func HasUnitAccess(principal *Principal, unitID uuid.UUID, allowed ...domain.TenantRole) bool {
	if principal == nil {
		return false
	}
	if principal.IsPlatformAdmin {
		return true
	}

	for _, scope := range principal.Roles {
		if scope.TenantID != principal.TenantID {
			continue
		}
		if scope.UnitID != nil && *scope.UnitID != unitID {
			continue
		}
		if len(allowed) == 0 || slices.Contains(allowed, scope.Role) {
			return true
		}
	}
	return false
}

func hasTenantRole(roles []domain.RoleScope, tenantID uuid.UUID, allowed []domain.TenantRole) bool {
	for _, scope := range roles {
		if scope.TenantID != tenantID {
			continue
		}
		if len(allowed) == 0 || slices.Contains(allowed, scope.Role) {
			return true
		}
	}
	return false
}

func extractBearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

