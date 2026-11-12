package admissions

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/storage"
	"github.com/pbdb/platform/internal/worker"
)

type demoExportRecord struct {
	TenantID uuid.UUID
	Job      ExportJob
	File     DemoExportFile
}

func (s *DemoStore) CreateExportJob(
	_ context.Context,
	tenantID, unitID, periodID uuid.UUID,
	format string,
	actor ActorContext,
) (*ExportJob, error) {
	tracks, ok := s.cat().FindPeriodTracks(tenantID, periodID)
	if !ok || len(tracks) == 0 || tracks[0].Unit.ID != unitID {
		return nil, ErrPeriodNotFound
	}
	unit := tracks[0].Unit
	period := tracks[0].Period

	s.mu.Lock()
	defer s.mu.Unlock()

	rows := s.collectExportRowsLocked(tenantID, unitID, periodID)
	filename, mimeType, content, err := worker.BuildNationalExport(format, unit.Code, rows)
	if err != nil {
		return nil, err
	}

	jobID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	storageKey := fmt.Sprintf("%s%s/%s/%s", demoExportKeyPrefix, tenantID.String(), jobID.String(), storage.SanitizeFilename(filename))
	rowCount := len(rows)

	job := ExportJob{
		ID:                jobID,
		UnitID:            unitID,
		UnitName:          unit.Name,
		PeriodID:          periodID,
		PeriodName:        period.Name,
		RequestedByUserID: actor.UserID,
		Format:            format,
		Status:            ExportStatusCompleted,
		ResultStorageKey:  &storageKey,
		RowCount:          &rowCount,
		CreatedAt:         now,
		CompletedAt:       &now,
	}

	s.exportJobs[jobID] = &demoExportRecord{
		TenantID: tenantID,
		Job:      job,
		File: DemoExportFile{
			Filename: filename,
			MimeType: mimeType,
			Content:  content,
		},
	}

	reason := fmt.Sprintf("Ekspor %s (%s) selesai dengan %d baris data.", format, unit.Code, rowCount)
	unitIDCopy := unitID
	actorIDCopy := actor.UserID
	s.tenantAudits = append([]TenantAuditLogItem{{
		ID:          uuid.Must(uuid.NewV7()),
		UnitID:      &unitIDCopy,
		ActorUserID: &actorIDCopy,
		ActorRole:   actor.RoleName,
		ActionType:  "export_requested",
		EntityType:  "export_job",
		EntityID:    jobID,
		Reason:      &reason,
		CreatedAt:   now,
	}}, s.tenantAudits...)

	copied := job
	return &copied, nil
}

func (s *DemoStore) GetExportJob(_ context.Context, tenantID, jobID uuid.UUID) (*ExportJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.exportJobs[jobID]
	if !ok || rec.TenantID != tenantID {
		return nil, nil
	}
	copied := rec.Job
	return &copied, nil
}

func (s *DemoStore) ListExportJobs(_ context.Context, tenantID uuid.UUID, allowedUnitIDs []uuid.UUID, limit int) ([]ExportJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := []ExportJob{}
	for _, rec := range s.exportJobs {
		if rec.TenantID != tenantID {
			continue
		}
		if len(allowedUnitIDs) > 0 && !slices.Contains(allowedUnitIDs, rec.Job.UnitID) {
			continue
		}
		out = append(out, rec.Job)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() > out[j].ID.String()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *DemoStore) GetDemoExportFile(_ context.Context, tenantID, jobID uuid.UUID) (*DemoExportFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.exportJobs[jobID]
	if !ok || rec.TenantID != tenantID {
		return nil, nil
	}
	return &DemoExportFile{
		Filename: rec.File.Filename,
		MimeType: rec.File.MimeType,
		Content:  slices.Clone(rec.File.Content),
	}, nil
}

func (s *DemoStore) ListTenantAuditLogs(_ context.Context, tenantID uuid.UUID, filter TenantAuditLogFilter) ([]TenantAuditLogItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := []TenantAuditLogItem{}

	for _, app := range s.apps {
		if !s.belongsToTenantLocked(app, tenantID) {
			continue
		}
		if filter.ApplicationID != nil && app.Application.ID != *filter.ApplicationID {
			continue
		}
		if len(filter.AllowedUnitIDs) > 0 && !slices.Contains(filter.AllowedUnitIDs, app.Application.UnitID) {
			continue
		}

		appID := app.Application.ID
		regNo := app.Application.RegistrationNo
		unitID := app.Application.UnitID
		var studentName *string
		if app.Profile != nil && strings.TrimSpace(app.Profile.FullName) != "" {
			n := app.Profile.FullName
			studentName = &n
		} else if app.Application.StudentName != nil {
			n := *app.Application.StudentName
			studentName = &n
		}

		for _, entry := range app.AuditLogs {
			if filter.ActionType != nil && entry.ActionType != *filter.ActionType {
				continue
			}
			items = append(items, TenantAuditLogItem{
				ID:             entry.ID,
				ApplicationID:  &appID,
				RegistrationNo: &regNo,
				StudentName:    studentName,
				UnitID:         &unitID,
				ActorUserID:    entry.ActorUserID,
				ActorName:      entry.ActorName,
				ActorRole:      entry.ActorRole,
				ActionType:     entry.ActionType,
				EntityType:     entry.EntityType,
				EntityID:       entry.EntityID,
				FromStatus:     entry.FromStatus,
				ToStatus:       entry.ToStatus,
				Reason:         entry.Reason,
				CreatedAt:      entry.CreatedAt,
			})
		}
	}

	if filter.ApplicationID == nil {
		for _, entry := range s.tenantAudits {
			if entry.UnitID != nil && !s.cat().UnitBelongsToTenant(tenantID, *entry.UnitID) {
				continue
			}
			if len(filter.AllowedUnitIDs) > 0 && (entry.UnitID == nil || !slices.Contains(filter.AllowedUnitIDs, *entry.UnitID)) {
				continue
			}
			if filter.ActionType != nil && entry.ActionType != *filter.ActionType {
				continue
			}
			items = append(items, entry)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID.String() > items[j].ID.String()
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *DemoStore) collectExportRowsLocked(tenantID, unitID, periodID uuid.UUID) []worker.ExportStudentRow {
	type exportCandidate struct {
		enrolledFirst bool
		sortName      string
		row           worker.ExportStudentRow
	}

	trackCodes := map[uuid.UUID]string{}
	if tracks, ok := s.cat().FindPeriodTracks(tenantID, periodID); ok {
		for _, t := range tracks {
			trackCodes[t.Track.ID] = t.Track.Code
		}
	}

	candidates := []exportCandidate{}
	for _, app := range s.apps {
		if !s.belongsToTenantLocked(app, tenantID) {
			continue
		}
		if app.Application.UnitID != unitID || app.Application.PeriodID != periodID {
			continue
		}
		switch app.Application.Status {
		case domain.StatusEnrolled, domain.StatusReEnrollment, domain.StatusAccepted:
		default:
			continue
		}

		p := app.Profile
		strPtr := func(v *string) string {
			if v == nil {
				return ""
			}
			return *v
		}
		int16Ptr := func(v *int16) string {
			if v == nil {
				return ""
			}
			return strconv.FormatInt(int64(*v), 10)
		}
		floatPtr := func(v *float64) string {
			if v == nil {
				return ""
			}
			return strconv.FormatFloat(*v, 'f', 6, 64)
		}
		intPtr := func(v *int) string {
			if v == nil {
				return ""
			}
			return strconv.Itoa(*v)
		}

		fullName := strPtr(app.Application.StudentName)
		row := worker.ExportStudentRow{
			RegistrationNo: app.Application.RegistrationNo,
			Status:         string(app.Application.Status),
			TrackCode:      trackCodes[app.Application.TrackID],
			TrackName:      app.Application.TrackName,
			FullName:       fullName,
			FinalScore:     strPtr(app.Application.FinalScore),
			RankPosition:   intPtr(app.Application.RankPosition),
		}
		if p != nil {
			if strings.TrimSpace(p.FullName) != "" {
				row.FullName = p.FullName
			}
			row.NIK = strPtr(p.NIK)
			row.KKNumber = strPtr(p.KKNumber)
			row.NISN = strPtr(p.NISN)
			row.BirthPlace = strPtr(p.BirthPlace)
			row.BirthDate = strPtr(p.BirthDate)
			row.Gender = strPtr(p.Gender)
			row.Religion = strPtr(p.Religion)
			if strings.TrimSpace(p.Citizenship) != "" {
				row.Citizenship = p.Citizenship
			} else {
				row.Citizenship = "WNI"
			}
			row.ChildOrder = int16Ptr(p.ChildOrder)
			row.SiblingsCount = int16Ptr(p.SiblingsCount)
			row.SpecialNeedsCode = strPtr(p.SpecialNeedsCode)
			row.AddressStreet = strPtr(p.AddressStreet)
			row.RT = strPtr(p.RT)
			row.RW = strPtr(p.RW)
			row.Hamlet = strPtr(p.Hamlet)
			row.VillageName = strPtr(p.VillageName)
			row.DistrictName = strPtr(p.DistrictName)
			row.RegencyName = strPtr(p.RegencyName)
			row.ProvinceName = strPtr(p.ProvinceName)
			row.PostalCode = strPtr(p.PostalCode)
			row.Latitude = floatPtr(p.Latitude)
			row.Longitude = floatPtr(p.Longitude)
			row.PrevSchoolName = strPtr(p.PrevSchoolName)
			row.PrevSchoolNPSN = strPtr(p.PrevSchoolNPSN)
			row.GraduationYear = int16Ptr(p.GraduationYear)
			row.IjazahNumber = strPtr(p.IjazahNumber)
		}

		var fatherWA, motherWA, guardianWA string
		for _, g := range app.Guardians {
			switch g.Relation {
			case "father":
				row.FatherName = g.FullName
				row.FatherNIK = strPtr(g.NIK)
				row.FatherOccupation = strPtr(g.Occupation)
				row.FatherIncome = strPtr(g.IncomeBracket)
				fatherWA = strPtr(g.PhoneWhatsApp)
			case "mother":
				row.MotherName = g.FullName
				row.MotherNIK = strPtr(g.NIK)
				row.MotherOccupation = strPtr(g.Occupation)
				row.MotherIncome = strPtr(g.IncomeBracket)
				motherWA = strPtr(g.PhoneWhatsApp)
			case "guardian":
				row.GuardianName = g.FullName
				row.GuardianNIK = strPtr(g.NIK)
				guardianWA = strPtr(g.PhoneWhatsApp)
			}
		}
		if guardianWA != "" {
			row.ContactWhatsApp = guardianWA
		} else if fatherWA != "" {
			row.ContactWhatsApp = fatherWA
		} else {
			row.ContactWhatsApp = motherWA
		}

		sortName := row.FullName
		if sortName == "" {
			sortName = row.RegistrationNo
		}
		candidates = append(candidates, exportCandidate{
			enrolledFirst: app.Application.Status == domain.StatusEnrolled,
			sortName:      strings.ToLower(sortName),
			row:           row,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].enrolledFirst != candidates[j].enrolledFirst {
			return candidates[i].enrolledFirst
		}
		return candidates[i].sortName < candidates[j].sortName
	})

	rows := make([]worker.ExportStudentRow, len(candidates))
	for i := range candidates {
		rows[i] = candidates[i].row
	}
	return rows
}
