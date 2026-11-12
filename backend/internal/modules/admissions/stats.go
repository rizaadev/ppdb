package admissions

import (
	"context"

	"github.com/google/uuid"
)

// DailyTrendPoint captures application volume for one day.
type DailyTrendPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// TrackStatItem summarizes quota progress and applicant distribution per track.
type TrackStatItem struct {
	TrackID        uuid.UUID `json:"track_id"`
	TrackCode      string    `json:"track_code"`
	DisplayName    string    `json:"display_name"`
	UnitID         uuid.UUID `json:"unit_id"`
	UnitName       string    `json:"unit_name"`
	Quota          int       `json:"quota"`
	ApplicantCount int       `json:"applicant_count"`
	AcceptedCount  int       `json:"accepted_count"`
}

// StatusCountItem provides counts grouped by application status.
type StatusCountItem struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// DashboardStats provides an executive overview of admissions KPIs, finance, and trends.
type DashboardStats struct {
	TotalApplicants       int                  `json:"total_applicants"`
	DraftCount            int                  `json:"draft_count"`
	SubmittedCount        int                  `json:"submitted_count"`
	VerificationCount     int                  `json:"verification_count"`
	RevisionRequiredCount int                  `json:"revision_required_count"`
	VerifiedCount         int                  `json:"verified_count"`
	AcceptedCount         int                  `json:"accepted_count"`
	WaitlistedCount       int                  `json:"waitlisted_count"`
	RejectedCount         int                  `json:"rejected_count"`
	ReEnrolledCount       int                  `json:"re_enrolled_count"`
	TotalRevenueIDR       int64                `json:"total_revenue_idr"`
	PendingRevenueIDR     int64                `json:"pending_revenue_idr"`
	PendingManualPayments int                  `json:"pending_manual_payments"`
	DailyTrends           []DailyTrendPoint    `json:"daily_trends"`
	TrackStats            []TrackStatItem      `json:"track_stats"`
	StatusBreakdown       []StatusCountItem    `json:"status_breakdown"`
	RecentActivities      []TenantAuditLogItem `json:"recent_activities"`
}

// GetDashboardStats aggregates admissions performance metrics for tenant administrators and staff.
func (s *Service) GetDashboardStats(ctx context.Context, tenantID uuid.UUID, unitID *uuid.UUID) (*DashboardStats, error) {
	return s.store.GetDashboardStats(ctx, tenantID, unitID)
}

