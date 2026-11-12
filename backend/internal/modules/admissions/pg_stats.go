package admissions

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pbdb/platform/internal/database"
)

// GetDashboardStats aggregates admissions performance metrics for tenant administrators and staff using PostgreSQL RLS.
func (s *PGStore) GetDashboardStats(ctx context.Context, tenantID uuid.UUID, unitID *uuid.UUID) (*DashboardStats, error) {
	stats := &DashboardStats{
		DailyTrends:      make([]DailyTrendPoint, 0),
		TrackStats:       make([]TrackStatItem, 0),
		StatusBreakdown:  make([]StatusCountItem, 0),
		RecentActivities: make([]TenantAuditLogItem, 0),
	}

	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		// 1. Status breakdown and aggregate applicant metrics
		statusRows, err := tx.Query(ctx, `
			SELECT status::text, count(*)
			FROM applications
			WHERE tenant_id = $1 AND ($2::uuid IS NULL OR unit_id = $2)
			GROUP BY status;
		`, tenantID, unitID)
		if err != nil {
			return err
		}
		defer statusRows.Close()

		for statusRows.Next() {
			var st string
			var count int
			if err := statusRows.Scan(&st, &count); err != nil {
				return err
			}
			stats.TotalApplicants += count
			stats.StatusBreakdown = append(stats.StatusBreakdown, StatusCountItem{
				Status: st,
				Count:  count,
			})
			switch st {
			case "draft_form":
				stats.DraftCount += count
			case "submitted":
				stats.SubmittedCount += count
			case "under_review", "verified_review":
				stats.VerificationCount += count
			case "revision_required":
				stats.RevisionRequiredCount += count
			case "verified_eligible":
				stats.VerifiedCount += count
			case "accepted":
				stats.AcceptedCount += count
			case "waitlisted":
				stats.WaitlistedCount += count
			case "rejected":
				stats.RejectedCount += count
			case "re_enrolled", "confirmed":
				stats.ReEnrolledCount += count
			}
		}

		// 2. Financial totals
		var paidRevenue, pendingRevenue int64
		var pendingManual int
		err = tx.QueryRow(ctx, `
			SELECT
				COALESCE(SUM(CASE WHEN status = 'paid' THEN amount_idr ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN status = 'pending' THEN amount_idr ELSE 0 END), 0),
				COUNT(CASE WHEN status = 'pending' AND method = 'manual_transfer' AND proof_storage_key IS NOT NULL THEN 1 END)
			FROM payment_transactions
			WHERE tenant_id = $1 AND ($2::uuid IS NULL OR unit_id = $2);
		`, tenantID, unitID).Scan(&paidRevenue, &pendingRevenue, &pendingManual)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		stats.TotalRevenueIDR = paidRevenue
		stats.PendingRevenueIDR = pendingRevenue
		stats.PendingManualPayments = pendingManual

		// 3. Daily trends (past 14 days)
		trendRows, err := tx.Query(ctx, `
			SELECT
				to_char(date_trunc('day', created_at), 'YYYY-MM-DD') AS reg_date,
				count(*) AS count
			FROM applications
			WHERE tenant_id = $1 AND ($2::uuid IS NULL OR unit_id = $2)
			  AND created_at >= NOW() - INTERVAL '14 days'
			GROUP BY reg_date
			ORDER BY reg_date ASC;
		`, tenantID, unitID)
		if err != nil {
			return err
		}
		defer trendRows.Close()

		trendMap := make(map[string]int)
		for trendRows.Next() {
			var dateStr string
			var count int
			if err := trendRows.Scan(&dateStr, &count); err != nil {
				return err
			}
			trendMap[dateStr] = count
		}

		// Ensure 14 contiguous days are populated for consistent chart rendering
		now := time.Now().UTC()
		for i := 13; i >= 0; i-- {
			d := now.AddDate(0, 0, -i).Format("2006-01-02")
			stats.DailyTrends = append(stats.DailyTrends, DailyTrendPoint{
				Date:  d,
				Count: trendMap[d],
			})
		}

		// 4. Track stats
		trackRows, err := tx.Query(ctx, `
			SELECT
				t.id, t.code, t.display_name, t.unit_id, COALESCE(u.name, ''), t.quota,
				COUNT(a.id) AS applicant_count,
				COUNT(CASE WHEN a.status IN ('accepted', 're_enrolled', 'confirmed') THEN 1 END) AS accepted_count
			FROM admission_tracks t
			JOIN units u ON u.id = t.unit_id AND u.tenant_id = t.tenant_id
			LEFT JOIN applications a ON a.track_id = t.id AND a.tenant_id = t.tenant_id
			WHERE t.tenant_id = $1 AND ($2::uuid IS NULL OR t.unit_id = $2) AND t.is_active = TRUE
			GROUP BY t.id, t.code, t.display_name, t.unit_id, u.name, t.quota
			ORDER BY t.display_name ASC;
		`, tenantID, unitID)
		if err != nil {
			return err
		}
		defer trackRows.Close()

		for trackRows.Next() {
			var item TrackStatItem
			if err := trackRows.Scan(
				&item.TrackID,
				&item.TrackCode,
				&item.DisplayName,
				&item.UnitID,
				&item.UnitName,
				&item.Quota,
				&item.ApplicantCount,
				&item.AcceptedCount,
			); err != nil {
				return err
			}
			stats.TrackStats = append(stats.TrackStats, item)
		}

		// 5. Recent audit logs
		auditRows, err := tx.Query(ctx, `
			SELECT
				l.id, l.application_id, COALESCE(a.registration_no, ''), COALESCE(a.student_name, ''),
				l.unit_id, l.actor_user_id, COALESCE(u.full_name, ''), l.actor_role,
				l.action_type, l.entity_type, l.entity_id, l.from_status, l.to_status,
				l.reason, l.created_at
			FROM status_audit_logs l
			LEFT JOIN applications a ON a.id = l.application_id AND a.tenant_id = l.tenant_id
			LEFT JOIN users u ON u.id = l.actor_user_id
			WHERE l.tenant_id = $1 AND ($2::uuid IS NULL OR l.unit_id = $2)
			ORDER BY l.created_at DESC
			LIMIT 10;
		`, tenantID, unitID)
		if err != nil {
			return err
		}
		defer auditRows.Close()

		for auditRows.Next() {
			var item TenantAuditLogItem
			if err := auditRows.Scan(
				&item.ID,
				&item.ApplicationID,
				&item.RegistrationNo,
				&item.StudentName,
				&item.UnitID,
				&item.ActorUserID,
				&item.ActorName,
				&item.ActorRole,
				&item.ActionType,
				&item.EntityType,
				&item.EntityID,
				&item.FromStatus,
				&item.ToStatus,
				&item.Reason,
				&item.CreatedAt,
			); err != nil {
				return err
			}
			stats.RecentActivities = append(stats.RecentActivities, item)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return stats, nil
}

