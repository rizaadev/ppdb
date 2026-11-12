-- +goose Up
-- +goose StatementBegin

CREATE TYPE payment_purpose AS ENUM (
    'registration_fee',
    're_enrollment_fee'
);

CREATE TYPE payment_method AS ENUM (
    'qris',
    'va',
    'ewallet',
    'manual_transfer'
);

CREATE TYPE payment_status AS ENUM (
    'pending',
    'paid',
    'failed',
    'expired',
    'refunded'
);

CREATE TYPE export_format AS ENUM (
    'dapodik_xlsx',
    'emis_xlsx',
    'custom_csv'
);

CREATE TYPE export_job_status AS ENUM (
    'pending',
    'processing',
    'completed',
    'failed'
);

-- Transaksi Pembayaran (Biaya Formulir & Daftar Ulang)
CREATE TABLE payment_transactions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    application_id UUID NOT NULL,
    purpose payment_purpose NOT NULL,
    method payment_method NOT NULL,
    provider TEXT NOT NULL DEFAULT 'manual',
    external_ref TEXT UNIQUE,
    amount_idr BIGINT NOT NULL CHECK (amount_idr > 0),
    admin_fee_idr BIGINT NOT NULL DEFAULT 0 CHECK (admin_fee_idr >= 0),
    status payment_status NOT NULL DEFAULT 'pending',
    proof_storage_key TEXT,
    verified_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    verification_note TEXT,
    expires_at TIMESTAMPTZ,
    paid_at TIMESTAMPTZ,
    raw_webhook JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE RESTRICT,
    UNIQUE (tenant_id, id)
);

CREATE INDEX idx_payment_transactions_app
    ON payment_transactions (tenant_id, application_id, purpose, created_at DESC);

CREATE INDEX idx_payment_transactions_pending_verify
    ON payment_transactions (tenant_id, status, created_at ASC)
    WHERE status = 'pending';

CREATE TRIGGER trg_payment_transactions_updated_at
    BEFORE UPDATE ON payment_transactions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Penjadwalan Sesi Tes / Wawancara (PMBM / Swasta / Pesantren)
CREATE TABLE assessment_schedules (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    application_id UUID NOT NULL,
    session_name TEXT NOT NULL,
    room_or_link TEXT,
    scheduled_at TIMESTAMPTZ NOT NULL,
    examiner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_assessment_schedules_app
    ON assessment_schedules (tenant_id, application_id, scheduled_at);

CREATE TRIGGER trg_assessment_schedules_updated_at
    BEFORE UPDATE ON assessment_schedules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Komponen Skor Seleksi (Akademik, BTQ, Wawancara, Jarak, Rapor, Prestasi)
CREATE TABLE assessment_scores (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    application_id UUID NOT NULL,
    component_code TEXT NOT NULL,
    raw_score NUMERIC(6, 2) NOT NULL,
    weight_pct NUMERIC(5, 2) NOT NULL DEFAULT 100.00 CHECK (weight_pct >= 0 AND weight_pct <= 100),
    weighted_score NUMERIC(8, 4) NOT NULL,
    examiner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    notes TEXT,
    scored_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (application_id, component_code)
);

CREATE INDEX idx_assessment_scores_app
    ON assessment_scores (tenant_id, application_id);

CREATE TRIGGER trg_assessment_scores_updated_at
    BEFORE UPDATE ON assessment_scores
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Audit Trail Permanen (Append-Only)
CREATE TABLE status_audit_logs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    application_id UUID,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_role TEXT NOT NULL,
    action_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID NOT NULL,
    from_status TEXT,
    to_status TEXT,
    payload_diff JSONB NOT NULL DEFAULT '{}'::jsonb,
    reason TEXT,
    ip_address INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE SET NULL
);

CREATE INDEX idx_status_audit_logs_app
    ON status_audit_logs (tenant_id, application_id, created_at DESC)
    WHERE application_id IS NOT NULL;

CREATE INDEX idx_status_audit_logs_tenant_time
    ON status_audit_logs (tenant_id, created_at DESC);

-- Log Pengiriman Notifikasi (WhatsApp & Email via River Worker)
CREATE TABLE notification_logs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    application_id UUID,
    channel TEXT NOT NULL,
    recipient TEXT NOT NULL,
    template_code TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    provider_message_id TEXT,
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE SET NULL
);

CREATE INDEX idx_notification_logs_app
    ON notification_logs (tenant_id, application_id, created_at DESC);

-- Pekerjaan Ekspor Data Massal (Dapodik / EMIS / CSV)
CREATE TABLE export_jobs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    unit_id UUID NOT NULL,
    period_id UUID NOT NULL,
    requested_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    format export_format NOT NULL,
    status export_job_status NOT NULL DEFAULT 'pending',
    result_storage_key TEXT,
    row_count INTEGER,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    FOREIGN KEY (tenant_id, unit_id) REFERENCES units(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, period_id) REFERENCES admission_periods(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_export_jobs_unit
    ON export_jobs (tenant_id, unit_id, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS export_jobs;
DROP TABLE IF EXISTS notification_logs;
DROP TABLE IF EXISTS status_audit_logs;
DROP TABLE IF EXISTS assessment_scores;
DROP TABLE IF EXISTS assessment_schedules;
DROP TABLE IF EXISTS payment_transactions;

DROP TYPE IF EXISTS export_job_status;
DROP TYPE IF EXISTS export_format;
DROP TYPE IF EXISTS payment_status;
DROP TYPE IF EXISTS payment_method;
DROP TYPE IF EXISTS payment_purpose;

-- +goose StatementEnd

