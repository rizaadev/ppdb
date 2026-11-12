package admissions

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
)

// GetDashboardStats aggregates admissions performance metrics from in-memory DemoStore records.
func (s *DemoStore) GetDashboardStats(_ context.Context, tenantID uuid.UUID, unitID *uuid.UUID) (*DashboardStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &DashboardStats{
		DailyTrends:      make([]DailyTrendPoint, 0),
		TrackStats:       make([]TrackStatItem, 0),
		StatusBreakdown:  make([]StatusCountItem, 0),
		RecentActivities: make([]TenantAuditLogItem, 0),
	}

	statusMap := make(map[string]int)
	trendMap := make(map[string]int)
	trackMap := make(map[uuid.UUID]*TrackStatItem)

	for _, appDetail := range s.apps {
		app := appDetail.Application
		if !s.belongsToTenantLocked(appDetail, tenantID) {
			continue
		}
		if unitID != nil && app.UnitID != *unitID {
			continue
		}

		stats.TotalApplicants++
		st := string(app.Status)
		statusMap[st]++

		isAccepted := false
		switch app.Status {
		case domain.StatusDraftForm:
			stats.DraftCount++
		case domain.StatusSubmitted:
			stats.SubmittedCount++
		case domain.StatusUnderVerification:
			stats.VerificationCount++
		case domain.StatusRevisionRequired:
			stats.RevisionRequiredCount++
		case domain.StatusVerified:
			stats.VerifiedCount++
		case domain.StatusAccepted:
			stats.AcceptedCount++
			isAccepted = true
		case domain.StatusWaitlisted:
			stats.WaitlistedCount++
		case domain.StatusRejected:
			stats.RejectedCount++
		case domain.StatusReEnrollment, domain.StatusEnrolled:
			stats.ReEnrolledCount++
			isAccepted = true
		}

		item, ok := trackMap[app.TrackID]
		if !ok {
			item = &TrackStatItem{
				TrackID:     app.TrackID,
				DisplayName: app.TrackName,
				UnitID:      app.UnitID,
				UnitName:    app.UnitName,
			}
			trackMap[app.TrackID] = item
		}
		item.ApplicantCount++
		if isAccepted {
			item.AcceptedCount++
		}

		d := app.CreatedAt.Format("2006-01-02")
		trendMap[d]++
	}

	for st, cnt := range statusMap {
		stats.StatusBreakdown = append(stats.StatusBreakdown, StatusCountItem{
			Status: st,
			Count:  cnt,
		})
	}

	for _, pay := range s.payments {
		appDetail, ok := s.apps[pay.ApplicationID]
		if !ok || !s.belongsToTenantLocked(appDetail, tenantID) {
			continue
		}
		if unitID != nil && pay.UnitID != *unitID {
			continue
		}
		if pay.Status == "paid" {
			stats.TotalRevenueIDR += pay.AmountIDR
		} else if pay.Status == "pending" {
			stats.PendingRevenueIDR += pay.AmountIDR
			if pay.Method == "manual_transfer" && pay.ProofStorageKey != nil {
				stats.PendingManualPayments++
			}
		}
	}

	now := time.Now().UTC()
	for i := 13; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		stats.DailyTrends = append(stats.DailyTrends, DailyTrendPoint{
			Date:  d,
			Count: trendMap[d],
		})
	}

	for _, item := range trackMap {
		stats.TrackStats = append(stats.TrackStats, *item)
	}

	for _, app := range s.apps {
		if !s.belongsToTenantLocked(app, tenantID) {
			continue
		}
		if unitID != nil && app.Application.UnitID != *unitID {
			continue
		}
		appID := app.Application.ID
		regNo := app.Application.RegistrationNo
		uID := app.Application.UnitID
		var studentName *string
		if app.Profile != nil && app.Profile.FullName != "" {
			n := app.Profile.FullName
			studentName = &n
		} else if app.Application.StudentName != nil {
			n := *app.Application.StudentName
			studentName = &n
		}
		for _, entry := range app.AuditLogs {
			stats.RecentActivities = append(stats.RecentActivities, TenantAuditLogItem{
				ID:             entry.ID,
				ApplicationID:  &appID,
				RegistrationNo: &regNo,
				StudentName:    studentName,
				UnitID:         &uID,
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
	for _, l := range s.tenantAudits {
		if unitID != nil && l.UnitID != nil && *l.UnitID != *unitID {
			continue
		}
		stats.RecentActivities = append(stats.RecentActivities, l)
	}

	sort.Slice(stats.RecentActivities, func(i, j int) bool {
		return stats.RecentActivities[i].CreatedAt.After(stats.RecentActivities[j].CreatedAt)
	})
	if len(stats.RecentActivities) > 10 {
		stats.RecentActivities = stats.RecentActivities[:10]
	}

	return stats, nil
}
