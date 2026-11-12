-- name: ListPublishedPeriodsByTenant :many
SELECT p.id, p.tenant_id, p.unit_id, u.name AS unit_name, u.education_level,
       p.academic_year, p.name, p.opens_at, p.closes_at, p.verification_deadline,
       p.announcement_at, p.re_enrollment_deadline, p.results_published_at
FROM admission_periods p
JOIN units u ON u.id = p.unit_id AND u.tenant_id = p.tenant_id
WHERE p.tenant_id = sqlc.arg('tenant_id')
  AND p.is_active = TRUE
  AND p.is_published = TRUE
  AND u.is_active = TRUE
ORDER BY p.opens_at ASC;

-- name: ListPeriodsByUnit :many
SELECT id, tenant_id, unit_id, academic_year, name, opens_at, closes_at,
       verification_deadline, announcement_at, re_enrollment_deadline,
       results_published_at, registration_seq, is_published, is_active, created_at, updated_at
FROM admission_periods
WHERE tenant_id = sqlc.arg('tenant_id')
  AND unit_id = sqlc.arg('unit_id')
ORDER BY opens_at DESC;

-- name: GetPeriodByID :one
SELECT id, tenant_id, unit_id, academic_year, name, opens_at, closes_at,
       verification_deadline, announcement_at, re_enrollment_deadline,
       results_published_at, registration_seq, is_published, is_active, created_at, updated_at
FROM admission_periods
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('period_id');

-- name: CreateAdmissionPeriod :one
INSERT INTO admission_periods (
    id, tenant_id, unit_id, academic_year, name, opens_at, closes_at,
    verification_deadline, announcement_at, re_enrollment_deadline, is_published
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('unit_id'),
    sqlc.arg('academic_year'), sqlc.arg('name'), sqlc.arg('opens_at'),
    sqlc.arg('closes_at'), sqlc.narg('verification_deadline'),
    sqlc.narg('announcement_at'), sqlc.narg('re_enrollment_deadline'),
    sqlc.arg('is_published')
)
RETURNING *;

-- name: NextPeriodRegistrationSeq :one
UPDATE admission_periods
SET registration_seq = registration_seq + 1
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('period_id')
RETURNING registration_seq;

-- name: ListTracksWithStatsByPeriod :many
SELECT t.id, t.tenant_id, t.period_id, t.code, t.display_name, t.description,
       t.selection_mode, t.quota, t.spillover_target_track_id,
       t.registration_fee_idr, t.re_enrollment_fee_idr,
       t.min_age_months, t.max_age_months, t.age_reference_date,
       t.requires_coordinates, t.requires_achievements,
       t.scoring_weights, t.custom_form_schema, t.sort_order, t.is_active,
       COUNT(a.id) FILTER (WHERE a.status NOT IN ('withdrawn', 'rejected'))::integer AS active_applicant_count,
       COUNT(a.id) FILTER (WHERE a.status IN ('accepted', 're_enrollment', 'enrolled'))::integer AS filled_quota_count
FROM admission_tracks t
LEFT JOIN applications a ON a.track_id = t.id AND a.tenant_id = t.tenant_id
WHERE t.tenant_id = sqlc.arg('tenant_id')
  AND t.period_id = sqlc.arg('period_id')
  AND t.is_active = TRUE
GROUP BY t.id
ORDER BY t.sort_order ASC, t.created_at ASC;

-- name: GetTrackByID :one
SELECT id, tenant_id, period_id, code, display_name, description, selection_mode,
       quota, spillover_target_track_id, registration_fee_idr, re_enrollment_fee_idr,
       min_age_months, max_age_months, age_reference_date,
       requires_coordinates, requires_achievements, scoring_weights,
       custom_form_schema, sort_order, is_active, created_at, updated_at
FROM admission_tracks
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('track_id');

-- name: CreateAdmissionTrack :one
INSERT INTO admission_tracks (
    id, tenant_id, period_id, code, display_name, description, selection_mode,
    quota, spillover_target_track_id, registration_fee_idr, re_enrollment_fee_idr,
    min_age_months, max_age_months, age_reference_date,
    requires_coordinates, requires_achievements, scoring_weights,
    custom_form_schema, sort_order
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('period_id'),
    sqlc.arg('code'), sqlc.arg('display_name'), sqlc.narg('description'),
    sqlc.arg('selection_mode'), sqlc.arg('quota'), sqlc.narg('spillover_target_track_id'),
    sqlc.arg('registration_fee_idr'), sqlc.arg('re_enrollment_fee_idr'),
    sqlc.narg('min_age_months'), sqlc.narg('max_age_months'), sqlc.narg('age_reference_date'),
    sqlc.arg('requires_coordinates'), sqlc.arg('requires_achievements'),
    sqlc.arg('scoring_weights'), sqlc.arg('custom_form_schema'), sqlc.arg('sort_order')
)
RETURNING *;

-- name: ListRequirementsByTrack :many
SELECT id, tenant_id, track_id, doc_code, label, description, is_required,
       accepted_mimes, max_size_kb, validation_rules, sort_order
FROM track_requirements
WHERE tenant_id = sqlc.arg('tenant_id')
  AND track_id = sqlc.arg('track_id')
ORDER BY sort_order ASC, created_at ASC;

-- name: UpsertTrackRequirement :one
INSERT INTO track_requirements (
    id, tenant_id, track_id, doc_code, label, description,
    is_required, accepted_mimes, max_size_kb, validation_rules, sort_order
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('track_id'),
    sqlc.arg('doc_code'), sqlc.arg('label'), sqlc.narg('description'),
    sqlc.arg('is_required'), sqlc.arg('accepted_mimes'), sqlc.arg('max_size_kb'),
    sqlc.arg('validation_rules'), sqlc.arg('sort_order')
)
ON CONFLICT (track_id, doc_code) DO UPDATE SET
    label = EXCLUDED.label,
    description = EXCLUDED.description,
    is_required = EXCLUDED.is_required,
    accepted_mimes = EXCLUDED.accepted_mimes,
    max_size_kb = EXCLUDED.max_size_kb,
    validation_rules = EXCLUDED.validation_rules,
    sort_order = EXCLUDED.sort_order
RETURNING *;

