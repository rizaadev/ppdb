package public

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/httpapi/middleware"
)

// PGStore backs tenant lookup and public portal queries directly with PostgreSQL under kernel RLS.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore initializes a PostgreSQL-backed public portal store.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) FindByHost(ctx context.Context, host string) (*middleware.ResolvedTenant, error) {
	return s.findTenant(ctx, `
		SELECT id, slug, name, institution_type::text, terminology_preset::text, theme_config, COALESCE(custom_domain, '')
		FROM tenants
		WHERE LOWER(custom_domain) = LOWER($1) AND domain_verified_at IS NOT NULL AND is_active = TRUE
	`, strings.ToLower(strings.TrimSpace(host)))
}

func (s *PGStore) FindBySlug(ctx context.Context, slug string) (*middleware.ResolvedTenant, error) {
	return s.findTenant(ctx, `
		SELECT id, slug, name, institution_type::text, terminology_preset::text, theme_config, COALESCE(custom_domain, '')
		FROM tenants
		WHERE LOWER(slug) = LOWER($1) AND is_active = TRUE
	`, strings.ToLower(strings.TrimSpace(slug)))
}

func (s *PGStore) findTenant(ctx context.Context, query, arg string) (*middleware.ResolvedTenant, error) {
	var t middleware.ResolvedTenant
	var instType, preset string
	var themeRaw []byte

	err := s.pool.QueryRow(ctx, query, arg).Scan(
		&t.ID, &t.Slug, &t.Name, &instType, &preset, &themeRaw, &t.CustomDomain,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.InstitutionType = domain.InstitutionType(instType)
	t.TerminologyPreset = domain.TerminologyPreset(preset)
	theme := parseStoredThemeConfig(themeRaw)
	t.PrimaryColor = theme.PrimaryColor
	t.Tagline = theme.Tagline
	return &t, nil
}

func (s *PGStore) GetPortalData(ctx context.Context, tenant *middleware.ResolvedTenant) (handlers.PublicPortalPayload, error) {
	payload := handlers.PublicPortalPayload{
		Tenant: handlers.PublicTenantView{
			ID:                   tenant.ID,
			Slug:                 tenant.Slug,
			Name:                 tenant.Name,
			InstitutionType:      tenant.InstitutionType,
			TerminologyPreset:    tenant.TerminologyPreset,
			TerminologyOverrides: map[string]string{},
			ThemeConfig: handlers.ThemeConfigView{
				PrimaryColor: tenant.PrimaryColor,
				Tagline:      tenant.Tagline,
			},
		},
		Units:   []handlers.UnitView{},
		Periods: []handlers.PeriodView{},
	}

	err := database.WithTenantTx(ctx, s.pool, tenant.ID, func(tx pgx.Tx) error {
		var overridesRaw, themeRaw []byte
		if err := tx.QueryRow(ctx, `
			SELECT terminology_overrides, theme_config
			FROM tenants
			WHERE id = $1
		`, tenant.ID).Scan(&overridesRaw, &themeRaw); err == nil {
			if len(overridesRaw) > 0 {
				var overrides map[string]string
				if json.Unmarshal(overridesRaw, &overrides) == nil && overrides != nil {
					payload.Tenant.TerminologyOverrides = overrides
				}
			}
			theme := toThemeConfigView(parseStoredThemeConfig(themeRaw))
			payload.Tenant.ThemeConfig.PrimaryColor = theme.PrimaryColor
			payload.Tenant.ThemeConfig.LogoURL = theme.LogoURL
			if theme.Tagline != "" || payload.Tenant.ThemeConfig.Tagline == "" {
				payload.Tenant.ThemeConfig.Tagline = theme.Tagline
			}
		}

		unitRows, err := tx.Query(ctx, `
			SELECT id, code, name, education_level::text, address, latitude, longitude, is_active
			FROM units
			WHERE tenant_id = $1 AND is_active = TRUE
			ORDER BY name ASC
		`, tenant.ID)
		if err != nil {
			return err
		}
		defer unitRows.Close()

		for unitRows.Next() {
			var u handlers.UnitView
			if err := unitRows.Scan(&u.ID, &u.Code, &u.Name, &u.EducationLevel, &u.Address, &u.Latitude, &u.Longitude, &u.IsActive); err != nil {
				return err
			}
			payload.Units = append(payload.Units, u)
		}
		if err := unitRows.Err(); err != nil {
			return err
		}

		periodRows, err := tx.Query(ctx, `
			SELECT id, unit_id, academic_year, name, opens_at, closes_at, announcement_at, results_published_at, is_published, is_active
			FROM admission_periods
			WHERE tenant_id = $1 AND is_active = TRUE AND is_published = TRUE
			ORDER BY opens_at ASC
		`, tenant.ID)
		if err != nil {
			return err
		}
		defer periodRows.Close()

		periodIndex := make(map[uuid.UUID]int)
		for periodRows.Next() {
			var p handlers.PeriodView
			if err := periodRows.Scan(&p.ID, &p.UnitID, &p.AcademicYear, &p.Name, &p.OpensAt, &p.ClosesAt, &p.AnnouncementAt, &p.ResultsPublishedAt, &p.IsPublished, &p.IsActive); err != nil {
				return err
			}
			p.Tracks = []handlers.TrackView{}
			periodIndex[p.ID] = len(payload.Periods)
			payload.Periods = append(payload.Periods, p)
		}
		if err := periodRows.Err(); err != nil {
			return err
		}

		trackRows, err := tx.Query(ctx, `
			SELECT t.id, t.period_id, p.unit_id, t.code, t.display_name, t.description, t.selection_mode::text,
			       t.quota, t.registration_fee_idr, t.re_enrollment_fee_idr, t.spillover_target_track_id,
			       t.requires_coordinates, t.requires_achievements,
			       t.min_age_months, t.max_age_months, TO_CHAR(t.age_reference_date, 'YYYY-MM-DD'),
			       COALESCE(t.scoring_weights, '{}'::jsonb), COALESCE(t.custom_form_schema, '[]'::jsonb),
			       t.is_active,
			       COUNT(a.id) FILTER (WHERE a.status NOT IN ('withdrawn', 'rejected'))::integer AS active_applicant_count
			FROM admission_tracks t
			JOIN admission_periods p ON p.id = t.period_id AND p.tenant_id = t.tenant_id
			LEFT JOIN applications a ON a.track_id = t.id AND a.tenant_id = t.tenant_id
			WHERE t.tenant_id = $1 AND t.is_active = TRUE
			GROUP BY t.id, p.unit_id
			ORDER BY t.sort_order ASC, t.created_at ASC
		`, tenant.ID)
		if err != nil {
			return err
		}
		defer trackRows.Close()

		type trackLoc struct {
			periodIdx int
			trackIdx  int
		}
		trackLocs := make(map[uuid.UUID]trackLoc)

		for trackRows.Next() {
			var tr handlers.TrackView
			var weightsRaw, schemaRaw []byte
			if err := trackRows.Scan(
				&tr.ID, &tr.PeriodID, &tr.UnitID, &tr.Code, &tr.DisplayName, &tr.Description, &tr.SelectionMode,
				&tr.Quota, &tr.RegistrationFeeIDR, &tr.ReEnrollmentFeeIDR, &tr.SpilloverTargetTrack,
				&tr.RequiresCoordinates, &tr.RequiresAchievements,
				&tr.MinAgeMonths, &tr.MaxAgeMonths, &tr.AgeReferenceDate,
				&weightsRaw, &schemaRaw, &tr.IsActive,
				&tr.ActiveApplicantCount,
			); err != nil {
				return err
			}
			if len(weightsRaw) > 0 {
				_ = json.Unmarshal(weightsRaw, &tr.ScoringWeights)
			}
			if len(schemaRaw) > 0 {
				_ = json.Unmarshal(schemaRaw, &tr.CustomFormSchema)
			}
			tr.Requirements = []handlers.RequirementView{}
			if idx, exists := periodIndex[tr.PeriodID]; exists {
				trackIdx := len(payload.Periods[idx].Tracks)
				payload.Periods[idx].Tracks = append(payload.Periods[idx].Tracks, tr)
				trackLocs[tr.ID] = trackLoc{periodIdx: idx, trackIdx: trackIdx}
			}
		}
		if err := trackRows.Err(); err != nil {
			return err
		}

		reqRows, err := tx.Query(ctx, `
			SELECT id, track_id, doc_code, label, description, is_required, accepted_mimes, max_size_kb
			FROM track_requirements
			WHERE tenant_id = $1
			ORDER BY sort_order ASC, created_at ASC
		`, tenant.ID)
		if err != nil {
			return err
		}
		defer reqRows.Close()

		for reqRows.Next() {
			var trackID uuid.UUID
			var reqView handlers.RequirementView
			if err := reqRows.Scan(
				&reqView.ID, &trackID, &reqView.DocCode, &reqView.Label,
				&reqView.Description, &reqView.IsRequired, &reqView.AcceptedMimes, &reqView.MaxSizeKB,
			); err != nil {
				return err
			}
			if loc, exists := trackLocs[trackID]; exists {
				tr := &payload.Periods[loc.periodIdx].Tracks[loc.trackIdx]
				tr.Requirements = append(tr.Requirements, reqView)
			}
		}
		return reqRows.Err()
	})
	return payload, err
}

func (s *PGStore) CheckStatus(ctx context.Context, tenantID uuid.UUID, registrationNo string) (*handlers.PublicStatusResult, error) {
	var res *handlers.PublicStatusResult
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var item handlers.PublicStatusResult
		var fullName *string
		var statusStr string

		err := tx.QueryRow(ctx, `
			SELECT a.registration_no, pr.full_name, u.name, t.display_name, p.academic_year, a.status::text, a.submitted_at
			FROM applications a
			JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
			JOIN admission_periods p ON p.id = a.period_id AND p.tenant_id = a.tenant_id
			JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
			LEFT JOIN applicant_profiles pr ON pr.application_id = a.id AND pr.tenant_id = a.tenant_id
			WHERE a.tenant_id = $1 AND a.registration_no = $2
		`, tenantID, registrationNo).Scan(
			&item.RegistrationNo, &fullName, &item.UnitName, &item.TrackName, &item.AcademicYear, &statusStr, &item.SubmittedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		item.Status = domain.ApplicationStatus(statusStr)
		if fullName != nil {
			item.MaskedName = maskFullName(*fullName)
		}
		res = &item
		return nil
	})
	return res, err
}

func (s *PGStore) GetTenantLogoKey(ctx context.Context, tenantID uuid.UUID) (string, error) {
	var themeRaw []byte
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(theme_config, '{}'::jsonb) FROM tenants WHERE id = $1 AND is_active = TRUE`, tenantID).Scan(&themeRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	theme := parseStoredThemeConfig(themeRaw)
	if theme.LogoStorageKey == nil {
		return "", nil
	}
	return *theme.LogoStorageKey, nil
}

func (s *PGStore) VerifyDocumentByID(ctx context.Context, tenantID, applicationID uuid.UUID) (*handlers.PublicDocumentVerifyResult, error) {
	var out *handlers.PublicDocumentVerifyResult
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var regNo, unitName, trackName, academicYear, statusStr string
		var fullName *string
		var finalScore *float64
		var submittedAt, resultsPublishedAt *time.Time

		err := tx.QueryRow(ctx, `
			SELECT a.registration_no, pr.full_name, u.name, t.display_name, p.academic_year,
			       a.status::text, a.final_score::double precision, a.submitted_at, p.results_published_at
			FROM applications a
			JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
			JOIN admission_periods p ON p.id = a.period_id AND p.tenant_id = a.tenant_id
			JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
			LEFT JOIN applicant_profiles pr ON pr.application_id = a.id AND pr.tenant_id = a.tenant_id
			WHERE a.tenant_id = $1 AND a.id = $2
		`, tenantID, applicationID).Scan(
			&regNo, &fullName, &unitName, &trackName, &academicYear,
			&statusStr, &finalScore, &submittedAt, &resultsPublishedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		status := domain.ApplicationStatus(statusStr)
		masked := "-"
		if fullName != nil && strings.TrimSpace(*fullName) != "" {
			masked = maskFullName(*fullName)
		}
		var exposedScore *float64
		if resultsPublishedAt != nil && !resultsPublishedAt.After(time.Now().UTC()) {
			exposedScore = finalScore
		}

		out = &handlers.PublicDocumentVerifyResult{
			Valid:          true,
			DocumentType:   documentTypeForStatus(status),
			RegistrationNo: regNo,
			MaskedName:     masked,
			UnitName:       unitName,
			TrackName:      trackName,
			AcademicYear:   academicYear,
			Status:         status,
			FinalScore:     exposedScore,
			SubmittedAt:    submittedAt,
			VerifiedAt:     time.Now().UTC(),
			Message:        "Dokumen terdaftar dan terverifikasi asli dalam sistem.",
		}
		return nil
	})
	return out, err
}
