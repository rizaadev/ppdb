package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/modules/admissions"
	"github.com/pbdb/platform/internal/storage"
)

// ApplicantHandler serves authenticated applicant/guardian registration endpoints.
type ApplicantHandler struct {
	Service *admissions.Service
}

// ListApplications handles GET /api/v1/applicant/applications.
func (h *ApplicantHandler) ListApplications(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	items, err := h.Service.ListApplicantApplications(r.Context(), tenant.ID, principal.UserID)
	if err != nil {
		writeAdmissionsError(w, err, "APPLICATIONS_LIST_FAILED", "Gagal memuat daftar pendaftaran.")
		return
	}

	respond.JSON(w, http.StatusOK, items, nil)
}

// CreateApplication handles POST /api/v1/applicant/applications.
func (h *ApplicantHandler) CreateApplication(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var req admissions.CreateApplicationInput
	if !decodeJSON(w, r, &req) {
		return
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	detail, err := h.Service.CreateApplication(
		r.Context(),
		tenant.ID,
		principal.UserID,
		req,
		idempotencyKey,
		actorFromRequest(principal.UserID, "applicant", r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "APPLICATION_CREATE_FAILED", "Gagal membuat pendaftaran baru.")
		return
	}

	respond.JSON(w, http.StatusCreated, detail, nil)
}

// GetApplication handles GET /api/v1/applicant/applications/{applicationId}.
func (h *ApplicantHandler) GetApplication(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	detail, err := h.Service.GetApplicantApplication(r.Context(), tenant.ID, principal.UserID, appID)
	if err != nil {
		writeAdmissionsError(w, err, "APPLICATION_FETCH_FAILED", "Gagal memuat detail pendaftaran.")
		return
	}

	respond.JSON(w, http.StatusOK, detail, nil)
}

// UpdateProfile handles PATCH /api/v1/applicant/applications/{applicationId}/profile and PATCH /{applicationId}.
func (h *ApplicantHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req admissions.UpdateProfileInput
	if !decodeJSON(w, r, &req) {
		return
	}

	updated, err := h.Service.UpdateProfile(r.Context(), tenant.ID, principal.UserID, appID, req)
	if err != nil {
		writeAdmissionsError(w, err, "PROFILE_UPDATE_FAILED", "Gagal menyimpan biodata calon murid.")
		return
	}

	respond.JSON(w, http.StatusOK, updated, nil)
}

// ReplaceGuardians handles PUT /api/v1/applicant/applications/{applicationId}/guardians.
func (h *ApplicantHandler) ReplaceGuardians(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req struct {
		Guardians []admissions.Guardian `json:"guardians"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	saved, err := h.Service.ReplaceGuardians(r.Context(), tenant.ID, principal.UserID, appID, req.Guardians)
	if err != nil {
		writeAdmissionsError(w, err, "GUARDIANS_SAVE_FAILED", "Gagal menyimpan data orang tua/wali.")
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{"guardians": saved}, nil)
}

// ReplaceAchievements handles PUT /api/v1/applicant/applications/{applicationId}/achievements.
func (h *ApplicantHandler) ReplaceAchievements(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req struct {
		Achievements []admissions.Achievement `json:"achievements"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	saved, err := h.Service.ReplaceAchievements(r.Context(), tenant.ID, principal.UserID, appID, req.Achievements)
	if err != nil {
		writeAdmissionsError(w, err, "ACHIEVEMENTS_SAVE_FAILED", "Gagal menyimpan daftar prestasi.")
		return
	}

	respond.JSON(w, http.StatusOK, map[string]any{"achievements": saved}, nil)
}

// PresignUpload handles POST /api/v1/applicant/applications/{applicationId}/documents/presign-upload.
func (h *ApplicantHandler) PresignUpload(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req admissions.PresignUploadInput
	if !decodeJSON(w, r, &req) {
		return
	}

	res, err := h.Service.PresignUpload(r.Context(), tenant.ID, principal.UserID, appID, req)
	if err != nil {
		writeAdmissionsError(w, err, "PRESIGN_UPLOAD_FAILED", "Gagal membuat URL unggahan berkas.")
		return
	}

	respond.JSON(w, http.StatusOK, res, nil)
}

// ConfirmDocument handles POST /api/v1/applicant/applications/{applicationId}/documents/confirm.
func (h *ApplicantHandler) ConfirmDocument(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req admissions.ConfirmDocumentInput
	if !decodeJSON(w, r, &req) {
		return
	}

	doc, err := h.Service.ConfirmDocument(r.Context(), tenant.ID, principal.UserID, appID, req)
	if err != nil {
		writeAdmissionsError(w, err, "DOCUMENT_CONFIRM_FAILED", "Gagal menyimpan metadata unggahan berkas.")
		return
	}

	respond.JSON(w, http.StatusOK, doc, nil)
}

// PresignDownload handles GET /api/v1/applicant/applications/{applicationId}/documents/{documentId}/presign-download.
func (h *ApplicantHandler) PresignDownload(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}
	docID, ok := parseUUIDParam(w, r, "documentId", "ID dokumen tidak valid.")
	if !ok {
		return
	}

	res, err := h.Service.PresignApplicantDownload(r.Context(), tenant.ID, principal.UserID, appID, docID)
	if err != nil {
		writeAdmissionsError(w, err, "PRESIGN_DOWNLOAD_FAILED", "Gagal membuat URL pratinjau dokumen.")
		return
	}

	respond.JSON(w, http.StatusOK, res, nil)
}

// SubmitApplication handles POST /api/v1/applicant/applications/{applicationId}/submit.
func (h *ApplicantHandler) SubmitApplication(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req struct {
		Version int `json:"version"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Version <= 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Versi dokumen wajib disertakan.", map[string]string{
			"version": "Versi dokumen harus lebih dari 0.",
		})
		return
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	submitted, err := h.Service.SubmitApplication(
		r.Context(),
		tenant.ID,
		principal.UserID,
		appID,
		req.Version,
		idempotencyKey,
		actorFromRequest(principal.UserID, "applicant", r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "SUBMIT_FAILED", "Gagal mengirim pendaftaran.")
		return
	}

	respond.JSON(w, http.StatusOK, submitted, nil)
}

func requireAuth(w http.ResponseWriter, r *http.Request) (*middleware.ResolvedTenant, *middleware.Principal, bool) {
	tenant, ok := requireTenant(w, r)
	if !ok {
		return nil, nil, false
	}
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Autentikasi diperlukan.", nil)
		return nil, nil, false
	}
	return tenant, principal, true
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, paramName, errMsg string) (uuid.UUID, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, paramName))
	parsed, err := uuid.Parse(raw)
	if err != nil || parsed == uuid.Nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_UUID", errMsg, nil)
		return uuid.Nil, false
	}
	return parsed, true
}

func actorFromRequest(userID uuid.UUID, roleName string, r *http.Request) admissions.ActorContext {
	return admissions.ActorContext{
		UserID:   userID,
		RoleName: roleName,
		IP:       clientMetaFromRequest(r).IP,
	}
}

func writeAdmissionsError(w http.ResponseWriter, err error, fallbackCode, fallbackMsg string) {
	var valErr *admissions.ValidationError
	var missingErr *admissions.MissingDocumentsError

	switch {
	case errors.As(err, &valErr):
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", valErr.Message, valErr.Fields)
	case errors.As(err, &missingErr):
		respond.Error(w, http.StatusConflict, "MISSING_REQUIRED_DOCUMENTS", "Berkas wajib belum lengkap atau masih ditolak: "+strings.Join(missingErr.MissingDocCodes, ", "), map[string]string{
			"documents": strings.Join(missingErr.MissingDocCodes, ", "),
		})
	case errors.Is(err, admissions.ErrApplicationNotFound):
		respond.Error(w, http.StatusNotFound, "APPLICATION_NOT_FOUND", "Data pendaftaran tidak ditemukan.", nil)
	case errors.Is(err, admissions.ErrTrackNotFound):
		respond.Error(w, http.StatusNotFound, "TRACK_NOT_FOUND", "Unit, periode, atau jalur pendaftaran tidak ditemukan / tidak aktif.", nil)
	case errors.Is(err, admissions.ErrRequirementNotFound):
		respond.Error(w, http.StatusNotFound, "REQUIREMENT_NOT_FOUND", "Syarat berkas jalur tidak ditemukan.", nil)
	case errors.Is(err, admissions.ErrDocumentNotFound):
		respond.Error(w, http.StatusNotFound, "DOCUMENT_NOT_FOUND", "Dokumen unggahan tidak ditemukan.", nil)
	case errors.Is(err, admissions.ErrVerificationQueueEmpty):
		respond.Error(w, http.StatusNotFound, "VERIFICATION_QUEUE_EMPTY", "Tidak ada pendaftar menunggu antrean verifikasi pada unit ini.", nil)
	case errors.Is(err, admissions.ErrPaymentNotFound):
		respond.Error(w, http.StatusNotFound, "PAYMENT_NOT_FOUND", "Transaksi pembayaran tidak ditemukan.", nil)
	case errors.Is(err, admissions.ErrPeriodNotFound):
		respond.Error(w, http.StatusNotFound, "PERIOD_NOT_FOUND", "Periode pendaftaran atau jalurnya tidak ditemukan.", nil)
	case errors.Is(err, admissions.ErrScheduleNotFound):
		respond.Error(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Jadwal asesmen atau wawancara tidak ditemukan.", nil)
	case errors.Is(err, admissions.ErrExportJobNotFound):
		respond.Error(w, http.StatusNotFound, "EXPORT_JOB_NOT_FOUND", "Pekerjaan ekspor data tidak ditemukan.", nil)
	case errors.Is(err, admissions.ErrExportNotReady):
		respond.Error(w, http.StatusConflict, "EXPORT_NOT_READY", "Berkas hasil ekspor masih diproses atau gagal dibuat.", nil)
	case errors.Is(err, admissions.ErrVersionConflict):
		respond.Error(w, http.StatusConflict, "VERSION_CONFLICT", "Data pendaftaran telah diperbarui. Muat ulang halaman untuk versi terbaru.", nil)
	case errors.Is(err, admissions.ErrApplicationLocked):
		respond.Error(w, http.StatusConflict, "APPLICATION_LOCKED", "Status pendaftaran atau tagihan saat ini tidak mengizinkan perubahan tersebut.", nil)
	case errors.Is(err, admissions.ErrPaymentAlreadyFinal):
		respond.Error(w, http.StatusConflict, "PAYMENT_ALREADY_FINAL", "Transaksi pembayaran sudah berstatus final.", nil)
	case errors.Is(err, admissions.ErrNoFeeConfigured):
		respond.Error(w, http.StatusConflict, "NO_FEE_CONFIGURED", "Jalur ini tidak memiliki tagihan biaya untuk tahap tersebut.", nil)
	case errors.Is(err, admissions.ErrManualProofRequired):
		respond.Error(w, http.StatusConflict, "MANUAL_PROOF_REQUIRED", "Bukti transfer manual belum diunggah oleh pendaftar.", nil)
	case errors.Is(err, admissions.ErrAgreementRequired):
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Pernyataan kesepakatan daftar ulang wajib disetujui.", map[string]string{
			"agreement_accepted": "Setujui pernyataan daftar ulang terlebih dahulu.",
		})
	case errors.Is(err, admissions.ErrInvalidWebhookSignature):
		respond.Error(w, http.StatusUnauthorized, "INVALID_WEBHOOK_SIGNATURE", "Signature webhook payment gateway tidak valid.", nil)
	case errors.Is(err, admissions.ErrMissingCoordinates):
		respond.Error(w, http.StatusConflict, "MISSING_COORDINATES", "Titik koordinat domisili wajib dilengkapi sebelum mengirim pendaftaran.", nil)
	case errors.Is(err, admissions.ErrMissingAchievements):
		respond.Error(w, http.StatusConflict, "MISSING_ACHIEVEMENTS", "Minimal satu prestasi atau piagam wajib diisi untuk jalur ini.", nil)
	case errors.Is(err, admissions.ErrPendingDocsRemain):
		respond.Error(w, http.StatusConflict, "DOCUMENTS_STILL_PENDING", "Seluruh berkas wajib harus lengkap dan diperiksa sebelum verifikasi diselesaikan.", nil)
	case errors.Is(err, admissions.ErrUnsupportedMime):
		respond.Error(w, http.StatusUnprocessableEntity, "UNSUPPORTED_FILE_TYPE", "Tipe berkas tidak diizinkan untuk persyaratan ini.", map[string]string{
			"mime_type": "Format MIME berkas tidak didukung.",
		})
	case errors.Is(err, admissions.ErrFileTooLarge):
		respond.Error(w, http.StatusUnprocessableEntity, "FILE_TOO_LARGE", "Ukuran berkas melebihi batas maksimum yang ditetapkan jalur.", map[string]string{
			"size_bytes": "Ukuran berkas melebihi batas maksimum.",
		})
	case errors.Is(err, admissions.ErrInvalidStorageKey):
		respond.Error(w, http.StatusUnprocessableEntity, "INVALID_STORAGE_KEY", "Lokasi kunci penyimpanan berkas tidak valid.", nil)
	case errors.Is(err, admissions.ErrReviewNoteRequired):
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Catatan alasan wajib diisi saat meminta revisi.", map[string]string{
			"review_note": "Alasan penolakan atau catatan revisi wajib diisi.",
		})
	case errors.Is(err, storage.ErrStorageNotConfigured):
		respond.Error(w, http.StatusServiceUnavailable, "STORAGE_NOT_CONFIGURED", "Penyimpanan berkas privat belum dikonfigurasi.", nil)
	case errors.Is(err, integrations.ErrGatewayNotConfigured):
		respond.Error(w, http.StatusServiceUnavailable, "PAYMENT_GATEWAY_NOT_CONFIGURED", "Payment gateway online belum dikonfigurasi untuk instansi ini.", nil)
	default:
		respond.Error(w, http.StatusInternalServerError, fallbackCode, fallbackMsg, nil)
	}
}

