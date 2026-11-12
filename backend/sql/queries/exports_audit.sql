-- name: InsertStatusAuditLog :one
INSERT INTO status_audit_logs (
    id, tenant_id, application_id, actor_user_id, actor_role, action_type,
    entity_type, entity_id, from_status, to_status, payload_diff, reason, ip_address
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.narg('application_id'),
    sqlc.narg('actor_user_id'), sqlc.arg('actor_role'), sqlc.arg('action_type'),
    sqlc.arg('entity_type'), sqlc.arg('entity_id'), sqlc.narg('from_status'),
    sqlc.narg('to_status'), sqlc.arg('payload_diff'), sqlc.narg('reason'),
    sqlc.narg('ip_address')
)
RETURNING *;

-- name: ListAuditLogsByApplication :many
SELECT l.id, l.tenant_id, l.application_id, l.actor_user_id, l.actor_role,
       l.action_type, l.entity_type, l.entity_id, l.from_status, l.to_status,
       l.payload_diff, l.reason, l.ip_address, l.created_at,
       u.full_name AS actor_name
FROM status_audit_logs l
LEFT JOIN users u ON u.id = l.actor_user_id
WHERE l.tenant_id = sqlc.arg('tenant_id')
  AND l.application_id = sqlc.arg('application_id')
ORDER BY l.created_at DESC
LIMIT sqlc.arg('page_limit');

-- name: ListAuditLogsByTenant :many
SELECT l.id, l.tenant_id, l.application_id, l.actor_user_id, l.actor_role,
       l.action_type, l.entity_type, l.entity_id, l.from_status, l.to_status,
       l.payload_diff, l.reason, l.ip_address, l.created_at,
       u.full_name AS actor_name
FROM status_audit_logs l
LEFT JOIN users u ON u.id = l.actor_user_id
WHERE l.tenant_id = sqlc.arg('tenant_id')
ORDER BY l.created_at DESC
LIMIT sqlc.arg('page_limit');

-- name: InsertNotificationLog :one
INSERT INTO notification_logs (
    id, tenant_id, application_id, channel, recipient, template_code, status
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.narg('application_id'),
    sqlc.arg('channel'), sqlc.arg('recipient'), sqlc.arg('template_code'), 'queued'
)
RETURNING *;

-- name: UpdateNotificationLogResult :exec
UPDATE notification_logs
SET status = sqlc.arg('status'),
    provider_message_id = sqlc.narg('provider_message_id'),
    error_message = sqlc.narg('error_message'),
    sent_at = CASE WHEN sqlc.arg('status') = 'sent' THEN NOW() ELSE sent_at END
WHERE id = sqlc.arg('id');

-- name: CreateExportJob :one
INSERT INTO export_jobs (
    id, tenant_id, unit_id, period_id, requested_by_user_id, format, status
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('unit_id'),
    sqlc.arg('period_id'), sqlc.arg('requested_by_user_id'),
    sqlc.arg('format'), 'pending'
)
RETURNING *;

-- name: GetExportJobByID :one
SELECT id, tenant_id, unit_id, period_id, requested_by_user_id, format,
       status, result_storage_key, row_count, error_message, created_at, completed_at
FROM export_jobs
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('job_id');

-- name: UpdateExportJobStatus :one
UPDATE export_jobs
SET status = sqlc.arg('status')::export_job_status,
    result_storage_key = COALESCE(sqlc.narg('result_storage_key'), result_storage_key),
    row_count = COALESCE(sqlc.narg('row_count'), row_count),
    error_message = COALESCE(sqlc.narg('error_message'), error_message),
    completed_at = CASE WHEN sqlc.arg('status')::export_job_status IN ('completed', 'failed') THEN NOW() ELSE completed_at END
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('job_id')
RETURNING *;

-- name: ListEnrolledStudentsForNationalExport :many
-- Mengambil seluruh murid berstatus enrolled beserta data ayah/ibu/wali untuk ekspor template Dapodik & EMIS
SELECT
    a.id AS application_id,
    a.registration_no,
    t.code AS track_code,
    t.display_name AS track_name,
    p.full_name,
    p.nik,
    p.kk_number,
    p.nisn,
    p.birth_place,
    p.birth_date,
    p.gender,
    p.religion,
    p.citizenship,
    p.child_order,
    p.siblings_count,
    p.special_needs_code,
    p.address_street,
    p.rt,
    p.rw,
    p.hamlet,
    p.village_code,
    p.village_name,
    p.district_code,
    p.district_name,
    p.regency_code,
    p.regency_name,
    p.province_code,
    p.province_name,
    p.postal_code,
    p.latitude,
    p.longitude,
    p.prev_school_name,
    p.prev_school_npsn,
    p.graduation_year,
    p.ijazah_number,
    f.full_name AS father_name,
    f.nik AS father_nik,
    f.birth_year AS father_birth_year,
    f.education_level AS father_education,
    f.occupation AS father_occupation,
    f.income_bracket AS father_income,
    m.full_name AS mother_name,
    m.nik AS mother_nik,
    m.birth_year AS mother_birth_year,
    m.education_level AS mother_education,
    m.occupation AS mother_occupation,
    m.income_bracket AS mother_income,
    w.full_name AS guardian_name,
    w.nik AS guardian_nik,
    w.birth_year AS guardian_birth_year,
    w.education_level AS guardian_education,
    w.occupation AS guardian_occupation,
    w.income_bracket AS guardian_income,
    COALESCE(w.phone_whatsapp, f.phone_whatsapp, m.phone_whatsapp) AS contact_whatsapp
FROM applications a
JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
JOIN applicant_profiles p ON p.application_id = a.id AND p.tenant_id = a.tenant_id
LEFT JOIN applicant_guardians f ON f.application_id = a.id AND f.tenant_id = a.tenant_id AND f.relation = 'father'
LEFT JOIN applicant_guardians m ON m.application_id = a.id AND m.tenant_id = a.tenant_id AND m.relation = 'mother'
LEFT JOIN applicant_guardians w ON w.application_id = a.id AND w.tenant_id = a.tenant_id AND w.relation = 'guardian'
WHERE a.tenant_id = sqlc.arg('tenant_id')
  AND a.unit_id = sqlc.arg('unit_id')
  AND a.period_id = sqlc.arg('period_id')
  AND a.status = 'enrolled'
ORDER BY p.full_name ASC;
