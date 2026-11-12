-- name: GetTenantBySlug :one
SELECT id, slug, name, institution_type, custom_domain, domain_verified_at,
       terminology_preset, terminology_overrides, theme_config,
       subscription_plan, subscription_expires_at, is_active, created_at, updated_at
FROM tenants
WHERE slug = sqlc.arg('slug')
  AND is_active = TRUE;

-- name: GetTenantByHost :one
SELECT id, slug, name, institution_type, custom_domain, domain_verified_at,
       terminology_preset, terminology_overrides, theme_config,
       subscription_plan, subscription_expires_at, is_active, created_at, updated_at
FROM tenants
WHERE custom_domain = sqlc.arg('custom_domain')
  AND domain_verified_at IS NOT NULL
  AND is_active = TRUE;

-- name: VerifyCustomDomainForTLS :one
SELECT id, slug
FROM tenants
WHERE custom_domain = sqlc.arg('domain')
  AND is_active = TRUE;

-- name: GetTenantByID :one
SELECT id, slug, name, institution_type, custom_domain, domain_verified_at,
       terminology_preset, terminology_overrides, theme_config,
       subscription_plan, subscription_expires_at, is_active, created_at, updated_at
FROM tenants
WHERE id = sqlc.arg('id');

-- name: UpdateTenantSettings :one
UPDATE tenants
SET name = COALESCE(sqlc.narg('name'), name),
    terminology_preset = COALESCE(sqlc.narg('terminology_preset'), terminology_preset),
    terminology_overrides = COALESCE(sqlc.narg('terminology_overrides'), terminology_overrides),
    theme_config = COALESCE(sqlc.narg('theme_config'), theme_config),
    custom_domain = COALESCE(sqlc.narg('custom_domain'), custom_domain)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListActiveUnitsByTenant :many
SELECT id, tenant_id, code, name, education_level, npsn_or_nsm, phone, email,
       address, latitude, longitude, timezone, settings, is_active, created_at, updated_at
FROM units
WHERE tenant_id = sqlc.arg('tenant_id')
  AND is_active = TRUE
ORDER BY name ASC;

-- name: GetUnitByID :one
SELECT id, tenant_id, code, name, education_level, npsn_or_nsm, phone, email,
       address, latitude, longitude, timezone, settings, is_active, created_at, updated_at
FROM units
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('unit_id');

-- name: CreateUnit :one
INSERT INTO units (
    id, tenant_id, code, name, education_level, npsn_or_nsm,
    phone, email, address, latitude, longitude, timezone, settings
) VALUES (
    sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('code'), sqlc.arg('name'),
    sqlc.arg('education_level'), sqlc.narg('npsn_or_nsm'), sqlc.narg('phone'),
    sqlc.narg('email'), sqlc.narg('address'), sqlc.narg('latitude'),
    sqlc.narg('longitude'), sqlc.arg('timezone'), sqlc.arg('settings')
)
RETURNING *;

-- name: UpdateUnit :one
UPDATE units
SET name = COALESCE(sqlc.narg('name'), name),
    npsn_or_nsm = COALESCE(sqlc.narg('npsn_or_nsm'), npsn_or_nsm),
    phone = COALESCE(sqlc.narg('phone'), phone),
    email = COALESCE(sqlc.narg('email'), email),
    address = COALESCE(sqlc.narg('address'), address),
    latitude = COALESCE(sqlc.narg('latitude'), latitude),
    longitude = COALESCE(sqlc.narg('longitude'), longitude),
    settings = COALESCE(sqlc.narg('settings'), settings),
    is_active = COALESCE(sqlc.narg('is_active'), is_active)
WHERE tenant_id = sqlc.arg('tenant_id')
  AND id = sqlc.arg('unit_id')
RETURNING *;

