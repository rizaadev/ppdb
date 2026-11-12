-- name: CreatePaymentTransaction :one
INSERT INTO payment_transactions (
    id, tenant_id, application_id, purpose, method, provider,
    external_ref, amount_idr, admin_fee_idr, status, expires_at
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('application_id'),
    sqlc.arg('purpose'), sqlc.arg('method'), sqlc.arg('provider'),
    sqlc.narg('external_ref'), sqlc.arg('amount_idr'), sqlc.arg('admin_fee_idr'),
    'pending', sqlc.narg('expires_at')
)
RETURNING *;

-- name: GetPaymentByID :one
SELECT id, tenant_id, application_id, purpose, method, provider, external_ref,
       amount_idr, admin_fee_idr, status, proof_storage_key, verified_by_user_id,
       verification_note, expires_at, paid_at, raw_webhook, created_at, updated_at
FROM payment_transactions
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('payment_id');

-- name: GetPaymentByExternalRef :one
SELECT id, tenant_id, application_id, purpose, method, provider, external_ref,
       amount_idr, admin_fee_idr, status, proof_storage_key, verified_by_user_id,
       verification_note, expires_at, paid_at, raw_webhook, created_at, updated_at
FROM payment_transactions
WHERE external_ref = sqlc.arg('external_ref');

-- name: ListPaymentsByApplication :many
SELECT id, tenant_id, application_id, purpose, method, provider, external_ref,
       amount_idr, admin_fee_idr, status, proof_storage_key, verified_by_user_id,
       verification_note, expires_at, paid_at, created_at, updated_at
FROM payment_transactions
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id')
ORDER BY created_at DESC;

-- name: AttachManualPaymentProof :one
UPDATE payment_transactions
SET proof_storage_key = sqlc.arg('proof_storage_key')
WHERE tenant_id = sqlc.arg('tenant_id')
  AND application_id = sqlc.arg('application_id')
  AND id = sqlc.arg('payment_id')
  AND method = 'manual_transfer'
  AND status = 'pending'
RETURNING *;

-- name: ListAdminPayments :many
SELECT p.id, p.tenant_id, p.application_id, p.purpose, p.method, p.provider,
       p.external_ref, p.amount_idr, p.status, p.proof_storage_key,
       p.expires_at, p.paid_at, p.created_at,
       a.registration_no, pr.full_name AS student_name
FROM payment_transactions p
JOIN applications a ON a.id = p.application_id AND a.tenant_id = p.tenant_id
LEFT JOIN applicant_profiles pr ON pr.application_id = a.id
WHERE p.tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('status')::payment_status IS NULL OR p.status = sqlc.narg('status')::payment_status)
  AND (sqlc.narg('method')::payment_method IS NULL OR p.method = sqlc.narg('method')::payment_method)
ORDER BY p.created_at DESC
LIMIT sqlc.arg('page_limit');

-- name: MarkPaymentPaidFromGateway :one
UPDATE payment_transactions
SET status = 'paid',
    paid_at = COALESCE(sqlc.narg('paid_at'), NOW()),
    raw_webhook = sqlc.arg('raw_webhook')
WHERE id = sqlc.arg('payment_id')
  AND status = 'pending'
RETURNING *;

-- name: VerifyManualPayment :one
UPDATE payment_transactions
SET status = sqlc.arg('new_status')::payment_status,
    verified_by_user_id = sqlc.arg('treasurer_user_id'),
    verification_note = sqlc.narg('verification_note'),
    paid_at = CASE WHEN sqlc.arg('new_status')::payment_status = 'paid' THEN NOW() ELSE paid_at END
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('payment_id')
  AND status = 'pending'
RETURNING *;

-- name: ExpireOverduePendingPayments :many
UPDATE payment_transactions
SET status = 'expired'
WHERE status = 'pending'
  AND expires_at IS NOT NULL
  AND expires_at < NOW()
RETURNING id, tenant_id, application_id;
