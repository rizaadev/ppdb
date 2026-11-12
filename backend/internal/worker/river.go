package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/storage"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// SendWhatsAppArgs represents a queued WhatsApp notification job.
type SendWhatsAppArgs struct {
	TenantID        uuid.UUID  `json:"tenant_id"`
	ApplicationID   *uuid.UUID `json:"application_id,omitempty"`
	NotificationLog uuid.UUID  `json:"notification_log_id"`
	RecipientPhone  string     `json:"recipient_phone"`
	TemplateCode    string     `json:"template_code"`
	Variables       []string   `json:"variables,omitempty"`
}

// Kind identifies the WhatsApp notification job type in River.
func (SendWhatsAppArgs) Kind() string { return "send_whatsapp_notification" }

// SendWhatsAppWorker processes outgoing WhatsApp notifications via configured gateway providers.
type SendWhatsAppWorker struct {
	river.WorkerDefaults[SendWhatsAppArgs]
	Pool   *pgxpool.Pool
	Sender integrations.WhatsAppSender
	Logger *slog.Logger
}

// Work executes the WhatsApp delivery job and records delivery status into notification_logs.
func (w *SendWhatsAppWorker) Work(ctx context.Context, job *river.Job[SendWhatsAppArgs]) error {
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sender := w.Sender
	if sender == nil {
		sender = &integrations.LogWhatsAppSender{Logger: logger}
	}

	res, sendErr := sender.Send(ctx, integrations.WhatsAppMessage{
		RecipientPhone: job.Args.RecipientPhone,
		TemplateCode:   job.Args.TemplateCode,
		Variables:      job.Args.Variables,
	})

	if w.Pool != nil && job.Args.NotificationLog != uuid.Nil {
		status := "sent"
		var msgID, errMsg *string
		if res.ProviderMessageID != "" {
			msgID = &res.ProviderMessageID
		}
		if sendErr != nil {
			status = "failed"
			s := sendErr.Error()
			errMsg = &s
		}
		updateFn := func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				UPDATE notification_logs
				SET status = $2,
				    provider_message_id = COALESCE($3, provider_message_id),
				    error_message = $4,
				    sent_at = CASE WHEN $2 = 'sent' THEN NOW() ELSE sent_at END
				WHERE id = $1
			`, job.Args.NotificationLog, status, msgID, errMsg)
			return err
		}
		var dbErr error
		if job.Args.TenantID != uuid.Nil {
			dbErr = database.WithTenantTx(ctx, w.Pool, job.Args.TenantID, updateFn)
		} else {
			dbErr = database.WithBypassRLSTx(ctx, w.Pool, updateFn)
		}
		if dbErr != nil {
			logger.ErrorContext(ctx, "failed to update notification_logs", "notification_log_id", job.Args.NotificationLog, "error", dbErr)
		}
	}

	if sendErr != nil {
		logger.WarnContext(ctx, "whatsapp delivery failed",
			"job_id", job.ID,
			"tenant_id", job.Args.TenantID,
			"template", job.Args.TemplateCode,
			"error", sendErr,
		)
		return sendErr
	}

	return nil
}

// GenerateExportArgs represents an asynchronous Dapodik/EMIS/CSV export job.
type GenerateExportArgs struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	ExportJobID uuid.UUID `json:"export_job_id"`
	Format      string    `json:"format"`
}

// Kind identifies the national export job type in River.
func (GenerateExportArgs) Kind() string { return "generate_national_export" }

// GenerateExportWorker builds Excel/CSV exports and stores them in private object storage.
type GenerateExportWorker struct {
	river.WorkerDefaults[GenerateExportArgs]
	Pool    *pgxpool.Pool
	Storage storage.ObjectStorage
	Logger  *slog.Logger
}

// Work executes the export job: collects enrolled candidates from PostgreSQL, builds XLSX/CSV, and uploads to object storage.
func (w *GenerateExportWorker) Work(ctx context.Context, job *river.Job[GenerateExportArgs]) error {
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if w.Pool == nil {
		return fmt.Errorf("database pool is required for export worker")
	}

	var unitID, periodID uuid.UUID
	var unitCode string
	err := database.WithTenantTx(ctx, w.Pool, job.Args.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE export_jobs e
			SET status = 'processing'::export_job_status
			FROM units u
			WHERE e.tenant_id = $1 AND e.id = $2
			  AND u.tenant_id = e.tenant_id AND u.id = e.unit_id
			RETURNING e.unit_id, e.period_id, u.code
		`, job.Args.TenantID, job.Args.ExportJobID).Scan(&unitID, &periodID, &unitCode)
	})
	if err != nil {
		return fmt.Errorf("claim export job: %w", err)
	}

	failJob := func(cause error) error {
		msg := cause.Error()
		_ = database.WithTenantTx(ctx, w.Pool, job.Args.TenantID, func(tx pgx.Tx) error {
			_, execErr := tx.Exec(ctx, `
				UPDATE export_jobs
				SET status = 'failed'::export_job_status,
				    error_message = $3,
				    completed_at = NOW()
				WHERE tenant_id = $1 AND id = $2
			`, job.Args.TenantID, job.Args.ExportJobID, msg)
			return execErr
		})
		return cause
	}

	if w.Storage == nil {
		return failJob(storage.ErrStorageNotConfigured)
	}

	rows, err := QueryExportRowsPG(ctx, w.Pool, job.Args.TenantID, unitID, periodID)
	if err != nil {
		return failJob(fmt.Errorf("query export rows: %w", err))
	}

	filename, mimeType, content, err := BuildNationalExport(job.Args.Format, unitCode, rows)
	if err != nil {
		return failJob(fmt.Errorf("build export payload: %w", err))
	}

	objectKey := storage.BuildExportObjectKey(job.Args.TenantID, job.Args.ExportJobID, filename)
	if err := w.Storage.PutObject(ctx, objectKey, mimeType, content); err != nil {
		return failJob(fmt.Errorf("upload export artifact: %w", err))
	}

	if err := database.WithTenantTx(ctx, w.Pool, job.Args.TenantID, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `
			UPDATE export_jobs
			SET status = 'completed'::export_job_status,
			    result_storage_key = $3,
			    row_count = $4,
			    completed_at = NOW()
			WHERE tenant_id = $1 AND id = $2
		`, job.Args.TenantID, job.Args.ExportJobID, objectKey, len(rows))
		return execErr
	}); err != nil {
		return fmt.Errorf("mark export job completed: %w", err)
	}

	logger.InfoContext(ctx, "export job completed",
		"job_id", job.ID,
		"export_job_id", job.Args.ExportJobID,
		"row_count", len(rows),
		"storage_key", objectKey,
	)
	return nil
}

// QueryExportRowsPG extracts accepted/re-enrolled/enrolled candidates and parent contacts for national exports.
func QueryExportRowsPG(ctx context.Context, pool *pgxpool.Pool, tenantID, unitID, periodID uuid.UUID) ([]ExportStudentRow, error) {
	var out []ExportStudentRow
	err := database.WithTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		rows, err := queryExportRowsTx(ctx, tx, tenantID, unitID, periodID)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	return out, err
}

func queryExportRowsTx(ctx context.Context, tx pgx.Tx, tenantID, unitID, periodID uuid.UUID) ([]ExportStudentRow, error) {
	pgRows, err := tx.Query(ctx, `
		SELECT
		    a.registration_no,
		    a.status::text,
		    t.code,
		    t.display_name,
		    COALESCE(p.full_name, ''),
		    COALESCE(p.nik, ''),
		    COALESCE(p.kk_number, ''),
		    COALESCE(p.nisn, ''),
		    COALESCE(p.birth_place, ''),
		    COALESCE(TO_CHAR(p.birth_date, 'YYYY-MM-DD'), ''),
		    COALESCE(p.gender::text, ''),
		    COALESCE(p.religion, ''),
		    COALESCE(p.citizenship, 'WNI'),
		    COALESCE(p.child_order::text, ''),
		    COALESCE(p.siblings_count::text, ''),
		    COALESCE(p.special_needs_code, ''),
		    COALESCE(p.address_street, ''),
		    COALESCE(p.rt, ''),
		    COALESCE(p.rw, ''),
		    COALESCE(p.hamlet, ''),
		    COALESCE(p.village_name, ''),
		    COALESCE(p.district_name, ''),
		    COALESCE(p.regency_name, ''),
		    COALESCE(p.province_name, ''),
		    COALESCE(p.postal_code, ''),
		    COALESCE(p.latitude::text, ''),
		    COALESCE(p.longitude::text, ''),
		    COALESCE(p.prev_school_name, ''),
		    COALESCE(p.prev_school_npsn, ''),
		    COALESCE(p.graduation_year::text, ''),
		    COALESCE(p.ijazah_number, ''),
		    COALESCE(f.full_name, ''),
		    COALESCE(f.nik, ''),
		    COALESCE(f.occupation, ''),
		    COALESCE(f.income_bracket, ''),
		    COALESCE(m.full_name, ''),
		    COALESCE(m.nik, ''),
		    COALESCE(m.occupation, ''),
		    COALESCE(m.income_bracket, ''),
		    COALESCE(w.full_name, ''),
		    COALESCE(w.nik, ''),
		    COALESCE(w.phone_whatsapp, f.phone_whatsapp, m.phone_whatsapp, ''),
		    COALESCE(a.final_score::text, ''),
		    COALESCE(a.rank_position::text, '')
		FROM applications a
		JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
		LEFT JOIN applicant_profiles p ON p.application_id = a.id AND p.tenant_id = a.tenant_id
		LEFT JOIN applicant_guardians f ON f.application_id = a.id AND f.tenant_id = a.tenant_id AND f.relation = 'father'
		LEFT JOIN applicant_guardians m ON m.application_id = a.id AND m.tenant_id = a.tenant_id AND m.relation = 'mother'
		LEFT JOIN applicant_guardians w ON w.application_id = a.id AND w.tenant_id = a.tenant_id AND w.relation = 'guardian'
		WHERE a.tenant_id = $1
		  AND a.unit_id = $2
		  AND a.period_id = $3
		  AND a.status IN ('enrolled', 're_enrollment', 'accepted')
		ORDER BY a.status = 'enrolled' DESC, COALESCE(p.full_name, a.registration_no) ASC
	`, tenantID, unitID, periodID)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()

	out := []ExportStudentRow{}
	for pgRows.Next() {
		var r ExportStudentRow
		if err := pgRows.Scan(
			&r.RegistrationNo, &r.Status, &r.TrackCode, &r.TrackName,
			&r.FullName, &r.NIK, &r.KKNumber, &r.NISN, &r.BirthPlace, &r.BirthDate,
			&r.Gender, &r.Religion, &r.Citizenship, &r.ChildOrder, &r.SiblingsCount, &r.SpecialNeedsCode,
			&r.AddressStreet, &r.RT, &r.RW, &r.Hamlet, &r.VillageName, &r.DistrictName,
			&r.RegencyName, &r.ProvinceName, &r.PostalCode, &r.Latitude, &r.Longitude,
			&r.PrevSchoolName, &r.PrevSchoolNPSN, &r.GraduationYear, &r.IjazahNumber,
			&r.FatherName, &r.FatherNIK, &r.FatherOccupation, &r.FatherIncome,
			&r.MotherName, &r.MotherNIK, &r.MotherOccupation, &r.MotherIncome,
			&r.GuardianName, &r.GuardianNIK, &r.ContactWhatsApp, &r.FinalScore, &r.RankPosition,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}

// ReconcileExpiredPaymentsArgs represents periodic maintenance to mark overdue pending transactions expired and prune stale idempotency keys.
type ReconcileExpiredPaymentsArgs struct{}

// Kind identifies the payment reconciliation job type in River.
func (ReconcileExpiredPaymentsArgs) Kind() string { return "reconcile_expired_payments" }

// ReconcileExpiredPaymentsWorker expires overdue pending payments and deletes expired idempotency records.
type ReconcileExpiredPaymentsWorker struct {
	river.WorkerDefaults[ReconcileExpiredPaymentsArgs]
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// Work performs periodic cleanup using bypass-RLS maintenance privilege.
func (w *ReconcileExpiredPaymentsWorker) Work(ctx context.Context, _ *river.Job[ReconcileExpiredPaymentsArgs]) error {
	if w.Pool == nil {
		return nil
	}
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var expiredCount, prunedKeys int64
	if err := database.WithBypassRLSTx(ctx, w.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE payment_transactions
			SET status = 'expired'::payment_status,
			    updated_at = NOW()
			WHERE status = 'pending'::payment_status
			  AND expires_at IS NOT NULL
			  AND expires_at < NOW()
		`)
		if err != nil {
			return fmt.Errorf("expire overdue payments: %w", err)
		}
		expiredCount = tag.RowsAffected()

		tag2, err := tx.Exec(ctx, `
			DELETE FROM idempotency_keys
			WHERE expires_at < NOW()
		`)
		if err != nil {
			return fmt.Errorf("prune expired idempotency keys: %w", err)
		}
		prunedKeys = tag2.RowsAffected()
		return nil
	}); err != nil {
		return err
	}

	if expiredCount > 0 || prunedKeys > 0 {
		logger.InfoContext(ctx, "maintenance reconcile completed",
			"expired_payments", expiredCount,
			"pruned_idempotency_keys", prunedKeys,
		)
	}
	return nil
}

// ExpireStaleLocksArgs represents periodic cleanup of abandoned verifier claim locks.
type ExpireStaleLocksArgs struct{}

// Kind identifies the stale verifier lock cleanup job type in River.
func (ExpireStaleLocksArgs) Kind() string { return "expire_stale_locks" }

// ExpireStaleLocksWorker releases verifier locks older than 30 minutes across tenants.
type ExpireStaleLocksWorker struct {
	river.WorkerDefaults[ExpireStaleLocksArgs]
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// Work clears expired application claim locks using bypass-RLS maintenance privilege.
func (w *ExpireStaleLocksWorker) Work(ctx context.Context, _ *river.Job[ExpireStaleLocksArgs]) error {
	if w.Pool == nil {
		return nil
	}
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var released int64
	if err := database.WithBypassRLSTx(ctx, w.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE applications
			SET claimed_by_verifier_id = NULL,
			    claimed_at = NULL,
			    updated_at = NOW()
			WHERE claimed_by_verifier_id IS NOT NULL
			  AND claimed_at < NOW() - INTERVAL '30 minutes'
		`)
		if err != nil {
			return fmt.Errorf("release stale verifier locks: %w", err)
		}
		released = tag.RowsAffected()
		return nil
	}); err != nil {
		return err
	}

	if released > 0 {
		logger.InfoContext(ctx, "released stale verifier claim locks", "count", released)
	}
	return nil
}

// ClientOptions supplies live storage and notification dependencies to River workers.
type ClientOptions struct {
	Logger   *slog.Logger
	Storage  storage.ObjectStorage
	WhatsApp integrations.WhatsAppSender
}

// EnsureRiverSchema applies River's internal PostgreSQL tables if missing.
func EnsureRiverSchema(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("create river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("migrate river schema: %w", err)
	}
	return nil
}

// NewClient creates a configured River client sharing the primary PostgreSQL pool.
func NewClient(pool *pgxpool.Pool, opts ClientOptions, startWorkers bool) (*river.Client[pgx.Tx], error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &SendWhatsAppWorker{
		Pool:   pool,
		Sender: opts.WhatsApp,
		Logger: logger,
	})
	river.AddWorker(workers, &GenerateExportWorker{
		Pool:    pool,
		Storage: opts.Storage,
		Logger:  logger,
	})
	river.AddWorker(workers, &ReconcileExpiredPaymentsWorker{
		Pool:   pool,
		Logger: logger,
	})
	river.AddWorker(workers, &ExpireStaleLocksWorker{
		Pool:   pool,
		Logger: logger,
	})

	cfg := &river.Config{
		Workers:    workers,
		JobTimeout: 2 * time.Minute,
	}
	if startWorkers {
		cfg.Queues = map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		}
		cfg.PeriodicJobs = []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(15*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return ReconcileExpiredPaymentsArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
			river.NewPeriodicJob(
				river.PeriodicInterval(15*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return ExpireStaleLocksArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		}
	}

	client, err := river.NewClient(riverpgxv5.New(pool), cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize river client: %w", err)
	}
	return client, nil
}
