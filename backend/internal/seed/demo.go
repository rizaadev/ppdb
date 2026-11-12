package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/modules/auth"
	"github.com/pbdb/platform/internal/modules/catalog"
)

// SeedDemoData idempotently populates the 3 white-label demo tenants, catalog tracks, and demo user accounts in PostgreSQL.
func SeedDemoData(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	if pool == nil {
		return fmt.Errorf("database pool is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return database.WithBypassRLSTx(ctx, pool, func(tx pgx.Tx) error {
		for _, tenant := range catalog.DemoTenants() {
			themeRaw, err := json.Marshal(map[string]any{
				"primary_color": tenant.PrimaryColor,
				"tagline":       tenant.Tagline,
			})
			if err != nil {
				return err
			}

			var customDomain *string
			if strings.TrimSpace(tenant.CustomDomain) != "" {
				d := strings.ToLower(strings.TrimSpace(tenant.CustomDomain))
				customDomain = &d
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO tenants (
					id, slug, name, institution_type, custom_domain, domain_verified_at,
					terminology_preset, theme_config, is_active
				) VALUES (
					$1, $2, $3, $4::institution_type, $5, CASE WHEN $5::text IS NOT NULL THEN NOW() ELSE NULL END,
					$6::terminology_preset, $7::jsonb, TRUE
				)
				ON CONFLICT (id) DO NOTHING
			`, tenant.ID, tenant.Slug, tenant.Name, string(tenant.InstitutionType), customDomain,
				string(tenant.TerminologyPreset), themeRaw)
			if err != nil {
				return fmt.Errorf("seed tenant %s: %w", tenant.Slug, err)
			}

			units, periods := catalog.PortalForTenant(tenant.Slug, tenant.Name, tenant.TerminologyPreset)
			for _, u := range units {
				_, err = tx.Exec(ctx, `
					INSERT INTO units (
						id, tenant_id, code, name, education_level, address, latitude, longitude, is_active
					) VALUES ($1, $2, $3, $4, $5::education_level, $6, $7, $8, TRUE)
					ON CONFLICT (id) DO NOTHING
				`, u.ID, tenant.ID, u.Code, u.Name, u.EducationLevel, u.Address, u.Latitude, u.Longitude)
				if err != nil {
					return fmt.Errorf("seed unit %s: %w", u.Code, err)
				}
			}

			for _, p := range periods {
				_, err = tx.Exec(ctx, `
					INSERT INTO admission_periods (
						id, tenant_id, unit_id, academic_year, name, opens_at, closes_at, is_published, is_active
					) VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE, TRUE)
					ON CONFLICT (id) DO NOTHING
				`, p.ID, tenant.ID, p.UnitID, p.AcademicYear, p.Name, p.OpensAt, p.ClosesAt)
				if err != nil {
					return fmt.Errorf("seed period %s: %w", p.Name, err)
				}

				// First pass inserts tracks without spillover FK so cross-track FK references resolve safely.
				for sortIdx, tr := range p.Tracks {
					_, err = tx.Exec(ctx, `
						INSERT INTO admission_tracks (
							id, tenant_id, period_id, code, display_name, description,
							selection_mode, quota, registration_fee_idr, re_enrollment_fee_idr,
							requires_coordinates, requires_achievements, sort_order, is_active
						) VALUES (
							$1, $2, $3, $4, $5, $6, $7::selection_mode, $8, $9, $10, $11, $12, $13, TRUE
						)
						ON CONFLICT (id) DO NOTHING
					`, tr.ID, tenant.ID, p.ID, tr.Code, tr.DisplayName, tr.Description,
						tr.SelectionMode, tr.Quota, tr.RegistrationFeeIDR, tr.ReEnrollmentFeeIDR,
						tr.RequiresCoordinates, tr.RequiresAchievements, sortIdx+1)
					if err != nil {
						return fmt.Errorf("seed track %s: %w", tr.Code, err)
					}
				}
				for _, tr := range p.Tracks {
					if tr.SpilloverTargetTrackID == nil {
						continue
					}
					_, err = tx.Exec(ctx, `
						UPDATE admission_tracks
						SET spillover_target_track_id = COALESCE(spillover_target_track_id, $3)
						WHERE tenant_id = $1 AND id = $2
					`, tenant.ID, tr.ID, *tr.SpilloverTargetTrackID)
					if err != nil {
						return fmt.Errorf("seed track spillover %s: %w", tr.Code, err)
					}
				}

				for _, tr := range p.Tracks {
					for reqIdx, r := range tr.Requirements {
						_, err = tx.Exec(ctx, `
							INSERT INTO track_requirements (
								id, tenant_id, track_id, doc_code, label, description,
								is_required, accepted_mimes, max_size_kb, sort_order
							) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
							ON CONFLICT (id) DO NOTHING
						`, r.ID, tenant.ID, tr.ID, r.DocCode, r.Label, r.Description,
							r.IsRequired, r.AcceptedMimes, r.MaxSizeKB, reqIdx+1)
						if err != nil {
							return fmt.Errorf("seed requirement %s: %w", r.DocCode, err)
						}
					}
				}
			}
		}

		for _, acc := range auth.DemoSeedAccounts() {
			u := acc.User
			_, err := tx.Exec(ctx, `
				INSERT INTO users (
					id, whatsapp_phone, email, password_hash, full_name, is_platform_admin, is_active, phone_verified_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (id) DO NOTHING
			`, u.ID, u.WhatsAppPhone, u.Email, u.PasswordHash, u.FullName, u.IsPlatformAdmin, u.IsActive, u.PhoneVerifiedAt)
			if err != nil {
				return fmt.Errorf("seed user %s: %w", u.FullName, err)
			}

			for _, scope := range acc.Roles {
				roleID := uuid.Must(uuid.NewV7())
				_, err = tx.Exec(ctx, `
					INSERT INTO user_tenant_roles (id, user_id, tenant_id, unit_id, role, is_active)
					VALUES ($1, $2, $3, $4, $5::tenant_role, TRUE)
					ON CONFLICT DO NOTHING
				`, roleID, u.ID, scope.TenantID, scope.UnitID, string(scope.Role))
				if err != nil {
					return fmt.Errorf("seed user role %s: %w", scope.Role, err)
				}
			}
		}

		logger.Info("seeded demo tenants, tracks, and user accounts")
		return nil
	})
}

