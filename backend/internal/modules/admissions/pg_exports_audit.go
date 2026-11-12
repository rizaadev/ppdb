package admissions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/worker"
)

func (s *PGStore) CreateExportJob(
	ctx context.Context,
	tenantID, unitID, periodID uuid.UUID,
	format string,
	actor ActorContext,
) (*ExportJob, error) {
	var job *ExportJob
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var unitName, periodName string
		err := tx.QueryRow(ctx, `
			SELECT u.name, p.name
			FROM admission_periods p
			JOIN units u ON u.id = p.unit_id AND u.tenant_id = p.tenant_id
			WHERE p.tenant_id = $1 AND u.id = $2 AND p.id = $3
		`, tenantID, unitID, periodID).Scan(&unitName, &periodName)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPeriodNotFound
		}
		if err != nil {
			return err
		}

		jobID := uuid.Must(uuid.NewV7())
		created := &ExportJob{
			ID:                jobID,
			UnitID:            unitID,
			UnitName:          unitName,
			PeriodID:          periodID,
			PeriodName:        periodName,
			RequestedByUserID: actor.UserID,
			Format:            format,
			Status:            ExportStatusPending,
		}

		if err := tx.QueryRow(ctx, `
			INSERT INTO export_jobs (
				id, tenant_id, unit_id, period_id, requested_by_user_id, format, status
			) VALUES (
				$1, $2, $3, $4, $5, $6::export_format, 'pending'::export_job_status
			)
			RETURNING created_at
		`, jobID, tenantID, unitID, periodID, actor.UserID, format).Scan(&created.CreatedAt); err != nil {
			return err
		}

		if err := insertAuditLogTx(ctx, tx, AuditWrite{
			TenantID:   tenantID,
			Actor:      actor,
			ActionType: "export_requested",
			EntityType: "export_job",
			EntityID:   jobID,
			PayloadDiff: map[string]any{
				"unit_id":   unitID.String(),
				"period_id": periodID.String(),
				"format":    format,
			},
		}); err != nil {
			return err
		}

		if s.riverClient != nil {
			if _, err := s.riverClient.InsertTx(ctx, tx, worker.GenerateExportArgs{
				TenantID:    tenantID,
				ExportJobID: jobID,
				Format:      format,
			}, nil); err != nil {
				return fmt.Errorf("enqueue river export job: %w", err)
			}
		}
		job = created
		return nil
	})
	return job, err
}

func (s *PGStore) GetExportJob(ctx context.Context, tenantID, jobID uuid.UUID) (*ExportJob, error) {
	var out *ExportJob
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var job ExportJob
		err := tx.QueryRow(ctx, `
			SELECT e.id, e.unit_id, u.name, e.period_id, p.name, e.requested_by_user_id,
			       e.format::text, e.status::text, e.result_storage_key, e.row_count,
			       e.error_message, e.created_at, e.completed_at
			FROM export_jobs e
			JOIN units u ON u.id = e.unit_id AND u.tenant_id = e.tenant_id
			JOIN admission_periods p ON p.id = e.period_id AND p.tenant_id = e.tenant_id
			WHERE e.tenant_id = $1 AND e.id = $2
		`, tenantID, jobID).Scan(
			&job.ID, &job.UnitID, &job.UnitName, &job.PeriodID, &job.PeriodName, &job.RequestedByUserID,
			&job.Format, &job.Status, &job.ResultStorageKey, &job.RowCount,
			&job.ErrorMessage, &job.CreatedAt, &job.CompletedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out = &job
		return nil
	})
	return out, err
}

func (s *PGStore) ListExportJobs(ctx context.Context, tenantID uuid.UUID, allowedUnitIDs []uuid.UUID, limit int) ([]ExportJob, error) {
	args := []any{tenantID}
	where := []string{"e.tenant_id = $1"}
	if len(allowedUnitIDs) > 0 {
		args = append(args, allowedUnitIDs)
		where = append(where, fmt.Sprintf("e.unit_id = ANY($%d)", len(args)))
	}
	args = append(args, limit)
	limitArg := len(args)

	query := fmt.Sprintf(`
		SELECT e.id, e.unit_id, u.name, e.period_id, p.name, e.requested_by_user_id,
		       e.format::text, e.status::text, e.result_storage_key, e.row_count,
		       e.error_message, e.created_at, e.completed_at
		FROM export_jobs e
		JOIN units u ON u.id = e.unit_id AND u.tenant_id = e.tenant_id
		JOIN admission_periods p ON p.id = e.period_id AND p.tenant_id = e.tenant_id
		WHERE %s
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $%d
	`, strings.Join(where, " AND "), limitArg)

	list := []ExportJob{}
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var job ExportJob
			if err := rows.Scan(
				&job.ID, &job.UnitID, &job.UnitName, &job.PeriodID, &job.PeriodName, &job.RequestedByUserID,
				&job.Format, &job.Status, &job.ResultStorageKey, &job.RowCount,
				&job.ErrorMessage, &job.CreatedAt, &job.CompletedAt,
			); err != nil {
				return err
			}
			list = append(list, job)
		}
		return rows.Err()
	})
	return list, err
}

func (s *PGStore) GetDemoExportFile(_ context.Context, _, _ uuid.UUID) (*DemoExportFile, error) {
	return nil, ErrExportNotReady
}

func (s *PGStore) ListTenantAuditLogs(ctx context.Context, tenantID uuid.UUID, filter TenantAuditLogFilter) ([]TenantAuditLogItem, error) {
	args := []any{tenantID}
	where := []string{"sal.tenant_id = $1"}

	if filter.ApplicationID != nil {
		args = append(args, *filter.ApplicationID)
		where = append(where, fmt.Sprintf("sal.application_id = $%d", len(args)))
	}
	if len(filter.AllowedUnitIDs) > 0 {
		args = append(args, filter.AllowedUnitIDs)
		where = append(where, fmt.Sprintf("COALESCE(a.unit_id, ej.unit_id) = ANY($%d)", len(args)))
	}
	if filter.ActionType != nil {
		args = append(args, *filter.ActionType)
		where = append(where, fmt.Sprintf("sal.action_type = $%d", len(args)))
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	args = append(args, limit)
	limitArg := len(args)

	query := fmt.Sprintf(`
		SELECT
		    sal.id,
		    sal.application_id,
		    a.registration_no,
		    COALESCE(p.full_name, a.registration_no),
		    COALESCE(a.unit_id, ej.unit_id),
		    sal.actor_user_id,
		    u.full_name,
		    sal.actor_role,
		    sal.action_type,
		    sal.entity_type,
		    sal.entity_id,
		    sal.from_status,
		    sal.to_status,
		    sal.reason,
		    sal.created_at
		FROM status_audit_logs sal
		LEFT JOIN users u ON u.id = sal.actor_user_id
		LEFT JOIN applications a ON a.tenant_id = sal.tenant_id AND a.id = sal.application_id
		LEFT JOIN applicant_profiles p ON p.tenant_id = a.tenant_id AND p.application_id = a.id
		LEFT JOIN export_jobs ej ON ej.tenant_id = sal.tenant_id AND sal.entity_type = 'export_job' AND ej.id = sal.entity_id
		WHERE %s
		ORDER BY sal.created_at DESC, sal.id DESC
		LIMIT $%d
	`, strings.Join(where, " AND "), limitArg)

	items := []TenantAuditLogItem{}
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item TenantAuditLogItem
			if err := rows.Scan(
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
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

