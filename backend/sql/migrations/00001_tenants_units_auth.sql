-- +goose Up
-- +goose StatementBegin

CREATE TYPE institution_type AS ENUM (
    'school',
    'madrasah',
    'foundation',
    'pesantren'
);

CREATE TYPE terminology_preset AS ENUM (
    'spmb',
    'pmbm',
    'mandiri'
);

CREATE TYPE education_level AS ENUM (
    'ra_tk',
    'sd_mi',
    'smp_mts',
    'sma_ma',
    'smk_mak',
    'pesantren'
);

CREATE TYPE tenant_role AS ENUM (
    'tenant_admin',
    'unit_admin',
    'verifier',
    'examiner',
    'treasurer',
    'principal'
);

CREATE TYPE otp_purpose AS ENUM (
    'register',
    'login',
    'reset_pin',
    'verify_phone'
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Tenants (Sekolah tunggal, Madrasah, Yayasan multi-unit, atau Pesantren)
CREATE TABLE tenants (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    institution_type institution_type NOT NULL DEFAULT 'school',
    custom_domain TEXT UNIQUE,
    domain_verified_at TIMESTAMPTZ,
    terminology_preset terminology_preset NOT NULL DEFAULT 'spmb',
    terminology_overrides JSONB NOT NULL DEFAULT '{}'::jsonb,
    theme_config JSONB NOT NULL DEFAULT '{"primary_color":"#1E3A5F","tagline":""}'::jsonb,
    subscription_plan TEXT NOT NULL DEFAULT 'starter',
    subscription_expires_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_tenants_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$')
);

CREATE INDEX idx_tenants_active_domain
    ON tenants (custom_domain)
    WHERE custom_domain IS NOT NULL AND is_active = TRUE;

CREATE TRIGGER trg_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Units / Jenjang Pendidikan di bawah Tenant
CREATE TABLE units (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    education_level education_level NOT NULL,
    npsn_or_nsm TEXT,
    phone TEXT,
    email TEXT,
    address TEXT,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, code),
    UNIQUE (tenant_id, id)
);

CREATE INDEX idx_units_tenant_active ON units (tenant_id, is_active);

CREATE TRIGGER trg_units_updated_at
    BEFORE UPDATE ON units
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Users (Pendaftar/Orang Tua & Staf/Panitia)
CREATE TABLE users (
    id UUID PRIMARY KEY,
    whatsapp_phone TEXT UNIQUE,
    email TEXT UNIQUE,
    password_hash TEXT NOT NULL,
    full_name TEXT NOT NULL,
    is_platform_admin BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    phone_verified_at TIMESTAMPTZ,
    email_verified_at TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_users_contact_present CHECK (whatsapp_phone IS NOT NULL OR email IS NOT NULL)
);

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Role assignment per Tenant / Unit
CREATE TABLE user_tenant_roles (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    unit_id UUID,
    role tenant_role NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, unit_id) REFERENCES units(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_tenant_admin_unit_null CHECK (
        (role = 'tenant_admin' AND unit_id IS NULL) OR (role <> 'tenant_admin')
    )
);

CREATE UNIQUE INDEX uq_user_tenant_role_scope
    ON user_tenant_roles (user_id, tenant_id, COALESCE(unit_id, '00000000-0000-0000-0000-000000000000'::uuid), role);

CREATE INDEX idx_user_tenant_roles_lookup
    ON user_tenant_roles (tenant_id, user_id)
    WHERE is_active = TRUE;

CREATE TRIGGER trg_user_tenant_roles_updated_at
    BEFORE UPDATE ON user_tenant_roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- OTP Challenges (WhatsApp-first verification)
CREATE TABLE otp_challenges (
    id UUID PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    whatsapp_phone TEXT NOT NULL,
    purpose otp_purpose NOT NULL,
    code_hash TEXT NOT NULL,
    attempt_count SMALLINT NOT NULL DEFAULT 0,
    max_attempts SMALLINT NOT NULL DEFAULT 5,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_otp_challenges_active
    ON otp_challenges (whatsapp_phone, purpose, expires_at DESC)
    WHERE consumed_at IS NULL;

-- Refresh Token Sessions
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL UNIQUE,
    user_agent TEXT,
    ip_address INET,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_sessions_user_active
    ON user_sessions (user_id, expires_at DESC)
    WHERE revoked_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS otp_challenges;
DROP TABLE IF EXISTS user_tenant_roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS units;
DROP TABLE IF EXISTS tenants;

DROP FUNCTION IF EXISTS set_updated_at();

DROP TYPE IF EXISTS otp_purpose;
DROP TYPE IF EXISTS tenant_role;
DROP TYPE IF EXISTS education_level;
DROP TYPE IF EXISTS terminology_preset;
DROP TYPE IF EXISTS institution_type;

-- +goose StatementEnd

