package public

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/handlers"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/modules/auth"
	"github.com/pbdb/platform/internal/modules/catalog"
)

type demoUnitRecord struct {
	catalog.UnitDef
	IsActive bool
}

type demoTrackRecord struct {
	catalog.TrackDef
	IsActive bool
}

type demoPeriodRecord struct {
	catalog.PeriodDef
	AnnouncementAt *time.Time
	IsPublished    bool
	IsActive       bool
	Tracks         []*demoTrackRecord
}

type demoStaffRecord struct {
	Member handlers.StaffMemberView
}

type demoTenantRecord struct {
	Tenant                  catalog.TenantDef
	Overrides               map[string]string
	LogoURL                 *string
	LogoStorageKey          *string
	SubscriptionPlan        string
	DomainVerified          bool
	DomainVerificationToken string
	PaymentConfig           domain.TenantPaymentConfig
	Staff                   map[uuid.UUID]*demoStaffRecord
	IsActive                bool
	CreatedAt               time.Time
	Units                   []*demoUnitRecord
	Periods                 []*demoPeriodRecord
}

// DemoStore provides mutable in-memory portal, catalog, and admin setup data for local development without PostgreSQL.
type DemoStore struct {
	mu     sync.RWMutex
	bySlug map[string]*demoTenantRecord
	byID   map[uuid.UUID]*demoTenantRecord
}

// NewDemoStore creates a pre-seeded mutable store containing demo tenants from the shared catalog.
func NewDemoStore() *DemoStore {
	s := &DemoStore{
		bySlug: make(map[string]*demoTenantRecord),
		byID:   make(map[uuid.UUID]*demoTenantRecord),
	}

	for _, t := range catalog.DemoTenants() {
		unitDefs, periodDefs := catalog.PortalForTenant(t.Slug, t.Name, t.TerminologyPreset)
		units := make([]*demoUnitRecord, 0, len(unitDefs))
		for _, u := range unitDefs {
			units = append(units, &demoUnitRecord{UnitDef: u, IsActive: true})
		}

		periods := make([]*demoPeriodRecord, 0, len(periodDefs))
		for _, p := range periodDefs {
			tracks := make([]*demoTrackRecord, 0, len(p.Tracks))
			for _, tr := range p.Tracks {
				trCopy := tr
				trCopy.Requirements = slices.Clone(tr.Requirements)
				tracks = append(tracks, &demoTrackRecord{TrackDef: trCopy, IsActive: true})
			}
			pCopy := p
			pCopy.Tracks = nil
			periods = append(periods, &demoPeriodRecord{
				PeriodDef:   pCopy,
				IsPublished: true,
				IsActive:    true,
				Tracks:      tracks,
			})
		}

		staffMap := make(map[uuid.UUID]*demoStaffRecord)
		for _, acc := range auth.DemoSeedAccounts() {
			var matched []handlers.StaffRoleView
			for _, sc := range acc.Roles {
				if sc.TenantID != t.ID {
					continue
				}
				var unitName *string
				if sc.UnitID != nil {
					for _, u := range units {
						if u.ID == *sc.UnitID {
							n := u.Name
							unitName = &n
							break
						}
					}
				}
				matched = append(matched, handlers.StaffRoleView{
					ID:       uuid.Must(uuid.NewV7()),
					Role:     sc.Role,
					UnitID:   sc.UnitID,
					UnitName: unitName,
					IsActive: true,
				})
			}
			if len(matched) > 0 {
				staffMap[acc.User.ID] = &demoStaffRecord{
					Member: handlers.StaffMemberView{
						UserID:          acc.User.ID,
						FullName:        acc.User.FullName,
						Email:           acc.User.Email,
						WhatsAppPhone:   acc.User.WhatsAppPhone,
						IsPlatformAdmin: acc.User.IsPlatformAdmin,
						Roles:           matched,
						CreatedAt:       time.Now().UTC(),
					},
				}
			}
		}

		rec := &demoTenantRecord{
			Tenant:                  t,
			Overrides:               map[string]string{},
			SubscriptionPlan:        "pro",
			DomainVerified:          t.CustomDomain != "",
			DomainVerificationToken: domain.GenerateDomainVerificationToken(),
			Staff:                   staffMap,
			IsActive:                true,
			CreatedAt:               time.Now().UTC(),
			Units:                   units,
			Periods:                 periods,
		}
		s.bySlug[strings.ToLower(t.Slug)] = rec
		s.byID[t.ID] = rec
	}

	return s
}

func toResolvedTenant(rec *demoTenantRecord) *middleware.ResolvedTenant {
	return &middleware.ResolvedTenant{
		ID:                rec.Tenant.ID,
		Slug:              rec.Tenant.Slug,
		Name:              rec.Tenant.Name,
		InstitutionType:   rec.Tenant.InstitutionType,
		TerminologyPreset: rec.Tenant.TerminologyPreset,
		PrimaryColor:      rec.Tenant.PrimaryColor,
		Tagline:           rec.Tenant.Tagline,
		CustomDomain:      rec.Tenant.CustomDomain,
	}
}

func (s *DemoStore) FindByHost(_ context.Context, host string) (*middleware.ResolvedTenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rec := range s.byID {
		if rec.IsActive && rec.DomainVerified && rec.Tenant.CustomDomain != "" && strings.EqualFold(rec.Tenant.CustomDomain, host) {
			return toResolvedTenant(rec), nil
		}
	}
	return nil, nil
}

func (s *DemoStore) FindBySlug(_ context.Context, slug string) (*middleware.ResolvedTenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.bySlug[strings.ToLower(slug)]
	if !ok || !rec.IsActive {
		return nil, nil
	}
	return toResolvedTenant(rec), nil
}

func toUnitView(u *demoUnitRecord) handlers.UnitView {
	addr := u.Address
	lat := u.Latitude
	lon := u.Longitude
	var addrPtr *string
	if addr != "" {
		addrPtr = &addr
	}
	var latPtr, lonPtr *float64
	if lat != 0 || lon != 0 {
		latPtr = &lat
		lonPtr = &lon
	}
	return handlers.UnitView{
		ID:             u.ID,
		Code:           u.Code,
		Name:           u.Name,
		EducationLevel: u.EducationLevel,
		Address:        addrPtr,
		Latitude:       latPtr,
		Longitude:      lonPtr,
		IsActive:       u.IsActive,
	}
}

func toTrackView(unitID, periodID uuid.UUID, tr *demoTrackRecord) handlers.TrackView {
	reqs := make([]handlers.RequirementView, 0, len(tr.Requirements))
	for _, r := range tr.Requirements {
		var descPtr *string
		if r.Description != "" {
			d := r.Description
			descPtr = &d
		}
		reqs = append(reqs, handlers.RequirementView{
			ID:            r.ID,
			DocCode:       r.DocCode,
			Label:         r.Label,
			Description:   descPtr,
			IsRequired:    r.IsRequired,
			AcceptedMimes: slices.Clone(r.AcceptedMimes),
			MaxSizeKB:     r.MaxSizeKB,
		})
	}
	return handlers.TrackView{
		ID:                   tr.ID,
		PeriodID:             periodID,
		UnitID:               unitID,
		Code:                 tr.Code,
		DisplayName:          tr.DisplayName,
		Description:          tr.Description,
		SelectionMode:        tr.SelectionMode,
		Quota:                tr.Quota,
		ActiveApplicantCount: tr.ActiveApplicantCount,
		RegistrationFeeIDR:   tr.RegistrationFeeIDR,
		ReEnrollmentFeeIDR:   tr.ReEnrollmentFeeIDR,
		SpilloverTargetTrack: tr.SpilloverTargetTrackID,
		RequiresCoordinates:  tr.RequiresCoordinates,
		RequiresAchievements: tr.RequiresAchievements,
		MinAgeMonths:         tr.MinAgeMonths,
		MaxAgeMonths:         tr.MaxAgeMonths,
		AgeReferenceDate:     tr.AgeReferenceDate,
		ScoringWeights:       maps.Clone(tr.ScoringWeights),
		CustomFormSchema:     slices.Clone(tr.CustomFormSchema),
		IsActive:             tr.IsActive,
		Requirements:         reqs,
	}
}

func toPeriodView(p *demoPeriodRecord) handlers.PeriodView {
	tracks := make([]handlers.TrackView, 0, len(p.Tracks))
	for _, tr := range p.Tracks {
		if !tr.IsActive {
			continue
		}
		tracks = append(tracks, toTrackView(p.UnitID, p.ID, tr))
	}
	return handlers.PeriodView{
		ID:             p.ID,
		UnitID:         p.UnitID,
		AcademicYear:   p.AcademicYear,
		Name:           p.Name,
		OpensAt:        p.OpensAt,
		ClosesAt:       p.ClosesAt,
		AnnouncementAt: p.AnnouncementAt,
		IsPublished:    p.IsPublished,
		IsActive:       p.IsActive,
		Tracks:         tracks,
	}
}

func (s *DemoStore) GetPortalData(_ context.Context, tenant *middleware.ResolvedTenant) (handlers.PublicPortalPayload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenant.ID]
	if !ok {
		return handlers.PublicPortalPayload{
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
		}, nil
	}

	activeUnits := make(map[uuid.UUID]struct{}, len(rec.Units))
	units := make([]handlers.UnitView, 0, len(rec.Units))
	for _, u := range rec.Units {
		if !u.IsActive {
			continue
		}
		activeUnits[u.ID] = struct{}{}
		units = append(units, toUnitView(u))
	}

	periods := make([]handlers.PeriodView, 0, len(rec.Periods))
	for _, p := range rec.Periods {
		if !p.IsActive || !p.IsPublished {
			continue
		}
		if _, unitOK := activeUnits[p.UnitID]; !unitOK {
			continue
		}
		periods = append(periods, toPeriodView(p))
	}

	logoURL := rec.LogoURL
	if computed := publicLogoURLFromStorageKey(rec.LogoStorageKey); computed != nil {
		logoURL = computed
	}
	return handlers.PublicPortalPayload{
		Tenant: handlers.PublicTenantView{
			ID:                   rec.Tenant.ID,
			Slug:                 rec.Tenant.Slug,
			Name:                 rec.Tenant.Name,
			InstitutionType:      rec.Tenant.InstitutionType,
			TerminologyPreset:    rec.Tenant.TerminologyPreset,
			TerminologyOverrides: maps.Clone(rec.Overrides),
			ThemeConfig: handlers.ThemeConfigView{
				PrimaryColor: rec.Tenant.PrimaryColor,
				LogoURL:      logoURL,
				Tagline:      rec.Tenant.Tagline,
			},
		},
		Units:   units,
		Periods: periods,
	}, nil
}

func (s *DemoStore) CheckStatus(_ context.Context, tenantID uuid.UUID, registrationNo string) (*handlers.PublicStatusResult, error) {
	if strings.EqualFold(registrationNo, "SPMB-2601-0001") {
		submitted := time.Now().Add(-48 * time.Hour).UTC()
		return &handlers.PublicStatusResult{
			RegistrationNo: "SPMB-2601-0001",
			MaskedName:     "A*** F***",
			UnitName:       "SMA Harapan Nusantara",
			TrackName:      "Jalur Domisili",
			AcademicYear:   "2026/2027",
			Status:         domain.StatusRevisionRequired,
			SubmittedAt:    &submitted,
		}, nil
	}
	_ = tenantID
	return nil, nil
}

func (s *DemoStore) GetTenantLogoKey(_ context.Context, tenantID uuid.UUID) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.byID[tenantID]
	if !ok || rec.LogoStorageKey == nil {
		return "", nil
	}
	return *rec.LogoStorageKey, nil
}

func (s *DemoStore) VerifyDocumentByID(_ context.Context, tenantID, _ uuid.UUID) (*handlers.PublicDocumentVerifyResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.byID[tenantID]; !ok {
		return nil, nil
	}
	return nil, nil
}

// --- catalog.Provider implementation backed by mutable DemoStore state ---

func (s *DemoStore) FindTrack(tenantID, unitID, periodID, trackID uuid.UUID) (*catalog.TrackLookup, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, false
	}
	var unit *demoUnitRecord
	for _, u := range rec.Units {
		if u.ID == unitID && u.IsActive {
			unit = u
			break
		}
	}
	if unit == nil {
		return nil, false
	}
	for _, p := range rec.Periods {
		if p.ID != periodID || p.UnitID != unitID || !p.IsActive || !p.IsPublished {
			continue
		}
		for _, tr := range p.Tracks {
			if tr.ID == trackID && tr.IsActive {
				return &catalog.TrackLookup{
					Tenant: rec.Tenant,
					Unit:   unit.UnitDef,
					Period: snapshotPeriodDef(p),
					Track:  snapshotTrackDef(tr),
				}, true
			}
		}
	}
	return nil, false
}

func (s *DemoStore) FindTrackByID(tenantID, trackID uuid.UUID) (*catalog.TrackLookup, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, false
	}
	units := make(map[uuid.UUID]*demoUnitRecord, len(rec.Units))
	for _, u := range rec.Units {
		units[u.ID] = u
	}
	for _, p := range rec.Periods {
		unit := units[p.UnitID]
		if unit == nil {
			continue
		}
		for _, tr := range p.Tracks {
			if tr.ID == trackID {
				return &catalog.TrackLookup{
					Tenant: rec.Tenant,
					Unit:   unit.UnitDef,
					Period: snapshotPeriodDef(p),
					Track:  snapshotTrackDef(tr),
				}, true
			}
		}
	}
	return nil, false
}

func (s *DemoStore) FindPeriodTracks(tenantID, periodID uuid.UUID) ([]catalog.TrackLookup, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, false
	}
	units := make(map[uuid.UUID]*demoUnitRecord, len(rec.Units))
	for _, u := range rec.Units {
		units[u.ID] = u
	}
	for _, p := range rec.Periods {
		if p.ID != periodID {
			continue
		}
		unit := units[p.UnitID]
		if unit == nil {
			return nil, false
		}
		out := make([]catalog.TrackLookup, 0, len(p.Tracks))
		for _, tr := range p.Tracks {
			if !tr.IsActive {
				continue
			}
			out = append(out, catalog.TrackLookup{
				Tenant: rec.Tenant,
				Unit:   unit.UnitDef,
				Period: snapshotPeriodDef(p),
				Track:  snapshotTrackDef(tr),
			})
		}
		return out, true
	}
	return nil, false
}

func (s *DemoStore) UnitBelongsToTenant(tenantID, unitID uuid.UUID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return false
	}
	for _, u := range rec.Units {
		if u.ID == unitID {
			return true
		}
	}
	return false
}

func snapshotTrackDef(tr *demoTrackRecord) catalog.TrackDef {
	cp := tr.TrackDef
	cp.ScoringWeights = maps.Clone(tr.ScoringWeights)
	cp.CustomFormSchema = slices.Clone(tr.CustomFormSchema)
	cp.Requirements = make([]catalog.RequirementDef, len(tr.Requirements))
	for i, r := range tr.Requirements {
		r.AcceptedMimes = slices.Clone(r.AcceptedMimes)
		cp.Requirements[i] = r
	}
	return cp
}

func snapshotPeriodDef(p *demoPeriodRecord) catalog.PeriodDef {
	cp := p.PeriodDef
	cp.Tracks = make([]catalog.TrackDef, 0, len(p.Tracks))
	for _, tr := range p.Tracks {
		if tr.IsActive {
			cp.Tracks = append(cp.Tracks, snapshotTrackDef(tr))
		}
	}
	return cp
}

// --- handlers.AdminSetupStore implementation on DemoStore ---

func toSettingsPayload(rec *demoTenantRecord) *handlers.TenantSettingsPayload {
	var custom, verifyTok *string
	if rec.Tenant.CustomDomain != "" {
		d := rec.Tenant.CustomDomain
		custom = &d
	}
	if rec.DomainVerificationToken != "" {
		t := rec.DomainVerificationToken
		verifyTok = &t
	}
	logoURL := rec.LogoURL
	if computed := publicLogoURLFromStorageKey(rec.LogoStorageKey); computed != nil {
		logoURL = computed
	}
	return &handlers.TenantSettingsPayload{
		ID:                   rec.Tenant.ID,
		Slug:                 rec.Tenant.Slug,
		Name:                 rec.Tenant.Name,
		InstitutionType:      rec.Tenant.InstitutionType,
		TerminologyPreset:    rec.Tenant.TerminologyPreset,
		TerminologyOverrides: maps.Clone(rec.Overrides),
		ThemeConfig: handlers.ThemeConfigView{
			PrimaryColor: rec.Tenant.PrimaryColor,
			LogoURL:      logoURL,
			Tagline:      rec.Tenant.Tagline,
		},
		CustomDomain:            custom,
		DomainVerified:          rec.DomainVerified,
		DomainVerificationToken: verifyTok,
		PaymentConfig:           toPaymentConfigView(rec.PaymentConfig),
	}
}

func (s *DemoStore) GetTenantSettings(_ context.Context, tenantID uuid.UUID) (*handlers.TenantSettingsPayload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	return toSettingsPayload(rec), nil
}

func (s *DemoStore) UpdateTenantSettings(_ context.Context, tenantID uuid.UUID, input handlers.UpdateTenantSettingsInput) (*handlers.TenantSettingsPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	if input.Name != nil {
		rec.Tenant.Name = *input.Name
	}
	if input.TerminologyPreset != nil {
		rec.Tenant.TerminologyPreset = *input.TerminologyPreset
	}
	if input.TerminologyOverrides != nil {
		rec.Overrides = maps.Clone(input.TerminologyOverrides)
	}
	if input.ThemeConfig != nil {
		if input.ThemeConfig.PrimaryColor != nil {
			rec.Tenant.PrimaryColor = *input.ThemeConfig.PrimaryColor
		}
		if input.ThemeConfig.Tagline != nil {
			rec.Tenant.Tagline = *input.ThemeConfig.Tagline
		}
		if input.ThemeConfig.LogoURL != nil {
			trimmed := strings.TrimSpace(*input.ThemeConfig.LogoURL)
			if trimmed == "" {
				rec.LogoURL = nil
				rec.LogoStorageKey = nil
			} else {
				rec.LogoURL = &trimmed
				if !strings.HasPrefix(trimmed, "/api/v1/public/logo") {
					rec.LogoStorageKey = nil
				}
			}
		}
	}
	if input.CustomDomain != nil {
		d := strings.ToLower(strings.TrimSpace(*input.CustomDomain))
		if d != "" {
			for otherID, other := range s.byID {
				if otherID != tenantID && strings.EqualFold(other.Tenant.CustomDomain, d) {
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
	if input.PaymentConfig != nil {
		rec.PaymentConfig = mergePaymentConfigUpdate(rec.PaymentConfig, input.PaymentConfig)
	}

	return toSettingsPayload(rec), nil
}

func (s *DemoStore) SetTenantLogoKey(_ context.Context, tenantID uuid.UUID, logoStorageKey *string) (*handlers.TenantSettingsPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	if logoStorageKey == nil || strings.TrimSpace(*logoStorageKey) == "" {
		rec.LogoStorageKey = nil
		rec.LogoURL = nil
	} else {
		k := strings.TrimSpace(*logoStorageKey)
		rec.LogoStorageKey = &k
		rec.LogoURL = publicLogoURLFromStorageKey(&k)
	}
	return toSettingsPayload(rec), nil
}

func (s *DemoStore) GetTenantDomainInfo(_ context.Context, tenantID uuid.UUID) (string, *string, *string, error) {
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

func (s *DemoStore) MarkTenantDomainVerified(_ context.Context, tenantID uuid.UUID) (*handlers.TenantSettingsPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok || rec.Tenant.CustomDomain == "" {
		return nil, handlers.ErrSetupNotFound
	}
	rec.DomainVerified = true
	return toSettingsPayload(rec), nil
}

func (s *DemoStore) GetTenantPaymentConfig(_ context.Context, tenantID uuid.UUID) (*domain.TenantPaymentConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, nil
	}
	cp := rec.PaymentConfig
	return &cp, nil
}

func (s *DemoStore) ListAdminUnits(_ context.Context, tenantID uuid.UUID) ([]handlers.UnitView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	out := make([]handlers.UnitView, 0, len(rec.Units))
	for _, u := range rec.Units {
		out = append(out, toUnitView(u))
	}
	return out, nil
}

func (s *DemoStore) CreateUnit(_ context.Context, tenantID uuid.UUID, input handlers.CreateUnitInput) (*handlers.UnitView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	for _, existing := range rec.Units {
		if strings.EqualFold(existing.Code, input.Code) {
			return nil, handlers.ErrSetupConflict
		}
	}

	var addr string
	if input.Address != nil {
		addr = strings.TrimSpace(*input.Address)
	}
	var lat, lon float64
	if input.Latitude != nil {
		lat = *input.Latitude
	}
	if input.Longitude != nil {
		lon = *input.Longitude
	}

	u := &demoUnitRecord{
		UnitDef: catalog.UnitDef{
			ID:             uuid.Must(uuid.NewV7()),
			Code:           input.Code,
			Name:           input.Name,
			EducationLevel: input.EducationLevel,
			Address:        addr,
			Latitude:       lat,
			Longitude:      lon,
		},
		IsActive: true,
	}
	rec.Units = append(rec.Units, u)
	view := toUnitView(u)
	return &view, nil
}

func (s *DemoStore) UpdateUnit(_ context.Context, tenantID, unitID uuid.UUID, input handlers.UpdateUnitInput) (*handlers.UnitView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	for _, u := range rec.Units {
		if u.ID != unitID {
			continue
		}
		if input.Name != nil {
			u.Name = *input.Name
		}
		if input.EducationLevel != nil {
			u.EducationLevel = *input.EducationLevel
		}
		if input.Address != nil {
			u.Address = strings.TrimSpace(*input.Address)
		}
		if input.Latitude != nil {
			u.Latitude = *input.Latitude
		}
		if input.Longitude != nil {
			u.Longitude = *input.Longitude
		}
		if input.IsActive != nil {
			u.IsActive = *input.IsActive
		}
		view := toUnitView(u)
		return &view, nil
	}
	return nil, handlers.ErrSetupNotFound
}

func (s *DemoStore) GetPeriodUnitID(_ context.Context, tenantID, periodID uuid.UUID) (uuid.UUID, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return uuid.Nil, false, nil
	}
	for _, p := range rec.Periods {
		if p.ID == periodID {
			return p.UnitID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (s *DemoStore) CreatePeriod(_ context.Context, tenantID uuid.UUID, input handlers.CreatePeriodInput) (*handlers.PeriodView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	unitExists := false
	for _, u := range rec.Units {
		if u.ID == input.UnitID {
			unitExists = true
			break
		}
	}
	if !unitExists {
		return nil, handlers.ErrSetupNotFound
	}

	published := true
	if input.IsPublished != nil {
		published = *input.IsPublished
	}
	p := &demoPeriodRecord{
		PeriodDef: catalog.PeriodDef{
			ID:           uuid.Must(uuid.NewV7()),
			UnitID:       input.UnitID,
			AcademicYear: input.AcademicYear,
			Name:         input.Name,
			OpensAt:      input.OpensAt.UTC(),
			ClosesAt:     input.ClosesAt.UTC(),
		},
		AnnouncementAt: input.AnnouncementAt,
		IsPublished:    published,
		IsActive:       true,
		Tracks:         []*demoTrackRecord{},
	}
	rec.Periods = append(rec.Periods, p)
	view := toPeriodView(p)
	return &view, nil
}

func (s *DemoStore) UpdatePeriod(_ context.Context, tenantID, periodID uuid.UUID, input handlers.UpdatePeriodInput) (*handlers.PeriodView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	for _, p := range rec.Periods {
		if p.ID != periodID {
			continue
		}
		opensAt := p.OpensAt
		closesAt := p.ClosesAt
		if input.OpensAt != nil {
			opensAt = input.OpensAt.UTC()
		}
		if input.ClosesAt != nil {
			closesAt = input.ClosesAt.UTC()
		}
		if !closesAt.After(opensAt) {
			return nil, &handlers.SetupValidationError{
				Message: "Tanggal tutup harus lebih lambat daripada tanggal buka.",
				Fields:  map[string]string{"closes_at": "Tanggal tutup harus lebih lambat daripada tanggal buka."},
			}
		}
		p.OpensAt = opensAt
		p.ClosesAt = closesAt
		if input.AcademicYear != nil {
			p.AcademicYear = *input.AcademicYear
		}
		if input.Name != nil {
			p.Name = *input.Name
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
		view := toPeriodView(p)
		return &view, nil
	}
	return nil, handlers.ErrSetupNotFound
}

func (s *DemoStore) GetTrackUnitID(_ context.Context, tenantID, trackID uuid.UUID) (uuid.UUID, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return uuid.Nil, false, nil
	}
	for _, p := range rec.Periods {
		for _, tr := range p.Tracks {
			if tr.ID == trackID {
				return p.UnitID, true, nil
			}
		}
	}
	return uuid.Nil, false, nil
}

func (s *DemoStore) CreateTrack(_ context.Context, tenantID uuid.UUID, input handlers.CreateTrackInput) (*handlers.TrackView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	for _, p := range rec.Periods {
		if p.ID != input.PeriodID {
			continue
		}
		for _, existing := range p.Tracks {
			if strings.EqualFold(existing.Code, input.Code) {
				return nil, handlers.ErrSetupConflict
			}
		}
		tr := &demoTrackRecord{
			TrackDef: catalog.TrackDef{
				ID:                     uuid.Must(uuid.NewV7()),
				Code:                   input.Code,
				DisplayName:            input.DisplayName,
				Description:            input.Description,
				SelectionMode:          input.SelectionMode,
				Quota:                  input.Quota,
				RegistrationFeeIDR:     input.RegistrationFeeIDR,
				ReEnrollmentFeeIDR:     input.ReEnrollmentFeeIDR,
				SpilloverTargetTrackID: input.SpilloverTargetTrackID,
				RequiresCoordinates:    input.RequiresCoordinates,
				RequiresAchievements:   input.RequiresAchievements,
				MinAgeMonths:           input.MinAgeMonths,
				MaxAgeMonths:           input.MaxAgeMonths,
				AgeReferenceDate:       input.AgeReferenceDate,
				ScoringWeights:         maps.Clone(input.ScoringWeights),
				CustomFormSchema:       slices.Clone(input.CustomFormSchema),
				Requirements:           []catalog.RequirementDef{},
			},
			IsActive: true,
		}
		p.Tracks = append(p.Tracks, tr)
		view := toTrackView(p.UnitID, p.ID, tr)
		return &view, nil
	}
	return nil, handlers.ErrSetupNotFound
}

func (s *DemoStore) UpdateTrack(_ context.Context, tenantID, trackID uuid.UUID, input handlers.UpdateTrackInput) (*handlers.TrackView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	for _, p := range rec.Periods {
		for _, tr := range p.Tracks {
			if tr.ID != trackID {
				continue
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
				tr.SpilloverTargetTrackID = input.SpilloverTargetTrackID
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
				tr.ScoringWeights = maps.Clone(input.ScoringWeights)
			}
			if input.CustomFormSchema != nil {
				tr.CustomFormSchema = slices.Clone(input.CustomFormSchema)
			}
			if input.IsActive != nil {
				tr.IsActive = *input.IsActive
			}
			view := toTrackView(p.UnitID, p.ID, tr)
			return &view, nil
		}
	}
	return nil, handlers.ErrSetupNotFound
}

func (s *DemoStore) ReplaceTrackRequirements(
	_ context.Context,
	tenantID, trackID uuid.UUID,
	items []handlers.TrackRequirementItemInput,
) ([]handlers.RequirementView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	for _, p := range rec.Periods {
		for _, tr := range p.Tracks {
			if tr.ID != trackID {
				continue
			}
			existingByCode := make(map[string]uuid.UUID, len(tr.Requirements))
			for _, old := range tr.Requirements {
				existingByCode[strings.ToLower(old.DocCode)] = old.ID
			}
			nextReqs := make([]catalog.RequirementDef, 0, len(items))
			out := make([]handlers.RequirementView, 0, len(items))
			for _, item := range items {
				reqID, had := existingByCode[item.DocCode]
				if !had {
					reqID = uuid.Must(uuid.NewV7())
				}
				var descStr string
				if item.Description != nil {
					descStr = strings.TrimSpace(*item.Description)
				}
				mimes := slices.Clone(item.AcceptedMimes)
				nextReqs = append(nextReqs, catalog.RequirementDef{
					ID:            reqID,
					DocCode:       item.DocCode,
					Label:         item.Label,
					Description:   descStr,
					IsRequired:    item.IsRequired,
					AcceptedMimes: mimes,
					MaxSizeKB:     item.MaxSizeKB,
				})
				var descPtr *string
				if descStr != "" {
					descPtr = &descStr
				}
				out = append(out, handlers.RequirementView{
					ID:            reqID,
					DocCode:       item.DocCode,
					Label:         item.Label,
					Description:   descPtr,
					IsRequired:    item.IsRequired,
					AcceptedMimes: slices.Clone(mimes),
					MaxSizeKB:     item.MaxSizeKB,
				})
			}
			tr.Requirements = nextReqs
			return out, nil
		}
	}
	return nil, handlers.ErrSetupNotFound
}

func (s *DemoStore) ListStaff(_ context.Context, tenantID uuid.UUID) ([]handlers.StaffMemberView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	out := make([]handlers.StaffMemberView, 0, len(rec.Staff))
	for _, entry := range rec.Staff {
		cp := entry.Member
		cp.Roles = slices.Clone(entry.Member.Roles)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].FullName) < strings.ToLower(out[j].FullName)
	})
	return out, nil
}

func (s *DemoStore) CreateOrAssignStaff(_ context.Context, tenantID uuid.UUID, input handlers.CreateStaffInput) (*handlers.StaffMemberView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	if rec.Staff == nil {
		rec.Staff = map[uuid.UUID]*demoStaffRecord{}
	}

	unitID := input.UnitID
	if input.Role == domain.RoleTenantAdmin {
		unitID = nil
	}
	var unitName *string
	if unitID != nil {
		found := false
		for _, u := range rec.Units {
			if u.ID == *unitID {
				n := u.Name
				unitName = &n
				found = true
				break
			}
		}
		if !found {
			return nil, handlers.ErrSetupNotFound
		}
	}

	var existing *demoStaffRecord
	for _, st := range rec.Staff {
		if st.Member.Email != nil && strings.EqualFold(*st.Member.Email, input.Email) {
			existing = st
			break
		}
	}
	if existing == nil {
		if len(input.Password) < 6 {
			return nil, &handlers.SetupValidationError{
				Message: "Password minimal 6 karakter wajib diisi untuk akun staf baru.",
				Fields:  map[string]string{"password": "Password minimal 6 karakter wajib diisi."},
			}
		}
		emailCopy := input.Email
		existing = &demoStaffRecord{
			Member: handlers.StaffMemberView{
				UserID:        uuid.Must(uuid.NewV7()),
				FullName:      input.FullName,
				Email:         &emailCopy,
				WhatsAppPhone: input.WhatsAppPhone,
				Roles:         []handlers.StaffRoleView{},
				CreatedAt:     time.Now().UTC(),
			},
		}
		rec.Staff[existing.Member.UserID] = existing
	}

	replaced := false
	for i := range existing.Member.Roles {
		r := &existing.Member.Roles[i]
		sameUnit := (r.UnitID == nil && unitID == nil) || (r.UnitID != nil && unitID != nil && *r.UnitID == *unitID)
		if r.Role == input.Role && sameUnit {
			r.IsActive = true
			r.UnitName = unitName
			replaced = true
			break
		}
	}
	if !replaced {
		existing.Member.Roles = append(existing.Member.Roles, handlers.StaffRoleView{
			ID:       uuid.Must(uuid.NewV7()),
			Role:     input.Role,
			UnitID:   unitID,
			UnitName: unitName,
			IsActive: true,
		})
	}

	cp := existing.Member
	cp.Roles = slices.Clone(existing.Member.Roles)
	return &cp, nil
}

func (s *DemoStore) UpdateStaff(_ context.Context, tenantID, actorUserID, targetUserID uuid.UUID, input handlers.UpdateStaffInput) (*handlers.StaffMemberView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.byID[tenantID]
	if !ok {
		return nil, handlers.ErrSetupNotFound
	}
	entry, ok := rec.Staff[targetUserID]
	if !ok || len(entry.Member.Roles) == 0 {
		return nil, handlers.ErrSetupNotFound
	}
	if input.IsActive != nil && !*input.IsActive && actorUserID == targetUserID {
		return nil, &handlers.SetupValidationError{
			Message: "Anda tidak dapat menonaktifkan peran akun Anda sendiri.",
			Fields:  map[string]string{"is_active": "Tidak dapat menonaktifkan akun sendiri."},
		}
	}
	if !entry.Member.IsPlatformAdmin {
		if input.FullName != nil {
			entry.Member.FullName = *input.FullName
		}
		if input.WhatsAppPhone != nil {
			if *input.WhatsAppPhone == "" {
				entry.Member.WhatsAppPhone = nil
			} else {
				p := *input.WhatsAppPhone
				entry.Member.WhatsAppPhone = &p
			}
		}
	}
	if input.Role != nil || input.UnitID != nil || input.ClearUnit {
		primary := &entry.Member.Roles[0]
		if input.Role != nil {
			primary.Role = *input.Role
		}
		if input.ClearUnit {
			primary.UnitID = nil
			primary.UnitName = nil
		}
		if input.UnitID != nil {
			primary.UnitID = input.UnitID
		}
		if primary.Role == domain.RoleTenantAdmin {
			primary.UnitID = nil
			primary.UnitName = nil
		} else if primary.UnitID != nil {
			found := false
			for _, u := range rec.Units {
				if u.ID == *primary.UnitID {
					n := u.Name
					primary.UnitName = &n
					found = true
					break
				}
			}
			if !found {
				return nil, handlers.ErrSetupNotFound
			}
		}
	}
	if input.IsActive != nil {
		for i := range entry.Member.Roles {
			entry.Member.Roles[i].IsActive = *input.IsActive
		}
	}

	cp := entry.Member
	cp.Roles = slices.Clone(entry.Member.Roles)
	return &cp, nil
}
