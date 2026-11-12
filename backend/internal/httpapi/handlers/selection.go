package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/httpapi/middleware"
	"github.com/pbdb/platform/internal/httpapi/respond"
	"github.com/pbdb/platform/internal/modules/admissions"
)

// SelectionHandler serves examiner scoring, ranking simulation, results publishing, and waitlist promotion endpoints.
type SelectionHandler struct {
	Service *admissions.Service
}

var (
	examinerMutationRoles = []domain.TenantRole{
		domain.RoleTenantAdmin,
		domain.RoleUnitAdmin,
		domain.RoleExaminer,
	}
	principalMutationRoles = []domain.TenantRole{
		domain.RoleTenantAdmin,
		domain.RoleUnitAdmin,
		domain.RolePrincipal,
	}
	scheduleMutationRoles = []domain.TenantRole{
		domain.RoleTenantAdmin,
		domain.RoleUnitAdmin,
		domain.RoleExaminer,
		domain.RolePrincipal,
	}
)

// UpsertScores handles PUT /api/v1/admin/applications/{applicationId}/scores.
func (h *SelectionHandler) UpsertScores(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	detail, ok := requireStaffAppAccess(
		w, r, h.Service, tenant.ID, principal, appID,
		"Hanya penguji atau admin berwenang yang dapat mengisi nilai seleksi unit ini.",
		examinerMutationRoles...,
	)
	if !ok {
		return
	}

	var req admissions.UpsertScoresInput
	if !decodeJSON(w, r, &req) {
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, detail.Application.UnitID)
	res, err := h.Service.UpsertScores(
		r.Context(),
		tenant.ID,
		appID,
		req,
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "SCORES_UPSERT_FAILED", "Gagal menyimpan komponen nilai seleksi.")
		return
	}

	respond.JSON(w, http.StatusOK, res, nil)
}

// SimulateTrackRanking handles POST /api/v1/admin/tracks/{trackId}/simulate-ranking.
func (h *SelectionHandler) SimulateTrackRanking(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	trackID, ok := parseUUIDParam(w, r, "trackId", "ID jalur pendaftaran tidak valid.")
	if !ok {
		return
	}

	track, err := h.Service.GetTrackForStaff(r.Context(), tenant.ID, trackID)
	if err != nil {
		writeAdmissionsError(w, err, "TRACK_FETCH_FAILED", "Gagal memuat metadata jalur pendaftaran.")
		return
	}
	if !middleware.HasUnitAccess(principal, track.UnitID, principalMutationRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Hanya kepala sekolah atau admin berwenang yang dapat melakukan simulasi perankingan unit ini.", nil)
		return
	}

	sim, err := h.Service.SimulateTrackRanking(r.Context(), tenant.ID, trackID)
	if err != nil {
		writeAdmissionsError(w, err, "SIMULATE_RANKING_FAILED", "Gagal menjalankan simulasi perankingan jalur.")
		return
	}

	respond.JSON(w, http.StatusOK, sim, nil)
}

// PublishPeriodResults handles POST /api/v1/admin/periods/{periodId}/publish-results.
func (h *SelectionHandler) PublishPeriodResults(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	periodID, ok := parseUUIDParam(w, r, "periodId", "ID periode pendaftaran tidak valid.")
	if !ok {
		return
	}

	tracks, err := h.Service.GetPeriodTracksForStaff(r.Context(), tenant.ID, periodID)
	if err != nil {
		writeAdmissionsError(w, err, "PERIOD_FETCH_FAILED", "Gagal memuat jalur periode pendaftaran.")
		return
	}
	unitID := tracks[0].UnitID
	if !middleware.HasUnitAccess(principal, unitID, principalMutationRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Hanya kepala sekolah atau admin berwenang yang dapat menerbitkan pengumuman hasil unit ini.", nil)
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, unitID)
	res, err := h.Service.PublishPeriodResults(
		r.Context(),
		tenant.ID,
		periodID,
		strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "PUBLISH_RESULTS_FAILED", "Gagal mengunci dan menerbitkan pengumuman hasil seleksi.")
		return
	}

	respond.JSON(w, http.StatusOK, res, nil)
}

// PromoteWaitlist handles POST /api/v1/admin/applications/{applicationId}/promote-waitlist.
func (h *SelectionHandler) PromoteWaitlist(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	appID, ok := parseUUIDParam(w, r, "applicationId", "ID pendaftaran tidak valid.")
	if !ok {
		return
	}

	detail, ok := requireStaffAppAccess(
		w, r, h.Service, tenant.ID, principal, appID,
		"Hanya kepala sekolah atau admin berwenang yang dapat menaikkan status cadangan unit ini.",
		principalMutationRoles...,
	)
	if !ok {
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, detail.Application.UnitID)
	updated, err := h.Service.PromoteWaitlist(
		r.Context(),
		tenant.ID,
		appID,
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "PROMOTE_WAITLIST_FAILED", "Gagal menaikkan pendaftar cadangan menjadi diterima.")
		return
	}

	respond.JSON(w, http.StatusOK, updated, nil)
}

// ListSchedules handles GET /api/v1/admin/schedules with RBAC unit-scoping.
func (h *SelectionHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
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
		"Anda tidak memiliki akses jadwal asesmen pada unit tersebut.",
	)
	if !ok {
		return
	}

	filter := admissions.AssessmentScheduleFilter{
		UnitID:         unitPtr,
		AllowedUnitIDs: allowedUnits,
		Limit:          100,
	}
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > 200 {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Parameter limit harus bernilai 1 sampai 200.", nil)
			return
		}
		filter.Limit = parsedLimit
	}
	if rawApp := strings.TrimSpace(q.Get("application_id")); rawApp != "" {
		parsed, err := uuid.Parse(rawApp)
		if err != nil || parsed == uuid.Nil {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Format application_id tidak valid.", nil)
			return
		}
		filter.ApplicationID = &parsed
	}
	if rawTrack := strings.TrimSpace(q.Get("track_id")); rawTrack != "" {
		parsed, err := uuid.Parse(rawTrack)
		if err != nil || parsed == uuid.Nil {
			respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Format track_id tidak valid.", nil)
			return
		}
		filter.TrackID = &parsed
	}

	rows, err := h.Service.ListAssessmentSchedules(r.Context(), tenant.ID, filter)
	if err != nil {
		writeAdmissionsError(w, err, "SCHEDULES_LIST_FAILED", "Gagal memuat jadwal asesmen dan wawancara.")
		return
	}

	respond.JSON(w, http.StatusOK, rows, nil)
}

// CreateSchedule handles POST /api/v1/admin/schedules to assign assessment sessions and queue WA alerts.
func (h *SelectionHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var req admissions.CreateAssessmentScheduleInput
	if !decodeJSON(w, r, &req) {
		return
	}

	targets := make([]uuid.UUID, 0, len(req.ApplicationIDs)+1)
	seen := map[uuid.UUID]bool{}
	if req.ApplicationID != nil && *req.ApplicationID != uuid.Nil {
		targets = append(targets, *req.ApplicationID)
		seen[*req.ApplicationID] = true
	}
	for _, id := range req.ApplicationIDs {
		if id != uuid.Nil && !seen[id] {
			targets = append(targets, id)
			seen[id] = true
		}
	}
	if len(targets) == 0 {
		respond.Error(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Pilih minimal satu pendaftaran peserta untuk dijadwalkan.", map[string]string{
			"application_ids": "Minimal satu peserta wajib dipilih.",
		})
		return
	}

	var firstUnitID uuid.UUID
	for _, appID := range targets {
		detail, ok := requireStaffAppAccess(
			w, r, h.Service, tenant.ID, principal, appID,
			"Hanya penguji, kepala sekolah, atau admin berwenang yang dapat mengatur jadwal asesmen unit ini.",
			scheduleMutationRoles...,
		)
		if !ok {
			return
		}
		if firstUnitID == uuid.Nil {
			firstUnitID = detail.Application.UnitID
		}
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, firstUnitID)
	created, err := h.Service.CreateAssessmentSchedule(
		r.Context(),
		tenant.ID,
		req,
		actorFromRequest(principal.UserID, roleName, r),
	)
	if err != nil {
		writeAdmissionsError(w, err, "SCHEDULE_CREATE_FAILED", "Gagal membuat jadwal asesmen.")
		return
	}

	respond.JSON(w, http.StatusCreated, created, nil)
}

// DeleteSchedule handles DELETE /api/v1/admin/schedules/{scheduleId} with unit-scoped RBAC verification.
func (h *SelectionHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	tenant, principal, ok := requireAuth(w, r)
	if !ok {
		return
	}
	scheduleID, ok := parseUUIDParam(w, r, "scheduleId", "ID jadwal asesmen tidak valid.")
	if !ok {
		return
	}

	schedules, err := h.Service.ListAssessmentSchedules(r.Context(), tenant.ID, admissions.AssessmentScheduleFilter{Limit: 200})
	if err != nil {
		writeAdmissionsError(w, err, "SCHEDULES_LIST_FAILED", "Gagal memeriksa data jadwal asesmen.")
		return
	}
	var found *admissions.AssessmentSchedule
	for i := range schedules {
		if schedules[i].ID == scheduleID {
			found = &schedules[i]
			break
		}
	}
	if found == nil {
		respond.Error(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Jadwal asesmen atau wawancara tidak ditemukan.", nil)
		return
	}
	if !middleware.HasUnitAccess(principal, found.UnitID, scheduleMutationRoles...) {
		respond.Error(w, http.StatusForbidden, "FORBIDDEN", "Hanya penguji, kepala sekolah, atau admin berwenang yang dapat menghapus jadwal unit ini.", nil)
		return
	}

	roleName := primaryRoleForUnit(principal, tenant.ID, found.UnitID)
	if err := h.Service.DeleteAssessmentSchedule(
		r.Context(),
		tenant.ID,
		scheduleID,
		actorFromRequest(principal.UserID, roleName, r),
	); err != nil {
		writeAdmissionsError(w, err, "SCHEDULE_DELETE_FAILED", "Gagal menghapus jadwal asesmen.")
		return
	}

	respond.JSON(w, http.StatusOK, map[string]bool{"deleted": true}, nil)
}

