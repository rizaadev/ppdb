package public

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/modules/catalog"
)

func (s *PGStore) ListPlatformTenants(ctx context.Context) ([]handlers.PlatformTenantView, error) {
	out := []handlers.PlatformTenantView{}
	err := database.WithBypassRLSTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT
				t.id, t.slug, t.name, t.institution_type::text, t.terminology_preset::text,
				COALESCE(t.theme_config, '{}'::jsonb), t.custom_domain, t.domain_verified_at IS NOT NULL,
				t.domain_verification_token,
				t.subscription_plan, t.subscription_expires_at, t.is_active, t.created_at,
				COUNT(u.id)::int AS unit_count
			FROM tenants t
			LEFT JOIN units u ON u.tenant_id = t.id AND u.is_active = TRUE
			GROUP BY t.id
			ORDER BY t.created_at DESC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item handlers.PlatformTenantView
			var instType, preset string
			var themeRaw []byte
			if err := rows.Scan(
				&item.ID, &item.Slug, &item.Name, &instType, &preset,
				&themeRaw, &item.CustomDomain, &item.DomainVerified, &item.DomainVerificationToken,
				&item.SubscriptionPlan, &item.SubscriptionExpiresAt, &item.IsActive, &item.CreatedAt,
				&item.UnitCount,
			); err != nil {
				return err
			}
			item.InstitutionType = domain.InstitutionType(instType)
			item.TerminologyPreset = domain.TerminologyPreset(preset)
			item.ThemeConfig = parseThemeConfigJSON(themeRaw)
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

func (s *PGStore) CreatePlatformTenant(ctx context.Context, input handlers.CreatePlatformTenantInput) (*handlers.PlatformTenantView, error) {
	var created *handlers.PlatformTenantView
	err := database.WithBypassRLSTx(ctx, s.pool, func(tx pgx.Tx) error {
		tenantID := uuid.Must(uuid.NewV7())
		themeRaw, err := json.Marshal(storedThemeConfig{
			PrimaryColor: input.PrimaryColor,
			Tagline:      input.Tagline,
		})
		if err != nil {
			return err
		}

		var customDomain, verifyToken *string
		if input.CustomDomain != nil && strings.TrimSpace(*input.CustomDomain) != "" {
			d := strings.ToLower(strings.TrimSpace(*input.CustomDomain))
			customDomain = &d
			tok := domain.GenerateDomainVerificationToken()
			verifyToken = &tok
		}

		var item handlers.PlatformTenantView
		var instType, preset string
		var returnedThemeRaw []byte
		err = tx.QueryRow(ctx, `
			INSERT INTO tenants (
				id, slug, name, institution_type, custom_domain, domain_verification_token,
				terminology_preset, theme_config, subscription_plan, is_active
			) VALUES (
				$1, $2, $3, $4::institution_type, $5, $6, $7::terminology_preset, $8::jsonb, $9, TRUE
			)
			RETURNING id, slug, name, institution_type::text, terminology_preset::text,
			          COALESCE(theme_config, '{}'::jsonb), custom_domain, domain_verified_at IS NOT NULL,
			          domain_verification_token, subscription_plan, subscription_expires_at, is_active, created_at
		`, tenantID, input.Slug, input.Name, string(input.InstitutionType), customDomain, verifyToken,
			string(input.TerminologyPreset), themeRaw, input.SubscriptionPlan).Scan(
			&item.ID, &item.Slug, &item.Name, &instType, &preset,
			&returnedThemeRaw, &item.CustomDomain, &item.DomainVerified,
			&item.DomainVerificationToken, &item.SubscriptionPlan, &item.SubscriptionExpiresAt, &item.IsActive, &item.CreatedAt,
		)
		if err != nil {
			return mapPGSetupError(err)
		}
		item.InstitutionType = domain.InstitutionType(instType)
		item.TerminologyPreset = domain.TerminologyPreset(preset)
		item.ThemeConfig = parseThemeConfigJSON(returnedThemeRaw)

		if input.StarterUnitCode != "" && input.StarterUnitName != "" {
			unitID := uuid.Must(uuid.NewV7())
			if _, err := tx.Exec(ctx, `
				INSERT INTO units (id, tenant_id, code, name, education_level, is_active)
				VALUES ($1, $2, $3, $4, $5::education_level, TRUE)
			`, unitID, tenantID, input.StarterUnitCode, input.StarterUnitName, input.StarterUnitLevel); err != nil {
				return mapPGSetupError(err)
			}
			item.UnitCount = 1

			periodID := uuid.Must(uuid.NewV7())
			now := time.Now().UTC()
			if _, err := tx.Exec(ctx, `
				INSERT INTO admission_periods (
					id, tenant_id, unit_id, academic_year, name, opens_at, closes_at, is_published, is_active
				) VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE, TRUE)
			`, periodID, tenantID, unitID, "2026/2027", "Gelombang 1", now, now.AddDate(0, 3, 0)); err != nil {
				return mapPGSetupError(err)
			}
		}

		created = &item
		return nil
	})
	return created, err
}

func (s *PGStore) UpdatePlatformTenant(ctx context.Context, tenantID uuid.UUID, input handlers.UpdatePlatformTenantInput) (*handlers.PlatformTenantView, error) {
	var updated *handlers.PlatformTenantView
	err := database.WithBypassRLSTx(ctx, s.pool, func(tx pgx.Tx) error {
		var curName, instType, preset, plan string
		var themeRaw []byte
		var customDomain, verifyToken *string
		var domainVerified, isActive bool
		var expiresAt *time.Time
		var createdAt time.Time

		err := tx.QueryRow(ctx, `
			SELECT name, institution_type::text, terminology_preset::text,
			       COALESCE(theme_config, '{}'::jsonb), custom_domain, domain_verified_at IS NOT NULL,
			       domain_verification_token, subscription_plan, subscription_expires_at, is_active, created_at
			FROM tenants
			WHERE id = $1
		`, tenantID).Scan(
			&curName, &instType, &preset,
			&themeRaw, &customDomain, &domainVerified,
			&verifyToken, &plan, &expiresAt, &isActive, &createdAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return handlers.ErrSetupNotFound
			}
			return mapPGSetupError(err)
		}

		theme := parseStoredThemeConfig(themeRaw)
		if input.Name != nil {
			curName = *input.Name
		}
		if input.InstitutionType != nil {
			instType = string(*input.InstitutionType)
		}
		if input.TerminologyPreset != nil {
			preset = string(*input.TerminologyPreset)
		}
		if input.PrimaryColor != nil {
			theme.PrimaryColor = *input.PrimaryColor
		}
		if input.Tagline != nil {
			theme.Tagline = *input.Tagline
		}
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
		if input.DomainVerified != nil {
			domainVerified = *input.DomainVerified && customDomain != nil
		}
		if input.SubscriptionPlan != nil {
			plan = *input.SubscriptionPlan
		}
		if input.IsActive != nil {
			isActive = *input.IsActive
		}

		newThemeRaw, err := json.Marshal(theme)
		if err != nil {
			return err
		}

		var item handlers.PlatformTenantView
		var retInst, retPreset string
		var retTheme []byte
		err = tx.QueryRow(ctx, `
			UPDATE tenants
			SET name = $2,
			    institution_type = $3::institution_type,
			    terminology_preset = $4::terminology_preset,
			    theme_config = $5::jsonb,
			    custom_domain = $6,
			    domain_verification_token = $7,
			    domain_verified_at = CASE WHEN $8::boolean AND $6::text IS NOT NULL THEN COALESCE(domain_verified_at, NOW()) ELSE NULL END,
			    subscription_plan = $9,
			    is_active = $10
			WHERE id = $1
			RETURNING id, slug, name, institution_type::text, terminology_preset::text,
			          COALESCE(theme_config, '{}'::jsonb), custom_domain, domain_verified_at IS NOT NULL,
			          domain_verification_token, subscription_plan, subscription_expires_at, is_active, created_at
		`, tenantID, curName, instType, preset, newThemeRaw, customDomain, verifyToken, domainVerified, plan, isActive).Scan(
			&item.ID, &item.Slug, &item.Name, &retInst, &retPreset,
			&retTheme, &item.CustomDomain, &item.DomainVerified,
			&item.DomainVerificationToken, &item.SubscriptionPlan, &item.SubscriptionExpiresAt, &item.IsActive, &item.CreatedAt,
		)
		if err != nil {
			return mapPGSetupError(err)
		}
		item.InstitutionType = domain.InstitutionType(retInst)
		item.TerminologyPreset = domain.TerminologyPreset(retPreset)
		item.ThemeConfig = parseThemeConfigJSON(retTheme)

		_ = tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM units WHERE tenant_id = $1 AND is_active = TRUE`, tenantID).Scan(&item.UnitCount)
		updated = &item
		return nil
	})
	return updated, err
}

func (s *PGStore) GetPlatformTenantDomainInfo(ctx context.Context, tenantID uuid.UUID) (string, *string, *string, error) {
	var slug string
	var customDomain, verifyToken *string
	err := database.WithBypassRLSTx(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT slug, custom_domain, domain_verification_token
			FROM tenants
			WHERE id = $1
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

func (s *PGStore) MarkPlatformTenantDomainVerified(ctx context.Context, tenantID uuid.UUID) (*handlers.PlatformTenantView, error) {
	v := true
	return s.UpdatePlatformTenant(ctx, tenantID, handlers.UpdatePlatformTenantInput{DomainVerified: &v})
}

// --- DemoStore PlatformTenantStore implementation ---

func (s *DemoStore) ListPlatformTenants(_ context.Context) ([]handlers.PlatformTenantView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]handlers.PlatformTenantView, 0, len(s.byID))
	for _, rec := range s.byID {
		out = append(out, toPlatformTenantView(rec))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *DemoStore) CreatePlatformTenant(_ context.Context, input handlers.CreatePlatformTenantInput) (*handlers.PlatformTenantView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	slugKey := strings.ToLower(strings.TrimSpace(input.Slug))
	if _, exists := s.bySlug[slugKey]; exists {
		return nil, handlers.ErrSetupConflict
	}
	var customDomain, verifyToken string
	if input.CustomDomain != nil {
		customDomain = strings.ToLower(strings.TrimSpace(*input.CustomDomain))
	}
	if customDomain != "" {
		for _, other := range s.byID {
			if other.Tenant.CustomDomain != "" && strings.EqualFold(other.Tenant.CustomDomain, customDomain) {
				return nil, handlers.ErrSetupConflict
			}
		}
		verifyToken = domain.GenerateDomainVerificationToken()
	}

	now := time.Now().UTC()
	tenantID := uuid.Must(uuid.NewV7())
	rec := &demoTenantRecord{
		Tenant: catalog.TenantDef{
			ID:                tenantID,
			Slug:              slugKey,
			Name:              input.Name,
			InstitutionType:   input.InstitutionType,
			TerminologyPreset: input.TerminologyPreset,
			PrimaryColor:      input.PrimaryColor,
			Tagline:           input.Tagline,
			CustomDomain:      customDomain,
		},
		Overrides:               map[string]string{},
		SubscriptionPlan:        input.SubscriptionPlan,
		DomainVerified:          false,
		DomainVerificationToken: verifyToken,
		IsActive:                true,
		CreatedAt:               now,
	}

	if input.StarterUnitCode != "" && input.StarterUnitName != "" {
		unitID := uuid.Must(uuid.NewV7())
		rec.Units = []*demoUnitRecord{
			{
				UnitDef: catalog.UnitDef{
					ID:             unitID,
					Code:           input.StarterUnitCode,
					Name:           input.StarterUnitName,
					EducationLevel: input.StarterUnitLevel,
				},
				IsActive: true,
			},
		}
		periodID := uuid.Must(uuid.NewV7())
		rec.Periods = []*demoPeriodRecord{
			{
				PeriodDef: catalog.PeriodDef{
					ID:           periodID,
					UnitID:       unitID,
					AcademicYear: "2026/2027",
					Name:         "Gelombang 1",
					OpensAt:      now,
					ClosesAt:     now.AddDate(0, 3, 0),
				},
				IsPublished: true,
				IsActive:    true,
			},
		}
	}

	s.bySlug[slugKey] = rec
	s.byID[tenantID] = rec
	view := toPlatformTenantView(rec)
	return &view, nil
}

func (s *DemoStore) UpdatePlatformTenant(_ context.Context, tenantID uuid.UUID, input handlers.UpdatePlatformTenantInput) (*handlers.PlatformTenantView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	if input.Name != nil {
		rec.Tenant.Name = *input.Name
	}
	if input.InstitutionType != nil {
		rec.Tenant.InstitutionType = *input.InstitutionType
	}
	if input.TerminologyPreset != nil {
		rec.Tenant.TerminologyPreset = *input.TerminologyPreset
	}
	if input.PrimaryColor != nil {
		rec.Tenant.PrimaryColor = *input.PrimaryColor
	}
	if input.Tagline != nil {
		rec.Tenant.Tagline = *input.Tagline
	}
	if input.CustomDomain != nil {
		d := strings.ToLower(strings.TrimSpace(*input.CustomDomain))
		if d != "" {
			for id, other := range s.byID {
				if id != tenantID && other.Tenant.CustomDomain != "" && strings.EqualFold(other.Tenant.CustomDomain, d) {
					return nil, handlers.ErrSetupConflict
				}
			}
		}
		if d == "" {
			rec.Tenant.CustomDomain = ""
			rec.DomainVerified = false
			rec.DomainVerificationToken = ""
		} else if !strings.EqualFold(rec.Tenant.CustomDomain, d) {
			rec.Tenant.CustomDomain = d
			rec.DomainVerified = false
			rec.DomainVerificationToken = domain.GenerateDomainVerificationToken()
		} else if rec.DomainVerificationToken == "" {
			rec.DomainVerificationToken = domain.GenerateDomainVerificationToken()
		}
	}
	if input.DomainVerified != nil {
		rec.DomainVerified = *input.DomainVerified && rec.Tenant.CustomDomain != ""
	}
	if input.SubscriptionPlan != nil {
		rec.SubscriptionPlan = *input.SubscriptionPlan
	}
	if input.IsActive != nil {
		rec.IsActive = *input.IsActive
	}

	view := toPlatformTenantView(rec)
	return &view, nil
}

func (s *DemoStore) GetPlatformTenantDomainInfo(_ context.Context, tenantID uuid.UUID) (string, *string, *string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return "", nil, nil, handlers.ErrSetupNotFound
	}
	var domPtr, tokPtr *string
	if rec.Tenant.CustomDomain != "" {
		d := rec.Tenant.CustomDomain
		domPtr = &d
		if rec.DomainVerificationToken == "" {
			rec.DomainVerificationToken = domain.GenerateDomainVerificationToken()
		}
		t := rec.DomainVerificationToken
		tokPtr = &t
	}
	return rec.Tenant.Slug, domPtr, tokPtr, nil
}

func (s *DemoStore) MarkPlatformTenantDomainVerified(_ context.Context, tenantID uuid.UUID) (*handlers.PlatformTenantView, error) {
	v := true
	return s.UpdatePlatformTenant(context.Background(), tenantID, handlers.UpdatePlatformTenantInput{DomainVerified: &v})
}

func toPlatformTenantView(rec *demoTenantRecord) handlers.PlatformTenantView {
	var domainPtr, tokPtr *string
	if rec.Tenant.CustomDomain != "" {
		d := rec.Tenant.CustomDomain
		domainPtr = &d
	}
	if rec.DomainVerificationToken != "" {
		t := rec.DomainVerificationToken
		tokPtr = &t
	}
	activeUnits := 0
	for _, u := range rec.Units {
		if u.IsActive {
			activeUnits++
		}
	}
	plan := rec.SubscriptionPlan
	if plan == "" {
		plan = "starter"
	}
	logoURL := rec.LogoURL
	if computed := publicLogoURLFromStorageKey(rec.LogoStorageKey); computed != nil {
		logoURL = computed
	}
	return handlers.PlatformTenantView{
		ID:                rec.Tenant.ID,
		Slug:              rec.Tenant.Slug,
		Name:              rec.Tenant.Name,
		InstitutionType:   rec.Tenant.InstitutionType,
		TerminologyPreset: rec.Tenant.TerminologyPreset,
		ThemeConfig: handlers.ThemeConfigView{
			PrimaryColor: rec.Tenant.PrimaryColor,
			LogoURL:      logoURL,
			Tagline:      rec.Tenant.Tagline,
		},
		CustomDomain:            domainPtr,
		DomainVerified:          rec.DomainVerified,
		DomainVerificationToken: tokPtr,
		SubscriptionPlan:        plan,
		IsActive:                rec.IsActive,
		UnitCount:               activeUnits,
		CreatedAt:               rec.CreatedAt,
	}
}
