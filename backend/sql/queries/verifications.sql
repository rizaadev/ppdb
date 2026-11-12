-- name: ListAdminApplicationsCursor :many
SELECT a.id, a.registration_no, a.status, a.unit_id, a.period_id, a.track_id,
       a.distance_meters, a.final_score, a.rank_position, a.submitted_at,
       a.verified_at, a.created_at, a.version,
       t.display_name AS track_name,
       pr.full_name AS student_name, pr.nisn
FROM applications a
JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
LEFT JOIN applicant_profiles pr ON pr.application_id = a.id
WHERE a.tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('unit_id')::uuid IS NULL OR a.unit_id = sqlc.narg('unit_id')::uuid)
  AND (sqlc.narg('period_id')::uuid IS NULL OR a.period_id = sqlc.narg('period_id')::uuid)
  AND (sqlc.narg('track_id')::uuid IS NULL OR a.track_id = sqlc.narg('track_id')::uuid)
  AND (sqlc.narg('status')::application_status IS NULL OR a.status = sqlc.narg('status')::application_status)
  AND (
      sqlc.narg('search')::text IS NULL
      OR a.registration_no ILIKE '%' || sqlc.narg('search')::text || '%'
      OR pr.full_name ILIKE '%' || sqlc.narg('search')::text || '%'
      OR pr.nisn = sqlc.narg('search')::text
  )
  AND (
      sqlc.narg('cursor_created_at')::timestamptz IS NULL
      OR (a.created_at, a.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg('page_limit');

-- name: ClaimNextVerificationApplication :one
UPDATE applications
SET status = 'under_verification',
    claimed_by_verifier_id = sqlc.arg('verifier_user_id'),
    claimed_at = NOW(),
    version = version + 1
WHERE id = (
    SELECT a.id
    FROM applications a
    WHERE a.tenant_id = sqlc.arg('tenant_id')
      AND a.unit_id = sqlc.arg('unit_id')
      AND a.status = 'submitted'
    ORDER BY a.submitted_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
AND tenant_id = sqlc.arg('tenant_id')
RETURNING *;

-- name: ClaimSpecificApplicationForVerification :one
UPDATE applications
SET status = 'under_verification',
    claimed_by_verifier_id = sqlc.arg('verifier_user_id'),
    claimed_at = NOW(),
    version = version + 1
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('application_id')
  AND status IN ('submitted', 'under_verification')
RETURNING *;

-- name: ReviewApplicantDocument :one
UPDATE applicant_documents
SET status = sqlc.arg('status'),
    reviewer_user_id = sqlc.arg('reviewer_user_id'),
    review_note = sqlc.narg('review_note'),
    reviewed_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id')
  AND id = sqlc.arg('document_id')
RETURNING *;

-- name: CountDocumentReviewSummary :one
SELECT
    COUNT(*)::integer AS total_uploaded,
    COUNT(*) FILTER (WHERE status = 'approved')::integer AS approved_count,
    COUNT(*) FILTER (WHERE status = 'rejected_for_revision')::integer AS rejected_count,
    COUNT(*) FILTER (WHERE status = 'pending')::integer AS pending_count
FROM applicant_documents
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id');

-- name: CountMissingRequiredDocuments :one
SELECT COUNT(*)::integer AS missing_count
FROM track_requirements tr
LEFT JOIN applicant_documents ad
    ON ad.requirement_id = tr.id
    AND ad.application_id = sqlc.arg('application_id')
    AND ad.tenant_id = tr.tenant_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')
  AND tr.track_id = sqlc.arg('track_id')
  AND tr.is_required = TRUE
  AND ad.id IS NULL;

