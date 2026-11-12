-- +goose Up
-- +goose StatementBegin

CREATE TYPE selection_mode AS ENUM (
    'rule_based',
    'test_based',
    'hybrid',
    'direct'
);

-- Periode & Gelombang Pendaftaran per Unit
CREATE TABLE admission_periods (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    unit_id UUID NOT NULL,
    academic_year TEXT NOT NULL,
    name TEXT NOT NULL,
    opens_at TIMESTAMPTZ NOT NULL,
    closes_at TIMESTAMPTZ NOT NULL,
    verification_deadline TIMESTAMPTZ,
    announcement_at TIMESTAMPTZ,
    re_enrollment_deadline TIMESTAMPTZ,
    results_published_at TIMESTAMPTZ,
    registration_seq INTEGER NOT NULL DEFAULT 0,
    is_published BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, unit_id) REFERENCES units(tenant_id, id) ON DELETE RESTRICT,
    UNIQUE (tenant_id, id),
    CONSTRAINT chk_period_window CHECK (closes_at > opens_at)
);

CREATE INDEX idx_admission_periods_public
    ON admission_periods (tenant_id, unit_id, opens_at, closes_at)
    WHERE is_active = TRUE AND is_published = TRUE;

CREATE TRIGGER trg_admission_periods_updated_at
    BEFORE UPDATE ON admission_periods
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Jalur Penerimaan per Periode (SPMB: Domisili/Afirmasi/Prestasi/Mutasi, PMBM: Reguler/Prestasi/Afirmasi, Mandiri)
CREATE TABLE admission_tracks (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    period_id UUID NOT NULL,
    code TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,
    selection_mode selection_mode NOT NULL DEFAULT 'rule_based',
    quota INTEGER NOT NULL CHECK (quota >= 0),
    spillover_target_track_id UUID,
    registration_fee_idr BIGINT NOT NULL DEFAULT 0 CHECK (registration_fee_idr >= 0),
    re_enrollment_fee_idr BIGINT NOT NULL DEFAULT 0 CHECK (re_enrollment_fee_idr >= 0),
    min_age_months INTEGER CHECK (min_age_months IS NULL OR min_age_months >= 0),
    max_age_months INTEGER CHECK (max_age_months IS NULL OR max_age_months >= 0),
    age_reference_date DATE,
    requires_coordinates BOOLEAN NOT NULL DEFAULT FALSE,
    requires_achievements BOOLEAN NOT NULL DEFAULT FALSE,
    scoring_weights JSONB NOT NULL DEFAULT '{}'::jsonb,
    custom_form_schema JSONB NOT NULL DEFAULT '[]'::jsonb,
    sort_order SMALLINT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, period_id) REFERENCES admission_periods(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, id),
    UNIQUE (period_id, code)
);

ALTER TABLE admission_tracks
    ADD CONSTRAINT fk_tracks_spillover_target
    FOREIGN KEY (tenant_id, spillover_target_track_id)
    REFERENCES admission_tracks(tenant_id, id)
    ON DELETE SET NULL;

CREATE INDEX idx_admission_tracks_period
    ON admission_tracks (tenant_id, period_id, sort_order)
    WHERE is_active = TRUE;

CREATE TRIGGER trg_admission_tracks_updated_at
    BEFORE UPDATE ON admission_tracks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Persyaratan Berkas & Validasi Khusus per Jalur
CREATE TABLE track_requirements (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    track_id UUID NOT NULL,
    doc_code TEXT NOT NULL,
    label TEXT NOT NULL,
    description TEXT,
    is_required BOOLEAN NOT NULL DEFAULT TRUE,
    accepted_mimes TEXT[] NOT NULL DEFAULT ARRAY['image/jpeg', 'image/png', 'image/webp', 'application/pdf'],
    max_size_kb INTEGER NOT NULL DEFAULT 2048 CHECK (max_size_kb > 0),
    validation_rules JSONB NOT NULL DEFAULT '{}'::jsonb,
    sort_order SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, track_id) REFERENCES admission_tracks(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, id),
    UNIQUE (track_id, doc_code)
);

CREATE INDEX idx_track_requirements_track
    ON track_requirements (tenant_id, track_id, sort_order);

CREATE TRIGGER trg_track_requirements_updated_at
    BEFORE UPDATE ON track_requirements
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS track_requirements;
DROP TABLE IF EXISTS admission_tracks;
DROP TABLE IF EXISTS admission_periods;

DROP TYPE IF EXISTS selection_mode;

-- +goose StatementEnd

