package catalog

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

// TenantDef describes a seeded demo institution.
type TenantDef struct {
	ID                uuid.UUID
	Slug              string
	Name              string
	InstitutionType   domain.InstitutionType
	TerminologyPreset domain.TerminologyPreset
	PrimaryColor      string
	Tagline           string
	CustomDomain      string
}

// UnitDef describes a seeded demo school/madrasah/pesantren unit.
type UnitDef struct {
	ID             uuid.UUID
	Code           string
	Name           string
	EducationLevel string
	Address        string
	Latitude       float64
	Longitude      float64
}

// PeriodDef describes a seeded admission period and its tracks.
type PeriodDef struct {
	ID           uuid.UUID
	UnitID       uuid.UUID
	AcademicYear string
	Name         string
	OpensAt      time.Time
	ClosesAt     time.Time
	Tracks       []TrackDef
}

// TrackDef describes a seeded admission track and its document checklist.
type TrackDef struct {
	ID                     uuid.UUID
	Code                   string
	DisplayName            string
	Description            *string
	SelectionMode          string
	Quota                  int
	ActiveApplicantCount   int
	RegistrationFeeIDR     int64
	ReEnrollmentFeeIDR     int64
	SpilloverTargetTrackID *uuid.UUID
	RequiresCoordinates    bool
	RequiresAchievements   bool
	MinAgeMonths           *int
	MaxAgeMonths           *int
	AgeReferenceDate       *string
	ScoringWeights         map[string]float64
	CustomFormSchema       []domain.CustomFormField
	Requirements           []RequirementDef
}

// RequirementDef describes an uploadable requirement configured for a track.
type RequirementDef struct {
	ID            uuid.UUID
	DocCode       string
	Label         string
	Description   string
	IsRequired    bool
	AcceptedMimes []string
	MaxSizeKB     int
}

// TrackLookup joins resolved tenant/unit/period/track details from the demo catalog.
type TrackLookup struct {
	Tenant TenantDef
	Unit   UnitDef
	Period PeriodDef
	Track  TrackDef
}

var demoTenants = []TenantDef{
	{
		ID:                uuid.MustParse("01960000-0000-7000-8000-000000000001"),
		Slug:              "demo-spmb",
		Name:              "SMA Harapan Nusantara",
		InstitutionType:   domain.InstitutionSchool,
		TerminologyPreset: domain.PresetSPMB,
		PrimaryColor:      "#1D4ED8",
		Tagline:           "Portal Resmi Sistem Penerimaan Murid Baru (SPMB) TA 2026/2027",
		CustomDomain:      "spmb.smaharapan.sch.id",
	},
	{
		ID:                uuid.MustParse("01960000-0000-7000-8000-000000000002"),
		Slug:              "demo-pmbm",
		Name:              "Madrasah Aliyah Al-Hamid",
		InstitutionType:   domain.InstitutionMadrasah,
		TerminologyPreset: domain.PresetPMBM,
		PrimaryColor:      "#047857",
		Tagline:           "Portal Resmi Penerimaan Murid Baru Madrasah (PMBM) TA 2026/2027",
		CustomDomain:      "pmbm.maalhamid.sch.id",
	},
	{
		ID:                uuid.MustParse("01960000-0000-7000-8000-000000000003"),
		Slug:              "demo-mandiri",
		Name:              "Yayasan Pendidikan & Pesantren Al-Ikhlas",
		InstitutionType:   domain.InstitutionPesantren,
		TerminologyPreset: domain.PresetMandiri,
		PrimaryColor:      "#4338CA",
		Tagline:           "Portal Pendaftaran Terpadu Sekolah Islam & Pondok Pesantren",
		CustomDomain:      "daftar.alikhlas.or.id",
	},
}

// DemoTenants returns copies of the seeded SPMB, PMBM, and Mandiri demo tenants.
func DemoTenants() []TenantDef {
	out := make([]TenantDef, len(demoTenants))
	copy(out, demoTenants)
	return out
}

// FindTenantByID looks up a seeded tenant by UUID.
func FindTenantByID(tenantID uuid.UUID) (TenantDef, bool) {
	for _, t := range demoTenants {
		if t.ID == tenantID {
			return t, true
		}
	}
	return TenantDef{}, false
}

// PortalForTenant builds deterministic demo units, periods, tracks, and requirements for a tenant.
func PortalForTenant(slug, name string, preset domain.TerminologyPreset) ([]UnitDef, []PeriodDef) {
	normSlug := strings.ToLower(strings.TrimSpace(slug))
	unitID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(normSlug+"-unit"))
	periodID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(normSlug+"-period"))
	now := time.Now().UTC()

	units := []UnitDef{
		{
			ID:             unitID,
			Code:           "MAIN",
			Name:           name,
			EducationLevel: "sma_ma",
			Address:        "Jl. Pendidikan No. 10, Jakarta Timur",
			Latitude:       -6.2415,
			Longitude:      106.8832,
		},
	}

	periods := []PeriodDef{
		{
			ID:           periodID,
			UnitID:       unitID,
			AcademicYear: "2026/2027",
			Name:         "Gelombang 1",
			OpensAt:      now.Add(-7 * 24 * time.Hour),
			ClosesAt:     now.Add(30 * 24 * time.Hour),
			Tracks:       buildTracks(preset, periodID),
		},
	}
	return units, periods
}

// FindTrack locates a matching active tenant/unit/period/track combination from the demo catalog.
func FindTrack(tenantID, unitID, periodID, trackID uuid.UUID) (*TrackLookup, bool) {
	tenant, ok := FindTenantByID(tenantID)
	if !ok {
		return nil, false
	}
	units, periods := PortalForTenant(tenant.Slug, tenant.Name, tenant.TerminologyPreset)

	var matchedUnit *UnitDef
	for i := range units {
		if units[i].ID == unitID {
			matchedUnit = &units[i]
			break
		}
	}
	if matchedUnit == nil {
		return nil, false
	}

	for _, period := range periods {
		if period.ID != periodID || period.UnitID != unitID {
			continue
		}
		for _, track := range period.Tracks {
			if track.ID == trackID {
				return &TrackLookup{
					Tenant: tenant,
					Unit:   *matchedUnit,
					Period: period,
					Track:  track,
				}, true
			}
		}
	}
	return nil, false
}

// FindTrackByID locates an active track by ID under tenantID regardless of caller unit/period filters.
func FindTrackByID(tenantID, trackID uuid.UUID) (*TrackLookup, bool) {
	tenant, ok := FindTenantByID(tenantID)
	if !ok {
		return nil, false
	}
	units, periods := PortalForTenant(tenant.Slug, tenant.Name, tenant.TerminologyPreset)
	unitByID := make(map[uuid.UUID]UnitDef, len(units))
	for _, u := range units {
		unitByID[u.ID] = u
	}
	for _, period := range periods {
		unit, hasUnit := unitByID[period.UnitID]
		if !hasUnit {
			continue
		}
		for _, track := range period.Tracks {
			if track.ID == trackID {
				return &TrackLookup{
					Tenant: tenant,
					Unit:   unit,
					Period: period,
					Track:  track,
				}, true
			}
		}
	}
	return nil, false
}

// FindPeriodTracks returns all active tracks under periodID for tenantID.
func FindPeriodTracks(tenantID, periodID uuid.UUID) ([]TrackLookup, bool) {
	tenant, ok := FindTenantByID(tenantID)
	if !ok {
		return nil, false
	}
	units, periods := PortalForTenant(tenant.Slug, tenant.Name, tenant.TerminologyPreset)
	unitByID := make(map[uuid.UUID]UnitDef, len(units))
	for _, u := range units {
		unitByID[u.ID] = u
	}
	for _, period := range periods {
		if period.ID != periodID {
			continue
		}
		unit, hasUnit := unitByID[period.UnitID]
		if !hasUnit {
			return nil, false
		}
		out := make([]TrackLookup, 0, len(period.Tracks))
		for _, track := range period.Tracks {
			out = append(out, TrackLookup{
				Tenant: tenant,
				Unit:   unit,
				Period: period,
				Track:  track,
			})
		}
		return out, true
	}
	return nil, false
}

// UnitBelongsToTenant reports whether unitID is configured under tenantID in the demo catalog.
func UnitBelongsToTenant(tenantID, unitID uuid.UUID) bool {
	tenant, ok := FindTenantByID(tenantID)
	if !ok {
		return false
	}
	units, _ := PortalForTenant(tenant.Slug, tenant.Name, tenant.TerminologyPreset)
	for _, u := range units {
		if u.ID == unitID {
			return true
		}
	}
	return false
}

func buildTracks(preset domain.TerminologyPreset, periodID uuid.UUID) []TrackDef {
	descDomisili := "Seleksi berdasarkan jarak domisili dan tanggal penerbitan Kartu Keluarga."
	descPrestasi := "Seleksi capaian nilai rapor, prestasi akademik/non-akademik, atau keagamaan."
	var tracks []TrackDef

	switch preset {
	case domain.PresetPMBM:
		regID := uuid.NewSHA1(periodID, []byte("reguler"))
		tracks = []TrackDef{
			{
				ID:                   regID,
				Code:                 "reguler",
				DisplayName:          "Jalur Reguler Madrasah",
				SelectionMode:        "hybrid",
				Quota:                120,
				ActiveApplicantCount: 84,
				RegistrationFeeIDR:   150000,
				ReEnrollmentFeeIDR:   1250000,
			},
			{
				ID:                     uuid.NewSHA1(periodID, []byte("prestasi")),
				Code:                   "prestasi",
				DisplayName:            "Jalur Prestasi & Tahfidz",
				Description:            &descPrestasi,
				SelectionMode:          "hybrid",
				Quota:                  40,
				ActiveApplicantCount:   31,
				RegistrationFeeIDR:     0,
				ReEnrollmentFeeIDR:     750000,
				SpilloverTargetTrackID: &regID,
				RequiresAchievements:   true,
			},
		}
	case domain.PresetMandiri:
		tracks = []TrackDef{
			{
				ID:                   uuid.NewSHA1(periodID, []byte("early-bird")),
				Code:                 "early_bird",
				DisplayName:          "Gelombang Awal (Reguler & Asrama)",
				SelectionMode:        "test_based",
				Quota:                150,
				ActiveApplicantCount: 92,
				RegistrationFeeIDR:   250000,
				ReEnrollmentFeeIDR:   2500000,
			},
			{
				ID:                   uuid.NewSHA1(periodID, []byte("beasiswa")),
				Code:                 "beasiswa",
				DisplayName:          "Jalur Beasiswa Yatim & Berprestasi",
				SelectionMode:        "hybrid",
				Quota:                30,
				ActiveApplicantCount: 19,
				RegistrationFeeIDR:   0,
				ReEnrollmentFeeIDR:   0,
				RequiresAchievements: true,
			},
		}
	default:
		domID := uuid.NewSHA1(periodID, []byte("domisili"))
		tracks = []TrackDef{
			{
				ID:                   domID,
				Code:                 "domisili",
				DisplayName:          "Jalur Domisili",
				Description:          &descDomisili,
				SelectionMode:        "rule_based",
				Quota:                140,
				ActiveApplicantCount: 112,
				RegistrationFeeIDR:   0,
				ReEnrollmentFeeIDR:   0,
				RequiresCoordinates:  true,
			},
			{
				ID:                     uuid.NewSHA1(periodID, []byte("prestasi")),
				Code:                   "prestasi",
				DisplayName:            "Jalur Prestasi",
				Description:            &descPrestasi,
				SelectionMode:          "rule_based",
				Quota:                  60,
				ActiveApplicantCount:   48,
				RegistrationFeeIDR:     0,
				ReEnrollmentFeeIDR:     0,
				SpilloverTargetTrackID: &domID,
				RequiresAchievements:   true,
			},
		}
	}

	for i := range tracks {
		tracks[i].Requirements = buildRequirements(tracks[i].ID, tracks[i].Code)
	}
	return tracks
}

func buildRequirements(trackID uuid.UUID, trackCode string) []RequirementDef {
	defaultMimes := []string{"image/jpeg", "image/png", "image/webp", "application/pdf"}
	reqs := []RequirementDef{
		{
			ID:            uuid.NewSHA1(trackID, []byte("kk")),
			DocCode:       "kk",
			Label:         "Kartu Keluarga (KK)",
			Description:   "Scan/foto Kartu Keluarga yang jelas dan terbaca.",
			IsRequired:    true,
			AcceptedMimes: defaultMimes,
			MaxSizeKB:     2048,
		},
		{
			ID:            uuid.NewSHA1(trackID, []byte("akta")),
			DocCode:       "akta",
			Label:         "Akta Kelahiran",
			Description:   "Scan/foto Akta Kelahiran calon murid.",
			IsRequired:    true,
			AcceptedMimes: defaultMimes,
			MaxSizeKB:     2048,
		},
	}

	switch trackCode {
	case "prestasi", "beasiswa":
		reqs = append(reqs, RequirementDef{
			ID:            uuid.NewSHA1(trackID, []byte("piagam")),
			DocCode:       "piagam",
			Label:         "Piagam / Sertifikat Prestasi",
			Description:   "Piagam/sertifikat prestasi atau syahadah tahfidz.",
			IsRequired:    true,
			AcceptedMimes: defaultMimes,
			MaxSizeKB:     4096,
		})
	default:
		reqs = append(reqs, RequirementDef{
			ID:            uuid.NewSHA1(trackID, []byte("kip")),
			DocCode:       "kip",
			Label:         "Kartu KIP / PKH (Opsional)",
			Description:   "Kartu Indonesia Pintar / PKH jika memiliki.",
			IsRequired:    false,
			AcceptedMimes: defaultMimes,
			MaxSizeKB:     2048,
		})
	}
	return reqs
}

