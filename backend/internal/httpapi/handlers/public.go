package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/admissions"
	"github.com/pbdb/platform/internal/storage"
)

// PublicPortalStore provides public portal configuration, logo lookup, QR verification, and masked status lookups.
type PublicPortalStore interface {
	GetPortalData(ctx context.Context, tenant *middleware.ResolvedTenant) (PublicPortalPayload, error)
	CheckStatus(ctx context.Context, tenantID uuid.UUID, registrationNo string) (*PublicStatusResult, error)
	GetTenantLogoKey(ctx context.Context, tenantID uuid.UUID) (string, error)
	VerifyDocumentByID(ctx context.Context, tenantID, applicationID uuid.UUID) (*PublicDocumentVerifyResult, error)
}

// PublicPortalPayload mirrors the OpenAPI PublicPortalConfig schema.
type PublicPortalPayload struct {
	Tenant PublicTenantView `json:"tenant"`
	Units  []UnitView       `json:"units"`
	Periods []PeriodView    `json:"periods"`
}

type PublicTenantView struct {
	ID                   uuid.UUID                `json:"id"`
	Slug                 string                   `json:"slug"`
	Name                 string                   `json:"name"`
	InstitutionType      domain.InstitutionType   `json:"institution_type"`
	TerminologyPreset    domain.TerminologyPreset `json:"terminology_preset"`
	TerminologyOverrides map[string]string        `json:"terminology_overrides"`
	ThemeConfig          ThemeConfigView          `json:"theme_config"`
}

type ThemeConfigView struct {
	PrimaryColor string  `json:"primary_color"`
	LogoURL      *string `json:"logo_url"`
	Tagline      string  `json:"tagline"`
}

type UnitView struct {
	ID             uuid.UUID `json:"id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	EducationLevel string    `json:"education_level"`
	Address        *string   `json:"address"`
	Latitude       *float64  `json:"latitude"`
	Longitude      *float64  `json:"longitude"`
	IsActive       bool      `json:"is_active"`
}

type PeriodView struct {
	ID                 uuid.UUID   `json:"id"`
	UnitID             uuid.UUID   `json:"unit_id"`
	AcademicYear       string      `json:"academic_year"`
	Name               string      `json:"name"`
	OpensAt            time.Time   `json:"opens_at"`
	ClosesAt           time.Time   `json:"closes_at"`
	AnnouncementAt     *time.Time  `json:"announcement_at"`
	ResultsPublishedAt *time.Time  `json:"results_published_at"`
	IsPublished        bool        `json:"is_published"`
	IsActive           bool        `json:"is_active"`
	Tracks             []TrackView `json:"tracks"`
}

type TrackView struct {
	ID                   uuid.UUID                `json:"id"`
	PeriodID             uuid.UUID                `json:"period_id,omitempty"`
	UnitID               uuid.UUID                `json:"unit_id,omitempty"`
	Code                 string                   `json:"code"`
	DisplayName          string                   `json:"display_name"`
	Description          *string                  `json:"description"`
	SelectionMode        string                   `json:"selection_mode"`
	Quota                int                      `json:"quota"`
	ActiveApplicantCount int                      `json:"active_applicant_count"`
	RegistrationFeeIDR   int64                    `json:"registration_fee_idr"`
	ReEnrollmentFeeIDR   int64                    `json:"re_enrollment_fee_idr"`
	SpilloverTargetTrack *uuid.UUID               `json:"spillover_target_track_id,omitempty"`
	RequiresCoordinates  bool                     `json:"requires_coordinates"`
	RequiresAchievements bool                     `json:"requires_achievements"`
	MinAgeMonths         *int                     `json:"min_age_months,omitempty"`
	MaxAgeMonths         *int                     `json:"max_age_months,omitempty"`
	AgeReferenceDate     *string                  `json:"age_reference_date,omitempty"`
	ScoringWeights       map[string]float64       `json:"scoring_weights,omitempty"`
	CustomFormSchema     []domain.CustomFormField `json:"custom_form_schema,omitempty"`
	IsActive             bool                     `json:"is_active"`
	Requirements         []RequirementView        `json:"requirements,omitempty"`
}

// RequirementView describes an uploadable document requirement configured for an admission track.
type RequirementView struct {
	ID            uuid.UUID `json:"id"`
	DocCode       string    `json:"doc_code"`
	Label         string    `json:"label"`
	Description   *string   `json:"description,omitempty"`
	IsRequired    bool      `json:"is_required"`
	AcceptedMimes []string  `json:"accepted_mimes"`
	MaxSizeKB     int       `json:"max_size_kb"`
}

// PublicStatusResult exposes safe, non-sensitive status details for quick status checks.
type PublicStatusResult struct {
	RegistrationNo string                   `json:"registration_no"`
	MaskedName     string                   `json:"masked_name"`
	UnitName       string                   `json:"unit_name"`
	TrackName      string                   `json:"track_name"`
	AcademicYear   string                   `json:"academic_year"`
	Status         domain.ApplicationStatus `json:"status"`
	SubmittedAt    *time.Time               `json:"submitted_at"`
}

// PublicDocumentVerifyResult returns HMAC-verified authenticity metadata for a printed Registration Card or Acceptance Letter.
type PublicDocumentVerifyResult struct {
	Valid          bool                     `json:"valid"`
	DocumentType   string                   `json:"document_type"`
	RegistrationNo string                   `json:"registration_no"`
	MaskedName     string                   `json:"masked_name"`
	TenantName     string                   `json:"tenant_name"`
	UnitName       string                   `json:"unit_name"`
	TrackName      string                   `json:"track_name"`
	AcademicYear   string                   `json:"academic_year"`
	Status         domain.ApplicationStatus `json:"status"`
	FinalScore     *float64                 `json:"final_score,omitempty"`
	SubmittedAt    *time.Time               `json:"submitted_at,omitempty"`
	VerifiedAt     time.Time                `json:"verified_at"`
	Message        string                   `json:"message"`
}

// PublicHandler serves unauthenticated public portal routes.
type PublicHandler struct {
	Store          PublicPortalStore
	Storage        storage.ObjectStorage
	Admissions     *admissions.Service
	DocumentSecret string
}

// GetPortal handles GET /api/v1/public/portal.
func (h *PublicHandler) GetPortal(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusNotFound, "TENANT_NOT_FOUND", "Instansi tidak ditemukan.", nil)
		return
	}

	payload, err := h.Store.GetPortalData(r.Context(), tenant)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "PORTAL_FETCH_FAILED", "Gagal memuat informasi portal.", nil)
		return
	}

	respond.JSON(w, http.StatusOK, payload, nil)
}

// CheckStatus handles POST /api/v1/public/check-status.
func (h *PublicHandler) CheckStatus(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusNotFound, "TENANT_NOT_FOUND", "Instansi tidak ditemukan.", nil)
		return
	}

	var req struct {
		RegistrationNo string `json:"registration_no"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.", nil)
		return
	}

	regNo := strings.TrimSpace(req.RegistrationNo)
	if regNo == "" {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Nomor pendaftaran wajib diisi.", map[string]string{
			"registration_no": "Nomor pendaftaran wajib diisi.",
		})
		return
	}

	result, err := h.Store.CheckStatus(r.Context(), tenant.ID, regNo)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "STATUS_CHECK_FAILED", "Gagal memeriksa status pendaftaran.", nil)
		return
	}
	if result == nil {
		respond.Error(w, http.StatusNotFound, "APPLICATION_NOT_FOUND", "Nomor pendaftaran tidak ditemukan.", nil)
		return
	}

	respond.JSON(w, http.StatusOK, result, nil)
}

// GetLogo handles GET /api/v1/public/logo and streams the tenant's uploaded logo from ObjectStorage.
func (h *PublicHandler) GetLogo(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusNotFound, "TENANT_NOT_FOUND", "Instansi tidak ditemukan.", nil)
		return
	}
	if h.Store == nil || h.Storage == nil {
		respond.Error(w, http.StatusNotFound, "LOGO_NOT_FOUND", "Logo instansi belum tersedia.", nil)
		return
	}

	logoKey, err := h.Store.GetTenantLogoKey(r.Context(), tenant.ID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "LOGO_FETCH_FAILED", "Gagal memuat logo instansi.", nil)
		return
	}
	if strings.TrimSpace(logoKey) == "" || !storage.ValidateTenantLogoKey(logoKey, tenant.ID) {
		respond.Error(w, http.StatusNotFound, "LOGO_NOT_FOUND", "Instansi belum mengunggah logo kustom.", nil)
		return
	}

	body, contentType, err := h.Storage.GetObject(r.Context(), logoKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			respond.Error(w, http.StatusNotFound, "LOGO_NOT_FOUND", "Berkas logo tidak ditemukan di penyimpanan.", nil)
			return
		}
		respond.Error(w, http.StatusInternalServerError, "LOGO_FETCH_FAILED", "Gagal membaca berkas logo.", nil)
		return
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// VerifyDocument handles GET /api/v1/public/verify-document?token=... to authenticate printed cards/letters via HMAC QR.
func (h *PublicHandler) VerifyDocument(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusNotFound, "TENANT_NOT_FOUND", "Instansi tidak ditemukan.", nil)
		return
	}
	if h.Store == nil || strings.TrimSpace(h.DocumentSecret) == "" {
		respond.Error(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Layanan verifikasi dokumen belum tersedia.", nil)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	appID, valid := domain.VerifyDocumentToken(h.DocumentSecret, token, tenant.ID)
	if !valid {
		respond.Error(w, http.StatusBadRequest, "INVALID_DOCUMENT_TOKEN", "Token verifikasi QR dokumen tidak valid atau berasal dari instansi lain.", nil)
		return
	}

	res, err := h.Store.VerifyDocumentByID(r.Context(), tenant.ID, appID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "DOCUMENT_VERIFY_FAILED", "Gagal memverifikasi dokumen.", nil)
		return
	}
	if res == nil && h.Admissions != nil {
		if detail, appErr := h.Admissions.GetApplicationForStaff(r.Context(), tenant.ID, appID); appErr == nil && detail != nil {
			res = buildPublicVerifyResultFromDetail(detail)
		}
	}
	if res == nil {
		respond.Error(w, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Data pendaftaran pada token dokumen tidak ditemukan.", nil)
		return
	}
	res.TenantName = tenant.Name
	respond.JSON(w, http.StatusOK, res, nil)
}

func buildPublicVerifyResultFromDetail(detail *admissions.ApplicationDetail) *PublicDocumentVerifyResult {
	app := detail.Application
	name := ""
	if detail.Profile != nil && strings.TrimSpace(detail.Profile.FullName) != "" {
		name = detail.Profile.FullName
	} else if app.StudentName != nil {
		name = *app.StudentName
	}
	masked := "-"
	if strings.TrimSpace(name) != "" {
		parts := strings.Fields(strings.TrimSpace(name))
		for i, part := range parts {
			runes := []rune(part)
			if len(runes) <= 1 {
				continue
			}
			parts[i] = string(runes[0]) + strings.Repeat("*", len(runes)-1)
		}
		masked = strings.Join(parts, " ")
	}
	docType := "Kartu Bukti Pendaftaran Resmi"
	switch app.Status {
	case domain.StatusAccepted, domain.StatusReEnrollment, domain.StatusEnrolled:
		docType = "Surat Keterangan Diterima / Daftar Ulang"
	}
	var finalScore *float64
	if app.FinalScore != nil && strings.TrimSpace(*app.FinalScore) != "" {
		if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(*app.FinalScore), 64); parseErr == nil {
			finalScore = &parsed
		}
	}
	return &PublicDocumentVerifyResult{
		Valid:          true,
		DocumentType:   docType,
		RegistrationNo: app.RegistrationNo,
		MaskedName:     masked,
		UnitName:       app.UnitName,
		TrackName:      app.TrackName,
		AcademicYear:   app.AcademicYear,
		Status:         app.Status,
		FinalScore:     finalScore,
		SubmittedAt:    app.SubmittedAt,
		VerifiedAt:     time.Now().UTC(),
		Message:        "Dokumen terdaftar dan terverifikasi asli dalam sistem.",
	}
}

