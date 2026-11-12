-- name: CreateApplication :one
INSERT INTO applications (
    id, tenant_id, unit_id, period_id, track_id, guardian_user_id,
    registration_no, status, extra_answers
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('unit_id'),
    sqlc.arg('period_id'), sqlc.arg('track_id'), sqlc.arg('guardian_user_id'),
    sqlc.arg('registration_no'), sqlc.arg('status'), sqlc.arg('extra_answers')
)
RETURNING *;

-- name: GetApplicationByID :one
SELECT id, tenant_id, unit_id, period_id, track_id, guardian_user_id,
       registration_no, status, claimed_by_verifier_id, claimed_at,
       distance_meters, final_score, rank_position, extra_answers, version,
       submitted_at, verified_at, decided_at, enrolled_at, withdrawn_at,
       withdrawn_reason, created_at, updated_at
FROM applications
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('application_id');

-- name: GetApplicationPublicStatus :one
SELECT a.id, a.registration_no, a.status, a.submitted_at, a.decided_at,
       u.name AS unit_name, t.display_name AS track_name, p.academic_year,
       p.results_published_at, pr.full_name
FROM applications a
JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
JOIN admission_periods p ON p.id = a.period_id AND p.tenant_id = a.tenant_id
JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
LEFT JOIN applicant_profiles pr ON pr.application_id = a.id
WHERE a.tenant_id = sqlc.arg('tenant_id')
  AND a.registration_no = sqlc.arg('registration_no');

-- name: ListApplicationsByGuardian :many
SELECT a.id, a.registration_no, a.status, a.unit_id, a.period_id, a.track_id,
       a.submitted_at, a.decided_at, a.enrolled_at, a.version, a.created_at,
       u.name AS unit_name, t.display_name AS track_name, p.name AS period_name,
       p.academic_year, pr.full_name AS student_name
FROM applications a
JOIN units u ON u.id = a.unit_id AND u.tenant_id = a.tenant_id
JOIN admission_periods p ON p.id = a.period_id AND p.tenant_id = a.tenant_id
JOIN admission_tracks t ON t.id = a.track_id AND t.tenant_id = a.tenant_id
LEFT JOIN applicant_profiles pr ON pr.application_id = a.id
WHERE a.tenant_id = sqlc.arg('tenant_id')
  AND a.guardian_user_id = sqlc.arg('guardian_user_id')
ORDER BY a.created_at DESC;

-- name: UpdateApplicationStatusOptimistic :one
UPDATE applications
SET status = sqlc.arg('new_status')::application_status,
    submitted_at = CASE WHEN sqlc.arg('new_status')::application_status = 'submitted' AND submitted_at IS NULL THEN NOW() ELSE submitted_at END,
    verified_at = CASE WHEN sqlc.arg('new_status')::application_status = 'verified' THEN NOW() ELSE verified_at END,
    enrolled_at = CASE WHEN sqlc.arg('new_status')::application_status = 'enrolled' THEN NOW() ELSE enrolled_at END,
    withdrawn_at = CASE WHEN sqlc.arg('new_status')::application_status = 'withdrawn' THEN NOW() ELSE withdrawn_at END,
    withdrawn_reason = COALESCE(sqlc.narg('withdrawn_reason'), withdrawn_reason),
    version = version + 1
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('application_id')
  AND version = sqlc.arg('expected_version')
RETURNING *;

-- name: UpdateApplicationExtraAndDistance :one
UPDATE applications
SET extra_answers = sqlc.arg('extra_answers'),
    distance_meters = sqlc.narg('distance_meters'),
    version = version + 1
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('application_id')
RETURNING *;

-- name: UpsertApplicantProfile :one
INSERT INTO applicant_profiles (
    application_id, tenant_id, full_name, nik, kk_number, kk_issued_date,
    nisn, birth_place, birth_date, gender, religion, citizenship,
    child_order, siblings_count, special_needs_code,
    address_street, rt, rw, hamlet, village_code, village_name,
    district_code, district_name, regency_code, regency_name,
    province_code, province_name, postal_code, latitude, longitude,
    prev_school_name, prev_school_npsn, graduation_year, ijazah_number
) VALUES (
    sqlc.arg('application_id'), sqlc.arg('tenant_id'), sqlc.arg('full_name'),
    sqlc.narg('nik'), sqlc.narg('kk_number'), sqlc.narg('kk_issued_date'),
    sqlc.narg('nisn'), sqlc.narg('birth_place'), sqlc.narg('birth_date'),
    sqlc.narg('gender'), sqlc.narg('religion'), sqlc.arg('citizenship'),
    sqlc.narg('child_order'), sqlc.narg('siblings_count'), sqlc.narg('special_needs_code'),
    sqlc.narg('address_street'), sqlc.narg('rt'), sqlc.narg('rw'), sqlc.narg('hamlet'),
    sqlc.narg('village_code'), sqlc.narg('village_name'),
    sqlc.narg('district_code'), sqlc.narg('district_name'),
    sqlc.narg('regency_code'), sqlc.narg('regency_name'),
    sqlc.narg('province_code'), sqlc.narg('province_name'),
    sqlc.narg('postal_code'), sqlc.narg('latitude'), sqlc.narg('longitude'),
    sqlc.narg('prev_school_name'), sqlc.narg('prev_school_npsn'),
    sqlc.narg('graduation_year'), sqlc.narg('ijazah_number')
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
RETURNING *;

-- name: GetApplicantProfile :one
SELECT application_id, tenant_id, full_name, nik, kk_number, kk_issued_date,
       nisn, birth_place, birth_date, gender, religion, citizenship,
       child_order, siblings_count, special_needs_code,
       address_street, rt, rw, hamlet, village_code, village_name,
       district_code, district_name, regency_code, regency_name,
       province_code, province_name, postal_code, latitude, longitude,
       prev_school_name, prev_school_npsn, graduation_year, ijazah_number,
       created_at, updated_at
FROM applicant_profiles
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id');

-- name: UpsertApplicantGuardian :one
INSERT INTO applicant_guardians (
    id, tenant_id, application_id, relation, full_name, nik, birth_year,
    education_level, occupation, income_bracket, phone_whatsapp, is_primary_contact
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('application_id'),
    sqlc.arg('relation'), sqlc.arg('full_name'), sqlc.narg('nik'),
    sqlc.narg('birth_year'), sqlc.narg('education_level'), sqlc.narg('occupation'),
    sqlc.narg('income_bracket'), sqlc.narg('phone_whatsapp'), sqlc.arg('is_primary_contact')
)
ON CONFLICT (application_id, relation) DO UPDATE SET
    full_name = EXCLUDED.full_name,
    nik = EXCLUDED.nik,
    birth_year = EXCLUDED.birth_year,
    education_level = EXCLUDED.education_level,
    occupation = EXCLUDED.occupation,
    income_bracket = EXCLUDED.income_bracket,
    phone_whatsapp = EXCLUDED.phone_whatsapp,
    is_primary_contact = EXCLUDED.is_primary_contact
RETURNING *;

-- name: ListApplicantGuardians :many
SELECT id, tenant_id, application_id, relation, full_name, nik, birth_year,
       education_level, occupation, income_bracket, phone_whatsapp, is_primary_contact
FROM applicant_guardians
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id')
ORDER BY relation ASC;

-- name: ListApplicantAchievements :many
SELECT id, tenant_id, application_id, category, title, level, rank_achieved,
       achievement_year, organizer, raw_points, verified_points, verified_by_user_id
FROM applicant_achievements
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id')
ORDER BY created_at ASC;

-- name: DeleteApplicantAchievementsByApplication :exec
DELETE FROM applicant_achievements
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id');

-- name: InsertApplicantAchievement :one
INSERT INTO applicant_achievements (
    id, tenant_id, application_id, category, title, level,
    rank_achieved, achievement_year, organizer, raw_points
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('application_id'),
    sqlc.arg('category'), sqlc.arg('title'), sqlc.arg('level'),
    sqlc.narg('rank_achieved'), sqlc.narg('achievement_year'),
    sqlc.narg('organizer'), sqlc.arg('raw_points')
)
RETURNING *;

-- name: UpsertApplicantDocument :one
INSERT INTO applicant_documents (
    id, tenant_id, application_id, requirement_id, storage_key,
    original_filename, mime_type, size_bytes, status
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('application_id'),
    sqlc.arg('requirement_id'), sqlc.arg('storage_key'),
    sqlc.arg('original_filename'), sqlc.arg('mime_type'), sqlc.arg('size_bytes'), 'pending'
)
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
RETURNING *;

-- name: ListApplicantDocumentsWithRequirements :many
SELECT d.id, d.tenant_id, d.application_id, d.requirement_id, d.storage_key,
       d.original_filename, d.mime_type, d.size_bytes, d.status,
       d.reviewer_user_id, d.review_note, d.reviewed_at, d.version,
       r.doc_code, r.label AS requirement_label, r.is_required
FROM applicant_documents d
JOIN track_requirements r ON r.id = d.requirement_id AND r.tenant_id = d.tenant_id
WHERE d.tenant_id = sqlc.arg('tenant_id')
  AND d.application_id = sqlc.arg('application_id')
ORDER BY r.sort_order ASC;

-- name: GetApplicantDocumentByID :one
SELECT id, tenant_id, application_id, requirement_id, storage_key,
       original_filename, mime_type, size_bytes, status,
       reviewer_user_id, review_note, reviewed_at, version
FROM applicant_documents
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id')
  AND id = sqlc.arg('document_id');
