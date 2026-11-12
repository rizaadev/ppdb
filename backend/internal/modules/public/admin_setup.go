package public

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/modules/auth"
)

func mapPGSetupError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return handlers.ErrSetupNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503":
			return handlers.ErrSetupConflict
		case "23514":
			return &handlers.SetupValidationError{Message: "Input melanggar batasan data sistem.", Fields: map[string]string{}}
		}
	}
	return err
}

type tenantSettingsRow struct {
	payload     handlers.TenantSettingsPayload
	storedTheme storedThemeConfig
	paymentCfg  domain.TenantPaymentConfig
}

func loadTenantSettingsTx(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) (*tenantSettingsRow, error) {
	var res tenantSettingsRow
	var instType, preset string
	var overridesRaw, themeRaw, paymentRaw []byte

	err := tx.QueryRow(ctx, `
		SELECT id, slug, name, institution_type::text, terminology_preset::text,
		       COALESCE(terminology_overrides, '{}'::jsonb),
		       COALESCE(theme_config, '{}'::jsonb),
		       custom_domain,
		       domain_verified_at IS NOT NULL,
		       domain_verification_token,
		       COALESCE(payment_config, '{}'::jsonb)
		FROM tenants
		WHERE id = $1 AND is_active = TRUE
	`, tenantID).Scan(
		&res.payload.ID, &res.payload.Slug, &res.payload.Name, &instType, &preset,
		&overridesRaw, &themeRaw,
		&res.payload.CustomDomain, &res.payload.DomainVerified, &res.payload.DomainVerificationToken,
		&paymentRaw,
	)
	if err != nil {
		return nil, mapPGSetupError(err)
	}

	res.payload.InstitutionType = domain.InstitutionType(instType)
	res.payload.TerminologyPreset = domain.TerminologyPreset(preset)
	res.payload.TerminologyOverrides = map[string]string{}
	if len(overridesRaw) > 0 {
		_ = json.Unmarshal(overridesRaw, &res.payload.TerminologyOverrides)
		if res.payload.TerminologyOverrides == nil {
			res.payload.TerminologyOverrides = map[string]string{}
		}
	}
	res.storedTheme = parseStoredThemeConfig(themeRaw)
	res.payload.ThemeConfig = toThemeConfigView(res.storedTheme)
	res.paymentCfg = parsePaymentConfigJSON(paymentRaw)
	res.payload.PaymentConfig = toPaymentConfigView(res.paymentCfg)
	return &res, nil
}

func (s *PGStore) GetTenantSettings(ctx context.Context, tenantID uuid.UUID) (*handlers.TenantSettingsPayload, error) {
	var out *handlers.TenantSettingsPayload
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		row, err := loadTenantSettingsTx(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		out = &row.payload
		return nil
	})
	return out, err
}

func (s *PGStore) UpdateTenantSettings(ctx context.Context, tenantID uuid.UUID, input handlers.UpdateTenantSettingsInput) (*handlers.TenantSettingsPayload, error) {
	var out *handlers.TenantSettingsPayload
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		current, err := loadTenantSettingsTx(ctx, tx, tenantID)
		if err != nil {
			return err
		}

		name := current.payload.Name
		if input.Name != nil {
			name = *input.Name
		}
		preset := current.payload.TerminologyPreset
		if input.TerminologyPreset != nil {
			preset = *input.TerminologyPreset
		}
		overrides := current.payload.TerminologyOverrides
		if input.TerminologyOverrides != nil {
			overrides = input.TerminologyOverrides
		}
		theme := current.storedTheme
		if input.ThemeConfig != nil {
			if input.ThemeConfig.PrimaryColor != nil {
				theme.PrimaryColor = *input.ThemeConfig.PrimaryColor
			}
			if input.ThemeConfig.Tagline != nil {
				theme.Tagline = *input.ThemeConfig.Tagline
			}
			if input.ThemeConfig.LogoURL != nil {
				trimmed := strings.TrimSpace(*input.ThemeConfig.LogoURL)
				if trimmed == "" {
					theme.LogoURL = nil
					theme.LogoStorageKey = nil
				} else {
					theme.LogoURL = &trimmed
					// External logo URL overrides uploaded storage key unless already set to internal logo proxy.
					if !strings.HasPrefix(trimmed, "/api/v1/public/logo") {
						theme.LogoStorageKey = nil
					}
				}
			}
		}

		customDomain := current.payload.CustomDomain
		domainVerified := current.payload.DomainVerified
		verifyToken := current.payload.DomainVerificationToken
		if input.CustomDomain != nil {
			d := strings.ToLower(strings.TrimSpace(*input.CustomDomain))
			if d == "" {
				customDomain = nil
				domainVerified = false
				verifyToken = nil
			} else if customDomain == nil || !strings.EqualFold(*customDomain, d) {
				customDomain = &d
				domainVerified = false
				tok := domain.GenerateDomainVerificationToken()
				verifyToken = &tok
			} else if verifyToken == nil || strings.TrimSpace(*verifyToken) == "" {
				tok := domain.GenerateDomainVerificationToken()
				verifyToken = &tok
			}
		}

		paymentCfg := mergePaymentConfigUpdate(current.paymentCfg, input.PaymentConfig)

		overridesJSON, err := json.Marshal(overrides)
		if err != nil {
			return err
		}
		themeJSON, err := json.Marshal(theme)
		if err != nil {
			return err
		}
		paymentJSON, err := json.Marshal(paymentCfg)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			UPDATE tenants
			SET name = $2,
			    terminology_preset = $3::terminology_preset,
			    terminology_overrides = $4::jsonb,
			    theme_config = $5::jsonb,
			    custom_domain = $6,
			    domain_verified_at = CASE WHEN $7::boolean AND $6::text IS NOT NULL THEN COALESCE(domain_verified_at, NOW()) ELSE NULL END,
			    domain_verification_token = $8,
			    payment_config = $9::jsonb
			WHERE id = $1
		`, tenantID, name, string(preset), string(overridesJSON), string(themeJSON), customDomain, domainVerified, verifyToken, string(paymentJSON))
		if err != nil {
			return mapPGSetupError(err)
		}

		reloaded, err := loadTenantSettingsTx(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		out = &reloaded.payload
		return nil
	})
	return out, err
}

func (s *PGStore) SetTenantLogoKey(ctx context.Context, tenantID uuid.UUID, logoStorageKey *string) (*handlers.TenantSettingsPayload, error) {
	var out *handlers.TenantSettingsPayload
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		current, err := loadTenantSettingsTx(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		theme := current.storedTheme
		if logoStorageKey == nil || strings.TrimSpace(*logoStorageKey) == "" {
			theme.LogoStorageKey = nil
			theme.LogoURL = nil
		} else {
			k := strings.TrimSpace(*logoStorageKey)
			theme.LogoStorageKey = &k
			theme.LogoURL = publicLogoURLFromStorageKey(&k)
		}
		themeJSON, err := json.Marshal(theme)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tenants SET theme_config = $2::jsonb WHERE id = $1`, tenantID, string(themeJSON)); err != nil {
			return mapPGSetupError(err)
		}
		reloaded, err := loadTenantSettingsTx(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		out = &reloaded.payload
		return nil
	})
	return out, err
}

func (s *PGStore) GetTenantDomainInfo(ctx context.Context, tenantID uuid.UUID) (string, *string, *string, error) {
	var slug string
	var customDomain, verifyToken *string
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT slug, custom_domain, domain_verification_token
			FROM tenants
			WHERE id = $1 AND is_active = TRUE
		`, tenantID).Scan(&slug, &customDomain, &verifyToken)
		if err != nil {
			return mapPGSetupError(err)
		}
		if customDomain != nil && strings.TrimSpace(*customDomain) != "" && (verifyToken == nil || strings.TrimSpace(*verifyToken) == "") {
			tok := domain.GenerateDomainVerificationToken()
			verifyToken = &tok
			_, _ = tx.Exec(ctx, `UPDATE tenants SET domain_verification_token = $2 WHERE id = $1`, tenantID, tok)
		}
		return nil
	})
	return slug, customDomain, verifyToken, err
}

func (s *PGStore) MarkTenantDomainVerified(ctx context.Context, tenantID uuid.UUID) (*handlers.TenantSettingsPayload, error) {
	var out *handlers.TenantSettingsPayload
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE tenants
			SET domain_verified_at = COALESCE(domain_verified_at, NOW())
			WHERE id = $1 AND custom_domain IS NOT NULL AND TRIM(custom_domain) <> ''
		`, tenantID)
		if err != nil {
			return mapPGSetupError(err)
		}
		if tag.RowsAffected() == 0 {
			return handlers.ErrSetupNotFound
		}
		reloaded, err := loadTenantSettingsTx(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		out = &reloaded.payload
		return nil
	})
	return out, err
}

// GetTenantPaymentConfig retrieves optional per-tenant Midtrans configuration from PostgreSQL.
func (s *PGStore) GetTenantPaymentConfig(ctx context.Context, tenantID uuid.UUID) (*domain.TenantPaymentConfig, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(payment_config, '{}'::jsonb) FROM tenants WHERE id = $1`, tenantID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := parsePaymentConfigJSON(raw)
	return &cfg, nil
}

func (s *PGStore) ListAdminUnits(ctx context.Context, tenantID uuid.UUID) ([]handlers.UnitView, error) {
	units := []handlers.UnitView{}
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, code, name, education_level::text, address, latitude, longitude, is_active
			FROM units
			WHERE tenant_id = $1
			ORDER BY name ASC
		`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var u handlers.UnitView
			if err := rows.Scan(&u.ID, &u.Code, &u.Name, &u.EducationLevel, &u.Address, &u.Latitude, &u.Longitude, &u.IsActive); err != nil {
				return err
			}
			units = append(units, u)
		}
		return rows.Err()
	})
	return units, err
}

func (s *PGStore) CreateUnit(ctx context.Context, tenantID uuid.UUID, input handlers.CreateUnitInput) (*handlers.UnitView, error) {
	var u handlers.UnitView
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		id := uuid.Must(uuid.NewV7())
		err := tx.QueryRow(ctx, `
			INSERT INTO units (
				id, tenant_id, code, name, education_level, npsn_or_nsm, address, latitude, longitude, is_active
			) VALUES (
				$1, $2, $3, $4, $5::education_level, $6, $7, $8, $9, TRUE
			)
			RETURNING id, code, name, education_level::text, address, latitude, longitude, is_active
		`, id, tenantID, input.Code, input.Name, input.EducationLevel, input.NPSNOrNSM, input.Address, input.Latitude, input.Longitude).Scan(
			&u.ID, &u.Code, &u.Name, &u.EducationLevel, &u.Address, &u.Latitude, &u.Longitude, &u.IsActive,
		)
		return mapPGSetupError(err)
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *PGStore) UpdateUnit(ctx context.Context, tenantID, unitID uuid.UUID, input handlers.UpdateUnitInput) (*handlers.UnitView, error) {
	var cur handlers.UnitView
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, code, name, education_level::text, address, latitude, longitude, is_active
			FROM units
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, unitID).Scan(
			&cur.ID, &cur.Code, &cur.Name, &cur.EducationLevel, &cur.Address, &cur.Latitude, &cur.Longitude, &cur.IsActive,
		)
		if err != nil {
			return mapPGSetupError(err)
		}

		if input.Name != nil {
			cur.Name = *input.Name
		}
		if input.EducationLevel != nil {
			cur.EducationLevel = *input.EducationLevel
		}
		if input.Address != nil {
			trimmed := strings.TrimSpace(*input.Address)
			cur.Address = &trimmed
		}
		if input.Latitude != nil {
			cur.Latitude = input.Latitude
		}
		if input.Longitude != nil {
			cur.Longitude = input.Longitude
		}
		if input.IsActive != nil {
			cur.IsActive = *input.IsActive
		}

		err = tx.QueryRow(ctx, `
			UPDATE units
			SET name = $3,
			    education_level = $4::education_level,
			    address = $5,
			    latitude = $6,
			    longitude = $7,
			    is_active = $8
			WHERE tenant_id = $1 AND id = $2
			RETURNING id, code, name, education_level::text, address, latitude, longitude, is_active
		`, tenantID, unitID, cur.Name, cur.EducationLevel, cur.Address, cur.Latitude, cur.Longitude, cur.IsActive).Scan(
			&cur.ID, &cur.Code, &cur.Name, &cur.EducationLevel, &cur.Address, &cur.Latitude, &cur.Longitude, &cur.IsActive,
		)
		return mapPGSetupError(err)
	})
	if err != nil {
		return nil, err
	}
	return &cur, nil
}

func (s *PGStore) GetPeriodUnitID(ctx context.Context, tenantID, periodID uuid.UUID) (uuid.UUID, bool, error) {
	var unitID uuid.UUID
	var found bool
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT unit_id FROM admission_periods WHERE tenant_id = $1 AND id = $2
		`, tenantID, periodID).Scan(&unitID)
		if errors.Is(err, pgx.ErrNoRows) {
			found = false
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	return unitID, found, err
}

func (s *PGStore) CreatePeriod(ctx context.Context, tenantID uuid.UUID, input handlers.CreatePeriodInput) (*handlers.PeriodView, error) {
	var p handlers.PeriodView
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		published := true
		if input.IsPublished != nil {
			published = *input.IsPublished
		}
		id := uuid.Must(uuid.NewV7())
		err := tx.QueryRow(ctx, `
			INSERT INTO admission_periods (
				id, tenant_id, unit_id, academic_year, name, opens_at, closes_at, announcement_at, is_published, is_active
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, TRUE)
			RETURNING id, unit_id, academic_year, name, opens_at, closes_at, announcement_at, results_published_at, is_published, is_active
		`, id, tenantID, input.UnitID, input.AcademicYear, input.Name, input.OpensAt.UTC(), input.ClosesAt.UTC(), input.AnnouncementAt, published).Scan(
			&p.ID, &p.UnitID, &p.AcademicYear, &p.Name, &p.OpensAt, &p.ClosesAt, &p.AnnouncementAt, &p.ResultsPublishedAt, &p.IsPublished, &p.IsActive,
		)
		return mapPGSetupError(err)
	})
	if err != nil {
		return nil, err
	}
	p.Tracks = []handlers.TrackView{}
	return &p, nil
}

func (s *PGStore) UpdatePeriod(ctx context.Context, tenantID, periodID uuid.UUID, input handlers.UpdatePeriodInput) (*handlers.PeriodView, error) {
	var p handlers.PeriodView
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, unit_id, academic_year, name, opens_at, closes_at, announcement_at, results_published_at, is_published, is_active
			FROM admission_periods
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, periodID).Scan(
			&p.ID, &p.UnitID, &p.AcademicYear, &p.Name, &p.OpensAt, &p.ClosesAt, &p.AnnouncementAt, &p.ResultsPublishedAt, &p.IsPublished, &p.IsActive,
		)
		if err != nil {
			return mapPGSetupError(err)
		}

		if input.AcademicYear != nil {
			p.AcademicYear = *input.AcademicYear
		}
		if input.Name != nil {
			p.Name = *input.Name
		}
		if input.OpensAt != nil {
			p.OpensAt = input.OpensAt.UTC()
		}
		if input.ClosesAt != nil {
			p.ClosesAt = input.ClosesAt.UTC()
		}
		if !p.ClosesAt.After(p.OpensAt) {
			return &handlers.SetupValidationError{
				Message: "Tanggal tutup harus lebih lambat daripada tanggal buka.",
				Fields:  map[string]string{"closes_at": "Tanggal tutup harus lebih lambat daripada tanggal buka."},
			}
		}
		if input.AnnouncementAt != nil {
			p.AnnouncementAt = input.AnnouncementAt
		}
		if input.IsPublished != nil {
			p.IsPublished = *input.IsPublished
		}
		if input.IsActive != nil {
			p.IsActive = *input.IsActive
		}

		err = tx.QueryRow(ctx, `
			UPDATE admission_periods
			SET academic_year = $3, name = $4, opens_at = $5, closes_at = $6,
			    announcement_at = $7, is_published = $8, is_active = $9
			WHERE tenant_id = $1 AND id = $2
			RETURNING id, unit_id, academic_year, name, opens_at, closes_at, announcement_at, results_published_at, is_published, is_active
		`, tenantID, periodID, p.AcademicYear, p.Name, p.OpensAt, p.ClosesAt, p.AnnouncementAt, p.IsPublished, p.IsActive).Scan(
			&p.ID, &p.UnitID, &p.AcademicYear, &p.Name, &p.OpensAt, &p.ClosesAt, &p.AnnouncementAt, &p.ResultsPublishedAt, &p.IsPublished, &p.IsActive,
		)
		return mapPGSetupError(err)
	})
	if err != nil {
		return nil, err
	}
	p.Tracks = []handlers.TrackView{}
	return &p, nil
}

func (s *PGStore) GetTrackUnitID(ctx context.Context, tenantID, trackID uuid.UUID) (uuid.UUID, bool, error) {
	var unitID uuid.UUID
	var found bool
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT p.unit_id
			FROM admission_tracks t
			JOIN admission_periods p ON p.id = t.period_id AND p.tenant_id = t.tenant_id
			WHERE t.tenant_id = $1 AND t.id = $2
		`, tenantID, trackID).Scan(&unitID)
		if errors.Is(err, pgx.ErrNoRows) {
			found = false
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	return unitID, found, err
}

func (s *PGStore) CreateTrack(ctx context.Context, tenantID uuid.UUID, input handlers.CreateTrackInput) (*handlers.TrackView, error) {
	var tr handlers.TrackView
	weightsJSON := "{}"
	if input.ScoringWeights != nil {
		if b, err := json.Marshal(input.ScoringWeights); err == nil {
			weightsJSON = string(b)
		}
	}
	schemaJSON := "[]"
	if input.CustomFormSchema != nil {
		if b, err := json.Marshal(input.CustomFormSchema); err == nil {
			schemaJSON = string(b)
		}
	}

	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		id := uuid.Must(uuid.NewV7())
		var retWeights, retSchema []byte
		err := tx.QueryRow(ctx, `
			WITH inserted AS (
				INSERT INTO admission_tracks (
					id, tenant_id, period_id, code, display_name, description, selection_mode,
					quota, spillover_target_track_id, registration_fee_idr, re_enrollment_fee_idr,
					requires_coordinates, requires_achievements,
					min_age_months, max_age_months, age_reference_date,
					scoring_weights, custom_form_schema, is_active
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7::selection_mode,
					$8, $9, $10, $11, $12, $13,
					$14, $15, NULLIF($16, '')::date,
					$17::jsonb, $18::jsonb, TRUE
				)
				RETURNING *
			)
			SELECT i.id, i.period_id, p.unit_id, i.code, i.display_name, i.description, i.selection_mode::text,
			       i.quota, i.registration_fee_idr, i.re_enrollment_fee_idr, i.spillover_target_track_id,
			       i.requires_coordinates, i.requires_achievements,
			       i.min_age_months, i.max_age_months, TO_CHAR(i.age_reference_date, 'YYYY-MM-DD'),
			       COALESCE(i.scoring_weights, '{}'::jsonb), COALESCE(i.custom_form_schema, '[]'::jsonb),
			       i.is_active
			FROM inserted i
			JOIN admission_periods p ON p.id = i.period_id AND p.tenant_id = i.tenant_id
		`,
			id, tenantID, input.PeriodID, input.Code, input.DisplayName, input.Description, input.SelectionMode,
			input.Quota, input.SpilloverTargetTrackID, input.RegistrationFeeIDR, input.ReEnrollmentFeeIDR,
			input.RequiresCoordinates, input.RequiresAchievements,
			input.MinAgeMonths, input.MaxAgeMonths, input.AgeReferenceDate,
			weightsJSON, schemaJSON,
		).Scan(
			&tr.ID, &tr.PeriodID, &tr.UnitID, &tr.Code, &tr.DisplayName, &tr.Description, &tr.SelectionMode,
			&tr.Quota, &tr.RegistrationFeeIDR, &tr.ReEnrollmentFeeIDR, &tr.SpilloverTargetTrack,
			&tr.RequiresCoordinates, &tr.RequiresAchievements,
			&tr.MinAgeMonths, &tr.MaxAgeMonths, &tr.AgeReferenceDate,
			&retWeights, &retSchema, &tr.IsActive,
		)
		if err != nil {
			return mapPGSetupError(err)
		}
		if len(retWeights) > 0 {
			_ = json.Unmarshal(retWeights, &tr.ScoringWeights)
		}
		if len(retSchema) > 0 {
			_ = json.Unmarshal(retSchema, &tr.CustomFormSchema)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	tr.Requirements = []handlers.RequirementView{}
	return &tr, nil
}

func (s *PGStore) UpdateTrack(ctx context.Context, tenantID, trackID uuid.UUID, input handlers.UpdateTrackInput) (*handlers.TrackView, error) {
	var tr handlers.TrackView
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var curWeights, curSchema []byte
		err := tx.QueryRow(ctx, `
			SELECT t.id, t.period_id, p.unit_id, t.code, t.display_name, t.description, t.selection_mode::text,
			       t.quota, t.registration_fee_idr, t.re_enrollment_fee_idr, t.spillover_target_track_id,
			       t.requires_coordinates, t.requires_achievements,
			       t.min_age_months, t.max_age_months, TO_CHAR(t.age_reference_date, 'YYYY-MM-DD'),
			       COALESCE(t.scoring_weights, '{}'::jsonb), COALESCE(t.custom_form_schema, '[]'::jsonb),
			       t.is_active
			FROM admission_tracks t
			JOIN admission_periods p ON p.id = t.period_id AND p.tenant_id = t.tenant_id
			WHERE t.tenant_id = $1 AND t.id = $2
		`, tenantID, trackID).Scan(
			&tr.ID, &tr.PeriodID, &tr.UnitID, &tr.Code, &tr.DisplayName, &tr.Description, &tr.SelectionMode,
			&tr.Quota, &tr.RegistrationFeeIDR, &tr.ReEnrollmentFeeIDR, &tr.SpilloverTargetTrack,
			&tr.RequiresCoordinates, &tr.RequiresAchievements,
			&tr.MinAgeMonths, &tr.MaxAgeMonths, &tr.AgeReferenceDate,
			&curWeights, &curSchema, &tr.IsActive,
		)
		if err != nil {
			return mapPGSetupError(err)
		}
		if len(curWeights) > 0 {
			_ = json.Unmarshal(curWeights, &tr.ScoringWeights)
		}
		if len(curSchema) > 0 {
			_ = json.Unmarshal(curSchema, &tr.CustomFormSchema)
		}

		if input.DisplayName != nil {
			tr.DisplayName = *input.DisplayName
		}
		if input.Description != nil {
			trimmed := strings.TrimSpace(*input.Description)
			if trimmed == "" {
				tr.Description = nil
			} else {
				tr.Description = &trimmed
			}
		}
		if input.SelectionMode != nil {
			tr.SelectionMode = *input.SelectionMode
		}
		if input.Quota != nil {
			tr.Quota = *input.Quota
		}
		if input.SpilloverTargetTrackID != nil {
			tr.SpilloverTargetTrack = input.SpilloverTargetTrackID
		}
		if input.RegistrationFeeIDR != nil {
			tr.RegistrationFeeIDR = *input.RegistrationFeeIDR
		}
		if input.ReEnrollmentFeeIDR != nil {
			tr.ReEnrollmentFeeIDR = *input.ReEnrollmentFeeIDR
		}
		if input.RequiresCoordinates != nil {
			tr.RequiresCoordinates = *input.RequiresCoordinates
		}
		if input.RequiresAchievements != nil {
			tr.RequiresAchievements = *input.RequiresAchievements
		}
		if input.MinAgeMonths != nil {
			tr.MinAgeMonths = input.MinAgeMonths
		}
		if input.MaxAgeMonths != nil {
			tr.MaxAgeMonths = input.MaxAgeMonths
		}
		if input.AgeReferenceDate != nil {
			tr.AgeReferenceDate = input.AgeReferenceDate
		}
		if input.ScoringWeights != nil {
			tr.ScoringWeights = input.ScoringWeights
		}
		if input.CustomFormSchema != nil {
			tr.CustomFormSchema = input.CustomFormSchema
		}
		if input.IsActive != nil {
			tr.IsActive = *input.IsActive
		}

		weightsJSON := "{}"
		if tr.ScoringWeights != nil {
			if b, err := json.Marshal(tr.ScoringWeights); err == nil {
				weightsJSON = string(b)
			}
		}
		schemaJSON := "[]"
		if tr.CustomFormSchema != nil {
			if b, err := json.Marshal(tr.CustomFormSchema); err == nil {
				schemaJSON = string(b)
			}
		}

		_, err = tx.Exec(ctx, `
			UPDATE admission_tracks
			SET display_name = $3,
			    description = $4,
			    selection_mode = $5::selection_mode,
			    quota = $6,
			    spillover_target_track_id = $7,
			    registration_fee_idr = $8,
			    re_enrollment_fee_idr = $9,
			    requires_coordinates = $10,
			    requires_achievements = $11,
			    min_age_months = $12,
			    max_age_months = $13,
			    age_reference_date = NULLIF($14, '')::date,
			    scoring_weights = $15::jsonb,
			    custom_form_schema = $16::jsonb,
			    is_active = $17
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, trackID, tr.DisplayName, tr.Description, tr.SelectionMode, tr.Quota,
			tr.SpilloverTargetTrack, tr.RegistrationFeeIDR, tr.ReEnrollmentFeeIDR,
			tr.RequiresCoordinates, tr.RequiresAchievements,
			tr.MinAgeMonths, tr.MaxAgeMonths, tr.AgeReferenceDate,
			weightsJSON, schemaJSON, tr.IsActive)
		return mapPGSetupError(err)
	})
	if err != nil {
		return nil, err
	}
	tr.Requirements = []handlers.RequirementView{}
	return &tr, nil
}

func (s *PGStore) ReplaceTrackRequirements(
	ctx context.Context,
	tenantID, trackID uuid.UUID,
	items []handlers.TrackRequirementItemInput,
) ([]handlers.RequirementView, error) {
	saved := []handlers.RequirementView{}
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM admission_tracks WHERE tenant_id = $1 AND id = $2)`, tenantID, trackID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return handlers.ErrSetupNotFound
		}

		codes := make([]string, 0, len(items))
		for idx, item := range items {
			codes = append(codes, item.DocCode)
			id := uuid.Must(uuid.NewV7())
			_, err := tx.Exec(ctx, `
				INSERT INTO track_requirements (
					id, tenant_id, track_id, doc_code, label, description, is_required, accepted_mimes, max_size_kb, sort_order
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (track_id, doc_code) DO UPDATE SET
					label = EXCLUDED.label,
					description = EXCLUDED.description,
					is_required = EXCLUDED.is_required,
					accepted_mimes = EXCLUDED.accepted_mimes,
					max_size_kb = EXCLUDED.max_size_kb,
					sort_order = EXCLUDED.sort_order
			`, id, tenantID, trackID, item.DocCode, item.Label, item.Description, item.IsRequired, item.AcceptedMimes, item.MaxSizeKB, idx+1)
			if err != nil {
				return mapPGSetupError(err)
			}
		}

		if len(codes) == 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM track_requirements WHERE tenant_id = $1 AND track_id = $2`, tenantID, trackID); err != nil {
				return mapPGSetupError(err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM track_requirements
				WHERE tenant_id = $1 AND track_id = $2 AND NOT (doc_code = ANY($3))
			`, tenantID, trackID, codes); err != nil {
				return mapPGSetupError(err)
			}
		}

		rows, err := tx.Query(ctx, `
			SELECT id, doc_code, label, description, is_required, accepted_mimes, max_size_kb
			FROM track_requirements
			WHERE tenant_id = $1 AND track_id = $2
			ORDER BY sort_order ASC, created_at ASC
		`, tenantID, trackID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var r handlers.RequirementView
			if err := rows.Scan(&r.ID, &r.DocCode, &r.Label, &r.Description, &r.IsRequired, &r.AcceptedMimes, &r.MaxSizeKB); err != nil {
				return err
			}
			saved = append(saved, r)
		}
		return rows.Err()
	})
	return saved, err
}

func (s *PGStore) ListStaff(ctx context.Context, tenantID uuid.UUID) ([]handlers.StaffMemberView, error) {
	members := []handlers.StaffMemberView{}
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		loaded, err := listStaffTx(ctx, tx, tenantID, nil)
		if err != nil {
			return err
		}
		members = loaded
		return nil
	})
	return members, err
}

func listStaffTx(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, filterUserID *uuid.UUID) ([]handlers.StaffMemberView, error) {
	query := `
		SELECT r.id, r.user_id, u.full_name, u.email, u.whatsapp_phone, u.is_platform_admin,
		       r.role::text, r.unit_id, un.name, r.is_active, u.created_at
		FROM user_tenant_roles r
		JOIN users u ON u.id = r.user_id
		LEFT JOIN units un ON un.id = r.unit_id AND un.tenant_id = r.tenant_id
		WHERE r.tenant_id = $1
	`
	args := []any{tenantID}
	if filterUserID != nil {
		query += ` AND r.user_id = $2`
		args = append(args, *filterUserID)
	}
	query += ` ORDER BY u.full_name ASC, r.created_at ASC`

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byUser := make(map[uuid.UUID]*handlers.StaffMemberView)
	order := []uuid.UUID{}
	for rows.Next() {
		var roleRow handlers.StaffRoleView
		var userID uuid.UUID
		var fullName string
		var email, phone *string
		var isPlatformAdmin bool
		var roleStr string
		var createdAt time.Time

		if err := rows.Scan(
			&roleRow.ID, &userID, &fullName, &email, &phone, &isPlatformAdmin,
			&roleStr, &roleRow.UnitID, &roleRow.UnitName, &roleRow.IsActive, &createdAt,
		); err != nil {
			return nil, err
		}
		roleRow.Role = domain.TenantRole(roleStr)
		member, exists := byUser[userID]
		if !exists {
			member = &handlers.StaffMemberView{
				UserID:          userID,
				FullName:        fullName,
				Email:           email,
				WhatsAppPhone:   phone,
				IsPlatformAdmin: isPlatformAdmin,
				Roles:           []handlers.StaffRoleView{},
				CreatedAt:       createdAt,
			}
			byUser[userID] = member
			order = append(order, userID)
		}
		member.Roles = append(member.Roles, roleRow)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]handlers.StaffMemberView, 0, len(order))
	for _, id := range order {
		out = append(out, *byUser[id])
	}
	return out, nil
}

func (s *PGStore) CreateOrAssignStaff(ctx context.Context, tenantID uuid.UUID, input handlers.CreateStaffInput) (*handlers.StaffMemberView, error) {
	var created *handlers.StaffMemberView
	unitID := input.UnitID
	if input.Role == domain.RoleTenantAdmin {
		unitID = nil
	}

	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		if unitID != nil {
			var unitExists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM units WHERE tenant_id = $1 AND id = $2)`, tenantID, *unitID).Scan(&unitExists); err != nil {
				return err
			}
			if !unitExists {
				return handlers.ErrSetupNotFound
			}
		}

		var userID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE LOWER(email) = LOWER($1)`, input.Email).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			if len(input.Password) < 6 {
				return &handlers.SetupValidationError{
					Message: "Password minimal 6 karakter wajib diisi untuk akun staf baru.",
					Fields:  map[string]string{"password": "Password minimal 6 karakter wajib diisi."},
				}
			}
			pwHash, hashErr := auth.HashPassword(input.Password)
			if hashErr != nil {
				return hashErr
			}
			userID = uuid.Must(uuid.NewV7())
			if _, err := tx.Exec(ctx, `
				INSERT INTO users (id, email, whatsapp_phone, password_hash, full_name, is_active)
				VALUES ($1, $2, $3, $4, $5, TRUE)
			`, userID, input.Email, input.WhatsAppPhone, pwHash, input.FullName); err != nil {
				return mapPGSetupError(err)
			}
		} else if err != nil {
			return err
		}

		// Replace or upsert identical scoped role row for this tenant.
		var roleID uuid.UUID
		roleErr := tx.QueryRow(ctx, `
			SELECT id
			FROM user_tenant_roles
			WHERE user_id = $1 AND tenant_id = $2
			  AND COALESCE(unit_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE($3::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
			  AND role = $4::tenant_role
		`, userID, tenantID, unitID, string(input.Role)).Scan(&roleID)
		if errors.Is(roleErr, pgx.ErrNoRows) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO user_tenant_roles (id, user_id, tenant_id, unit_id, role, is_active)
				VALUES ($1, $2, $3, $4, $5::tenant_role, TRUE)
			`, uuid.Must(uuid.NewV7()), userID, tenantID, unitID, string(input.Role)); err != nil {
				return mapPGSetupError(err)
			}
		} else if roleErr != nil {
			return roleErr
		} else {
			if _, err := tx.Exec(ctx, `UPDATE user_tenant_roles SET is_active = TRUE WHERE id = $1`, roleID); err != nil {
				return err
			}
		}

		loaded, err := listStaffTx(ctx, tx, tenantID, &userID)
		if err != nil {
			return err
		}
		if len(loaded) == 0 {
			return handlers.ErrSetupNotFound
		}
		created = &loaded[0]
		return nil
	})
	return created, err
}

func (s *PGStore) UpdateStaff(ctx context.Context, tenantID, actorUserID, targetUserID uuid.UUID, input handlers.UpdateStaffInput) (*handlers.StaffMemberView, error) {
	var updated *handlers.StaffMemberView
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		loaded, err := listStaffTx(ctx, tx, tenantID, &targetUserID)
		if err != nil {
			return err
		}
		if len(loaded) == 0 {
			return handlers.ErrSetupNotFound
		}
		member := loaded[0]

		if input.IsActive != nil && !*input.IsActive && actorUserID == targetUserID {
			return &handlers.SetupValidationError{
				Message: "Anda tidak dapat menonaktifkan peran akun Anda sendiri.",
				Fields:  map[string]string{"is_active": "Tidak dapat menonaktifkan akun sendiri."},
			}
		}

		// Only allow profile/password edits on non-platform accounts from tenant admin console.
		if !member.IsPlatformAdmin {
			if input.FullName != nil {
				if _, err := tx.Exec(ctx, `UPDATE users SET full_name = $2 WHERE id = $1`, targetUserID, *input.FullName); err != nil {
					return mapPGSetupError(err)
				}
			}
			if input.WhatsAppPhone != nil {
				var phonePtr *string
				if *input.WhatsAppPhone != "" {
					phonePtr = input.WhatsAppPhone
				}
				if _, err := tx.Exec(ctx, `UPDATE users SET whatsapp_phone = $2 WHERE id = $1`, targetUserID, phonePtr); err != nil {
					return mapPGSetupError(err)
				}
			}
			if input.Password != nil && *input.Password != "" {
				pwHash, err := auth.HashPassword(*input.Password)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, targetUserID, pwHash); err != nil {
					return err
				}
			}
		}

		if input.Role != nil || input.UnitID != nil || input.ClearUnit {
			primary := member.Roles[0]
			nextRole := primary.Role
			if input.Role != nil {
				nextRole = *input.Role
			}
			nextUnit := primary.UnitID
			if input.ClearUnit {
				nextUnit = nil
			}
			if input.UnitID != nil {
				nextUnit = input.UnitID
			}
			if nextRole == domain.RoleTenantAdmin {
				nextUnit = nil
			}
			if nextUnit != nil {
				var unitExists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM units WHERE tenant_id = $1 AND id = $2)`, tenantID, *nextUnit).Scan(&unitExists); err != nil {
					return err
				}
				if !unitExists {
					return handlers.ErrSetupNotFound
				}
			}

			// Remove conflicting duplicate if changing role+unit into another existing row, then update primary row.
			_, _ = tx.Exec(ctx, `
				DELETE FROM user_tenant_roles
				WHERE tenant_id = $1 AND user_id = $2 AND id <> $3
				  AND role = $4::tenant_role
				  AND COALESCE(unit_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE($5::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
			`, tenantID, targetUserID, primary.ID, string(nextRole), nextUnit)

			if _, err := tx.Exec(ctx, `
				UPDATE user_tenant_roles
				SET role = $3::tenant_role, unit_id = $4
				WHERE tenant_id = $1 AND id = $2
			`, tenantID, primary.ID, string(nextRole), nextUnit); err != nil {
				return mapPGSetupError(err)
			}
		}

		if input.IsActive != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE user_tenant_roles SET is_active = $3 WHERE tenant_id = $1 AND user_id = $2
			`, tenantID, targetUserID, *input.IsActive); err != nil {
				return err
			}
		}

		reloaded, err := listStaffTx(ctx, tx, tenantID, &targetUserID)
		if err != nil {
			return err
		}
		if len(reloaded) == 0 {
			return handlers.ErrSetupNotFound
		}
		updated = &reloaded[0]
		return nil
	})
	return updated, err
}
