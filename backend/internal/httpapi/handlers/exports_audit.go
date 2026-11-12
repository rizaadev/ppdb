package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/admissions"
)

// ExportsAuditHandler exposes national Dapodik/EMIS/CSV export jobs and tenant audit logs.
type ExportsAuditHandler struct {
	Service *admissions.Service
}

var exportAuditRoles = []domain.TenantRole{
	domain.RoleTenantAdmin,
	domain.RoleUnitAdmin,
	domain.RoleVerifier,
	domain.RoleTreasurer,
	domain.RolePrincipal,
}

// CreateExport handles POST /api/v1/admin/exports.
func (h *ExportsAuditHandler) CreateExport(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var req admissions.CreateExportJobInput
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.UnitID == uuid.Nil {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Unit sekolah wajib dipilih.", map[string]string{
			"unit_id": "Unit sekolah wajib dipilih.",
		})
		return
	}
	if !middleware.HasUnitAccess(principal, req.UnitID, exportAuditRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki izin mengekspor data untuk unit ini.", nil)
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, req.UnitID)
	job, err := h.Service.CreateExportJob(
		r.Context(),
		tenant.ID,
		req,
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "EXPORT_CREATE_FAILED", "Gagal menjadwalkan ekspor data pendaftar.")
		return
	}

	respond.JSON(w, http.StatusAccepted, job, nil)
}

// ListExports handles GET /api/v1/admin/exports.
func (h *ExportsAuditHandler) ListExports(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	limit := 25
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Parameter limit harus bernilai 1 sampai 100.", nil)
			return
		}
		limit = parsed
	}

	unitPtr, allowedUnits, ok := resolveStaffUnitScope(w, principal, tenant.ID, r.URL.Query().Get("unit_id"), "Anda tidak memiliki akses ekspor ke unit pendidikan tersebut.")
	if !ok {
		return
	}
	if unitPtr != nil {
		allowedUnits = []uuid.UUID{*unitPtr}
	}

	jobs, err := h.Service.ListExportJobs(r.Context(), tenant.ID, allowedUnits, limit)
	if err != nil {
		writeAdmissionsError(w, err, "EXPORT_LIST_FAILED", "Gagal memuat riwayat pekerjaan ekspor.")
		return
	}

	respond.JSON(w, http.StatusOK, jobs, nil)
}

// GetExport handles GET /api/v1/admin/exports/{jobId}.
func (h *ExportsAuditHandler) GetExport(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	jobID, ok := parseUUIDParam(w, r, "jobId", "ID pekerjaan ekspor tidak valid.")
	if !ok {
		return
	}

	job, err := h.Service.GetExportJob(r.Context(), tenant.ID, jobID)
	if err != nil {
		writeAdmissionsError(w, err, "EXPORT_FETCH_FAILED", "Gagal memuat status pekerjaan ekspor.")
		return
	}
	if !middleware.HasUnitAccess(principal, job.UnitID, exportAuditRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki akses ke pekerjaan ekspor unit ini.", nil)
		return
	}

	respond.JSON(w, http.StatusOK, job, nil)
}

// DownloadExport handles GET /api/v1/admin/exports/{jobId}/download.
func (h *ExportsAuditHandler) DownloadExport(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	jobID, ok := parseUUIDParam(w, r, "jobId", "ID pekerjaan ekspor tidak valid.")
	if !ok {
		return
	}

	job, err := h.Service.GetExportJob(r.Context(), tenant.ID, jobID)
	if err != nil {
		writeAdmissionsError(w, err, "EXPORT_FETCH_FAILED", "Gagal memuat pekerjaan ekspor.")
		return
	}
	if !middleware.HasUnitAccess(principal, job.UnitID, exportAuditRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki akses mengunduh ekspor unit ini.", nil)
		return
	}

	file, redirectURL, _, err := h.Service.DownloadExportArtifact(r.Context(), tenant.ID, jobID)
	if err != nil {
		writeAdmissionsError(w, err, "EXPORT_DOWNLOAD_FAILED", "Berkas hasil ekspor belum siap diunduh.")
		return
	}
	if redirectURL != "" {
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", file.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, file.Filename))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Content)
}

// ListAuditLogs handles GET /api/v1/admin/audit-logs.
func (h *ExportsAuditHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	filter := admissions.TenantAuditLogFilter{
		Limit: 100,
	}

	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 200 {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Parameter limit harus bernilai 1 sampai 200.", nil)
			return
		}
		filter.Limit = parsed
	}

	if rawAppID := strings.TrimSpace(q.Get("application_id")); rawAppID != "" {
		parsedApp, err := uuid.Parse(rawAppID)
		if err != nil || parsedApp == uuid.Nil {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Parameter application_id tidak valid.", nil)
			return
		}
		filter.ApplicationID = &parsedApp
	}

	if action := strings.TrimSpace(q.Get("action_type")); action != "" {
		filter.ActionType = &action
	}

	unitPtr, allowedUnits, ok := resolveStaffUnitScope(w, principal, tenant.ID, q.Get("unit_id"), "Anda tidak memiliki akses audit log untuk unit tersebut.")
	if !ok {
		return
	}
	if unitPtr != nil {
		allowedUnits = []uuid.UUID{*unitPtr}
	}
	filter.AllowedUnitIDs = allowedUnits

	logs, err := h.Service.ListTenantAuditLogs(r.Context(), tenant.ID, filter)
	if err != nil {
		writeAdmissionsError(w, err, "AUDIT_LOG_FETCH_FAILED", "Gagal memuat riwayat audit log.")
		return
	}

	respond.JSON(w, http.StatusOK, logs, nil)
}

