package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/auth"
	"github.com/pbdb/platform/internal/storage"
)

var (
	ErrSetupNotFound   = errors.New("catalog entity not found")
	ErrSetupConflict   = errors.New("duplicate code or entity in use")
	ErrSetupValidation = errors.New("setup validation failed")
	hexColorPattern    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	slugCodePattern    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{1,31}$`)
)

// SetupValidationError holds structured field errors returned as 422 JSON.
type SetupValidationError struct {
	Message string
	Fields  map[string]string
}

func (e *SetupValidationError) Error() string { return e.Message }

// AdminSetupStore provides full white-label branding, unit, period, track, staff, and document requirement CRUD.
type AdminSetupStore interface {
	GetTenantSettings(ctx context.Context, tenantID uuid.UUID) (*TenantSettingsPayload, error)
	UpdateTenantSettings(ctx context.Context, tenantID uuid.UUID, input UpdateTenantSettingsInput) (*TenantSettingsPayload, error)
	SetTenantLogoKey(ctx context.Context, tenantID uuid.UUID, logoStorageKey *string) (*TenantSettingsPayload, error)
	GetTenantDomainInfo(ctx context.Context, tenantID uuid.UUID) (slug string, customDomain *string, token *string, err error)
	MarkTenantDomainVerified(ctx context.Context, tenantID uuid.UUID) (*TenantSettingsPayload, error)
	ListAdminUnits(ctx context.Context, tenantID uuid.UUID) ([]UnitView, error)
	CreateUnit(ctx context.Context, tenantID uuid.UUID, input CreateUnitInput) (*UnitView, error)
	UpdateUnit(ctx context.Context, tenantID, unitID uuid.UUID, input UpdateUnitInput) (*UnitView, error)
	GetPeriodUnitID(ctx context.Context, tenantID, periodID uuid.UUID) (uuid.UUID, bool, error)
	CreatePeriod(ctx context.Context, tenantID uuid.UUID, input CreatePeriodInput) (*PeriodView, error)
	UpdatePeriod(ctx context.Context, tenantID, periodID uuid.UUID, input UpdatePeriodInput) (*PeriodView, error)
	GetTrackUnitID(ctx context.Context, tenantID, trackID uuid.UUID) (uuid.UUID, bool, error)
	CreateTrack(ctx context.Context, tenantID uuid.UUID, input CreateTrackInput) (*TrackView, error)
	UpdateTrack(ctx context.Context, tenantID, trackID uuid.UUID, input UpdateTrackInput) (*TrackView, error)
	ReplaceTrackRequirements(ctx context.Context, tenantID, trackID uuid.UUID, items []TrackRequirementItemInput) ([]RequirementView, error)
	ListStaff(ctx context.Context, tenantID uuid.UUID) ([]StaffMemberView, error)
	CreateOrAssignStaff(ctx context.Context, tenantID uuid.UUID, input CreateStaffInput) (*StaffMemberView, error)
	UpdateStaff(ctx context.Context, tenantID, actorUserID, targetUserID uuid.UUID, input UpdateStaffInput) (*StaffMemberView, error)
}

// AdminTenantHandler serves white-label tenant & admissions catalog setup endpoints protected by RBAC.
type AdminTenantHandler struct {
	PublicStore PublicPortalStore
	SetupStore  AdminSetupStore
	Storage     storage.ObjectStorage
	DNSResolver DNSResolver
	BaseDomain  string
}

// PaymentConfigView exposes non-secret Midtrans settings and a masked indicator of the server key.
type PaymentConfigView struct {
	MidtransMerchantID          string `json:"midtrans_merchant_id,omitempty"`
	MidtransClientKey           string `json:"midtrans_client_key,omitempty"`
	MidtransSandbox             *bool  `json:"midtrans_sandbox,omitempty"`
	MidtransServerKeyConfigured bool   `json:"midtrans_server_key_configured"`
	MidtransServerKeyMasked     string `json:"midtrans_server_key_masked,omitempty"`
}

// PaymentConfigUpdate updates optional per-tenant Midtrans configuration; nil server key preserves existing secret.
type PaymentConfigUpdate struct {
	MidtransMerchantID *string `json:"midtrans_merchant_id"`
	MidtransClientKey  *string `json:"midtrans_client_key"`
	MidtransServerKey  *string `json:"midtrans_server_key"`
	MidtransSandbox    *bool   `json:"midtrans_sandbox"`
}

// TenantSettingsPayload represents white-label tenant settings for administrative panels.
type TenantSettingsPayload struct {
	ID                      uuid.UUID                `json:"id"`
	Slug                    string                   `json:"slug"`
	Name                    string                   `json:"name"`
	InstitutionType         domain.InstitutionType   `json:"institution_type"`
	TerminologyPreset       domain.TerminologyPreset `json:"terminology_preset"`
	TerminologyOverrides    map[string]string        `json:"terminology_overrides"`
	ThemeConfig             ThemeConfigView          `json:"theme_config"`
	CustomDomain            *string                  `json:"custom_domain,omitempty"`
	DomainVerified          bool                     `json:"domain_verified"`
	DomainVerificationToken *string                  `json:"domain_verification_token,omitempty"`
	PaymentConfig           PaymentConfigView        `json:"payment_config"`
}

// ThemeConfigUpdate captures partial branding updates for tenant white-label theme_config JSON.
type ThemeConfigUpdate struct {
	PrimaryColor *string `json:"primary_color"`
	LogoURL      *string `json:"logo_url"`
	Tagline      *string `json:"tagline"`
}

// UpdateTenantSettingsInput captures white-label settings fields editable by tenant_admin.
type UpdateTenantSettingsInput struct {
	Name                 *string                   `json:"name"`
	TerminologyPreset    *domain.TerminologyPreset `json:"terminology_preset"`
	TerminologyOverrides map[string]string         `json:"terminology_overrides"`
	ThemeConfig          *ThemeConfigUpdate        `json:"theme_config"`
	CustomDomain         *string                   `json:"custom_domain"`
	PaymentConfig        *PaymentConfigUpdate      `json:"payment_config"`
}

// StaffRoleView describes one role assignment for a staff member inside the current tenant.
type StaffRoleView struct {
	ID       uuid.UUID         `json:"id"`
	Role     domain.TenantRole `json:"role"`
	UnitID   *uuid.UUID        `json:"unit_id,omitempty"`
	UnitName *string           `json:"unit_name,omitempty"`
	IsActive bool              `json:"is_active"`
}

// StaffMemberView represents a tenant staff member and their assigned roles.
type StaffMemberView struct {
	UserID          uuid.UUID       `json:"user_id"`
	FullName        string          `json:"full_name"`
	Email           *string         `json:"email,omitempty"`
	WhatsAppPhone   *string         `json:"whatsapp_phone,omitempty"`
	IsPlatformAdmin bool            `json:"is_platform_admin"`
	Roles           []StaffRoleView `json:"roles"`
	CreatedAt       time.Time       `json:"created_at"`
}

// CreateStaffInput creates or attaches a staff account with an RBAC role inside the current tenant.
type CreateStaffInput struct {
	FullName      string            `json:"full_name"`
	Email         string            `json:"email"`
	WhatsAppPhone *string           `json:"whatsapp_phone"`
	Password      string            `json:"password"`
	Role          domain.TenantRole `json:"role"`
	UnitID        *uuid.UUID        `json:"unit_id"`
}

// UpdateStaffInput modifies a tenant staff member's profile, role scope, or active status.
type UpdateStaffInput struct {
	FullName      *string            `json:"full_name"`
	WhatsAppPhone *string            `json:"whatsapp_phone"`
	Role          *domain.TenantRole `json:"role"`
	UnitID        *uuid.UUID         `json:"unit_id"`
	ClearUnit     bool               `json:"clear_unit"`
	IsActive      *bool              `json:"is_active"`
	Password      *string            `json:"password"`
}

// CreateUnitInput defines fields required to create an education unit/jenjang under a tenant.
type CreateUnitInput struct {
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	EducationLevel string   `json:"education_level"`
	NPSNOrNSM      *string  `json:"npsn_or_nsm"`
	Address        *string  `json:"address"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
}

// UpdateUnitInput allows partial updates to a unit's name, coordinates, or active flag.
type UpdateUnitInput struct {
	Name           *string  `json:"name"`
	EducationLevel *string  `json:"education_level"`
	Address        *string  `json:"address"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
	IsActive       *bool    `json:"is_active"`
}

// CreatePeriodInput opens a registration wave/period under a unit.
type CreatePeriodInput struct {
	UnitID         uuid.UUID  `json:"unit_id"`
	AcademicYear   string     `json:"academic_year"`
	Name           string     `json:"name"`
	OpensAt        time.Time  `json:"opens_at"`
	ClosesAt       time.Time  `json:"closes_at"`
	AnnouncementAt *time.Time `json:"announcement_at"`
	IsPublished    *bool      `json:"is_published"`
}

// UpdatePeriodInput modifies registration schedule dates, labels, or visibility.
type UpdatePeriodInput struct {
	AcademicYear   *string    `json:"academic_year"`
	Name           *string    `json:"name"`
	OpensAt        *time.Time `json:"opens_at"`
	ClosesAt       *time.Time `json:"closes_at"`
	AnnouncementAt *time.Time `json:"announcement_at"`
	IsPublished    *bool      `json:"is_published"`
	IsActive       *bool      `json:"is_active"`
}

// CreateTrackInput adds an admission track with quota, fees, scoring weights, custom schema, and selection rules under a period.
type CreateTrackInput struct {
	PeriodID               uuid.UUID                `json:"period_id"`
	Code                   string                   `json:"code"`
	DisplayName            string                   `json:"display_name"`
	Description            *string                  `json:"description"`
	SelectionMode          string                   `json:"selection_mode"`
	Quota                  int                      `json:"quota"`
	SpilloverTargetTrackID *uuid.UUID               `json:"spillover_target_track_id"`
	RegistrationFeeIDR     int64                    `json:"registration_fee_idr"`
	ReEnrollmentFeeIDR     int64                    `json:"re_enrollment_fee_idr"`
	RequiresCoordinates    bool                     `json:"requires_coordinates"`
	RequiresAchievements   bool                     `json:"requires_achievements"`
	MinAgeMonths           *int                     `json:"min_age_months"`
	MaxAgeMonths           *int                     `json:"max_age_months"`
	AgeReferenceDate       *string                  `json:"age_reference_date"`
	ScoringWeights         map[string]float64       `json:"scoring_weights"`
	CustomFormSchema       []domain.CustomFormField `json:"custom_form_schema"`
}

// UpdateTrackInput modifies an admission track's quota, fees, weights, form schema, or flags.
type UpdateTrackInput struct {
	DisplayName            *string                  `json:"display_name"`
	Description            *string                  `json:"description"`
	SelectionMode          *string                  `json:"selection_mode"`
	Quota                  *int                     `json:"quota"`
	SpilloverTargetTrackID *uuid.UUID               `json:"spillover_target_track_id"`
	RegistrationFeeIDR     *int64                   `json:"registration_fee_idr"`
	ReEnrollmentFeeIDR     *int64                   `json:"re_enrollment_fee_idr"`
	RequiresCoordinates    *bool                    `json:"requires_coordinates"`
	RequiresAchievements   *bool                    `json:"requires_achievements"`
	MinAgeMonths           *int                     `json:"min_age_months"`
	MaxAgeMonths           *int                     `json:"max_age_months"`
	AgeReferenceDate       *string                  `json:"age_reference_date"`
	ScoringWeights         map[string]float64       `json:"scoring_weights"`
	CustomFormSchema       []domain.CustomFormField `json:"custom_form_schema"`
	IsActive               *bool                    `json:"is_active"`
}

// TrackRequirementItemInput describes one document upload requirement for a track.
type TrackRequirementItemInput struct {
	DocCode       string   `json:"doc_code"`
	Label         string   `json:"label"`
	Description   *string  `json:"description"`
	IsRequired    bool     `json:"is_required"`
	AcceptedMimes []string `json:"accepted_mimes"`
	MaxSizeKB     int      `json:"max_size_kb"`
}

// ReplaceTrackRequirementsInput sets the full ordered document checklist for a track.
type ReplaceTrackRequirementsInput struct {
	Requirements []TrackRequirementItemInput `json:"requirements"`
}

func (h *AdminTenantHandler) setupStore() AdminSetupStore {
	if h.SetupStore != nil {
		return h.SetupStore
	}
	if s, ok := h.PublicStore.(AdminSetupStore); ok {
		return s
	}
	return nil
}

// GetSettings handles GET /api/v1/admin/tenant/settings.
func (h *AdminTenantHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	tenant, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if store := h.setupStore(); store != nil {
		payload, err := store.GetTenantSettings(r.Context(), tenant.ID)
		if err != nil {
			writeSetupError(w, err, "SETTINGS_FETCH_FAILED", "Gagal memuat pengaturan instansi.")
			return
		}
		respond.JSON(w, http.StatusOK, payload, nil)
		return
	}

	payload := TenantSettingsPayload{
		ID:                   tenant.ID,
		Slug:                 tenant.Slug,
		Name:                 tenant.Name,
		InstitutionType:      tenant.InstitutionType,
		TerminologyPreset:    tenant.TerminologyPreset,
		TerminologyOverrides: map[string]string{},
		ThemeConfig: ThemeConfigView{
			PrimaryColor: tenant.PrimaryColor,
			Tagline:      tenant.Tagline,
		},
	}
	if tenant.CustomDomain != "" {
		domainCopy := tenant.CustomDomain
		payload.CustomDomain = &domainCopy
	}
	if h.PublicStore != nil {
		portal, err := h.PublicStore.GetPortalData(r.Context(), tenant)
		if err != nil {
			respond.Error(w, http.StatusInternalServerError, "SETTINGS_FETCH_FAILED", "Gagal memuat pengaturan instansi.", nil)
			return
		}
		if portal.Tenant.TerminologyOverrides != nil {
			payload.TerminologyOverrides = portal.Tenant.TerminologyOverrides
		}
		payload.ThemeConfig = portal.Tenant.ThemeConfig
	}
	respond.JSON(w, http.StatusOK, payload, nil)
}

// UpdateSettings handles PATCH /api/v1/admin/tenant/settings.
func (h *AdminTenantHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Penyimpanan pengaturan instansi belum tersedia.", nil)
		return
	}

	var req UpdateTenantSettingsInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateTenantSettingsUpdate(&req); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Pengaturan instansi tidak valid.")
		return
	}

	updated, err := store.UpdateTenantSettings(r.Context(), tenant.ID, req)
	if err != nil {
		writeSetupError(w, err, "SETTINGS_UPDATE_FAILED", "Gagal menyimpan pengaturan instansi.")
		return
	}
	respond.JSON(w, http.StatusOK, updated, nil)
}

// ListUnits handles GET /api/v1/admin/units.
func (h *AdminTenantHandler) ListUnits(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Daftar unit tidak tersedia.", nil)
		return
	}

	units, err := store.ListAdminUnits(r.Context(), tenant.ID)
	if err != nil {
		writeSetupError(w, err, "UNITS_LIST_FAILED", "Gagal memuat daftar unit sekolah.")
		return
	}

	allowedUnits, allAccess := allowedUnitsInTenant(principal, tenant.ID)
	if !allAccess {
		filtered := make([]UnitView, 0, len(units))
		for _, u := range units {
			if slices.Contains(allowedUnits, u.ID) {
				filtered = append(filtered, u)
			}
		}
		units = filtered
	}
	respond.JSON(w, http.StatusOK, units, nil)
}

// CreateUnit handles POST /api/v1/admin/units.
func (h *AdminTenantHandler) CreateUnit(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur tambah unit belum tersedia.", nil)
		return
	}

	var req CreateUnitInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateCreateUnit(&req); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Data unit pendidikan tidak valid.")
		return
	}

	created, err := store.CreateUnit(r.Context(), tenant.ID, req)
	if err != nil {
		writeSetupError(w, err, "UNIT_CREATE_FAILED", "Gagal membuat unit pendidikan baru.")
		return
	}
	respond.JSON(w, http.StatusCreated, created, nil)
}

// UpdateUnit handles PATCH /api/v1/admin/units/{unitId}.
func (h *AdminTenantHandler) UpdateUnit(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	unitID, ok := parseUUIDParam(w, r, "unitId", "ID unit sekolah tidak valid.")
	if !ok {
		return
	}
	if !middleware.HasUnitAccess(principal, unitID, domain.RoleTenantAdmin, domain.RoleUnitAdmin) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin mengubah unit sekolah ini.", nil)
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur ubah unit belum tersedia.", nil)
		return
	}

	var req UpdateUnitInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateUpdateUnit(&req); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Perubahan data unit tidak valid.")
		return
	}

	updated, err := store.UpdateUnit(r.Context(), tenant.ID, unitID, req)
	if err != nil {
		writeSetupError(w, err, "UNIT_UPDATE_FAILED", "Gagal memperbarui unit pendidikan.")
		return
	}
	respond.JSON(w, http.StatusOK, updated, nil)
}

// CreatePeriod handles POST /api/v1/admin/periods.
func (h *AdminTenantHandler) CreatePeriod(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur tambah periode belum tersedia.", nil)
		return
	}

	var req CreatePeriodInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.UnitID == uuid.Nil {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Unit pendidikan wajib dipilih.", map[string]string{
			"unit_id": "Unit wajib dipilih.",
		})
		return
	}
	if !middleware.HasUnitAccess(principal, req.UnitID, domain.RoleTenantAdmin, domain.RoleUnitAdmin, domain.RolePrincipal) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin membuka periode pendaftaran untuk unit ini.", nil)
		return
	}
	if err := validateCreatePeriod(&req); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Jadwal periode pendaftaran tidak valid.")
		return
	}

	created, err := store.CreatePeriod(r.Context(), tenant.ID, req)
	if err != nil {
		writeSetupError(w, err, "PERIOD_CREATE_FAILED", "Gagal membuat periode pendaftaran baru.")
		return
	}
	respond.JSON(w, http.StatusCreated, created, nil)
}

// UpdatePeriod handles PATCH /api/v1/admin/periods/{periodId}.
func (h *AdminTenantHandler) UpdatePeriod(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	periodID, ok := parseUUIDParam(w, r, "periodId", "ID periode pendaftaran tidak valid.")
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur ubah periode belum tersedia.", nil)
		return
	}
	unitID, found, err := store.GetPeriodUnitID(r.Context(), tenant.ID, periodID)
	if err != nil {
		writeSetupError(w, err, "PERIOD_FETCH_FAILED", "Gagal memuat metadata periode.")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "SETUP_NOT_FOUND", "Periode pendaftaran tidak ditemukan.", nil)
		return
	}
	if !middleware.HasUnitAccess(principal, unitID, domain.RoleTenantAdmin, domain.RoleUnitAdmin, domain.RolePrincipal) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin mengubah periode pendaftaran unit ini.", nil)
		return
	}

	var req UpdatePeriodInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Nama periode tidak boleh kosong.", map[string]string{
				"name": "Nama periode tidak boleh kosong.",
			})
			return
		}
		req.Name = &trimmed
	}
	if req.AcademicYear != nil {
		trimmed := strings.TrimSpace(*req.AcademicYear)
		if trimmed == "" {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Tahun ajaran tidak boleh kosong.", map[string]string{
				"academic_year": "Tahun ajaran tidak boleh kosong.",
			})
			return
		}
		req.AcademicYear = &trimmed
	}

	updated, err := store.UpdatePeriod(r.Context(), tenant.ID, periodID, req)
	if err != nil {
		writeSetupError(w, err, "PERIOD_UPDATE_FAILED", "Gagal memperbarui periode pendaftaran.")
		return
	}
	respond.JSON(w, http.StatusOK, updated, nil)
}

// CreateTrack handles POST /api/v1/admin/tracks.
func (h *AdminTenantHandler) CreateTrack(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur tambah jalur belum tersedia.", nil)
		return
	}

	var req CreateTrackInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.PeriodID == uuid.Nil {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Periode pendaftaran wajib dipilih.", map[string]string{
			"period_id": "Periode wajib dipilih.",
		})
		return
	}
	unitID, found, err := store.GetPeriodUnitID(r.Context(), tenant.ID, req.PeriodID)
	if err != nil {
		writeSetupError(w, err, "PERIOD_FETCH_FAILED", "Gagal memuat unit dari periode.")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "SETUP_NOT_FOUND", "Periode pendaftaran tidak ditemukan.", nil)
		return
	}
	if !middleware.HasUnitAccess(principal, unitID, domain.RoleTenantAdmin, domain.RoleUnitAdmin, domain.RolePrincipal) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin menambah jalur pada unit ini.", nil)
		return
	}
	if err := validateCreateTrack(&req); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Data jalur pendaftaran tidak valid.")
		return
	}

	created, err := store.CreateTrack(r.Context(), tenant.ID, req)
	if err != nil {
		writeSetupError(w, err, "TRACK_CREATE_FAILED", "Gagal membuat jalur pendaftaran baru.")
		return
	}
	respond.JSON(w, http.StatusCreated, created, nil)
}

// UpdateTrack handles PATCH /api/v1/admin/tracks/{trackId}.
func (h *AdminTenantHandler) UpdateTrack(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	trackID, ok := parseUUIDParam(w, r, "trackId", "ID jalur pendaftaran tidak valid.")
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur ubah jalur belum tersedia.", nil)
		return
	}
	unitID, found, err := store.GetTrackUnitID(r.Context(), tenant.ID, trackID)
	if err != nil {
		writeSetupError(w, err, "TRACK_FETCH_FAILED", "Gagal memuat informasi unit jalur.")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "SETUP_NOT_FOUND", "Jalur pendaftaran tidak ditemukan.", nil)
		return
	}
	if !middleware.HasUnitAccess(principal, unitID, domain.RoleTenantAdmin, domain.RoleUnitAdmin, domain.RolePrincipal) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin mengubah jalur pada unit ini.", nil)
		return
	}

	var req UpdateTrackInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateUpdateTrack(&req); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Perubahan data jalur tidak valid.")
		return
	}

	updated, err := store.UpdateTrack(r.Context(), tenant.ID, trackID, req)
	if err != nil {
		writeSetupError(w, err, "TRACK_UPDATE_FAILED", "Gagal memperbarui jalur pendaftaran.")
		return
	}
	respond.JSON(w, http.StatusOK, updated, nil)
}

// ReplaceTrackRequirements handles PUT /api/v1/admin/tracks/{trackId}/requirements.
func (h *AdminTenantHandler) ReplaceTrackRequirements(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	trackID, ok := parseUUIDParam(w, r, "trackId", "ID jalur pendaftaran tidak valid.")
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur pengaturan syarat berkas belum tersedia.", nil)
		return
	}
	unitID, found, err := store.GetTrackUnitID(r.Context(), tenant.ID, trackID)
	if err != nil {
		writeSetupError(w, err, "TRACK_FETCH_FAILED", "Gagal memuat informasi unit jalur.")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "SETUP_NOT_FOUND", "Jalur pendaftaran tidak ditemukan.", nil)
		return
	}
	if !middleware.HasUnitAccess(principal, unitID, domain.RoleTenantAdmin, domain.RoleUnitAdmin) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Hanya admin berwenang yang dapat mengubah syarat berkas unit ini.", nil)
		return
	}

	var req ReplaceTrackRequirementsInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateTrackRequirements(req.Requirements); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Daftar syarat berkas tidak valid.")
		return
	}

	saved, err := store.ReplaceTrackRequirements(r.Context(), tenant.ID, trackID, req.Requirements)
	if err != nil {
		writeSetupError(w, err, "REQUIREMENTS_SAVE_FAILED", "Gagal menyimpan daftar syarat berkas jalur.")
		return
	}
	respond.JSON(w, http.StatusOK, saved, nil)
}

// PresignLogoUpload handles POST /api/v1/admin/tenant/logo/presign.
func (h *AdminTenantHandler) PresignLogoUpload(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if h.Storage == nil {
		respond.Error(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Penyimpanan objek logo belum dikonfigurasi.", nil)
		return
	}

	var req struct {
		FileName  string `json:"file_name"`
		MimeType  string `json:"mime_type"`
		SizeBytes int64  `json:"size_bytes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.FileName = strings.TrimSpace(req.FileName)
	req.MimeType = strings.ToLower(strings.TrimSpace(req.MimeType))
	if req.FileName == "" {
		req.FileName = "logo.png"
	}
	allowedLogoMimes := map[string]bool{
		"image/png":     true,
		"image/jpeg":    true,
		"image/webp":    true,
		"image/svg+xml": true,
	}
	if !allowedLogoMimes[req.MimeType] {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Format logo harus PNG, JPEG, WEBP, atau SVG.", map[string]string{
			"mime_type": "Gunakan PNG, JPEG, WEBP, atau SVG.",
		})
		return
	}
	if req.SizeBytes <= 0 || req.SizeBytes > 2*1024*1024 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Ukuran logo wajib 1 byte hingga 2 MB.", map[string]string{
			"size_bytes": "Ukuran maksimal logo 2 MB.",
		})
		return
	}

	key := storage.BuildTenantLogoKey(tenant.ID, req.FileName)
	presigned, err := h.Storage.PresignPutObject(key, req.MimeType, 10*time.Minute)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "LOGO_PRESIGN_FAILED", "Gagal membuat URL unggah logo.", nil)
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{
		"storage_key":      key,
		"upload_url":       presigned.URL,
		"method":           "PUT",
		"required_headers": presigned.RequiredHeaders,
		"expires_at":       presigned.ExpiresAt,
	}, nil)
}

// ConfirmLogoUpload handles POST /api/v1/admin/tenant/logo/confirm.
func (h *AdminTenantHandler) ConfirmLogoUpload(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil || h.Storage == nil {
		respond.Error(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Layanan konfirmasi logo belum tersedia.", nil)
		return
	}

	var req struct {
		StorageKey string `json:"storage_key"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.StorageKey = strings.TrimSpace(req.StorageKey)
	if !storage.ValidateTenantLogoKey(req.StorageKey, tenant.ID) {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Storage key logo tidak valid untuk instansi ini.", nil)
		return
	}

	meta, err := h.Storage.HeadObject(r.Context(), req.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			respond.Error(w, http.StatusUnprocessableEntity, "OBJECT_NOT_UPLOADED", "Berkas logo belum ditemukan di object storage.", nil)
			return
		}
		respond.Error(w, http.StatusBadGateway, "STORAGE_CHECK_FAILED", "Gagal memverifikasi berkas logo di storage.", nil)
		return
	}
	if meta.SizeBytes <= 0 || meta.SizeBytes > 2*1024*1024 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Ukuran berkas logo tidak valid (maksimal 2 MB).", nil)
		return
	}

	updated, err := store.SetTenantLogoKey(r.Context(), tenant.ID, &req.StorageKey)
	if err != nil {
		writeSetupError(w, err, "LOGO_SAVE_FAILED", "Gagal menyimpan logo instansi.")
		return
	}
	respond.JSON(w, http.StatusOK, updated, nil)
}

// VerifyDomain handles POST /api/v1/admin/tenant/domain/verify.
func (h *AdminTenantHandler) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Layanan verifikasi domain belum tersedia.", nil)
		return
	}

	slug, domainPtr, tokenPtr, err := store.GetTenantDomainInfo(r.Context(), tenant.ID)
	if err != nil {
		writeSetupError(w, err, "DOMAIN_FETCH_FAILED", "Gagal memuat pengaturan domain instansi.")
		return
	}
	if domainPtr == nil || strings.TrimSpace(*domainPtr) == "" {
		respond.Error(w, http.StatusUnprocessableEntity, "CUSTOM_DOMAIN_REQUIRED", "Simpan domain kustom terlebih dahulu sebelum verifikasi DNS.", map[string]string{
			"custom_domain": "Domain kustom belum dikonfigurasi.",
		})
		return
	}

	var tokenStr string
	if tokenPtr != nil {
		tokenStr = *tokenPtr
	}
	var cnameTarget string
	if strings.TrimSpace(h.BaseDomain) != "" {
		cnameTarget = slug + "." + strings.TrimSpace(h.BaseDomain)
	}

	res := VerifyCustomDomainDNS(r.Context(), h.DNSResolver, *domainPtr, tokenStr, cnameTarget)
	var updatedSettings *TenantSettingsPayload
	if res.Verified {
		updatedSettings, err = store.MarkTenantDomainVerified(r.Context(), tenant.ID)
		if err != nil {
			writeSetupError(w, err, "DOMAIN_VERIFY_SAVE_FAILED", "DNS cocok namun gagal menyimpan status verifikasi domain.")
			return
		}
	}

	respond.JSON(w, http.StatusOK, struct {
		Verification DomainVerificationResult `json:"verification"`
		Settings     *TenantSettingsPayload   `json:"settings,omitempty"`
	}{
		Verification: res,
		Settings:     updatedSettings,
	}, nil)
}

// ListStaff handles GET /api/v1/admin/staff.
func (h *AdminTenantHandler) ListStaff(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Daftar staf belum tersedia.", nil)
		return
	}

	members, err := store.ListStaff(r.Context(), tenant.ID)
	if err != nil {
		writeSetupError(w, err, "STAFF_LIST_FAILED", "Gagal memuat daftar staf & panitia.")
		return
	}
	respond.JSON(w, http.StatusOK, members, nil)
}

// CreateStaff handles POST /api/v1/admin/staff.
func (h *AdminTenantHandler) CreateStaff(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur tambah staf belum tersedia.", nil)
		return
	}

	var req CreateStaffInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateCreateStaff(&req); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Data staf panitia tidak valid.")
		return
	}

	created, err := store.CreateOrAssignStaff(r.Context(), tenant.ID, req)
	if err != nil {
		writeSetupError(w, err, "STAFF_CREATE_FAILED", "Gagal menambahkan staf panitia.")
		return
	}
	respond.JSON(w, http.StatusCreated, created, nil)
}

// UpdateStaff handles PATCH /api/v1/admin/staff/{userId}.
func (h *AdminTenantHandler) UpdateStaff(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	targetUserID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "userId")))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_USER_ID", "ID staf tidak valid.", nil)
		return
	}
	store := h.setupStore()
	if store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SETUP_UNAVAILABLE", "Fitur ubah staf belum tersedia.", nil)
		return
	}

	var req UpdateStaffInput
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateUpdateStaff(&req); err != nil {
		writeSetupError(w, err, "VALIDATION_ERROR", "Perubahan data staf tidak valid.")
		return
	}

	updated, err := store.UpdateStaff(r.Context(), tenant.ID, principal.UserID, targetUserID, req)
	if err != nil {
		writeSetupError(w, err, "STAFF_UPDATE_FAILED", "Gagal memperbarui data staf.")
		return
	}
	respond.JSON(w, http.StatusOK, updated, nil)
}

func validateTenantSettingsUpdate(req *UpdateTenantSettingsInput) error {
	fields := map[string]string{}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if len(n) < 3 {
			fields["name"] = "Nama instansi minimal 3 karakter."
		} else {
			req.Name = &n
		}
	}
	if req.TerminologyPreset != nil {
		p := domain.TerminologyPreset(strings.ToLower(strings.TrimSpace(string(*req.TerminologyPreset))))
		switch p {
		case domain.PresetSPMB, domain.PresetPMBM, domain.PresetMandiri:
			req.TerminologyPreset = &p
		default:
			fields["terminology_preset"] = "Preset terminologi harus spmb, pmbm, atau mandiri."
		}
	}
	if req.ThemeConfig != nil {
		if req.ThemeConfig.PrimaryColor != nil {
			c := strings.TrimSpace(*req.ThemeConfig.PrimaryColor)
			if !hexColorPattern.MatchString(c) {
				fields["theme_config.primary_color"] = "Warna utama harus berupa kode hex valid (contoh: #1D4ED8)."
			} else {
				c = strings.ToUpper(c)
				req.ThemeConfig.PrimaryColor = &c
			}
		}
		if req.ThemeConfig.Tagline != nil {
			t := strings.TrimSpace(*req.ThemeConfig.Tagline)
			req.ThemeConfig.Tagline = &t
		}
	}
	if req.CustomDomain != nil {
		d := strings.ToLower(strings.TrimSpace(*req.CustomDomain))
		req.CustomDomain = &d
	}
	if req.PaymentConfig != nil {
		if req.PaymentConfig.MidtransMerchantID != nil {
			m := strings.TrimSpace(*req.PaymentConfig.MidtransMerchantID)
			req.PaymentConfig.MidtransMerchantID = &m
		}
		if req.PaymentConfig.MidtransClientKey != nil {
			c := strings.TrimSpace(*req.PaymentConfig.MidtransClientKey)
			req.PaymentConfig.MidtransClientKey = &c
		}
		if req.PaymentConfig.MidtransServerKey != nil {
			s := strings.TrimSpace(*req.PaymentConfig.MidtransServerKey)
			if strings.HasPrefix(s, "****") {
				// Ignore masked echo returned from UI forms so stored secret stays intact.
				req.PaymentConfig.MidtransServerKey = nil
			} else {
				req.PaymentConfig.MidtransServerKey = &s
			}
		}
	}
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Pengaturan white-label tidak valid.", Fields: fields}
	}
	return nil
}

func validateEducationLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "ra_tk", "sd_mi", "smp_mts", "sma_ma", "smk_mak", "pesantren":
		return true
	default:
		return false
	}
}

func validateSelectionMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "rule_based", "test_based", "hybrid", "direct":
		return true
	default:
		return false
	}
}

func validateLatLon(lat, lon *float64, fields map[string]string) {
	if lat != nil && (*lat < -90 || *lat > 90) {
		fields["latitude"] = "Latitude harus berada antara -90 dan 90."
	}
	if lon != nil && (*lon < -180 || *lon > 180) {
		fields["longitude"] = "Longitude harus berada antara -180 dan 180."
	}
}

func validateCreateUnit(req *CreateUnitInput) error {
	fields := map[string]string{}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	req.EducationLevel = strings.ToLower(strings.TrimSpace(req.EducationLevel))

	if !slugCodePattern.MatchString(req.Code) {
		fields["code"] = "Kode unit harus 2-32 karakter alfanumerik/strip/underscore."
	}
	if len(req.Name) < 3 {
		fields["name"] = "Nama unit minimal 3 karakter."
	}
	if !validateEducationLevel(req.EducationLevel) {
		fields["education_level"] = "Jenjang pendidikan tidak dikenali."
	}
	validateLatLon(req.Latitude, req.Longitude, fields)
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Data unit sekolah tidak valid.", Fields: fields}
	}
	return nil
}

func validateUpdateUnit(req *UpdateUnitInput) error {
	fields := map[string]string{}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if len(n) < 3 {
			fields["name"] = "Nama unit minimal 3 karakter."
		} else {
			req.Name = &n
		}
	}
	if req.EducationLevel != nil {
		lvl := strings.ToLower(strings.TrimSpace(*req.EducationLevel))
		if !validateEducationLevel(lvl) {
			fields["education_level"] = "Jenjang pendidikan tidak dikenali."
		} else {
			req.EducationLevel = &lvl
		}
	}
	validateLatLon(req.Latitude, req.Longitude, fields)
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Perubahan unit sekolah tidak valid.", Fields: fields}
	}
	return nil
}

func validateCreatePeriod(req *CreatePeriodInput) error {
	fields := map[string]string{}
	req.AcademicYear = strings.TrimSpace(req.AcademicYear)
	req.Name = strings.TrimSpace(req.Name)
	if len(req.AcademicYear) < 4 {
		fields["academic_year"] = "Tahun ajaran wajib diisi (misal 2026/2027)."
	}
	if len(req.Name) < 3 {
		fields["name"] = "Nama gelombang atau periode minimal 3 karakter."
	}
	if req.OpensAt.IsZero() {
		fields["opens_at"] = "Tanggal buka pendaftaran wajib diisi."
	}
	if req.ClosesAt.IsZero() {
		fields["closes_at"] = "Tanggal tutup pendaftaran wajib diisi."
	} else if !req.OpensAt.IsZero() && !req.ClosesAt.After(req.OpensAt) {
		fields["closes_at"] = "Tanggal tutup harus lebih lambat daripada tanggal buka."
	}
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Data periode pendaftaran tidak valid.", Fields: fields}
	}
	return nil
}

func validateCreateTrack(req *CreateTrackInput) error {
	fields := map[string]string{}
	req.Code = strings.ToLower(strings.TrimSpace(req.Code))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.SelectionMode = strings.ToLower(strings.TrimSpace(req.SelectionMode))
	if req.SelectionMode == "" {
		req.SelectionMode = "rule_based"
	}

	if !slugCodePattern.MatchString(req.Code) {
		fields["code"] = "Kode jalur harus 2-32 karakter alfanumerik/strip/underscore."
	}
	if len(req.DisplayName) < 3 {
		fields["display_name"] = "Nama jalur minimal 3 karakter."
	}
	if !validateSelectionMode(req.SelectionMode) {
		fields["selection_mode"] = "Mode seleksi harus rule_based, test_based, hybrid, atau direct."
	}
	if req.Quota < 0 {
		fields["quota"] = "Kuota tidak boleh negatif."
	}
	if req.RegistrationFeeIDR < 0 {
		fields["registration_fee_idr"] = "Biaya formulir tidak boleh negatif."
	}
	if req.ReEnrollmentFeeIDR < 0 {
		fields["re_enrollment_fee_idr"] = "Biaya daftar ulang tidak boleh negatif."
	}
	validateTrackConfigFields(req.MinAgeMonths, req.MaxAgeMonths, &req.AgeReferenceDate, req.ScoringWeights, req.CustomFormSchema, fields)
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Data jalur pendaftaran tidak valid.", Fields: fields}
	}
	return nil
}

func validateUpdateTrack(req *UpdateTrackInput) error {
	fields := map[string]string{}
	if req.DisplayName != nil {
		d := strings.TrimSpace(*req.DisplayName)
		if len(d) < 3 {
			fields["display_name"] = "Nama jalur minimal 3 karakter."
		} else {
			req.DisplayName = &d
		}
	}
	if req.SelectionMode != nil {
		m := strings.ToLower(strings.TrimSpace(*req.SelectionMode))
		if !validateSelectionMode(m) {
			fields["selection_mode"] = "Mode seleksi tidak valid."
		} else {
			req.SelectionMode = &m
		}
	}
	if req.Quota != nil && *req.Quota < 0 {
		fields["quota"] = "Kuota tidak boleh negatif."
	}
	if req.RegistrationFeeIDR != nil && *req.RegistrationFeeIDR < 0 {
		fields["registration_fee_idr"] = "Biaya formulir tidak boleh negatif."
	}
	if req.ReEnrollmentFeeIDR != nil && *req.ReEnrollmentFeeIDR < 0 {
		fields["re_enrollment_fee_idr"] = "Biaya daftar ulang tidak boleh negatif."
	}
	validateTrackConfigFields(req.MinAgeMonths, req.MaxAgeMonths, &req.AgeReferenceDate, req.ScoringWeights, req.CustomFormSchema, fields)
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Perubahan jalur tidak valid.", Fields: fields}
	}
	return nil
}

func validateTrackConfigFields(
	minAge, maxAge *int,
	ageRef **string,
	weights map[string]float64,
	formSchema []domain.CustomFormField,
	fields map[string]string,
) {
	if minAge != nil && (*minAge < 0 || *minAge > 600) {
		fields["min_age_months"] = "Usia minimal harus antara 0 hingga 600 bulan."
	}
	if maxAge != nil && (*maxAge < 0 || *maxAge > 600) {
		fields["max_age_months"] = "Usia maksimal harus antara 0 hingga 600 bulan."
	}
	if minAge != nil && maxAge != nil && *minAge > *maxAge {
		fields["max_age_months"] = "Usia maksimal tidak boleh lebih kecil daripada usia minimal."
	}
	if ageRef != nil && *ageRef != nil {
		trimmed := strings.TrimSpace(**ageRef)
		if trimmed == "" {
			*ageRef = nil
		} else if _, err := time.Parse("2006-01-02", trimmed); err != nil {
			fields["age_reference_date"] = "Tanggal acuan usia harus berformat YYYY-MM-DD."
		} else {
			*ageRef = &trimmed
		}
	}
	for code, w := range weights {
		if strings.TrimSpace(code) == "" {
			fields["scoring_weights"] = "Kode komponen bobot penilaian tidak boleh kosong."
			break
		}
		if w < 0 || w > 100 {
			fields["scoring_weights"] = fmt.Sprintf("Bobot komponen %s harus bernilai 0 hingga 100.", code)
			break
		}
	}
	seenKeys := make(map[string]struct{}, len(formSchema))
	for idx := range formSchema {
		formSchema[idx].Key = strings.ToLower(strings.TrimSpace(formSchema[idx].Key))
		formSchema[idx].Label = strings.TrimSpace(formSchema[idx].Label)
		formSchema[idx].Type = strings.ToLower(strings.TrimSpace(formSchema[idx].Type))
		if formSchema[idx].Type == "" {
			formSchema[idx].Type = "text"
		}
		if !slugCodePattern.MatchString(formSchema[idx].Key) {
			fields[fmt.Sprintf("custom_form_schema[%d].key", idx)] = "Key field kustom harus alfanumerik/underscore/strip."
		} else if _, dup := seenKeys[formSchema[idx].Key]; dup {
			fields[fmt.Sprintf("custom_form_schema[%d].key", idx)] = "Key field kustom tidak boleh duplikat."
		} else {
			seenKeys[formSchema[idx].Key] = struct{}{}
		}
		if formSchema[idx].Label == "" {
			fields[fmt.Sprintf("custom_form_schema[%d].label", idx)] = "Label field kustom wajib diisi."
		}
		switch formSchema[idx].Type {
		case "text", "textarea", "number", "select", "date", "checkbox":
		default:
			fields[fmt.Sprintf("custom_form_schema[%d].type", idx)] = "Tipe field harus text, textarea, number, select, date, atau checkbox."
		}
		if formSchema[idx].Type == "select" && len(formSchema[idx].Options) == 0 {
			fields[fmt.Sprintf("custom_form_schema[%d].options", idx)] = "Field pilihan (select) minimal membutuhkan 1 opsi."
		}
	}
}

func isAllowedStaffRole(role domain.TenantRole) bool {
	switch role {
	case domain.RoleTenantAdmin, domain.RoleUnitAdmin, domain.RoleVerifier,
		domain.RoleTreasurer, domain.RoleExaminer, domain.RolePrincipal:
		return true
	default:
		return false
	}
}

func validateCreateStaff(req *CreateStaffInput) error {
	fields := map[string]string{}
	req.FullName = strings.TrimSpace(req.FullName)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Role = domain.TenantRole(strings.ToLower(strings.TrimSpace(string(req.Role))))

	if len(req.FullName) < 2 {
		fields["full_name"] = "Nama lengkap staf minimal 2 karakter."
	}
	if !strings.Contains(req.Email, "@") || len(req.Email) < 5 {
		fields["email"] = "Email staf wajib diisi dengan format valid."
	}
	if req.WhatsAppPhone != nil {
		raw := strings.TrimSpace(*req.WhatsAppPhone)
		if raw == "" {
			req.WhatsAppPhone = nil
		} else if norm, err := auth.NormalizeIndonesianPhone(raw); err != nil {
			fields["whatsapp_phone"] = "Nomor WhatsApp tidak valid (gunakan format 08... atau +628...)."
		} else {
			req.WhatsAppPhone = &norm
		}
	}
	if req.Password != "" && len(req.Password) < 6 {
		fields["password"] = "Password staf minimal 6 karakter."
	}
	if !isAllowedStaffRole(req.Role) {
		fields["role"] = "Peran staf harus tenant_admin, unit_admin, verifier, treasurer, examiner, atau principal."
	}
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Data staf panitia tidak valid.", Fields: fields}
	}
	return nil
}

func validateUpdateStaff(req *UpdateStaffInput) error {
	fields := map[string]string{}
	if req.FullName != nil {
		n := strings.TrimSpace(*req.FullName)
		if len(n) < 2 {
			fields["full_name"] = "Nama lengkap staf minimal 2 karakter."
		} else {
			req.FullName = &n
		}
	}
	if req.WhatsAppPhone != nil {
		raw := strings.TrimSpace(*req.WhatsAppPhone)
		if raw == "" {
			empty := ""
			req.WhatsAppPhone = &empty
		} else if norm, err := auth.NormalizeIndonesianPhone(raw); err != nil {
			fields["whatsapp_phone"] = "Nomor WhatsApp tidak valid."
		} else {
			req.WhatsAppPhone = &norm
		}
	}
	if req.Role != nil {
		r := domain.TenantRole(strings.ToLower(strings.TrimSpace(string(*req.Role))))
		if !isAllowedStaffRole(r) {
			fields["role"] = "Peran staf tidak valid."
		} else {
			req.Role = &r
		}
	}
	if req.Password != nil && *req.Password != "" && len(*req.Password) < 6 {
		fields["password"] = "Password minimal 6 karakter."
	}
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Perubahan data staf tidak valid.", Fields: fields}
	}
	return nil
}

func validateTrackRequirements(items []TrackRequirementItemInput) error {
	fields := map[string]string{}
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		items[i].DocCode = strings.ToLower(strings.TrimSpace(items[i].DocCode))
		items[i].Label = strings.TrimSpace(items[i].Label)
		if !slugCodePattern.MatchString(items[i].DocCode) {
			fields["doc_code"] = "Setiap syarat berkas harus memiliki doc_code alfanumerik valid."
		} else if _, dup := seen[items[i].DocCode]; dup {
			fields["doc_code"] = "doc_code tidak boleh duplikat dalam satu jalur."
		} else {
			seen[items[i].DocCode] = struct{}{}
		}
		if items[i].Label == "" {
			fields["label"] = "Label syarat berkas wajib diisi."
		}
		if items[i].MaxSizeKB <= 0 {
			items[i].MaxSizeKB = 2048
		}
		if items[i].MaxSizeKB > 15360 {
			fields["max_size_kb"] = "Ukuran berkas maksimal tidak boleh melebihi 15360 KB (15 MB)."
		}
		if len(items[i].AcceptedMimes) == 0 {
			items[i].AcceptedMimes = []string{"image/jpeg", "image/png", "image/webp", "application/pdf"}
		}
	}
	if len(fields) > 0 {
		return &SetupValidationError{Message: "Syarat berkas jalur tidak valid.", Fields: fields}
	}
	return nil
}

func writeSetupError(w http.ResponseWriter, err error, fallbackCode, fallbackMsg string) {
	var valErr *SetupValidationError
	switch {
	case errors.As(err, &valErr):
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", valErr.Message, valErr.Fields)
	case errors.Is(err, ErrSetupNotFound):
		respond.Error(w, http.StatusNotFound, "SETUP_NOT_FOUND", "Entitas konfigurasi yang dituju tidak ditemukan.", nil)
	case errors.Is(err, ErrSetupConflict):
		respond.Error(w, http.StatusConflict, "SETUP_CONFLICT", "Kode entitas duplikat atau masih digunakan oleh data pendaftaran aktif.", nil)
	default:
		respond.Error(w, http.StatusInternalServerError, fallbackCode, fallbackMsg, nil)
	}
}
