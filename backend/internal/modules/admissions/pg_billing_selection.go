package admissions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/integrations"
)

func (s *PGStore) GetTrackByID(ctx context.Context, tenantID, trackID uuid.UUID) (*CatalogTrack, error) {
	var out *CatalogTrack
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		cat, err := scanCatalogTrackRow(tenantID, tx.QueryRow(ctx, `
			SELECT `+pgSelectCatalogTrackCols+`
			FROM admission_tracks t
			JOIN admission_periods p ON p.id = t.period_id AND p.tenant_id = t.tenant_id
			JOIN units u ON u.id = p.unit_id AND u.tenant_id = p.tenant_id
			WHERE t.tenant_id = $1 AND t.id = $2
		`, tenantID, trackID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		reqs, err := s.loadRequirements(ctx, tx, tenantID, trackID)
		if err != nil {
			return err
		}
		cat.Requirements = reqs
		out = &cat
		return nil
	})
	return out, err
}

func (s *PGStore) ListPeriodTracks(ctx context.Context, tenantID, periodID uuid.UUID) ([]CatalogTrack, error) {
	var list []CatalogTrack
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+pgSelectCatalogTrackCols+`
			FROM admission_tracks t
			JOIN admission_periods p ON p.id = t.period_id AND p.tenant_id = t.tenant_id
			JOIN units u ON u.id = p.unit_id AND u.tenant_id = p.tenant_id
			WHERE t.tenant_id = $1 AND p.id = $2 AND t.is_active = TRUE
			ORDER BY t.sort_order ASC, t.created_at ASC
		`, tenantID, periodID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			cat, err := scanCatalogTrackRow(tenantID, rows)
			if err != nil {
				return err
			}
			list = append(list, cat)
		}
		return rows.Err()
	})
	return list, err
}

func (s *PGStore) CreatePayment(ctx context.Context, payment PaymentTransaction) (*PaymentTransaction, error) {
	tenantID := payment.TenantID
	if tenantID == uuid.Nil {
		return nil, ErrApplicationNotFound
	}
	var out *PaymentTransaction
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var created PaymentTransaction
		err := tx.QueryRow(ctx, `
			INSERT INTO payment_transactions (
				id, tenant_id, application_id, purpose, method, provider, external_ref,
				amount_idr, status, payment_url, expires_at, created_at
			) VALUES (
				$1, $2, $3,
				$4::payment_purpose, $5::payment_method, $6, $7,
				$8, $9::payment_status, $10, $11, $12
			)
			RETURNING tenant_id, id, application_id, purpose::text, method::text, provider, external_ref,
			          amount_idr, status::text, payment_url, proof_storage_key,
			          verified_by_user_id, verification_note, paid_at, expires_at, created_at
		`,
			payment.ID, tenantID, payment.ApplicationID, payment.Purpose, payment.Method, payment.Provider,
			payment.ExternalRef, payment.AmountIDR, payment.Status, payment.PaymentURL, payment.ExpiresAt, payment.CreatedAt,
		).Scan(
			&created.TenantID, &created.ID, &created.ApplicationID, &created.Purpose, &created.Method, &created.Provider,
			&created.ExternalRef, &created.AmountIDR, &created.Status, &created.PaymentURL, &created.ProofStorageKey,
			&created.VerifiedByUserID, &created.VerificationNote, &created.PaidAt, &created.ExpiresAt, &created.CreatedAt,
		)
		if err != nil {
			return err
		}
		created.RegistrationNo = payment.RegistrationNo
		created.StudentName = payment.StudentName
		created.UnitID = payment.UnitID
		created.UnitName = payment.UnitName
		created.TrackName = payment.TrackName
		out = &created
		return nil
	})
	return out, err
}

const pgSelectPaymentCols = `
	pt.tenant_id, pt.id, pt.application_id, a.registration_no, ap.full_name, a.unit_id, u.name, t.display_name,
	pt.purpose::text, pt.method::text, pt.provider, pt.external_ref, pt.amount_idr,
	pt.status::text, pt.payment_url, pt.proof_storage_key, pt.verified_by_user_id,
	pt.verification_note, pt.paid_at, pt.expires_at, pt.created_at
`

func scanPaymentRow(row interface{ Scan(dest ...any) error }) (*PaymentTransaction, error) {
	var p PaymentTransaction
	err := row.Scan(
		&p.TenantID, &p.ID, &p.ApplicationID, &p.RegistrationNo, &p.StudentName, &p.UnitID, &p.UnitName, &p.TrackName,
		&p.Purpose, &p.Method, &p.Provider, &p.ExternalRef, &p.AmountIDR,
		&p.Status, &p.PaymentURL, &p.ProofStorageKey, &p.VerifiedByUserID,
		&p.VerificationNote, &p.PaidAt, &p.ExpiresAt, &p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *PGStore) getPaymentByIDTx(ctx context.Context, q queryRunner, tenantID, paymentID uuid.UUID) (*PaymentTransaction, error) {
	row := q.QueryRow(ctx, `
		SELECT `+pgSelectPaymentCols+`
		FROM payment_transactions pt
		JOIN applications a ON a.id = pt.application_id AND a.tenant_id = pt.tenant_id
		JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
		JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
		LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
		WHERE pt.tenant_id = $1 AND pt.id = $2
	`, tenantID, paymentID)
	p, err := scanPaymentRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (s *PGStore) GetPaymentByID(ctx context.Context, tenantID, paymentID uuid.UUID) (*PaymentTransaction, error) {
	var out *PaymentTransaction
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var err error
		out, err = s.getPaymentByIDTx(ctx, tx, tenantID, paymentID)
		return err
	})
	return out, err
}

func (s *PGStore) GetPaymentByExternalRef(ctx context.Context, tenantID uuid.UUID, externalRef string) (*PaymentTransaction, error) {
	var out *PaymentTransaction
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT `+pgSelectPaymentCols+`
			FROM payment_transactions pt
			JOIN applications a ON a.id = pt.application_id AND a.tenant_id = pt.tenant_id
			JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
			JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
			LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
			WHERE pt.tenant_id = $1 AND pt.external_ref = $2
		`, tenantID, externalRef)
		p, err := scanPaymentRow(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		out = p
		return err
	})
	return out, err
}

// ResolvePaymentByExternalRef resolves a payment transaction by order_id/external_ref without prior tenant context.
func (s *PGStore) ResolvePaymentByExternalRef(ctx context.Context, externalRef string) (*PaymentTransaction, error) {
	var out *PaymentTransaction
	err := database.WithBypassRLSTx(ctx, s.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT `+pgSelectPaymentCols+`
			FROM payment_transactions pt
			JOIN applications a ON a.id = pt.application_id AND a.tenant_id = pt.tenant_id
			JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
			JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
			LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
			WHERE pt.external_ref = $1
		`, externalRef)
		p, err := scanPaymentRow(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		out = p
		return err
	})
	return out, err
}

func (s *PGStore) loadPaymentsByApp(ctx context.Context, q queryRunner, tenantID, applicationID uuid.UUID) ([]PaymentTransaction, error) {
	rows, err := q.Query(ctx, `
		SELECT `+pgSelectPaymentCols+`
		FROM payment_transactions pt
		JOIN applications a ON a.id = pt.application_id AND a.tenant_id = pt.tenant_id
		JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
		JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
		LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
		WHERE pt.tenant_id = $1 AND pt.application_id = $2
		ORDER BY pt.created_at DESC
	`, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []PaymentTransaction{}
	for rows.Next() {
		p, err := scanPaymentRow(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}

func (s *PGStore) ListPaymentsByApplication(ctx context.Context, tenantID, applicationID uuid.UUID) ([]PaymentTransaction, error) {
	var list []PaymentTransaction
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var err error
		list, err = s.loadPaymentsByApp(ctx, tx, tenantID, applicationID)
		return err
	})
	return list, err
}

func (s *PGStore) AttachManualPaymentProof(
	ctx context.Context,
	tenantID, applicationID, paymentID uuid.UUID,
	proofStorageKey string,
) (*PaymentTransaction, error) {
	var out *PaymentTransaction
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE payment_transactions
			SET proof_storage_key = $4
			WHERE tenant_id = $1 AND application_id = $2 AND id = $3
			  AND method = 'manual_transfer' AND status = 'pending'
		`, tenantID, applicationID, paymentID, proofStorageKey)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			existing, err := s.getPaymentByIDTx(ctx, tx, tenantID, paymentID)
			if err != nil {
				return err
			}
			if existing == nil || existing.ApplicationID != applicationID {
				return ErrPaymentNotFound
			}
			return ErrApplicationLocked
		}
		out, err = s.getPaymentByIDTx(ctx, tx, tenantID, paymentID)
		return err
	})
	return out, err
}

func (s *PGStore) ListAdminPayments(ctx context.Context, tenantID uuid.UUID, filter AdminPaymentFilter) ([]PaymentTransaction, error) {
	conds := []string{"pt.tenant_id = $1"}
	args := []any{tenantID}
	argIdx := 2

	if filter.UnitID != nil {
		conds = append(conds, fmt.Sprintf("a.unit_id = $%d", argIdx))
		args = append(args, *filter.UnitID)
		argIdx++
	} else if len(filter.AllowedUnitIDs) > 0 {
		conds = append(conds, fmt.Sprintf("a.unit_id = ANY($%d)", argIdx))
		args = append(args, filter.AllowedUnitIDs)
		argIdx++
	}
	if filter.Status != nil {
		conds = append(conds, fmt.Sprintf("pt.status = $%d::payment_status", argIdx))
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Method != nil {
		conds = append(conds, fmt.Sprintf("pt.method = $%d::payment_method", argIdx))
		args = append(args, *filter.Method)
		argIdx++
	}
	if filter.Purpose != nil {
		conds = append(conds, fmt.Sprintf("pt.purpose = $%d::payment_purpose", argIdx))
		args = append(args, *filter.Purpose)
		argIdx++
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	args = append(args, limit)

	sqlStr := `
		SELECT ` + pgSelectPaymentCols + `
		FROM payment_transactions pt
		JOIN applications a ON a.id = pt.application_id AND a.tenant_id = pt.tenant_id
		JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
		JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
		LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
		WHERE ` + strings.Join(conds, " AND ") + `
		ORDER BY pt.created_at DESC
		LIMIT $` + fmt.Sprintf("%d", argIdx)

	list := []PaymentTransaction{}
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sqlStr, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			p, err := scanPaymentRow(rows)
			if err != nil {
				return err
			}
			list = append(list, *p)
		}
		return rows.Err()
	})
	return list, err
}

func (s *PGStore) VerifyPayment(
	ctx context.Context,
	tenantID, paymentID uuid.UUID,
	decision string,
	verifierUserID *uuid.UUID,
	note *string,
	paidAt time.Time,
	rawWebhook []byte,
	actor ActorContext,
) (*PaymentTransaction, *ApplicationDetail, error) {
	var outPay *PaymentTransaction
	var outApp *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var appID uuid.UUID
		var purpose, curPayStatus, appStatusStr, regNo string
		var studentName *string
		err := tx.QueryRow(ctx, `
			SELECT pt.application_id, pt.purpose::text, pt.status::text, a.status::text, a.registration_no, ap.full_name
			FROM payment_transactions pt
			JOIN applications a ON a.id = pt.application_id AND a.tenant_id = pt.tenant_id
			LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
			WHERE pt.tenant_id = $1 AND pt.id = $2
			FOR UPDATE OF pt, a
		`, tenantID, paymentID).Scan(&appID, &purpose, &curPayStatus, &appStatusStr, &regNo, &studentName)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return err
		}
		if curPayStatus != PaymentStatusPending {
			return ErrPaymentAlreadyFinal
		}

		var paidAtParam *time.Time
		if decision == PaymentStatusPaid {
			ts := paidAt.UTC()
			paidAtParam = &ts
		}
		var rawWebhookParam *string
		if len(rawWebhook) > 0 {
			str := string(rawWebhook)
			rawWebhookParam = &str
		}
		if _, err := tx.Exec(ctx, `
			UPDATE payment_transactions
			SET status = $3::payment_status,
			    verified_by_user_id = $4,
			    verification_note = $5,
			    paid_at = COALESCE($6, paid_at),
			    raw_webhook = COALESCE($7::jsonb, raw_webhook)
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, paymentID, decision, verifierUserID, note, paidAtParam, rawWebhookParam); err != nil {
			return err
		}

		fromPay := PaymentStatusPending
		if err := insertAuditLogTx(ctx, tx, AuditWrite{
			TenantID:    tenantID,
			Application: &appID,
			Actor:       actor,
			ActionType:  "payment_" + decision,
			EntityType:  "payment_transaction",
			EntityID:    paymentID,
			FromStatus:  &fromPay,
			ToStatus:    &decision,
			Reason:      note,
		}); err != nil {
			return err
		}

		if decision == PaymentStatusPaid {
			curAppStatus := domain.ApplicationStatus(appStatusStr)
			sName := "Calon Murid"
			if studentName != nil && *studentName != "" {
				sName = *studentName
			}
			if purpose == PaymentPurposeRegistration && curAppStatus == domain.StatusAwaitingPayment {
				if _, err := tx.Exec(ctx, `
					UPDATE applications
					SET status = 'draft_form'::application_status, version = version + 1
					WHERE tenant_id = $1 AND id = $2
				`, tenantID, appID); err != nil {
					return err
				}
				fromStr := string(domain.StatusAwaitingPayment)
				midStr := string(domain.StatusPaymentVerified)
				toStr := string(domain.StatusDraftForm)
				if err := insertAuditLogTx(ctx, tx, AuditWrite{
					TenantID: tenantID, Application: &appID, Actor: actor,
					ActionType: "payment_verified", EntityType: "application", EntityID: appID,
					FromStatus: &fromStr, ToStatus: &midStr,
				}); err != nil {
					return err
				}
				if err := insertAuditLogTx(ctx, tx, AuditWrite{
					TenantID: tenantID, Application: &appID, Actor: actor,
					ActionType: "form_unlocked", EntityType: "application", EntityID: appID,
					FromStatus: &midStr, ToStatus: &toStr,
				}); err != nil {
					return err
				}
				if err := s.enqueueWhatsAppTx(ctx, tx, NotificationWrite{
					TenantID: tenantID, ApplicationID: appID, TemplateCode: integrations.TemplatePaymentConfirmed,
					Variables: []string{regNo, sName},
				}); err != nil {
					return err
				}
			} else if purpose == PaymentPurposeReEnrollment && curAppStatus == domain.StatusReEnrollment {
				if _, err := tx.Exec(ctx, `
					UPDATE applications
					SET status = 'enrolled'::application_status, version = version + 1
					WHERE tenant_id = $1 AND id = $2
				`, tenantID, appID); err != nil {
					return err
				}
				fromStr := string(domain.StatusReEnrollment)
				toStr := string(domain.StatusEnrolled)
				if err := insertAuditLogTx(ctx, tx, AuditWrite{
					TenantID: tenantID, Application: &appID, Actor: actor,
					ActionType: "re_enrollment_paid", EntityType: "application", EntityID: appID,
					FromStatus: &fromStr, ToStatus: &toStr,
				}); err != nil {
					return err
				}
				if err := s.enqueueWhatsAppTx(ctx, tx, NotificationWrite{
					TenantID: tenantID, ApplicationID: appID, TemplateCode: integrations.TemplateEnrollmentConfirmed,
					Variables: []string{regNo, sName},
				}); err != nil {
					return err
				}
			}
		}

		outPay, err = s.getPaymentByIDTx(ctx, tx, tenantID, paymentID)
		if err != nil {
			return err
		}
		outApp, err = s.loadApplicationDetailTx(ctx, tx, tenantID, appID)
		return err
	})
	return outPay, outApp, err
}

func (s *PGStore) ConfirmReEnrollment(
	ctx context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion int,
	fromStatus, toStatus domain.ApplicationStatus,
	extraAnswers map[string]any,
	audit AuditWrite,
	notify *NotificationWrite,
) (*ApplicationDetail, error) {
	var detail *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var curVersion int
		var curStatus string
		err := tx.QueryRow(ctx, `
			SELECT version, status::text FROM applications
			WHERE tenant_id = $1 AND id = $2 FOR UPDATE
		`, tenantID, applicationID).Scan(&curVersion, &curStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApplicationNotFound
		}
		if err != nil {
			return err
		}
		if curVersion != expectedVersion {
			return ErrVersionConflict
		}
		if domain.ApplicationStatus(curStatus) != fromStatus {
			return ErrApplicationLocked
		}

		var extraParam *string
		if extraAnswers != nil {
			raw, err := marshalJSONMap(extraAnswers)
			if err != nil {
				return err
			}
			extraParam = &raw
		}

		if _, err := tx.Exec(ctx, `
			UPDATE applications
			SET status = $3::application_status,
			    extra_answers = COALESCE(extra_answers, '{}'::jsonb) || COALESCE($4::jsonb, '{}'::jsonb),
			    version = version + 1
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, applicationID, string(toStatus), extraParam); err != nil {
			return err
		}
		if err := insertAuditLogTx(ctx, tx, audit); err != nil {
			return err
		}
		if notify != nil {
			if err := s.enqueueWhatsAppTx(ctx, tx, *notify); err != nil {
				return err
			}
		}
		detail, err = s.loadApplicationDetailTx(ctx, tx, tenantID, applicationID)
		return err
	})
	return detail, err
}

func (s *PGStore) loadScoresByApp(ctx context.Context, q queryRunner, tenantID, applicationID uuid.UUID) ([]AssessmentScore, error) {
	rows, err := q.Query(ctx, `
		SELECT id, application_id, component_code, raw_score::double precision,
		       weight_pct::double precision, weighted_score::double precision,
		       notes, examiner_user_id, updated_at
		FROM assessment_scores
		WHERE tenant_id = $1 AND application_id = $2
		ORDER BY component_code ASC
	`, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []AssessmentScore{}
	for rows.Next() {
		var sc AssessmentScore
		if err := rows.Scan(
			&sc.ID, &sc.ApplicationID, &sc.ComponentCode, &sc.RawScore,
			&sc.WeightPct, &sc.WeightedScore, &sc.Notes, &sc.ExaminerUserID, &sc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, sc)
	}
	return list, rows.Err()
}

func (s *PGStore) UpsertApplicationScores(
	ctx context.Context,
	tenantID, applicationID uuid.UUID,
	examinerUserID uuid.UUID,
	inputs []ScoreComponentInput,
	actor ActorContext,
) (*UpsertScoresResult, error) {
	var res *UpsertScoresResult
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var statusStr string
		err := tx.QueryRow(ctx, `
			SELECT status::text FROM applications
			WHERE tenant_id = $1 AND id = $2 FOR UPDATE
		`, tenantID, applicationID).Scan(&statusStr)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApplicationNotFound
		}
		if err != nil {
			return err
		}

		for _, sc := range inputs {
			weighted := sc.RawScore * (sc.WeightPct / 100.0)
			var notesPtr *string
			if sc.Notes != "" {
				n := sc.Notes
				notesPtr = &n
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO assessment_scores (
					id, tenant_id, application_id, examiner_user_id, component_code,
					raw_score, weight_pct, weighted_score, notes
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				ON CONFLICT (application_id, component_code) DO UPDATE SET
					examiner_user_id = EXCLUDED.examiner_user_id,
					raw_score = EXCLUDED.raw_score,
					weight_pct = EXCLUDED.weight_pct,
					weighted_score = EXCLUDED.weighted_score,
					notes = EXCLUDED.notes,
					updated_at = NOW()
			`, uuid.Must(uuid.NewV7()), tenantID, applicationID, examinerUserID, sc.ComponentCode, sc.RawScore, sc.WeightPct, weighted, notesPtr); err != nil {
				return err
			}
		}

		var total float64
		if err := tx.QueryRow(ctx, `
			UPDATE applications a
			SET final_score = COALESCE((
				SELECT SUM(s.weighted_score) FROM assessment_scores s
				WHERE s.tenant_id = a.tenant_id AND s.application_id = a.id
			), 0),
			status = CASE
				WHEN a.status = 'verified'::application_status THEN 'selection_in_progress'::application_status
				ELSE a.status
			END,
			version = a.version + 1
			WHERE a.tenant_id = $1 AND a.id = $2
			RETURNING COALESCE(final_score::double precision, 0)
		`, tenantID, applicationID).Scan(&total); err != nil {
			return err
		}

		var fromPtr, toPtr *string
		if domain.ApplicationStatus(statusStr) == domain.StatusVerified {
			f := string(domain.StatusVerified)
			t := string(domain.StatusSelectionInProgress)
			fromPtr, toPtr = &f, &t
		}
		if err := insertAuditLogTx(ctx, tx, AuditWrite{
			TenantID: tenantID, Application: &applicationID, Actor: actor,
			ActionType: "assessment_scored", EntityType: "application", EntityID: applicationID,
			FromStatus: fromPtr, ToStatus: toPtr,
		}); err != nil {
			return err
		}

		detail, err := s.loadApplicationDetailTx(ctx, tx, tenantID, applicationID)
		if err != nil {
			return err
		}
		res = &UpsertScoresResult{
			Application: detail,
			Scores:      detail.Scores,
			FinalScore:  total,
		}
		return nil
	})
	return res, err
}

func (s *PGStore) loadRankingPoolForTrack(ctx context.Context, q queryRunner, tenantID, trackID uuid.UUID) ([]*ApplicationDetail, error) {
	rows, err := q.Query(ctx, `
		SELECT a.id, a.registration_no, a.status::text, ap.full_name, ap.birth_date, ap.kk_issued_date,
		       a.distance_meters, a.final_score::text, a.submitted_at, a.created_at
		FROM applications a
		LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
		WHERE a.tenant_id = $1 AND a.track_id = $2
		  AND a.status IN ('verified', 'selection_in_progress', 'ranked', 'accepted', 'waitlisted', 'rejected')
	`, tenantID, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ApplicationDetail
	for rows.Next() {
		var id uuid.UUID
		var regNo, st string
		var studentName *string
		var birthDate, kkDate *time.Time
		var dist *int
		var finalScore *string
		var submittedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&id, &regNo, &st, &studentName, &birthDate, &kkDate, &dist, &finalScore, &submittedAt, &createdAt); err != nil {
			return nil, err
		}
		list = append(list, &ApplicationDetail{
			Application: ApplicationSummary{
				ID:             id,
				RegistrationNo: regNo,
				Status:         domain.ApplicationStatus(st),
				StudentName:    studentName,
				DistanceMeters: dist,
				FinalScore:     finalScore,
				SubmittedAt:    submittedAt,
				CreatedAt:      createdAt,
			},
			Profile: &ApplicantProfile{
				BirthDate:    formatOptionalDate(birthDate),
				KKIssuedDate: formatOptionalDate(kkDate),
			},
		})
	}
	return list, rows.Err()
}

func (s *PGStore) SimulateTrackRanking(ctx context.Context, tenantID, trackID uuid.UUID) (*TrackRankingSimulation, error) {
	var out *TrackRankingSimulation
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		cat, err := scanCatalogTrackRow(tenantID, tx.QueryRow(ctx, `
			SELECT `+pgSelectCatalogTrackCols+`
			FROM admission_tracks t
			JOIN admission_periods p ON p.id = t.period_id AND p.tenant_id = t.tenant_id
			JOIN units u ON u.id = p.unit_id AND u.tenant_id = p.tenant_id
			WHERE t.tenant_id = $1 AND t.id = $2
		`, tenantID, trackID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrackNotFound
		}
		if err != nil {
			return err
		}
		pool, err := s.loadRankingPoolForTrack(ctx, tx, tenantID, trackID)
		if err != nil {
			return err
		}
		sim := computeTrackSimulation(cat, pool)
		out = &sim
		return nil
	})
	return out, err
}

func (s *PGStore) PublishPeriodResults(
	ctx context.Context,
	tenantID, periodID uuid.UUID,
	actor ActorContext,
	publishedAt time.Time,
) (*PublishResultsResult, error) {
	tracks, err := s.ListPeriodTracks(ctx, tenantID, periodID)
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return nil, ErrPeriodNotFound
	}

	ts := publishedAt.UTC()
	out := PublishResultsResult{
		PeriodID:    periodID,
		PublishedAt: ts,
		Tracks:      make([]TrackRankingSimulation, 0, len(tracks)),
	}

	err = database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		for _, tr := range tracks {
			pool, err := s.loadRankingPoolForTrack(ctx, tx, tenantID, tr.TrackID)
			if err != nil {
				return err
			}
			sim := computeTrackSimulation(tr, pool)
			for _, cand := range sim.Candidates {
				if !isPreDecisionStatus(cand.CurrentStatus) {
					continue
				}
				if _, err := tx.Exec(ctx, `
					UPDATE applications
					SET status = $3::application_status,
					    rank_position = $4,
					    decided_at = $5,
					    version = version + 1
					WHERE tenant_id = $1 AND id = $2
				`, tenantID, cand.ApplicationID, string(cand.SimulatedDecision), cand.RankPosition, ts); err != nil {
					return err
				}
				fromStr := string(cand.CurrentStatus)
				toStr := string(cand.SimulatedDecision)
				appID := cand.ApplicationID
				if err := insertAuditLogTx(ctx, tx, AuditWrite{
					TenantID: tenantID, Application: &appID, Actor: actor,
					ActionType: "results_published", EntityType: "application", EntityID: appID,
					FromStatus: &fromStr, ToStatus: &toStr,
				}); err != nil {
					return err
				}
				sName := "Calon Murid"
				if cand.StudentName != nil && *cand.StudentName != "" {
					sName = *cand.StudentName
				}
				tplCode := integrations.TemplateSelectionRejected
				switch cand.SimulatedDecision {
				case domain.StatusAccepted:
					tplCode = integrations.TemplateSelectionAccepted
				case domain.StatusWaitlisted:
					tplCode = integrations.TemplateSelectionWaitlisted
				}
				if err := s.enqueueWhatsAppTx(ctx, tx, NotificationWrite{
					TenantID: tenantID, ApplicationID: appID, TemplateCode: tplCode,
					Variables: []string{cand.RegistrationNo, sName, toStr},
				}); err != nil {
					return err
				}
			}
			out.TotalAccepted += sim.AcceptedCount
			out.TotalWaitlisted += sim.WaitlistedCount
			out.TotalRejected += sim.RejectedCount
			out.Tracks = append(out.Tracks, sim)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE admission_periods
			SET results_published_at = COALESCE(results_published_at, $3)
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, periodID, ts); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *PGStore) PromoteWaitlist(ctx context.Context, tenantID, applicationID uuid.UUID, actor ActorContext) (*ApplicationDetail, error) {
	var detail *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var statusStr, regNo string
		var studentName *string
		err := tx.QueryRow(ctx, `
			SELECT a.status::text, a.registration_no, ap.full_name
			FROM applications a
			LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
			WHERE a.tenant_id = $1 AND a.id = $2
			FOR UPDATE OF a
		`, tenantID, applicationID).Scan(&statusStr, &regNo, &studentName)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApplicationNotFound
		}
		if err != nil {
			return err
		}
		if domain.ApplicationStatus(statusStr) != domain.StatusWaitlisted {
			return ErrApplicationLocked
		}

		if _, err := tx.Exec(ctx, `
			UPDATE applications
			SET status = 'accepted'::application_status, decided_at = NOW(), version = version + 1
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, applicationID); err != nil {
			return err
		}
		fromStr := string(domain.StatusWaitlisted)
		toStr := string(domain.StatusAccepted)
		if err := insertAuditLogTx(ctx, tx, AuditWrite{
			TenantID: tenantID, Application: &applicationID, Actor: actor,
			ActionType: "waitlist_promoted", EntityType: "application", EntityID: applicationID,
			FromStatus: &fromStr, ToStatus: &toStr,
		}); err != nil {
			return err
		}
		sName := "Calon Murid"
		if studentName != nil && *studentName != "" {
			sName = *studentName
		}
		if err := s.enqueueWhatsAppTx(ctx, tx, NotificationWrite{
			TenantID: tenantID, ApplicationID: applicationID, TemplateCode: integrations.TemplateWaitlistPromoted,
			Variables: []string{regNo, sName},
		}); err != nil {
			return err
		}
		detail, err = s.loadApplicationDetailTx(ctx, tx, tenantID, applicationID)
		return err
	})
	return detail, err
}

func (s *PGStore) listSchedulesTx(ctx context.Context, q queryRunner, tenantID uuid.UUID, filter AssessmentScheduleFilter) ([]AssessmentSchedule, error) {
	conds := []string{"sc.tenant_id = $1"}
	args := []any{tenantID}
	idx := 2

	if filter.ApplicationID != nil {
		conds = append(conds, fmt.Sprintf("sc.application_id = $%d", idx))
		args = append(args, *filter.ApplicationID)
		idx++
	}
	if filter.UnitID != nil {
		conds = append(conds, fmt.Sprintf("a.unit_id = $%d", idx))
		args = append(args, *filter.UnitID)
		idx++
	} else if len(filter.AllowedUnitIDs) > 0 {
		conds = append(conds, fmt.Sprintf("a.unit_id = ANY($%d)", idx))
		args = append(args, filter.AllowedUnitIDs)
		idx++
	}
	if filter.TrackID != nil {
		conds = append(conds, fmt.Sprintf("a.track_id = $%d", idx))
		args = append(args, *filter.TrackID)
		idx++
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	args = append(args, limit)

	rows, err := q.Query(ctx, `
		SELECT sc.id, sc.application_id, a.registration_no, ap.full_name,
		       a.unit_id, u.name, t.display_name, sc.session_name,
		       sc.room_or_link, sc.scheduled_at, sc.examiner_user_id, ex.full_name,
		       sc.created_at, sc.updated_at
		FROM assessment_schedules sc
		JOIN applications a ON a.id = sc.application_id AND a.tenant_id = sc.tenant_id
		JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
		JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
		LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
		LEFT JOIN users ex ON ex.id = sc.examiner_user_id
		WHERE `+strings.Join(conds, " AND ")+`
		ORDER BY sc.scheduled_at ASC, sc.created_at ASC
		LIMIT $`+fmt.Sprintf("%d", idx), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []AssessmentSchedule{}
	for rows.Next() {
		var sc AssessmentSchedule
		if err := rows.Scan(
			&sc.ID, &sc.ApplicationID, &sc.RegistrationNo, &sc.StudentName,
			&sc.UnitID, &sc.UnitName, &sc.TrackName, &sc.SessionName,
			&sc.RoomOrLink, &sc.ScheduledAt, &sc.ExaminerUserID, &sc.ExaminerName,
			&sc.CreatedAt, &sc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, sc)
	}
	return list, rows.Err()
}

func (s *PGStore) ListAssessmentSchedules(ctx context.Context, tenantID uuid.UUID, filter AssessmentScheduleFilter) ([]AssessmentSchedule, error) {
	var list []AssessmentSchedule
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var err error
		list, err = s.listSchedulesTx(ctx, tx, tenantID, filter)
		return err
	})
	return list, err
}

func (s *PGStore) CreateAssessmentSchedules(ctx context.Context, tenantID uuid.UUID, inputs []AssessmentSchedule, actor ActorContext) ([]AssessmentSchedule, error) {
	out := make([]AssessmentSchedule, 0, len(inputs))
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		wib := time.FixedZone("WIB", 7*3600)
		for _, in := range inputs {
			var regNo string
			var studentName *string
			err := tx.QueryRow(ctx, `
				SELECT a.registration_no, ap.full_name
				FROM applications a
				LEFT JOIN applicant_profiles ap ON ap.application_id = a.id
				WHERE a.tenant_id = $1 AND a.id = $2
			`, tenantID, in.ApplicationID).Scan(&regNo, &studentName)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrApplicationNotFound
			}
			if err != nil {
				return err
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO assessment_schedules (
					id, tenant_id, application_id, session_name, room_or_link, scheduled_at, examiner_user_id
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, in.ID, tenantID, in.ApplicationID, in.SessionName, in.RoomOrLink, in.ScheduledAt, in.ExaminerUserID); err != nil {
				return err
			}

			appID := in.ApplicationID
			reason := fmt.Sprintf("Jadwal %s (%s)", in.SessionName, in.ScheduledAt.In(wib).Format("02 Jan 2006 15:04 WIB"))
			if err := insertAuditLogTx(ctx, tx, AuditWrite{
				TenantID: tenantID, Application: &appID, Actor: actor,
				ActionType: "assessment_scheduled", EntityType: "assessment_schedule", EntityID: in.ID,
				Reason: &reason,
			}); err != nil {
				return err
			}

			sName := "Calon Murid"
			if studentName != nil && *studentName != "" {
				sName = *studentName
			}
			room := "Lihat portal pendaftar"
			if in.RoomOrLink != nil && *in.RoomOrLink != "" {
				room = *in.RoomOrLink
			}
			if err := s.enqueueWhatsAppTx(ctx, tx, NotificationWrite{
				TenantID:      tenantID,
				ApplicationID: in.ApplicationID,
				TemplateCode:  integrations.TemplateAssessmentScheduled,
				Variables: []string{
					regNo, sName, in.SessionName,
					in.ScheduledAt.In(wib).Format("02 Jan 2006 15:04 WIB"), room,
				},
			}); err != nil {
				return err
			}

			items, err := s.listSchedulesTx(ctx, tx, tenantID, AssessmentScheduleFilter{ApplicationID: &appID})
			if err != nil {
				return err
			}
			for _, it := range items {
				if it.ID == in.ID {
					out = append(out, it)
					break
				}
			}
		}
		return nil
	})
	return out, err
}

func (s *PGStore) DeleteAssessmentSchedule(ctx context.Context, tenantID, scheduleID uuid.UUID, actor ActorContext) error {
	return database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var appID uuid.UUID
		err := tx.QueryRow(ctx, `
			DELETE FROM assessment_schedules
			WHERE tenant_id = $1 AND id = $2
			RETURNING application_id
		`, tenantID, scheduleID).Scan(&appID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrScheduleNotFound
		}
		if err != nil {
			return err
		}
		return insertAuditLogTx(ctx, tx, AuditWrite{
			TenantID: tenantID, Application: &appID, Actor: actor,
			ActionType: "assessment_schedule_deleted", EntityType: "assessment_schedule", EntityID: scheduleID,
		})
	})
}

