package handlers

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/auth"
)

var tenantSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// PlatformTenantView represents a tenant record managed from the SaaS superadmin console.
type PlatformTenantView struct {
	ID                    uuid.UUID                `json:"id"`
	Slug                  string                   `json:"slug"`
	Name                  string                   `json:"name"`
	InstitutionType       domain.InstitutionType   `json:"institution_type"`
	TerminologyPreset     domain.TerminologyPreset `json:"terminology_preset"`
	ThemeConfig           ThemeConfigView          `json:"theme_config"`
	CustomDomain            *string                  `json:"custom_domain,omitempty"`
	DomainVerified          bool                     `json:"domain_verified"`
	DomainVerificationToken *string                  `json:"domain_verification_token,omitempty"`
	SubscriptionPlan        string                   `json:"subscription_plan"`
	SubscriptionExpiresAt *time.Time               `json:"subscription_expires_at,omitempty"`
	IsActive              bool                     `json:"is_active"`
	UnitCount             int                      `json:"unit_count"`
	CreatedAt             time.Time                `json:"created_at"`
}

// CreatePlatformTenantInput defines attributes to onboard a new educational institution tenant.
type CreatePlatformTenantInput struct {
	Slug              string                   `json:"slug"`
	Name              string                   `json:"name"`
	InstitutionType   domain.InstitutionType   `json:"institution_type"`
	TerminologyPreset domain.TerminologyPreset `json:"terminology_preset"`
	PrimaryColor      string                   `json:"primary_color"`
	Tagline           string                   `json:"tagline"`
	CustomDomain      *string                  `json:"custom_domain"`
	SubscriptionPlan  string                   `json:"subscription_plan"`
	StarterUnitCode   string                   `json:"starter_unit_code"`
	StarterUnitName   string                   `json:"starter_unit_name"`
	StarterUnitLevel  string                   `json:"starter_unit_level"`
	AdminEmail        string                   `json:"admin_email"`
	AdminFullName     string                   `json:"admin_full_name"`
	AdminPassword     string                   `json:"admin_password"`
}

// UpdatePlatformTenantInput defines tenant-level fields adjustable by platform superadmins.
type UpdatePlatformTenantInput struct {
	Name              *string                   `json:"name"`
	InstitutionType   *domain.InstitutionType   `json:"institution_type"`
	TerminologyPreset *domain.TerminologyPreset `json:"terminology_preset"`
	PrimaryColor      *string                   `json:"primary_color"`
	Tagline           *string                   `json:"tagline"`
	CustomDomain      *string                   `json:"custom_domain"`
	DomainVerified    *bool                     `json:"domain_verified"`
	SubscriptionPlan  *string                   `json:"subscription_plan"`
	IsActive          *bool                     `json:"is_active"`
}

// PlatformTenantStore persists platform-wide tenant creation, listing, and status updates.
type PlatformTenantStore interface {
	ListPlatformTenants(ctx context.Context) ([]PlatformTenantView, error)
	CreatePlatformTenant(ctx context.Context, input CreatePlatformTenantInput) (*PlatformTenantView, error)
	UpdatePlatformTenant(ctx context.Context, tenantID uuid.UUID, input UpdatePlatformTenantInput) (*PlatformTenantView, error)
	GetPlatformTenantDomainInfo(ctx context.Context, tenantID uuid.UUID) (slug string, customDomain *string, token *string, err error)
	MarkPlatformTenantDomainVerified(ctx context.Context, tenantID uuid.UUID) (*PlatformTenantView, error)
}

// PlatformAuthService abstracts platform superadmin login and initial tenant admin provisioning.
type PlatformAuthService interface {
	LoginPlatformAdmin(ctx context.Context, email, password string, meta auth.ClientMeta) (*auth.TokenBundle, error)
	ProvisionTenantAdmin(ctx context.Context, tenantID uuid.UUID, email, fullName, rawPassword string) (*auth.User, error)
}

// PlatformHandler serves SaaS superadmin login and multi-tenant onboarding endpoints.
type PlatformHandler struct {
	Store       PlatformTenantStore
	Auth        PlatformAuthService
	DNSResolver DNSResolver
	BaseDomain  string
}

// PlatformInitialAdminSummary identifies the initial tenant_admin account created during onboarding.
type PlatformInitialAdminSummary struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	FullName string    `json:"full_name"`
}

// PlatformCreateTenantResult bundles the newly created tenant and optional initial admin account.
type PlatformCreateTenantResult struct {
	Tenant       PlatformTenantView           `json:"tenant"`
	InitialAdmin *PlatformInitialAdminSummary `json:"initial_admin,omitempty"`
}

// Login handles POST /api/v1/platform/auth/login without requiring a tenant subdomain header.
func (h *PlatformHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Layanan autentikasi platform belum tersedia.", nil)
		return
	}
	var req struct {
		Identifier string `json:"identifier"`
		Email      string `json:"email"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = strings.TrimSpace(req.Identifier)
	}
	fields := map[string]string{}
	if email == "" {
		fields["email"] = "Email superadmin wajib diisi."
	}
	if req.Password == "" {
		fields["password"] = "Password wajib diisi."
	}
	if len(fields) > 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Kredensial login platform belum lengkap.", fields)
		return
	}

	bundle, err := h.Auth.LoginPlatformAdmin(r.Context(), email, req.Password, clientMetaFromRequest(r))
	if err != nil {
		if errors.Is(err, auth.ErrPlatformAdminRequired) {
			respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Akun ini bukan Superadmin Platform.", nil)
			return
		}
		writeAuthError(w, err, "PLATFORM_LOGIN_FAILED", "Gagal masuk ke konsol platform.")
		return
	}
	respond.JSON(w, http.StatusOK, bundle, nil)
}

// ListTenants handles GET /api/v1/platform/tenants.
func (h *PlatformHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Penyimpanan data tenant belum tersedia.", nil)
		return
	}
	items, err := h.Store.ListPlatformTenants(r.Context())
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "LIST_TENANTS_FAILED", "Gagal memuat daftar instansi.", nil)
		return
	}
	respond.JSON(w, http.StatusOK, items, nil)
}

// CreateTenant handles POST /api/v1/platform/tenants.
func (h *PlatformHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Penyimpanan data tenant belum tersedia.", nil)
		return
	}
	var req CreatePlatformTenantInput
	if !decodeJSON(w, r, &req) {
		return
	}

	fields := validateCreatePlatformTenant(&req)
	if len(fields) > 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Data pendaftaran instansi baru tidak valid.", fields)
		return
	}

	created, err := h.Store.CreatePlatformTenant(r.Context(), req)
	if err != nil {
		writeSetupError(w, err, "CREATE_TENANT_FAILED", "Gagal mendaftarkan instansi baru.")
		return
	}

	result := PlatformCreateTenantResult{Tenant: *created}
	if req.AdminEmail != "" && h.Auth != nil {
		adminUser, err := h.Auth.ProvisionTenantAdmin(r.Context(), created.ID, req.AdminEmail, req.AdminFullName, req.AdminPassword)
		if err != nil {
			writeAuthError(w, err, "PROVISION_ADMIN_FAILED", "Tenant dibuat namun gagal membuat akun Admin Instansi.")
			return
		}
		emailOut := req.AdminEmail
		if adminUser.Email != nil {
			emailOut = *adminUser.Email
		}
		result.InitialAdmin = &PlatformInitialAdminSummary{
			ID:       adminUser.ID,
			Email:    emailOut,
			FullName: adminUser.FullName,
		}
	}

	respond.JSON(w, http.StatusCreated, result, nil)
}

// UpdateTenant handles PATCH /api/v1/platform/tenants/{tenantId}.
func (h *PlatformHandler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Penyimpanan data tenant belum tersedia.", nil)
		return
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "tenantId")))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_TENANT_ID", "ID instansi tidak valid.", nil)
		return
	}

	var req UpdatePlatformTenantInput
	if !decodeJSON(w, r, &req) {
		return
	}
	fields := validateUpdatePlatformTenant(&req)
	if len(fields) > 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Perubahan data instansi tidak valid.", fields)
		return
	}

	updated, err := h.Store.UpdatePlatformTenant(r.Context(), tenantID, req)
	if err != nil {
		writeSetupError(w, err, "UPDATE_TENANT_FAILED", "Gagal memperbarui data instansi.")
		return
	}
	respond.JSON(w, http.StatusOK, updated, nil)
}

// VerifyDomain handles POST /api/v1/platform/tenants/{tenantId}/verify-domain using live DNS TXT/CNAME checks.
func (h *PlatformHandler) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		respond.Error(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Penyimpanan data tenant belum tersedia.", nil)
		return
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "tenantId")))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_TENANT_ID", "ID instansi tidak valid.", nil)
		return
	}

	slug, domainPtr, tokenPtr, err := h.Store.GetPlatformTenantDomainInfo(r.Context(), tenantID)
	if err != nil {
		writeSetupError(w, err, "TENANT_FETCH_FAILED", "Gagal memuat informasi domain instansi.")
		return
	}
	if domainPtr == nil || strings.TrimSpace(*domainPtr) == "" {
		respond.Error(w, http.StatusUnprocessableEntity, "CUSTOM_DOMAIN_REQUIRED", "Domain kustom belum diisi untuk instansi ini.", nil)
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
	var updatedTenant *PlatformTenantView
	if res.Verified {
		updatedTenant, err = h.Store.MarkPlatformTenantDomainVerified(r.Context(), tenantID)
		if err != nil {
			writeSetupError(w, err, "DOMAIN_VERIFY_SAVE_FAILED", "DNS cocok namun gagal menyimpan status verifikasi.")
			return
		}
	}

	respond.JSON(w, http.StatusOK, struct {
		Verification DomainVerificationResult `json:"verification"`
		Tenant       *PlatformTenantView      `json:"tenant,omitempty"`
	}{
		Verification: res,
		Tenant:       updatedTenant,
	}, nil)
}

func isValidInstitutionType(v domain.InstitutionType) bool {
	switch v {
	case domain.InstitutionSchool, domain.InstitutionMadrasah, domain.InstitutionFoundation, domain.InstitutionPesantren:
		return true
	default:
		return false
	}
}

func isValidPreset(v domain.TerminologyPreset) bool {
	switch v {
	case domain.PresetSPMB, domain.PresetPMBM, domain.PresetMandiri:
		return true
	default:
		return false
	}
}

func validateCreatePlatformTenant(req *CreatePlatformTenantInput) map[string]string {
	fields := map[string]string{}
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.Name = strings.TrimSpace(req.Name)
	req.PrimaryColor = strings.ToUpper(strings.TrimSpace(req.PrimaryColor))
	req.Tagline = strings.TrimSpace(req.Tagline)
	req.SubscriptionPlan = strings.ToLower(strings.TrimSpace(req.SubscriptionPlan))
	req.StarterUnitCode = strings.ToUpper(strings.TrimSpace(req.StarterUnitCode))
	req.StarterUnitName = strings.TrimSpace(req.StarterUnitName)
	req.StarterUnitLevel = strings.ToLower(strings.TrimSpace(req.StarterUnitLevel))
	req.AdminEmail = strings.ToLower(strings.TrimSpace(req.AdminEmail))
	req.AdminFullName = strings.TrimSpace(req.AdminFullName)

	if !tenantSlugPattern.MatchString(req.Slug) {
		fields["slug"] = "Slug harus 3-63 karakter huruf kecil, angka, atau strip (-)."
	}
	if len(req.Name) < 3 {
		fields["name"] = "Nama instansi minimal 3 karakter."
	}
	if !isValidInstitutionType(req.InstitutionType) {
		fields["institution_type"] = "Jenis lembaga harus school, madrasah, foundation, atau pesantren."
	}
	if !isValidPreset(req.TerminologyPreset) {
		fields["terminology_preset"] = "Preset harus spmb, pmbm, atau mandiri."
	}
	if req.PrimaryColor == "" {
		req.PrimaryColor = "#1E3A5F"
	} else if !hexColorPattern.MatchString(req.PrimaryColor) {
		fields["primary_color"] = "Warna utama harus berupa hex valid (contoh: #1E3A5F)."
	}
	if req.SubscriptionPlan == "" {
		req.SubscriptionPlan = "starter"
	}
	if req.CustomDomain != nil {
		d := strings.ToLower(strings.TrimSpace(*req.CustomDomain))
		req.CustomDomain = &d
	}

	if req.StarterUnitCode != "" || req.StarterUnitName != "" {
		if !slugCodePattern.MatchString(req.StarterUnitCode) {
			fields["starter_unit_code"] = "Kode unit awal harus 2-32 karakter alfanumerik/strip."
		}
		if len(req.StarterUnitName) < 3 {
			fields["starter_unit_name"] = "Nama unit awal minimal 3 karakter."
		}
		if req.StarterUnitLevel == "" {
			req.StarterUnitLevel = "smp_mts"
		} else if !validateEducationLevel(req.StarterUnitLevel) {
			fields["starter_unit_level"] = "Jenjang unit awal tidak valid."
		}
	}

	if req.AdminEmail != "" || req.AdminFullName != "" || req.AdminPassword != "" {
		if !strings.Contains(req.AdminEmail, "@") {
			fields["admin_email"] = "Email admin instansi harus valid."
		}
		if len(req.AdminFullName) < 2 {
			fields["admin_full_name"] = "Nama lengkap admin instansi wajib diisi."
		}
		if len(req.AdminPassword) < 6 {
			fields["admin_password"] = "Password admin instansi minimal 6 karakter."
		}
	}
	return fields
}

func validateUpdatePlatformTenant(req *UpdatePlatformTenantInput) map[string]string {
	fields := map[string]string{}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if len(trimmed) < 3 {
			fields["name"] = "Nama instansi minimal 3 karakter."
		} else {
			req.Name = &trimmed
		}
	}
	if req.InstitutionType != nil && !isValidInstitutionType(*req.InstitutionType) {
		fields["institution_type"] = "Jenis lembaga tidak valid."
	}
	if req.TerminologyPreset != nil && !isValidPreset(*req.TerminologyPreset) {
		fields["terminology_preset"] = "Preset tidak valid."
	}
	if req.PrimaryColor != nil {
		c := strings.ToUpper(strings.TrimSpace(*req.PrimaryColor))
		if !hexColorPattern.MatchString(c) {
			fields["primary_color"] = "Warna utama harus berupa hex valid."
		} else {
			req.PrimaryColor = &c
		}
	}
	if req.Tagline != nil {
		t := strings.TrimSpace(*req.Tagline)
		req.Tagline = &t
	}
	if req.CustomDomain != nil {
		d := strings.ToLower(strings.TrimSpace(*req.CustomDomain))
		req.CustomDomain = &d
	}
	if req.SubscriptionPlan != nil {
		p := strings.ToLower(strings.TrimSpace(*req.SubscriptionPlan))
		if p == "" {
			fields["subscription_plan"] = "Paket langganan tidak boleh kosong."
		} else {
			req.SubscriptionPlan = &p
		}
	}
	return fields
}

