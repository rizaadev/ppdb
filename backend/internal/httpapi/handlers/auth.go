package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/auth"
)

// AuthHandler serves authentication, WhatsApp OTP, token refresh, logout, and profile endpoints.
type AuthHandler struct {
	Service *auth.Service
}

// RequestOTP handles POST /api/v1/auth/otp/request.
func (h *AuthHandler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	tenant, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		WhatsAppPhone string `json:"whatsapp_phone"`
		Purpose       string `json:"purpose"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	fields := make(map[string]string)
	if strings.TrimSpace(req.WhatsAppPhone) == "" {
		fields["whatsapp_phone"] = "Nomor WhatsApp wajib diisi."
	}
	if !domain.IsValidOTPPurpose(req.Purpose) {
		fields["purpose"] = "Tujuan OTP tidak valid."
	}
	if len(fields) > 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Data permintaan OTP belum lengkap.", fields)
		return
	}

	res, err := h.Service.RequestOTP(r.Context(), tenant.ID, req.WhatsAppPhone, domain.OTPPurpose(req.Purpose))
	if err != nil {
		if errors.Is(err, auth.ErrRateLimited) {
			if res.RetryAfterSecond > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(res.RetryAfterSecond))
			}
			respond.Error(w, http.StatusTooManyRequests, "OTP_RATE_LIMITED", "Permintaan OTP terlalu sering. Tunggu beberapa saat sebelum mencoba lagi.", nil)
			return
		}
		writeAuthError(w, err, "OTP_REQUEST_FAILED", "Gagal memproses permintaan kode OTP.")
		return
	}

	respond.JSON(w, http.StatusAccepted, res, nil)
}

// VerifyOTP handles POST /api/v1/auth/otp/verify.
func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	tenant, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		WhatsAppPhone string `json:"whatsapp_phone"`
		Purpose       string `json:"purpose"`
		Code          string `json:"code"`
		FullName      string `json:"full_name"`
		Password      string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	fields := make(map[string]string)
	if strings.TrimSpace(req.WhatsAppPhone) == "" {
		fields["whatsapp_phone"] = "Nomor WhatsApp wajib diisi."
	}
	if !domain.IsValidOTPPurpose(req.Purpose) {
		fields["purpose"] = "Tujuan OTP tidak valid."
	}
	if strings.TrimSpace(req.Code) == "" {
		fields["code"] = "Kode OTP wajib diisi."
	}
	if len(fields) > 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Data verifikasi OTP belum lengkap.", fields)
		return
	}

	bundle, err := h.Service.VerifyOTP(
		r.Context(),
		tenant.ID,
		req.WhatsAppPhone,
		domain.OTPPurpose(req.Purpose),
		req.Code,
		req.FullName,
		req.Password,
		clientMetaFromRequest(r),
	)
	if err != nil {
		writeAuthError(w, err, "OTP_VERIFY_FAILED", "Gagal memverifikasi kode OTP.")
		return
	}

	respond.JSON(w, http.StatusOK, bundle, nil)
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	tenant, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	fields := make(map[string]string)
	if strings.TrimSpace(req.Identifier) == "" {
		fields["identifier"] = "Email atau nomor WhatsApp wajib diisi."
	}
	if req.Password == "" {
		fields["password"] = "Password wajib diisi."
	}
	if len(fields) > 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Kredensial login belum lengkap.", fields)
		return
	}

	bundle, err := h.Service.Login(r.Context(), tenant.ID, req.Identifier, req.Password, clientMetaFromRequest(r))
	if err != nil {
		writeAuthError(w, err, "LOGIN_FAILED", "Gagal memproses login.")
		return
	}

	respond.JSON(w, http.StatusOK, bundle, nil)
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	tenant, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Refresh token wajib diisi.", map[string]string{
			"refresh_token": "Refresh token wajib diisi.",
		})
		return
	}

	bundle, err := h.Service.Refresh(r.Context(), tenant.ID, req.RefreshToken, clientMetaFromRequest(r))
	if err != nil {
		writeAuthError(w, err, "REFRESH_FAILED", "Gagal memperbarui sesi login.")
		return
	}

	respond.JSON(w, http.StatusOK, bundle, nil)
}

// Logout handles POST /api/v1/auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireTenant(w, r); !ok {
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			respond.Error(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.", nil)
			return
		}
	}

	if err := h.Service.Logout(r.Context(), req.RefreshToken); err != nil {
		respond.Error(w, http.StatusInternalServerError, "LOGOUT_FAILED", "Gagal mengakhiri sesi.", nil)
		return
	}

	respond.JSON(w, http.StatusOK, map[string]bool{"revoked": true}, nil)
}

// ResetPassword handles POST /api/v1/auth/password/reset to update forgotten passwords after OTP verification.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	tenant, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		WhatsAppPhone string `json:"whatsapp_phone"`
		Code          string `json:"code"`
		NewPassword   string `json:"new_password"`
		Password      string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	newPassword := req.NewPassword
	if newPassword == "" {
		newPassword = req.Password
	}
	fields := make(map[string]string)
	if strings.TrimSpace(req.WhatsAppPhone) == "" {
		fields["whatsapp_phone"] = "Nomor WhatsApp wajib diisi."
	}
	if strings.TrimSpace(req.Code) == "" {
		fields["code"] = "Kode OTP wajib diisi."
	}
	if len(newPassword) < 6 {
		fields["new_password"] = "Password baru minimal 6 karakter."
	}
	if len(fields) > 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Data reset password belum lengkap atau password kurang dari 6 karakter.", fields)
		return
	}

	bundle, err := h.Service.ResetPasswordViaOTP(
		r.Context(),
		tenant.ID,
		req.WhatsAppPhone,
		req.Code,
		newPassword,
		clientMetaFromRequest(r),
	)
	if err != nil {
		writeAuthError(w, err, "PASSWORD_RESET_FAILED", "Gagal mereset password akun.")
		return
	}

	respond.JSON(w, http.StatusOK, bundle, nil)
}

// Me handles GET /api/v1/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	tenant, ok := requireTenant(w, r)
	if !ok {
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Autentikasi diperlukan.", nil)
		return
	}

	me, err := h.Service.GetMe(r.Context(), principal.UserID, tenant.ID)
	if err != nil {
		writeAuthError(w, err, "PROFILE_FETCH_FAILED", "Gagal memuat profil pengguna.")
		return
	}

	respond.JSON(w, http.StatusOK, me, nil)
}

func requireTenant(w http.ResponseWriter, r *http.Request) (*middleware.ResolvedTenant, bool) {
	tenant, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusNotFound, "TENANT_NOT_FOUND", "Instansi tidak ditemukan.", nil)
		return nil, false
	}
	return tenant, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_JSON", "Body permintaan JSON tidak boleh kosong.", nil)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_JSON", "Format JSON tidak valid.", nil)
		return false
	}
	return true
}

func clientMetaFromRequest(r *http.Request) auth.ClientMeta {
	meta := auth.ClientMeta{
		UserAgent: strings.TrimSpace(r.UserAgent()),
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if remote != "" {
		if addrPort, err := netip.ParseAddrPort(remote); err == nil {
			addr := addrPort.Addr()
			meta.IP = &addr
		} else if addr, err := netip.ParseAddr(remote); err == nil {
			meta.IP = &addr
		}
	}
	return meta
}

func writeAuthError(w http.ResponseWriter, err error, fallbackCode, fallbackMsg string) {
	switch {
	case errors.Is(err, auth.ErrInvalidPhone):
		respond.Error(w, http.StatusUnprocessableEntity, "INVALID_PHONE", "Format nomor WhatsApp tidak valid (gunakan 08... atau +628...).", map[string]string{
			"whatsapp_phone": "Nomor WhatsApp harus valid di Indonesia.",
		})
	case errors.Is(err, auth.ErrFullNameRequired):
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Nama lengkap wajib diisi.", map[string]string{
			"full_name": "Nama lengkap wajib diisi.",
		})
	case errors.Is(err, auth.ErrPasswordTooShort):
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Password minimal 6 karakter.", map[string]string{
			"password": "Password minimal 6 karakter.",
		})
	case errors.Is(err, auth.ErrPhoneAlreadyExists):
		respond.Error(w, http.StatusConflict, "PHONE_ALREADY_EXISTS", "Nomor WhatsApp sudah terdaftar.", nil)
	case errors.Is(err, auth.ErrOTPInvalidOrExpired):
		respond.Error(w, http.StatusBadRequest, "OTP_INVALID_OR_EXPIRED", "Kode OTP salah atau sudah kedaluwarsa.", nil)
	case errors.Is(err, auth.ErrOTPMaxAttempts):
		respond.Error(w, http.StatusTooManyRequests, "OTP_MAX_ATTEMPTS", "Batas percobaan OTP terlampaui. Silakan minta kode OTP baru.", nil)
	case errors.Is(err, auth.ErrInvalidCredentials):
		respond.Error(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Identifier atau password salah.", nil)
	case errors.Is(err, auth.ErrSessionInvalid):
		respond.Error(w, http.StatusUnauthorized, "SESSION_INVALID", "Sesi login tidak valid atau telah berakhir.", nil)
	case errors.Is(err, auth.ErrUserNotFound):
		respond.Error(w, http.StatusNotFound, "USER_NOT_FOUND", "Akun pengguna tidak ditemukan.", nil)
	default:
		respond.Error(w, http.StatusInternalServerError, fallbackCode, fallbackMsg, nil)
	}
}

