-- +goose Up
-- +goose StatementBegin

-- 1. Fix missing payment_url column on payment_transactions (used by Midtrans Snap / gateway checkout)
ALTER TABLE payment_transactions
    ADD COLUMN IF NOT EXISTS payment_url TEXT;

-- 2. Custom domain verification token and optional per-tenant Midtrans configuration
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS domain_verification_token TEXT,
    ADD COLUMN IF NOT EXISTS payment_config JSONB NOT NULL DEFAULT '{}'::jsonb;

-- 3. Scheduling index for assessment sessions
CREATE INDEX IF NOT EXISTS idx_assessment_schedules_tenant_time
    ON assessment_schedules (tenant_id, scheduled_at ASC);

-- 4. Distributed HTTP Idempotency-Key persistence in PostgreSQL
CREATE TABLE IF NOT EXISTS idempotency_keys (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    status_code SMALLINT NOT NULL,
    response_json JSONB,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, user_id, scope, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_keys_expires
    ON idempotency_keys (expires_at);

-- 5. Non-superuser runtime role for PostgreSQL Row-Level Security enforcement
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pbdb_app') THEN
        CREATE ROLE pbdb_app NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
    BEGIN
        EXECUTE format('GRANT pbdb_app TO %I', current_user);
    EXCEPTION WHEN OTHERS THEN
        NULL;
    END;
END $$;

GRANT USAGE ON SCHEMA public TO pbdb_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO pbdb_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO pbdb_app;

-- 6. Enable & Force Row-Level Security across all 17 tenant-scoped tables
DO $$
DECLARE
    tbl TEXT;
    tenant_tables TEXT[] := ARRAY[
        'units',
        'user_tenant_roles',
        'admission_periods',
        'admission_tracks',
        'track_requirements',
        'applications',
        'applicant_profiles',
        'applicant_guardians',
        'applicant_achievements',
        'applicant_documents',
        'payment_transactions',
        'assessment_schedules',
        'assessment_scores',
        'status_audit_logs',
        'notification_logs',
        'export_jobs',
        'idempotency_keys'
    ];
BEGIN
    FOREACH tbl IN ARRAY tenant_tables
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', tbl);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', tbl);
        IF NOT EXISTS (
            SELECT 1 FROM pg_policies
            WHERE schemaname = 'public' AND tablename = tbl AND policyname = 'tenant_isolation_policy'
        ) THEN
            EXECUTE format(
                'CREATE POLICY tenant_isolation_policy ON %I
                 USING (
                     COALESCE(current_setting(''app.bypass_rls'', true), '''') = ''on''
                     OR tenant_id = NULLIF(current_setting(''app.current_tenant_id'', true), '''')::uuid
                 )
                 WITH CHECK (
                     COALESCE(current_setting(''app.bypass_rls'', true), '''') = ''on''
                     OR tenant_id = NULLIF(current_setting(''app.current_tenant_id'', true), '''')::uuid
                 )',
                tbl
            );
        END IF;
    END LOOP;
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DO $$
DECLARE
    tbl TEXT;
    tenant_tables TEXT[] := ARRAY[
        'idempotency_keys',
        'export_jobs',
        'notification_logs',
        'status_audit_logs',
        'assessment_scores',
        'assessment_schedules',
        'payment_transactions',
        'applicant_documents',
        'applicant_achievements',
        'applicant_guardians',
        'applicant_profiles',
        'applications',
        'track_requirements',
        'admission_tracks',
        'admission_periods',
        'user_tenant_roles',
        'units'
    ];
BEGIN
    FOREACH tbl IN ARRAY tenant_tables
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_policy ON %I', tbl);
        EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', tbl);
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', tbl);
    END LOOP;
END $$;

DROP TABLE IF EXISTS idempotency_keys;
DROP INDEX IF EXISTS idx_assessment_schedules_tenant_time;
ALTER TABLE tenants
    DROP COLUMN IF EXISTS payment_config,
    DROP COLUMN IF EXISTS domain_verification_token;
ALTER TABLE payment_transactions
    DROP COLUMN IF EXISTS payment_url;

-- +goose StatementEnd

