-- name: CreateUser :one
INSERT INTO users (
    id, whatsapp_phone, email, password_hash, full_name, phone_verified_at, email_verified_at
) VALUES (
    sqlc.arg('id'), sqlc.narg('whatsapp_phone'), sqlc.narg('email'),
    sqlc.arg('password_hash'), sqlc.arg('full_name'),
    sqlc.narg('phone_verified_at'), sqlc.narg('email_verified_at')
)
RETURNING *;

-- name: GetUserByID :one
SELECT id, whatsapp_phone, email, password_hash, full_name, is_platform_admin,
       is_active, phone_verified_at, email_verified_at, last_login_at, created_at, updated_at
FROM users
WHERE id = sqlc.arg('id')
  AND is_active = TRUE;

-- name: GetUserByWhatsAppPhone :one
SELECT id, whatsapp_phone, email, password_hash, full_name, is_platform_admin,
       is_active, phone_verified_at, email_verified_at, last_login_at, created_at, updated_at
FROM users
WHERE whatsapp_phone = sqlc.arg('whatsapp_phone')
  AND is_active = TRUE;

-- name: GetUserByEmail :one
SELECT id, whatsapp_phone, email, password_hash, full_name, is_platform_admin,
       is_active, phone_verified_at, email_verified_at, last_login_at, created_at, updated_at
FROM users
WHERE email = sqlc.arg('email')
  AND is_active = TRUE;

-- name: MarkUserPhoneVerified :exec
UPDATE users
SET phone_verified_at = COALESCE(phone_verified_at, NOW()),
    last_login_at = NOW()
WHERE id = sqlc.arg('user_id');

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login_at = NOW()
WHERE id = sqlc.arg('user_id');

-- name: ListUserRolesInTenant :many
SELECT utr.id, utr.user_id, utr.tenant_id, utr.unit_id, utr.role, utr.is_active
FROM user_tenant_roles utr
WHERE utr.user_id = sqlc.arg('user_id')
  AND utr.tenant_id = sqlc.arg('tenant_id')
  AND utr.is_active = TRUE;

-- name: AssignTenantRole :one
INSERT INTO user_tenant_roles (
    id, user_id, tenant_id, unit_id, role
) VALUES (
    sqlc.arg('id'), sqlc.arg('user_id'), sqlc.arg('tenant_id'),
    sqlc.narg('unit_id'), sqlc.arg('role')
)
RETURNING *;

-- name: CreateOTPChallenge :one
INSERT INTO otp_challenges (
    id, tenant_id, whatsapp_phone, purpose, code_hash, expires_at
) VALUES (
    sqlc.arg('id'), sqlc.narg('tenant_id'), sqlc.arg('whatsapp_phone'),
    sqlc.arg('purpose'), sqlc.arg('code_hash'), sqlc.arg('expires_at')
)
RETURNING *;

-- name: GetLatestActiveOTP :one
SELECT id, tenant_id, whatsapp_phone, purpose, code_hash, attempt_count,
       max_attempts, expires_at, consumed_at, created_at
FROM otp_challenges
WHERE whatsapp_phone = sqlc.arg('whatsapp_phone')
  AND purpose = sqlc.arg('purpose')
  AND consumed_at IS NULL
  AND expires_at > NOW()
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementOTPAttempt :one
UPDATE otp_challenges
SET attempt_count = attempt_count + 1
WHERE id = sqlc.arg('id')
RETURNING attempt_count, max_attempts;

-- name: ConsumeOTPChallenge :exec
UPDATE otp_challenges
SET consumed_at = NOW()
WHERE id = sqlc.arg('id');

-- name: CreateUserSession :one
INSERT INTO user_sessions (
    id, user_id, tenant_id, refresh_token_hash, user_agent, ip_address, expires_at
) VALUES (
    sqlc.arg('id'), sqlc.arg('user_id'), sqlc.narg('tenant_id'),
    sqlc.arg('refresh_token_hash'), sqlc.narg('user_agent'),
    sqlc.narg('ip_address'), sqlc.arg('expires_at')
)
RETURNING *;

-- name: GetActiveSessionByTokenHash :one
SELECT id, user_id, tenant_id, refresh_token_hash, expires_at, revoked_at
FROM user_sessions
WHERE refresh_token_hash = sqlc.arg('refresh_token_hash')
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- name: RevokeSession :exec
UPDATE user_sessions
SET revoked_at = NOW()
WHERE id = sqlc.arg('session_id');

