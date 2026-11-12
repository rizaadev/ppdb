package admissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pbdb/platform/internal/database"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/worker"
	"github.com/riverqueue/river"
)

// PGStore persists applicant registrations, documents, and verifier workflows in PostgreSQL.
type PGStore struct {
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
}

// NewPGStore initializes a PostgreSQL-backed admissions store with optional River transactional enqueueing.
func NewPGStore(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *PGStore {
	return &PGStore{pool: pool, riverClient: riverClient}
}

const pgSelectCatalogTrackCols = `
	u.id, u.name, u.latitude, u.longitude,
	p.id, p.name, p.academic_year,
	t.id, t.code, t.display_name, t.selection_mode::text, t.quota,
	t.spillover_target_track_id, t.registration_fee_idr, t.re_enrollment_fee_idr,
	t.min_age_months, t.max_age_months, t.age_reference_date,
	t.requires_coordinates, t.requires_achievements,
	t.scoring_weights, t.custom_form_schema
`

func scanCatalogTrackRow(tenantID uuid.UUID, row interface{ Scan(dest ...any) error }) (CatalogTrack, error) {
	var cat CatalogTrack
	var ageRefDate *time.Time
	var weightsRaw, schemaRaw []byte
	cat.TenantID = tenantID
	err := row.Scan(
		&cat.UnitID, &cat.UnitName, &cat.UnitLat, &cat.UnitLon,
		&cat.PeriodID, &cat.PeriodName, &cat.AcademicYear,
		&cat.TrackID, &cat.TrackCode, &cat.TrackName, &cat.SelectionMode, &cat.Quota,
		&cat.SpilloverTargetTrack, &cat.RegistrationFeeIDR, &cat.ReEnrollmentFeeIDR,
		&cat.MinAgeMonths, &cat.MaxAgeMonths, &ageRefDate,
		&cat.RequiresCoordinates, &cat.RequiresAchievements,
		&weightsRaw, &schemaRaw,
	)
	if err != nil {
		return cat, err
	}
	cat.AgeReferenceDate = formatOptionalDate(ageRefDate)
	cat.ScoringWeights = map[string]float64{}
	if len(weightsRaw) > 0 {
		_ = json.Unmarshal(weightsRaw, &cat.ScoringWeights)
	}
	cat.CustomFormSchema = []domain.CustomFormField{}
	if len(schemaRaw) > 0 {
		_ = json.Unmarshal(schemaRaw, &cat.CustomFormSchema)
	}
	return cat, nil
}

func (s *PGStore) GetCatalogTrack(ctx context.Context, tenantID, unitID, periodID, trackID uuid.UUID) (*CatalogTrack, error) {
	var out *CatalogTrack
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		cat, err := scanCatalogTrackRow(tenantID, tx.QueryRow(ctx, `
			SELECT `+pgSelectCatalogTrackCols+`
			FROM admission_tracks t
			JOIN admission_periods p ON p.id = t.period_id AND p.tenant_id = t.tenant_id
			JOIN units u ON u.id = p.unit_id AND u.tenant_id = p.tenant_id
			WHERE t.tenant_id = $1
			  AND u.id = $2
			  AND p.id = $3
			  AND t.id = $4
			  AND u.is_active = TRUE
			  AND p.is_active = TRUE
			  AND p.is_published = TRUE
			  AND t.is_active = TRUE
		`, tenantID, unitID, periodID, trackID))
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

type queryRunner interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *PGStore) loadRequirements(ctx context.Context, q queryRunner, tenantID, trackID uuid.UUID) ([]TrackRequirement, error) {
	rows, err := q.Query(ctx, `
		SELECT id, track_id, doc_code, label, description, is_required, accepted_mimes, max_size_kb, sort_order
		FROM track_requirements
		WHERE tenant_id = $1 AND track_id = $2
		ORDER BY sort_order ASC, created_at ASC
	`, tenantID, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reqs := []TrackRequirement{}
	for rows.Next() {
		var r TrackRequirement
		if err := rows.Scan(&r.ID, &r.TrackID, &r.DocCode, &r.Label, &r.Description, &r.IsRequired, &r.AcceptedMimes, &r.MaxSizeKB, &r.SortOrder); err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	return reqs, rows.Err()
}

func (s *PGStore) ListApplicationsByGuardian(ctx context.Context, tenantID, guardianUserID uuid.UUID) ([]ApplicationSummary, error) {
	var list []ApplicationSummary
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.registration_no, a.status::text, a.unit_id, u.name,
			       a.period_id, p.name, p.academic_year, a.track_id, t.display_name,
			       pr.full_name, pr.nisn, a.distance_meters, a.final_score::text, a.rank_position,
			       a.version, a.submitted_at, a.verified_at, a.decided_at, a.created_at
			FROM applications a
			JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
			JOIN admission_periods p ON p.id = a.period_id AND p.tenant_id = a.tenant_id
			JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
			LEFT JOIN applicant_profiles pr ON pr.application_id = a.id
			WHERE a.tenant_id = $1 AND a.guardian_user_id = $2
			ORDER BY a.created_at DESC, a.id DESC
		`, tenantID, guardianUserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		list, err = scanApplicationSummaries(rows)
		return err
	})
	return list, err
}

func scanApplicationSummaries(rows pgx.Rows) ([]ApplicationSummary, error) {
	list := []ApplicationSummary{}
	for rows.Next() {
		var item ApplicationSummary
		var statusStr string
		if err := rows.Scan(
			&item.ID, &item.RegistrationNo, &statusStr, &item.UnitID, &item.UnitName,
			&item.PeriodID, &item.PeriodName, &item.AcademicYear, &item.TrackID, &item.TrackName,
			&item.StudentName, &item.NISN, &item.DistanceMeters, &item.FinalScore, &item.RankPosition,
			&item.Version, &item.SubmittedAt, &item.VerifiedAt, &item.DecidedAt, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.Status = domain.ApplicationStatus(statusStr)
		list = append(list, item)
	}
	return list, rows.Err()
}

func (s *PGStore) CreateApplication(
	ctx context.Context,
	appID uuid.UUID,
	tenantID uuid.UUID,
	catalog CatalogTrack,
	guardianUserID uuid.UUID,
	initialStatus domain.ApplicationStatus,
	studentName string,
	extraAnswers map[string]any,
	actor ActorContext,
) (*ApplicationDetail, error) {
	var detail *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var seq int
		if err := tx.QueryRow(ctx, `
			UPDATE admission_periods
			SET registration_seq = registration_seq + 1
			WHERE tenant_id = $1 AND id = $2
			RETURNING registration_seq
		`, tenantID, catalog.PeriodID).Scan(&seq); err != nil {
			return err
		}

		regNo := formatRegistrationNo(catalog.TrackCode, catalog.AcademicYear, seq)
		extraJSON, err := marshalJSONMap(extraAnswers)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO applications (
				id, tenant_id, unit_id, period_id, track_id, guardian_user_id,
				registration_no, status, extra_answers
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::application_status, $9::jsonb)
		`, appID, tenantID, catalog.UnitID, catalog.PeriodID, catalog.TrackID, guardianUserID, regNo, string(initialStatus), extraJSON); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO applicant_profiles (application_id, tenant_id, full_name, citizenship)
			VALUES ($1, $2, $3, 'WNI')
		`, appID, tenantID, studentName); err != nil {
			return err
		}

		toStatusStr := string(initialStatus)
		if err := insertAuditLogTx(ctx, tx, AuditWrite{
			ID:          uuid.Must(uuid.NewV7()),
			TenantID:    tenantID,
			Application: &appID,
			Actor:       actor,
			ActionType:  "application_created",
			EntityType:  "application",
			EntityID:    appID,
			ToStatus:    &toStatusStr,
		}); err != nil {
			return err
		}

		detail, err = s.loadApplicationDetailTx(ctx, tx, tenantID, appID)
		return err
	})
	return detail, err
}

func (s *PGStore) GetApplicationDetail(ctx context.Context, tenantID, applicationID uuid.UUID) (*ApplicationDetail, error) {
	var detail *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var err error
		detail, err = s.loadApplicationDetailTx(ctx, tx, tenantID, applicationID)
		return err
	})
	return detail, err
}

func (s *PGStore) loadApplicationDetailTx(ctx context.Context, q queryRunner, tenantID, applicationID uuid.UUID) (*ApplicationDetail, error) {
	var detail ApplicationDetail
	var statusStr string
	var extraRaw, weightsRaw, schemaRaw []byte

	err := q.QueryRow(ctx, `
		SELECT a.id, a.registration_no, a.status::text, a.unit_id, u.name, u.latitude, u.longitude,
		       a.period_id, p.name, p.academic_year, p.results_published_at,
		       a.track_id, t.display_name, t.registration_fee_idr, t.re_enrollment_fee_idr,
		       t.requires_coordinates, t.requires_achievements, t.scoring_weights, t.custom_form_schema,
		       a.guardian_user_id, a.claimed_by_verifier_id, a.claimed_at,
		       a.distance_meters, a.final_score::text, a.rank_position, a.extra_answers,
		       a.version, a.submitted_at, a.verified_at, a.decided_at, a.created_at
		FROM applications a
		JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
		JOIN admission_periods p ON p.id = a.period_id AND p.tenant_id = a.tenant_id
		JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
		WHERE a.tenant_id = $1 AND a.id = $2
	`, tenantID, applicationID).Scan(
		&detail.Application.ID, &detail.Application.RegistrationNo, &statusStr,
		&detail.Application.UnitID, &detail.Application.UnitName, &detail.UnitLatitude, &detail.UnitLongitude,
		&detail.Application.PeriodID, &detail.Application.PeriodName, &detail.Application.AcademicYear, &detail.Application.ResultsPublishedAt,
		&detail.Application.TrackID, &detail.Application.TrackName, &detail.RegistrationFeeIDR, &detail.ReEnrollmentFeeIDR,
		&detail.RequiresCoordinates, &detail.RequiresAchievements, &weightsRaw, &schemaRaw,
		&detail.GuardianUserID, &detail.ClaimedByVerifierID, &detail.ClaimedAt,
		&detail.Application.DistanceMeters, &detail.Application.FinalScore, &detail.Application.RankPosition, &extraRaw,
		&detail.Application.Version, &detail.Application.SubmittedAt, &detail.Application.VerifiedAt, &detail.Application.DecidedAt, &detail.Application.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	detail.Application.Status = domain.ApplicationStatus(statusStr)
	detail.ExtraAnswers = map[string]any{}
	if len(extraRaw) > 0 {
		_ = json.Unmarshal(extraRaw, &detail.ExtraAnswers)
	}
	detail.TrackScoringWeights = map[string]float64{}
	if len(weightsRaw) > 0 {
		_ = json.Unmarshal(weightsRaw, &detail.TrackScoringWeights)
	}
	detail.CustomFormSchema = []domain.CustomFormField{}
	if len(schemaRaw) > 0 {
		_ = json.Unmarshal(schemaRaw, &detail.CustomFormSchema)
	}

	profile, err := s.loadProfile(ctx, q, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	detail.Profile = profile
	if profile != nil {
		name := profile.FullName
		detail.Application.StudentName = &name
		detail.Application.NISN = profile.NISN
	}

	if detail.Guardians, err = s.loadGuardians(ctx, q, tenantID, applicationID); err != nil {
		return nil, err
	}
	if detail.Achievements, err = s.loadAchievements(ctx, q, tenantID, applicationID); err != nil {
		return nil, err
	}
	if detail.Requirements, err = s.loadRequirements(ctx, q, tenantID, detail.Application.TrackID); err != nil {
		return nil, err
	}
	if detail.Documents, err = s.loadDocuments(ctx, q, tenantID, applicationID); err != nil {
		return nil, err
	}
	if detail.Payments, err = s.loadPaymentsByApp(ctx, q, tenantID, applicationID); err != nil {
		return nil, err
	}
	if detail.Scores, err = s.loadScoresByApp(ctx, q, tenantID, applicationID); err != nil {
		return nil, err
	}
	if detail.Schedules, err = s.listSchedulesTx(ctx, q, tenantID, AssessmentScheduleFilter{ApplicationID: &applicationID}); err != nil {
		return nil, err
	}
	if detail.AuditLogs, err = s.loadAuditLogs(ctx, q, tenantID, applicationID); err != nil {
		return nil, err
	}

	return &detail, nil
}

func (s *PGStore) loadProfile(ctx context.Context, q queryRunner, tenantID, applicationID uuid.UUID) (*ApplicantProfile, error) {
	var p ApplicantProfile
	var kkDate, birthDate *time.Time
	var gender *string

	err := q.QueryRow(ctx, `
		SELECT full_name, nik, kk_number, kk_issued_date, nisn, birth_place, birth_date,
		       gender::text, religion, citizenship, child_order, siblings_count, special_needs_code,
		       address_street, rt, rw, hamlet, village_code, village_name,
		       district_code, district_name, regency_code, regency_name,
		       province_code, province_name, postal_code, latitude, longitude,
		       prev_school_name, prev_school_npsn, graduation_year, ijazah_number
		FROM applicant_profiles
		WHERE tenant_id = $1 AND application_id = $2
	`, tenantID, applicationID).Scan(
		&p.FullName, &p.NIK, &p.KKNumber, &kkDate, &p.NISN, &p.BirthPlace, &birthDate,
		&gender, &p.Religion, &p.Citizenship, &p.ChildOrder, &p.SiblingsCount, &p.SpecialNeedsCode,
		&p.AddressStreet, &p.RT, &p.RW, &p.Hamlet, &p.VillageCode, &p.VillageName,
		&p.DistrictCode, &p.DistrictName, &p.RegencyCode, &p.RegencyName,
		&p.ProvinceCode, &p.ProvinceName, &p.PostalCode, &p.Latitude, &p.Longitude,
		&p.PrevSchoolName, &p.PrevSchoolNPSN, &p.GraduationYear, &p.IjazahNumber,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.KKIssuedDate = formatOptionalDate(kkDate)
	p.BirthDate = formatOptionalDate(birthDate)
	p.Gender = gender
	return &p, nil
}

func (s *PGStore) loadGuardians(ctx context.Context, q queryRunner, tenantID, applicationID uuid.UUID) ([]Guardian, error) {
	rows, err := q.Query(ctx, `
		SELECT id, relation::text, full_name, nik, birth_year, education_level,
		       occupation, income_bracket, phone_whatsapp, is_primary_contact
		FROM applicant_guardians
		WHERE tenant_id = $1 AND application_id = $2
		ORDER BY relation ASC
	`, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Guardian{}
	for rows.Next() {
		var g Guardian
		if err := rows.Scan(&g.ID, &g.Relation, &g.FullName, &g.NIK, &g.BirthYear, &g.EducationLevel, &g.Occupation, &g.IncomeBracket, &g.PhoneWhatsApp, &g.IsPrimaryContact); err != nil {
			return nil, err
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func (s *PGStore) loadAchievements(ctx context.Context, q queryRunner, tenantID, applicationID uuid.UUID) ([]Achievement, error) {
	rows, err := q.Query(ctx, `
		SELECT id, category, title, level, rank_achieved, achievement_year, organizer,
		       raw_points::double precision, verified_points::double precision, verified_by_user_id
		FROM applicant_achievements
		WHERE tenant_id = $1 AND application_id = $2
		ORDER BY created_at ASC
	`, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Achievement{}
	for rows.Next() {
		var a Achievement
		if err := rows.Scan(&a.ID, &a.Category, &a.Title, &a.Level, &a.RankAchieved, &a.AchievementYear, &a.Organizer, &a.RawPoints, &a.VerifiedPoints, &a.VerifiedByUserID); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *PGStore) loadDocuments(ctx context.Context, q queryRunner, tenantID, applicationID uuid.UUID) ([]Document, error) {
	rows, err := q.Query(ctx, `
		SELECT d.id, d.requirement_id, r.doc_code, r.label, r.is_required,
		       d.storage_key, d.original_filename, d.mime_type, d.size_bytes,
		       d.status::text, d.reviewer_user_id, d.review_note, d.reviewed_at, d.version
		FROM applicant_documents d
		JOIN track_requirements r ON r.id = d.requirement_id AND r.tenant_id = d.tenant_id
		WHERE d.tenant_id = $1 AND d.application_id = $2
		ORDER BY r.sort_order ASC, d.created_at ASC
	`, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Document{}
	for rows.Next() {
		var d Document
		var statusStr string
		if err := rows.Scan(
			&d.ID, &d.RequirementID, &d.DocCode, &d.RequirementLabel, &d.IsRequired,
			&d.StorageKey, &d.OriginalFilename, &d.MimeType, &d.SizeBytes,
			&statusStr, &d.ReviewerUserID, &d.ReviewNote, &d.ReviewedAt, &d.Version,
		); err != nil {
			return nil, err
		}
		d.Status = domain.DocumentReviewStatus(statusStr)
		list = append(list, d)
	}
	return list, rows.Err()
}

func (s *PGStore) loadAuditLogs(ctx context.Context, q queryRunner, tenantID, applicationID uuid.UUID) ([]AuditEntry, error) {
	rows, err := q.Query(ctx, `
		SELECT l.id, l.actor_user_id, u.full_name, l.actor_role, l.action_type,
		       l.entity_type, l.entity_id, l.from_status, l.to_status, l.reason, l.created_at
		FROM status_audit_logs l
		LEFT JOIN users u ON u.id = l.actor_user_id
		WHERE l.tenant_id = $1 AND l.application_id = $2
		ORDER BY l.created_at DESC
		LIMIT 50
	`, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []AuditEntry{}
	for rows.Next() {
		var a AuditEntry
		if err := rows.Scan(&a.ID, &a.ActorUserID, &a.ActorName, &a.ActorRole, &a.ActionType, &a.EntityType, &a.EntityID, &a.FromStatus, &a.ToStatus, &a.Reason, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *PGStore) SaveApplicantProfile(
	ctx context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion *int,
	profile ApplicantProfile,
	extraAnswers map[string]any,
	distanceMeters *int,
) (*ApplicationDetail, error) {
	var detail *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var currentVersion int
		err := tx.QueryRow(ctx, `
			SELECT version FROM applications
			WHERE tenant_id = $1 AND id = $2
			FOR UPDATE
		`, tenantID, applicationID).Scan(&currentVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApplicationNotFound
		}
		if err != nil {
			return err
		}
		if expectedVersion != nil && *expectedVersion != currentVersion {
			return ErrVersionConflict
		}

		citizenship := strings.TrimSpace(profile.Citizenship)
		if citizenship == "" {
			citizenship = "WNI"
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO applicant_profiles (
				application_id, tenant_id, full_name, nik, kk_number, kk_issued_date,
				nisn, birth_place, birth_date, gender, religion, citizenship,
				child_order, siblings_count, special_needs_code,
				address_street, rt, rw, hamlet, village_code, village_name,
				district_code, district_name, regency_code, regency_name,
				province_code, province_name, postal_code, latitude, longitude,
				prev_school_name, prev_school_npsn, graduation_year, ijazah_number
			) VALUES (
				$1, $2, $3, $4, $5, NULLIF($6::text, '')::date,
				$7, $8, NULLIF($9::text, '')::date, NULLIF($10::text, '')::gender_type, $11, $12,
				$13, $14, $15,
				$16, $17, $18, $19, $20, $21,
				$22, $23, $24, $25,
				$26, $27, $28, $29, $30,
				$31, $32, $33, $34
			)
			ON CONFLICT (application_id) DO UPDATE SET
				full_name = EXCLUDED.full_name,
				nik = EXCLUDED.nik,
				kk_number = EXCLUDED.kk_number,
				kk_issued_date = EXCLUDED.kk_issued_date,
				nisn = EXCLUDED.nisn,
				birth_place = EXCLUDED.birth_place,
				birth_date = EXCLUDED.birth_date,
				gender = EXCLUDED.gender,
				religion = EXCLUDED.religion,
				citizenship = EXCLUDED.citizenship,
				child_order = EXCLUDED.child_order,
				siblings_count = EXCLUDED.siblings_count,
				special_needs_code = EXCLUDED.special_needs_code,
				address_street = EXCLUDED.address_street,
				rt = EXCLUDED.rt,
				rw = EXCLUDED.rw,
				hamlet = EXCLUDED.hamlet,
				village_code = EXCLUDED.village_code,
				village_name = EXCLUDED.village_name,
				district_code = EXCLUDED.district_code,
				district_name = EXCLUDED.district_name,
				regency_code = EXCLUDED.regency_code,
				regency_name = EXCLUDED.regency_name,
				province_code = EXCLUDED.province_code,
				province_name = EXCLUDED.province_name,
				postal_code = EXCLUDED.postal_code,
				latitude = EXCLUDED.latitude,
				longitude = EXCLUDED.longitude,
				prev_school_name = EXCLUDED.prev_school_name,
				prev_school_npsn = EXCLUDED.prev_school_npsn,
				graduation_year = EXCLUDED.graduation_year,
				ijazah_number = EXCLUDED.ijazah_number
		`,
			applicationID, tenantID, profile.FullName, profile.NIK, profile.KKNumber, profile.KKIssuedDate,
			profile.NISN, profile.BirthPlace, profile.BirthDate, profile.Gender, profile.Religion, citizenship,
			profile.ChildOrder, profile.SiblingsCount, profile.SpecialNeedsCode,
			profile.AddressStreet, profile.RT, profile.RW, profile.Hamlet, profile.VillageCode, profile.VillageName,
			profile.DistrictCode, profile.DistrictName, profile.RegencyCode, profile.RegencyName,
			profile.ProvinceCode, profile.ProvinceName, profile.PostalCode, profile.Latitude, profile.Longitude,
			profile.PrevSchoolName, profile.PrevSchoolNPSN, profile.GraduationYear, profile.IjazahNumber,
		); err != nil {
			return err
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
			SET distance_meters = $3,
			    extra_answers = COALESCE($4::jsonb, extra_answers),
			    version = version + 1
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, applicationID, distanceMeters, extraParam); err != nil {
			return err
		}

		detail, err = s.loadApplicationDetailTx(ctx, tx, tenantID, applicationID)
		return err
	})
	return detail, err
}

func (s *PGStore) ReplaceGuardians(ctx context.Context, tenantID, applicationID uuid.UUID, guardians []Guardian) ([]Guardian, error) {
	var out []Guardian
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		if _, err := lockApplicationForChildReplace(ctx, tx, tenantID, applicationID); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `DELETE FROM applicant_guardians WHERE tenant_id = $1 AND application_id = $2`, tenantID, applicationID); err != nil {
			return err
		}

		for _, g := range guardians {
			if _, err := tx.Exec(ctx, `
				INSERT INTO applicant_guardians (
					id, tenant_id, application_id, relation, full_name, nik, birth_year,
					education_level, occupation, income_bracket, phone_whatsapp, is_primary_contact
				) VALUES ($1, $2, $3, $4::guardian_relation, $5, $6, $7, $8, $9, $10, $11, $12)
			`, g.ID, tenantID, applicationID, g.Relation, g.FullName, g.NIK, g.BirthYear, g.EducationLevel, g.Occupation, g.IncomeBracket, g.PhoneWhatsApp, g.IsPrimaryContact); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE applications SET version = version + 1 WHERE tenant_id = $1 AND id = $2`, tenantID, applicationID); err != nil {
			return err
		}
		var err error
		out, err = s.loadGuardians(ctx, tx, tenantID, applicationID)
		return err
	})
	return out, err
}

func (s *PGStore) ReplaceAchievements(ctx context.Context, tenantID, applicationID uuid.UUID, achievements []Achievement) ([]Achievement, error) {
	var out []Achievement
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		if _, err := lockApplicationForChildReplace(ctx, tx, tenantID, applicationID); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `DELETE FROM applicant_achievements WHERE tenant_id = $1 AND application_id = $2`, tenantID, applicationID); err != nil {
			return err
		}

		for _, a := range achievements {
			if _, err := tx.Exec(ctx, `
				INSERT INTO applicant_achievements (
					id, tenant_id, application_id, category, title, level, rank_achieved,
					achievement_year, organizer, raw_points
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`, a.ID, tenantID, applicationID, a.Category, a.Title, a.Level, a.RankAchieved, a.AchievementYear, a.Organizer, a.RawPoints); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE applications SET version = version + 1 WHERE tenant_id = $1 AND id = $2`, tenantID, applicationID); err != nil {
			return err
		}
		var err error
		out, err = s.loadAchievements(ctx, tx, tenantID, applicationID)
		return err
	})
	return out, err
}

func lockApplicationForChildReplace(ctx context.Context, tx pgx.Tx, tenantID, applicationID uuid.UUID) (int, error) {
	var version int
	err := tx.QueryRow(ctx, `
		SELECT version FROM applications WHERE tenant_id = $1 AND id = $2 FOR UPDATE
	`, tenantID, applicationID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrApplicationNotFound
	}
	return version, err
}

func (s *PGStore) UpsertDocument(
	ctx context.Context,
	tenantID, applicationID, docID uuid.UUID,
	input ConfirmDocumentInput,
) (*Document, error) {
	var out *Document
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var doc Document
		var statusStr string
		err := tx.QueryRow(ctx, `
			WITH upserted AS (
				INSERT INTO applicant_documents (
					id, tenant_id, application_id, requirement_id, storage_key,
					original_filename, mime_type, size_bytes, status
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
				ON CONFLICT (application_id, requirement_id) DO UPDATE SET
					storage_key = EXCLUDED.storage_key,
					original_filename = EXCLUDED.original_filename,
					mime_type = EXCLUDED.mime_type,
					size_bytes = EXCLUDED.size_bytes,
					status = 'pending',
					reviewer_user_id = NULL,
					review_note = NULL,
					reviewed_at = NULL,
					version = applicant_documents.version + 1
				RETURNING id, requirement_id, storage_key, original_filename, mime_type, size_bytes,
				          status::text, reviewer_user_id, review_note, reviewed_at, version
			)
			SELECT u.id, u.requirement_id, r.doc_code, r.label, r.is_required,
			       u.storage_key, u.original_filename, u.mime_type, u.size_bytes,
			       u.status, u.reviewer_user_id, u.review_note, u.reviewed_at, u.version
			FROM upserted u
			JOIN track_requirements r ON r.id = u.requirement_id AND r.tenant_id = $2
		`, docID, tenantID, applicationID, input.RequirementID, input.StorageKey, input.OriginalFilename, input.MimeType, input.SizeBytes).Scan(
			&doc.ID, &doc.RequirementID, &doc.DocCode, &doc.RequirementLabel, &doc.IsRequired,
			&doc.StorageKey, &doc.OriginalFilename, &doc.MimeType, &doc.SizeBytes,
			&statusStr, &doc.ReviewerUserID, &doc.ReviewNote, &doc.ReviewedAt, &doc.Version,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRequirementNotFound
		}
		if err != nil {
			return err
		}
		doc.Status = domain.DocumentReviewStatus(statusStr)
		out = &doc
		return nil
	})
	return out, err
}

func (s *PGStore) GetDocumentByID(ctx context.Context, tenantID, applicationID, documentID uuid.UUID) (*Document, error) {
	var out *Document
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var doc Document
		var statusStr string
		err := tx.QueryRow(ctx, `
			SELECT d.id, d.requirement_id, r.doc_code, r.label, r.is_required,
			       d.storage_key, d.original_filename, d.mime_type, d.size_bytes,
			       d.status::text, d.reviewer_user_id, d.review_note, d.reviewed_at, d.version
			FROM applicant_documents d
			JOIN track_requirements r ON r.id = d.requirement_id AND r.tenant_id = d.tenant_id
			WHERE d.tenant_id = $1 AND d.application_id = $2 AND d.id = $3
		`, tenantID, applicationID, documentID).Scan(
			&doc.ID, &doc.RequirementID, &doc.DocCode, &doc.RequirementLabel, &doc.IsRequired,
			&doc.StorageKey, &doc.OriginalFilename, &doc.MimeType, &doc.SizeBytes,
			&statusStr, &doc.ReviewerUserID, &doc.ReviewNote, &doc.ReviewedAt, &doc.Version,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		doc.Status = domain.DocumentReviewStatus(statusStr)
		out = &doc
		return nil
	})
	return out, err
}

func (s *PGStore) SubmitApplication(
	ctx context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion int,
	fromStatus domain.ApplicationStatus,
	audit AuditWrite,
	notify NotificationWrite,
) (*ApplicationDetail, error) {
	var detail *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var currentVersion int
		var currentStatusStr string
		err := tx.QueryRow(ctx, `
			SELECT version, status::text FROM applications
			WHERE tenant_id = $1 AND id = $2
			FOR UPDATE
		`, tenantID, applicationID).Scan(&currentVersion, &currentStatusStr)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApplicationNotFound
		}
		if err != nil {
			return err
		}
		if currentVersion != expectedVersion {
			return ErrVersionConflict
		}
		if domain.ApplicationStatus(currentStatusStr) != fromStatus {
			return ErrApplicationLocked
		}

		if _, err := tx.Exec(ctx, `
			UPDATE applications
			SET status = 'submitted',
			    submitted_at = COALESCE(submitted_at, NOW()),
			    version = version + 1
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, applicationID); err != nil {
			return err
		}

		if err := insertAuditLogTx(ctx, tx, audit); err != nil {
			return err
		}
		if err := s.enqueueWhatsAppTx(ctx, tx, notify); err != nil {
			return err
		}

		detail, err = s.loadApplicationDetailTx(ctx, tx, tenantID, applicationID)
		return err
	})
	return detail, err
}

func (s *PGStore) ListApplicationsForStaff(ctx context.Context, tenantID uuid.UUID, filter StaffListFilter) ([]ApplicationSummary, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	var statusParam *string
	if filter.Status != nil {
		str := string(*filter.Status)
		statusParam = &str
	}
	var searchParam *string
	if trimmed := strings.TrimSpace(filter.Search); trimmed != "" {
		searchParam = &trimmed
	}
	var allowedUnits []uuid.UUID
	if len(filter.AllowedUnitIDs) > 0 {
		allowedUnits = filter.AllowedUnitIDs
	}

	var list []ApplicationSummary
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.registration_no, a.status::text, a.unit_id, u.name,
			       a.period_id, p.name, p.academic_year, a.track_id, t.display_name,
			       pr.full_name, pr.nisn, a.distance_meters, a.final_score::text, a.rank_position,
			       a.version, a.submitted_at, a.verified_at, a.decided_at, a.created_at
			FROM applications a
			JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
			JOIN admission_periods p ON p.id = a.period_id AND p.tenant_id = a.tenant_id
			JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
			LEFT JOIN applicant_profiles pr ON pr.application_id = a.id
			WHERE a.tenant_id = $1
			  AND ($2::uuid IS NULL OR a.unit_id = $2::uuid)
			  AND ($3::uuid[] IS NULL OR a.unit_id = ANY($3::uuid[]))
			  AND ($4::uuid IS NULL OR a.period_id = $4::uuid)
			  AND ($5::uuid IS NULL OR a.track_id = $5::uuid)
			  AND ($6::text IS NULL OR a.status::text = $6::text)
			  AND (
			      $7::text IS NULL
			      OR a.registration_no ILIKE '%' || $7::text || '%'
			      OR pr.full_name ILIKE '%' || $7::text || '%'
			      OR pr.nisn = $7::text
			  )
			  AND (
			      $8::timestamptz IS NULL OR $9::uuid IS NULL
			      OR (a.created_at, a.id) < ($8::timestamptz, $9::uuid)
			  )
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $10
		`,
			tenantID, filter.UnitID, allowedUnits, filter.PeriodID, filter.TrackID,
			statusParam, searchParam, filter.CursorCreatedAt, filter.CursorID, limit,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		list, err = scanApplicationSummaries(rows)
		return err
	})
	return list, err
}

func (s *PGStore) ClaimNextForVerification(
	ctx context.Context,
	tenantID, unitID, verifierUserID uuid.UUID,
	audit AuditWrite,
) (*ApplicationDetail, error) {
	var detail *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var appID uuid.UUID
		err := tx.QueryRow(ctx, `
			UPDATE applications
			SET status = 'under_verification',
			    claimed_by_verifier_id = $3,
			    claimed_at = NOW(),
			    version = version + 1
			WHERE id = (
				SELECT a.id
				FROM applications a
				WHERE a.tenant_id = $1 AND a.unit_id = $2 AND a.status = 'submitted'
				ORDER BY a.submitted_at ASC NULLS FIRST, a.created_at ASC
				FOR UPDATE SKIP LOCKED
				LIMIT 1
			)
			AND tenant_id = $1
			RETURNING id
		`, tenantID, unitID, verifierUserID).Scan(&appID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		audit.EntityID = appID
		audit.Application = &appID
		if err := insertAuditLogTx(ctx, tx, audit); err != nil {
			return err
		}
		detail, err = s.loadApplicationDetailTx(ctx, tx, tenantID, appID)
		return err
	})
	return detail, err
}

func (s *PGStore) ReviewDocument(
	ctx context.Context,
	tenantID, applicationID, documentID, reviewerUserID uuid.UUID,
	status domain.DocumentReviewStatus,
	note *string,
	audit AuditWrite,
) (*Document, error) {
	var out *Document
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var statusStr string
		err := tx.QueryRow(ctx, `
			SELECT status::text FROM applications
			WHERE tenant_id = $1 AND id = $2
			FOR UPDATE
		`, tenantID, applicationID).Scan(&statusStr)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApplicationNotFound
		}
		if err != nil {
			return err
		}

		appStatus := domain.ApplicationStatus(statusStr)
		if appStatus != domain.StatusSubmitted && appStatus != domain.StatusUnderVerification {
			return ErrApplicationLocked
		}
		if appStatus == domain.StatusSubmitted {
			if _, err := tx.Exec(ctx, `
				UPDATE applications
				SET status = 'under_verification',
				    claimed_by_verifier_id = COALESCE(claimed_by_verifier_id, $3),
				    claimed_at = COALESCE(claimed_at, NOW()),
				    version = version + 1
				WHERE tenant_id = $1 AND id = $2
			`, tenantID, applicationID, reviewerUserID); err != nil {
				return err
			}
		}

		var doc Document
		var docStatus string
		err = tx.QueryRow(ctx, `
			WITH updated AS (
				UPDATE applicant_documents
				SET status = $4::document_review_status,
				    reviewer_user_id = $5,
				    review_note = $6,
				    reviewed_at = NOW()
				WHERE tenant_id = $1 AND application_id = $2 AND id = $3
				RETURNING id, requirement_id, storage_key, original_filename, mime_type, size_bytes,
				          status::text, reviewer_user_id, review_note, reviewed_at, version
			)
			SELECT u.id, u.requirement_id, r.doc_code, r.label, r.is_required,
			       u.storage_key, u.original_filename, u.mime_type, u.size_bytes,
			       u.status, u.reviewer_user_id, u.review_note, u.reviewed_at, u.version
			FROM updated u
			JOIN track_requirements r ON r.id = u.requirement_id AND r.tenant_id = $1
		`, tenantID, applicationID, documentID, string(status), reviewerUserID, note).Scan(
			&doc.ID, &doc.RequirementID, &doc.DocCode, &doc.RequirementLabel, &doc.IsRequired,
			&doc.StorageKey, &doc.OriginalFilename, &doc.MimeType, &doc.SizeBytes,
			&docStatus, &doc.ReviewerUserID, &doc.ReviewNote, &doc.ReviewedAt, &doc.Version,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDocumentNotFound
		}
		if err != nil {
			return err
		}
		doc.Status = domain.DocumentReviewStatus(docStatus)

		if err := insertAuditLogTx(ctx, tx, audit); err != nil {
			return err
		}
		out = &doc
		return nil
	})
	return out, err
}

func (s *PGStore) CompleteVerification(
	ctx context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion int,
	fromStatus, toStatus domain.ApplicationStatus,
	reviewedAt time.Time,
	audit AuditWrite,
	notify NotificationWrite,
) (*ApplicationDetail, error) {
	var detail *ApplicationDetail
	err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var curVersion int
		var curStatusStr string
		err := tx.QueryRow(ctx, `
			SELECT version, status::text FROM applications
			WHERE tenant_id = $1 AND id = $2
			FOR UPDATE
		`, tenantID, applicationID).Scan(&curVersion, &curStatusStr)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrApplicationNotFound
		}
		if err != nil {
			return err
		}
		if curVersion != expectedVersion {
			return ErrVersionConflict
		}
		if domain.ApplicationStatus(curStatusStr) != fromStatus {
			return ErrApplicationLocked
		}

		var verifiedAtParam *time.Time
		if toStatus == domain.StatusVerified {
			ts := reviewedAt.UTC()
			verifiedAtParam = &ts
		}

		if _, err := tx.Exec(ctx, `
			UPDATE applications
			SET status = $3::application_status,
			    verified_at = COALESCE($4, verified_at),
			    version = version + 1
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, applicationID, string(toStatus), verifiedAtParam); err != nil {
			return err
		}

		if err := insertAuditLogTx(ctx, tx, audit); err != nil {
			return err
		}
		if err := s.enqueueWhatsAppTx(ctx, tx, notify); err != nil {
			return err
		}

		detail, err = s.loadApplicationDetailTx(ctx, tx, tenantID, applicationID)
		return err
	})
	return detail, err
}

func insertAuditLogTx(ctx context.Context, tx pgx.Tx, entry AuditWrite) error {
	id := entry.ID
	if id == uuid.Nil {
		id = uuid.Must(uuid.NewV7())
	}
	var payloadJSON []byte
	if entry.PayloadDiff != nil {
		raw, err := json.Marshal(entry.PayloadDiff)
		if err != nil {
			return err
		}
		payloadJSON = raw
	} else {
		payloadJSON = []byte("{}")
	}

	var actorIP *netip.Addr = entry.Actor.IP
	var actorUserID *uuid.UUID
	if entry.Actor.UserID != uuid.Nil {
		uid := entry.Actor.UserID
		actorUserID = &uid
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO status_audit_logs (
			id, tenant_id, application_id, actor_user_id, actor_role, action_type,
			entity_type, entity_id, from_status, to_status, payload_diff, reason, ip_address
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12, $13)
	`,
		id, entry.TenantID, entry.Application, actorUserID, entry.Actor.RoleName,
		entry.ActionType, entry.EntityType, entry.EntityID, entry.FromStatus, entry.ToStatus,
		string(payloadJSON), entry.Reason, actorIP,
	)
	return err
}

func (s *PGStore) enqueueWhatsAppTx(ctx context.Context, tx pgx.Tx, notify NotificationWrite) error {
	if notify.TemplateCode == "" {
		return nil
	}
	var phone string
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT g.phone_whatsapp FROM applicant_guardians g
			 WHERE g.tenant_id = a.tenant_id AND g.application_id = a.id
			   AND g.phone_whatsapp IS NOT NULL AND btrim(g.phone_whatsapp) <> ''
			 ORDER BY g.is_primary_contact DESC,
			          CASE g.relation WHEN 'guardian' THEN 0 WHEN 'father' THEN 1 ELSE 2 END
			 LIMIT 1),
			u.whatsapp_phone,
			''
		)
		FROM applications a
		JOIN users u ON u.id = a.guardian_user_id
		WHERE a.tenant_id = $1 AND a.id = $2
	`, notify.TenantID, notify.ApplicationID).Scan(&phone)
	phone = strings.TrimSpace(phone)
	if err != nil || phone == "" {
		return err
	}

	logID := notify.ID
	if logID == uuid.Nil {
		logID = uuid.Must(uuid.NewV7())
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_logs (id, tenant_id, application_id, channel, recipient, template_code, status)
		VALUES ($1, $2, $3, 'whatsapp', $4, $5, 'queued')
	`, logID, notify.TenantID, notify.ApplicationID, phone, notify.TemplateCode); err != nil {
		return err
	}

	if s.riverClient != nil {
		appID := notify.ApplicationID
		_, err := s.riverClient.InsertTx(ctx, tx, worker.SendWhatsAppArgs{
			TenantID:        notify.TenantID,
			ApplicationID:   &appID,
			NotificationLog: logID,
			RecipientPhone:  phone,
			TemplateCode:    notify.TemplateCode,
			Variables:       notify.Variables,
		}, nil)
		if err != nil {
			return fmt.Errorf("enqueue river notification: %w", err)
		}
	}
	return nil
}

func marshalJSONMap(m map[string]any) (string, error) {
	if m == nil {
		return "{}", nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func formatOptionalDate(ts *time.Time) *string {
	if ts == nil {
		return nil
	}
	formatted := ts.UTC().Format("2006-01-02")
	return &formatted
}

