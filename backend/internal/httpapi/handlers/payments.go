package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/admissions"
)

// PaymentsHandler exposes applicant billing, re-enrollment, treasurer reconciliation, and gateway webhook endpoints.
type PaymentsHandler struct {
	Service           *admissions.Service
	WebhookSecret     string
	MidtransServerKey string
}

var treasurerMutationRoles = []domain.TenantRole{
	domain.RoleTenantAdmin,
	domain.RoleUnitAdmin,
	domain.RoleTreasurer,
}

// CreatePayment handles POST /api/v1/applicant/applications/{applicationId}/payments.
func (h *PaymentsHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req admissions.CreatePaymentInput
	if !decodeJSON(w, r, &req) {
		return
	}

	created, err := h.Service.CreatePayment(
		r.Context(),
		tenant.ID,
		principal.UserID,
		appID,
		req,
		strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		actorFromRequest(principal.UserID, "applicant", r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENT_CREATE_FAILED", "Gagal membuat transaksi tagihan pembayaran.")
		return
	}

	respond.JSON(w, http.StatusCreated, created, nil)
}

// ListApplicantPayments handles GET /api/v1/applicant/applications/{applicationId}/payments.
func (h *PaymentsHandler) ListApplicantPayments(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	list, err := h.Service.ListApplicantPayments(r.Context(), tenant.ID, principal.UserID, appID)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENTS_LIST_FAILED", "Gagal memuat riwayat transaksi pembayaran.")
		return
	}

	respond.JSON(w, http.StatusOK, list, nil)
}

// PresignManualProof handles POST /api/v1/applicant/applications/{applicationId}/payments/{paymentId}/presign-proof.
func (h *PaymentsHandler) PresignManualProof(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}
	paymentID, ok := parseUUIDParam(w, r, "paymentId", "ID transaksi pembayaran tidak valid.")
	if !ok {
		return
	}

	var req admissions.PresignProofInput
	if !decodeJSON(w, r, &req) {
		return
	}

	signed, err := h.Service.PresignManualProof(r.Context(), tenant.ID, principal.UserID, appID, paymentID, req)
	if err != nil {
		writeAdmissionsError(w, err, "PRESIGN_PROOF_FAILED", "Gagal membuat URL unggah bukti transfer.")
		return
	}

	respond.JSON(w, http.StatusOK, signed, nil)
}

// AttachManualProof handles POST /api/v1/applicant/applications/{applicationId}/payments/{paymentId}/manual-proof.
func (h *PaymentsHandler) AttachManualProof(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}
	paymentID, ok := parseUUIDParam(w, r, "paymentId", "ID transaksi pembayaran tidak valid.")
	if !ok {
		return
	}

	var req admissions.AttachManualProofInput
	if !decodeJSON(w, r, &req) {
		return
	}

	updated, err := h.Service.AttachManualProof(r.Context(), tenant.ID, principal.UserID, appID, paymentID, req)
	if err != nil {
		writeAdmissionsError(w, err, "MANUAL_PROOF_ATTACH_FAILED", "Gagal menyimpan bukti transfer manual.")
		return
	}

	respond.JSON(w, http.StatusOK, updated, nil)
}

// ReEnroll handles POST /api/v1/applicant/applications/{applicationId}/re-enroll.
func (h *PaymentsHandler) ReEnroll(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req admissions.ReEnrollInput
	if !decodeJSON(w, r, &req) {
		return
	}

	updated, err := h.Service.ReEnroll(
		r.Context(),
		tenant.ID,
		principal.UserID,
		appID,
		req,
		actorFromRequest(principal.UserID, "applicant", r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "RE_ENROLL_FAILED", "Gagal memproses konfirmasi daftar ulang.")
		return
	}

	respond.JSON(w, http.StatusOK, updated, nil)
}

// ListAdminPayments handles GET /api/v1/admin/payments with unit-scoped RBAC.
func (h *PaymentsHandler) ListAdminPayments(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	unitPtr, allowedUnits, ok := resolveStaffUnitScope(
		w,
		principal,
		tenant.ID,
		q.Get("unit_id"),
		"Anda tidak memiliki akses transaksi keuangan pada unit tersebut.",
	)
	if !ok {
		return
	}

	filter := admissions.AdminPaymentFilter{
		UnitID:         unitPtr,
		AllowedUnitIDs: allowedUnits,
		Limit:          50,
	}
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > 100 {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Parameter limit harus bernilai 1 sampai 100.", nil)
			return
		}
		filter.Limit = parsedLimit
	}
	if st := strings.TrimSpace(q.Get("status")); st != "" {
		filter.Status = &st
	}
	if m := strings.TrimSpace(q.Get("method")); m != "" {
		filter.Method = &m
	}
	if p := strings.TrimSpace(q.Get("purpose")); p != "" {
		filter.Purpose = &p
	}

	rows, err := h.Service.ListAdminPayments(r.Context(), tenant.ID, filter)
	if err != nil {
		writeAdmissionsError(w, err, "ADMIN_PAYMENTS_LIST_FAILED", "Gagal memuat daftar pembayaran.")
		return
	}

	respond.JSON(w, http.StatusOK, rows, nil)
}

// PresignAdminPaymentProof handles GET /api/v1/admin/payments/{paymentId}/presign-proof.
func (h *PaymentsHandler) PresignAdminPaymentProof(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	paymentID, ok := parseUUIDParam(w, r, "paymentId", "ID transaksi pembayaran tidak valid.")
	if !ok {
		return
	}

	payment, err := h.Service.GetAdminPayment(r.Context(), tenant.ID, paymentID)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENT_FETCH_FAILED", "Gagal memuat data transaksi.")
		return
	}
	if !middleware.HasUnitAccess(principal, payment.UnitID) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki akses transaksi pembayaran unit ini.", nil)
		return
	}

	signed, err := h.Service.PresignAdminPaymentProof(r.Context(), tenant.ID, paymentID)
	if err != nil {
		writeAdmissionsError(w, err, "PRESIGN_PROOF_FAILED", "Gagal membuat URL pratinjau bukti transfer.")
		return
	}

	respond.JSON(w, http.StatusOK, signed, nil)
}

// VerifyManualPayment handles POST /api/v1/admin/payments/{paymentId}/verify-manual.
func (h *PaymentsHandler) VerifyManualPayment(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	paymentID, ok := parseUUIDParam(w, r, "paymentId", "ID transaksi pembayaran tidak valid.")
	if !ok {
		return
	}

	payment, err := h.Service.GetAdminPayment(r.Context(), tenant.ID, paymentID)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENT_FETCH_FAILED", "Gagal memuat data transaksi.")
		return
	}
	if !middleware.HasUnitAccess(principal, payment.UnitID, treasurerMutationRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Hanya bendahara atau admin berwenang yang dapat memverifikasi pembayaran unit ini.", nil)
		return
	}

	var req admissions.VerifyManualPaymentInput
	if !decodeJSON(w, r, &req) {
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, payment.UnitID)
	res, err := h.Service.VerifyManualPayment(
		r.Context(),
		tenant.ID,
		paymentID,
		req,
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENT_VERIFY_FAILED", "Gagal memproses verifikasi pembayaran manual.")
		return
	}

	respond.JSON(w, http.StatusOK, res, nil)
}

// SyncApplicantPayment handles POST /api/v1/applicant/applications/{applicationId}/payments/{paymentId}/sync.
func (h *PaymentsHandler) SyncApplicantPayment(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}
	paymentID, ok := parseUUIDParam(w, r, "paymentId", "ID transaksi pembayaran tidak valid.")
	if !ok {
		return
	}

	payments, err := h.Service.ListApplicantPayments(r.Context(), tenant.ID, principal.UserID, appID)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENTS_LIST_FAILED", "Gagal memverifikasi kepemilikan tagihan.")
		return
	}
	owned := false
	for _, p := range payments {
		if p.ID == paymentID {
			owned = true
			break
		}
	}
	if !owned {
		respond.Error(w, http.StatusNotFound, "PAYMENT_NOT_FOUND", "Transaksi pembayaran tidak ditemukan pada pendaftaran ini.", nil)
		return
	}

	uid := principal.UserID
	res, err := h.Service.SyncPaymentStatus(
		r.Context(),
		tenant.ID,
		&uid,
		paymentID,
		actorFromRequest(principal.UserID, "applicant", r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENT_SYNC_FAILED", "Gagal menyinkronkan status pembayaran dari payment gateway.")
		return
	}

	respond.JSON(w, http.StatusOK, res, nil)
}

// SyncAdminPayment handles POST /api/v1/admin/payments/{paymentId}/sync for treasurer/admin gateway reconciliation.
func (h *PaymentsHandler) SyncAdminPayment(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	paymentID, ok := parseUUIDParam(w, r, "paymentId", "ID transaksi pembayaran tidak valid.")
	if !ok {
		return
	}

	payment, err := h.Service.GetAdminPayment(r.Context(), tenant.ID, paymentID)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENT_FETCH_FAILED", "Gagal memuat data transaksi.")
		return
	}
	if !middleware.HasUnitAccess(principal, payment.UnitID, treasurerMutationRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Hanya bendahara atau admin berwenang yang dapat menyinkronkan pembayaran unit ini.", nil)
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, payment.UnitID)
	res, err := h.Service.SyncPaymentStatus(
		r.Context(),
		tenant.ID,
		nil,
		paymentID,
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "PAYMENT_SYNC_FAILED", "Gagal menyinkronkan status pembayaran dari payment gateway.")
		return
	}

	respond.JSON(w, http.StatusOK, res, nil)
}

// HandleWebhook handles POST /api/v1/webhooks/payments/{provider} for both Midtrans SHA512 and HMAC-SHA256 callbacks.
func (h *PaymentsHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	var tenantID uuid.UUID
	if tenant, ok := middleware.TenantFromContext(r.Context()); ok {
		tenantID = tenant.ID
	}
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		respond.Error(w, http.StatusBadRequest, "INVALID_PROVIDER", "Provider payment gateway wajib disertakan.", nil)
		return
	}

	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_BODY", "Gagal membaca payload webhook.", nil)
		return
	}

	var input admissions.PaymentWebhookInput
	if err := json.Unmarshal(rawBody, &input); err != nil {
		respond.Error(w, http.StatusBadRequest, "INVALID_JSON", "Payload webhook harus berformat JSON valid.", nil)
		return
	}
	input.Provider = provider

	secret := h.WebhookSecret
	if (strings.TrimSpace(input.SignatureKey) != "" || (strings.EqualFold(provider, "midtrans") && strings.TrimSpace(input.OrderID) != "")) &&
		strings.TrimSpace(h.MidtransServerKey) != "" {
		secret = h.MidtransServerKey
	}

	sig := r.Header.Get("X-Webhook-Signature")
	res, alreadyProcessed, err := h.Service.HandlePaymentWebhook(
		r.Context(),
		tenantID,
		rawBody,
		sig,
		secret,
		input,
	)
	if err != nil {
		writeAdmissionsError(w, err, "WEBHOOK_FAILED", "Gagal memproses callback payment gateway.")
		return
	}

	respond.JSON(w, http.StatusOK, res, map[string]any{
		"already_processed": alreadyProcessed,
		"provider":          provider,
	})
}

