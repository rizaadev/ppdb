-- name: UpsertAssessmentSchedule :one
INSERT INTO assessment_schedules (
    id, tenant_id, application_id, session_name, room_or_link,
    scheduled_at, examiner_user_id
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('application_id'),
    sqlc.arg('session_name'), sqlc.narg('room_or_link'),
    sqlc.arg('scheduled_at'), sqlc.narg('examiner_user_id')
)
RETURNING *;

-- name: ListAssessmentSchedulesByApplication :many
SELECT id, tenant_id, application_id, session_name, room_or_link,
       scheduled_at, examiner_user_id, created_at
FROM assessment_schedules
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id')
ORDER BY scheduled_at ASC;

-- name: UpsertAssessmentScore :one
INSERT INTO assessment_scores (
    id, tenant_id, application_id, component_code, raw_score,
    weight_pct, weighted_score, examiner_user_id, notes
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('application_id'),
    sqlc.arg('component_code'), sqlc.arg('raw_score'), sqlc.arg('weight_pct'),
    sqlc.arg('weighted_score'), sqlc.narg('examiner_user_id'), sqlc.narg('notes')
)
ON CONFLICT (application_id, component_code) DO UPDATE SET
    raw_score = EXCLUDED.raw_score,
    weight_pct = EXCLUDED.weight_pct,
    weighted_score = EXCLUDED.weighted_score,
    examiner_user_id = EXCLUDED.examiner_user_id,
    notes = EXCLUDED.notes,
    scored_at = NOW()
RETURNING *;

-- name: ListAssessmentScoresByApplication :many
SELECT id, tenant_id, application_id, component_code, raw_score,
       weight_pct, weighted_score, examiner_user_id, notes, scored_at
FROM assessment_scores
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id')
ORDER BY component_code ASC;

-- name: RecalculateApplicationFinalScore :one
UPDATE applications
SET final_score = (
    SELECT COALESCE(SUM(weighted_score), 0)::numeric(8,4)
    FROM assessment_scores s
    WHERE s.tenant_id = applications.tenant_id
      AND s.application_id = applications.id
)
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('application_id')
RETURNING id, final_score;

-- name: SimulateTrackRanking :many
-- Urutan pemeringkatan: final_score tertinggi, lalu jarak domisili terdekat, lalu usia tertua (birth_date ASC), lalu waktu submit awal
SELECT
    a.id,
    a.registration_no,
    a.status,
    a.final_score,
    a.distance_meters,
    a.submitted_at,
    p.full_name,
    p.birth_date,
    ROW_NUMBER() OVER (
        ORDER BY
            COALESCE(a.final_score, 0) DESC,
            COALESCE(a.distance_meters, 2147483647) ASC,
            COALESCE(p.birth_date, DATE '2100-01-01') ASC,
            a.submitted_at ASC NULLS LAST
    )::integer AS computed_rank
FROM applications a
JOIN applicant_profiles p ON p.application_id = a.id
WHERE a.tenant_id = sqlc.arg('tenant_id')
  AND a.track_id = sqlc.arg('track_id')
  AND a.status IN ('verified', 'selection_in_progress', 'ranked')
ORDER BY computed_rank ASC;

-- name: ApplyApplicationRankingDecision :one
UPDATE applications
SET final_score = COALESCE(sqlc.narg('final_score'), final_score),
    rank_position = sqlc.arg('rank_position'),
    status = sqlc.arg('decision_status'),
    decided_at = NOW(),
    version = version + 1
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('application_id')
  AND status IN ('verified', 'selection_in_progress', 'ranked', 'waitlisted')
RETURNING *;

-- name: MarkPeriodResultsPublished :one
UPDATE admission_periods
SET results_published_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('period_id')
RETURNING *;

