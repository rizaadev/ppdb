package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/admissions"
)

// StaffApplicationsHandler serves panitia & verifier endpoints protected by tenant/unit RBAC.
type StaffApplicationsHandler struct {
	Service *admissions.Service
}

var verifierMutationRoles = []domain.TenantRole{
	domain.RoleTenantAdmin,
	domain.RoleUnitAdmin,
	domain.RoleVerifier,
}

// List handles GET /api/v1/admin/applications with RBAC unit-scoping and keyset pagination.
func (h *StaffApplicationsHandler) List(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	filter := admissions.StaffListFilter{
		Search: strings.TrimSpace(q.Get("search")),
		Limit:  25,
	}

	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > 100 {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Parameter limit harus bernilai 1 sampai 100.", map[string]string{
				"limit": "Limit harus antara 1 hingga 100.",
			})
			return
		}
		filter.Limit = parsedLimit
	}

	unitPtr, allowedUnits, ok := resolveStaffUnitScope(w, principal, tenant.ID, q.Get("unit_id"), "Anda tidak memiliki akses ke unit pendidikan tersebut.")
	if !ok {
		return
	}
	filter.UnitID = unitPtr
	filter.AllowedUnitIDs = allowedUnits

	if err := parseOptionalQueryFilter(&filter, q.Get("period_id"), q.Get("track_id"), q.Get("status"), q.Get("cursor")); err != nil {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	requestedLimit := filter.Limit
	filter.Limit = requestedLimit + 1

	rows, err := h.Service.ListApplicationsForStaff(r.Context(), tenant.ID, filter)
	if err != nil {
		writeAdmissionsError(w, err, "STAFF_APPLICATIONS_LIST_FAILED", "Gagal memuat daftar pendaftar.")
		return
	}

	hasMore := len(rows) > requestedLimit
	if hasMore {
		rows = rows[:requestedLimit]
	}

	var nextCursor string
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		nextCursor = encodeKeysetCursor(last.CreatedAt, last.ID)
	}

	respond.JSON(w, http.StatusOK, rows, map[string]any{
		"limit":       requestedLimit,
		"has_more":    hasMore,
		"next_cursor": nextCursor,
	})
}

// GetDetail handles GET /api/v1/admin/applications/{applicationId}.
func (h *StaffApplicationsHandler) GetDetail(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	detail, err := h.Service.GetApplicationForStaff(r.Context(), tenant.ID, appID)
	if err != nil {
		writeAdmissionsError(w, err, "APPLICATION_FETCH_FAILED", "Gagal memuat detail berkas pendaftar.")
		return
	}
	if !middleware.HasUnitAccess(principal, detail.Application.UnitID) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki akses ke unit pendaftaran ini.", nil)
		return
	}

	respond.JSON(w, http.StatusOK, detail, nil)
}

// PresignDocumentDownload handles GET /api/v1/admin/applications/{applicationId}/documents/{documentId}/presign-download.
func (h *StaffApplicationsHandler) PresignDocumentDownload(w http.ResponseWriter, r *http.Request) {
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

	detail, err := h.Service.GetApplicationForStaff(r.Context(), tenant.ID, appID)
	if err != nil {
		writeAdmissionsError(w, err, "APPLICATION_FETCH_FAILED", "Gagal memuat data pendaftaran.")
		return
	}
	if !middleware.HasUnitAccess(principal, detail.Application.UnitID) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki akses dokumen pada unit ini.", nil)
		return
	}

	signed, err := h.Service.PresignStaffDownload(r.Context(), tenant.ID, appID, docID)
	if err != nil {
		writeAdmissionsError(w, err, "PRESIGN_DOWNLOAD_FAILED", "Gagal membuat URL pratinjau dokumen.")
		return
	}

	respond.JSON(w, http.StatusOK, signed, nil)
}

// ClaimNext handles POST /api/v1/admin/applications/claim-next.
func (h *StaffApplicationsHandler) ClaimNext(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var req struct {
		UnitID uuid.UUID `json:"unit_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.UnitID == uuid.Nil {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "unit_id wajib diisi.", map[string]string{
			"unit_id": "Unit sekolah wajib dipilih.",
		})
		return
	}

	if !middleware.HasUnitAccess(principal, req.UnitID, verifierMutationRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin verifikasi pada unit ini.", nil)
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, req.UnitID)
	claimed, err := h.Service.ClaimNextForVerification(r.Context(), tenant.ID, req.UnitID, actorFromRequest(principal.UserID, roleName, r))
	if err != nil {
		writeAdmissionsError(w, err, "CLAIM_NEXT_FAILED", "Gagal mengambil antrean verifikasi berikutnya.")
		return
	}

	respond.JSON(w, http.StatusOK, claimed, nil)
}

// ReviewDocument handles PATCH /api/v1/admin/applications/{applicationId}/documents/{documentId}/review.
func (h *StaffApplicationsHandler) ReviewDocument(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Status     string `json:"status"`
		ReviewNote string `json:"review_note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !domain.IsValidDocumentReviewStatus(req.Status) || domain.DocumentReviewStatus(req.Status) == domain.DocStatusPending {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Status review dokumen harus approved atau rejected_for_revision.", map[string]string{
			"status": "Status tidak valid.",
		})
		return
	}

	detail, err := h.Service.GetApplicationForStaff(r.Context(), tenant.ID, appID)
	if err != nil {
		writeAdmissionsError(w, err, "APPLICATION_FETCH_FAILED", "Gagal memuat data pendaftaran.")
		return
	}
	if !middleware.HasUnitAccess(principal, detail.Application.UnitID, verifierMutationRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin verifikasi dokumen pada unit ini.", nil)
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, detail.Application.UnitID)
	doc, err := h.Service.ReviewDocument(
		r.Context(),
		tenant.ID,
		appID,
		docID,
		domain.DocumentReviewStatus(req.Status),
		req.ReviewNote,
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "DOCUMENT_REVIEW_FAILED", "Gagal menyimpan hasil verifikasi dokumen.")
		return
	}

	respond.JSON(w, http.StatusOK, doc, nil)
}

// CompleteVerification handles POST /api/v1/admin/applications/{applicationId}/complete-verification.
func (h *StaffApplicationsHandler) CompleteVerification(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	var req struct {
		Version     int    `json:"version"`
		SummaryNote string `json:"summary_note"`
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

	detail, err := h.Service.GetApplicationForStaff(r.Context(), tenant.ID, appID)
	if err != nil {
		writeAdmissionsError(w, err, "APPLICATION_FETCH_FAILED", "Gagal memuat data pendaftaran.")
		return
	}
	if !middleware.HasUnitAccess(principal, detail.Application.UnitID, verifierMutationRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin menyelesaikan verifikasi pada unit ini.", nil)
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, detail.Application.UnitID)
	updated, err := h.Service.CompleteVerification(
		r.Context(),
		tenant.ID,
		appID,
		req.Version,
		req.SummaryNote,
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "VERIFICATION_COMPLETE_FAILED", "Gagal menyelesaikan proses verifikasi.")
		return
	}

	respond.JSON(w, http.StatusOK, updated, nil)
}

func resolveStaffUnitScope(
	w http.ResponseWriter,
	principal *middleware.Principal,
	tenantID uuid.UUID,
	unitParam string,
	forbiddenMsg string,
	allowedRoles ...domain.TenantRole,
) (*uuid.UUID, []uuid.UUID, bool) {
	if trimmed := strings.TrimSpace(unitParam); trimmed != "" {
		unitID, err := uuid.Parse(trimmed)
		if err != nil || unitID == uuid.Nil {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Format unit_id tidak valid.", nil)
			return nil, nil, false
		}
		if !middleware.HasUnitAccess(principal, unitID, allowedRoles...) {
			respond.Error(w, http.StatusForbidden, "FORBIDDEN", forbiddenMsg, nil)
			return nil, nil, false
		}
		return &unitID, nil, true
	}

	allowedUnits, tenantWide := allowedUnitsInTenant(principal, tenantID)
	if !tenantWide && len(allowedUnits) == 0 {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Tidak ada unit pendidikan yang ditugaskan untuk akun ini.", nil)
		return nil, nil, false
	}
	return nil, allowedUnits, true
}

func requireStaffAppAccess(
	w http.ResponseWriter,
	r *http.Request,
	svc *admissions.Service,
	tenantID uuid.UUID,
	principal *middleware.Principal,
	appID uuid.UUID,
	forbiddenMsg string,
	allowedRoles ...domain.TenantRole,
) (*admissions.ApplicationDetail, bool) {
	detail, err := svc.GetApplicationForStaff(r.Context(), tenantID, appID)
	if err != nil {
		writeAdmissionsError(w, err, "APPLICATION_FETCH_FAILED", "Gagal memuat data pendaftaran.")
		return nil, false
	}
	if !middleware.HasUnitAccess(principal, detail.Application.UnitID, allowedRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", forbiddenMsg, nil)
		return nil, false
	}
	return detail, true
}

func allowedUnitsInTenant(principal *middleware.Principal, tenantID uuid.UUID) ([]uuid.UUID, bool) {
	if principal.IsPlatformAdmin {
		return nil, true
	}
	var unitIDs []uuid.UUID
	for _, scope := range principal.Roles {
		if scope.TenantID != tenantID {
			continue
		}
		if scope.UnitID == nil {
			return nil, true
		}
		unitIDs = append(unitIDs, *scope.UnitID)
	}
	return unitIDs, false
}

func primaryRoleForUnit(principal *middleware.Principal, tenantID, unitID uuid.UUID) string {
	if principal.IsPlatformAdmin {
		return "platform_admin"
	}
	for _, scope := range principal.Roles {
		if scope.TenantID == tenantID && scope.UnitID != nil && *scope.UnitID == unitID {
			return string(scope.Role)
		}
	}
	for _, scope := range principal.Roles {
		if scope.TenantID == tenantID {
			return string(scope.Role)
		}
	}
	return "staff"
}

func parseOptionalQueryFilter(filter *admissions.StaffListFilter, periodRaw, trackRaw, statusRaw, cursorRaw string) error {
	if p := strings.TrimSpace(periodRaw); p != "" {
		id, err := uuid.Parse(p)
		if err != nil || id == uuid.Nil {
			return fmt.Errorf("parameter period_id tidak valid")
		}
		filter.PeriodID = &id
	}
	if t := strings.TrimSpace(trackRaw); t != "" {
		id, err := uuid.Parse(t)
		if err != nil || id == uuid.Nil {
			return fmt.Errorf("parameter track_id tidak valid")
		}
		filter.TrackID = &id
	}
	if s := strings.TrimSpace(statusRaw); s != "" {
		st := domain.ApplicationStatus(s)
		filter.Status = &st
	}
	if c := strings.TrimSpace(cursorRaw); c != "" {
		ts, id, err := decodeKeysetCursor(c)
		if err != nil {
			return fmt.Errorf("parameter cursor tidak valid")
		}
		filter.CursorCreatedAt = &ts
		filter.CursorID = &id
	}
	return nil
}

func encodeKeysetCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeKeysetCursor(encoded string) (time.Time, uuid.UUID, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	tsStr, idStr, found := strings.Cut(string(decoded), "|")
	if !found {
		return time.Time{}, uuid.Nil, fmt.Errorf("malformed cursor")
	}
	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return ts, id, nil
}

// GetDashboardStats handles GET /api/v1/admin/dashboard/stats for committee overview.
func (h *StaffApplicationsHandler) GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var unitID *uuid.UUID
	if rawUnit := strings.TrimSpace(r.URL.Query().Get("unit_id")); rawUnit != "" {
		parsed, err := uuid.Parse(rawUnit)
		if err != nil || parsed == uuid.Nil {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Parameter unit_id tidak valid.", map[string]string{
				"unit_id": "Unit ID tidak valid.",
			})
			return
		}
		if !middleware.HasUnitAccess(principal, parsed, domain.RoleTenantAdmin, domain.RoleUnitAdmin, domain.RoleVerifier, domain.RoleExaminer, domain.RoleTreasurer, domain.RolePrincipal) {
			respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki akses ke unit ini.", nil)
			return
		}
		unitID = &parsed
	}

	stats, err := h.Service.GetDashboardStats(r.Context(), tenant.ID, unitID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "DASHBOARD_STATS_ERROR", "Gagal memuat ringkasan statistik pendaftaran.", nil)
		return
	}

	respond.JSON(w, http.StatusOK, stats, nil)
}


