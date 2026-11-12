-- +goose Up
-- +goose StatementBegin

CREATE TYPE application_status AS ENUM (
    'account_created',
    'track_selected',
    'awaiting_payment',
    'payment_verified',
    'draft_form',
    'submitted',
    'under_verification',
    'revision_required',
    'verified',
    'selection_in_progress',
    'ranked',
    'accepted',
    'waitlisted',
    'rejected',
    're_enrollment',
    'enrolled',
    'withdrawn'
);

CREATE TYPE gender_type AS ENUM ('L', 'P');

CREATE TYPE guardian_relation AS ENUM ('father', 'mother', 'guardian');

CREATE TYPE document_review_status AS ENUM (
    'pending',
    'approved',
    'rejected_for_revision'
);

-- Transaksi Inti Pendaftaran Calon Murid
CREATE TABLE applications (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    unit_id UUID NOT NULL,
    period_id UUID NOT NULL,
    track_id UUID NOT NULL,
    guardian_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    registration_no TEXT NOT NULL,
    status application_status NOT NULL DEFAULT 'track_selected',
    claimed_by_verifier_id UUID REFERENCES users(id) ON DELETE SET NULL,
    claimed_at TIMESTAMPTZ,
    distance_meters INTEGER CHECK (distance_meters IS NULL OR distance_meters >= 0),
    final_score NUMERIC(8, 4),
    rank_position INTEGER CHECK (rank_position IS NULL OR rank_position > 0),
    extra_answers JSONB NOT NULL DEFAULT '{}'::jsonb,
    version INTEGER NOT NULL DEFAULT 1,
    submitted_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    decided_at TIMESTAMPTZ,
    enrolled_at TIMESTAMPTZ,
    withdrawn_at TIMESTAMPTZ,
    withdrawn_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, unit_id) REFERENCES units(tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, period_id) REFERENCES admission_periods(tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, track_id) REFERENCES admission_tracks(tenant_id, id) ON DELETE RESTRICT,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, registration_no)
);

-- Indeks untuk antrean verifikasi, pagination keyset admin, dan portal orang tua
CREATE INDEX idx_applications_verification_queue
    ON applications (tenant_id, unit_id, submitted_at ASC)
    WHERE status = 'submitted';

CREATE INDEX idx_applications_admin_cursor
    ON applications (tenant_id, unit_id, status, created_at DESC, id DESC);

CREATE INDEX idx_applications_guardian
    ON applications (tenant_id, guardian_user_id, created_at DESC);

CREATE INDEX idx_applications_track_ranking
    ON applications (tenant_id, track_id, status, final_score DESC NULLS LAST, distance_meters ASC NULLS LAST);

CREATE TRIGGER trg_applications_updated_at
    BEFORE UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Biodata Standar Nasional (Dapodik & EMIS Ready)
CREATE TABLE applicant_profiles (
    application_id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    full_name TEXT NOT NULL,
    nik TEXT,
    kk_number TEXT,
    kk_issued_date DATE,
    nisn TEXT,
    birth_place TEXT,
    birth_date DATE,
    gender gender_type,
    religion TEXT,
    citizenship TEXT NOT NULL DEFAULT 'WNI',
    child_order SMALLINT CHECK (child_order IS NULL OR child_order > 0),
    siblings_count SMALLINT CHECK (siblings_count IS NULL OR siblings_count >= 0),
    special_needs_code TEXT,
    address_street TEXT,
    rt TEXT,
    rw TEXT,
    hamlet TEXT,
    village_code TEXT,
    village_name TEXT,
    district_code TEXT,
    district_name TEXT,
    regency_code TEXT,
    regency_name TEXT,
    province_code TEXT,
    province_name TEXT,
    postal_code TEXT,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    prev_school_name TEXT,
    prev_school_npsn TEXT,
    graduation_year SMALLINT,
    ijazah_number TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_profile_nik_len CHECK (nik IS NULL OR nik ~ '^[0-9]{16}$'),
    CONSTRAINT chk_profile_kk_len CHECK (kk_number IS NULL OR kk_number ~ '^[0-9]{16}$'),
    CONSTRAINT chk_profile_nisn_len CHECK (nisn IS NULL OR nisn ~ '^[0-9]{10}$')
);

CREATE INDEX idx_applicant_profiles_lookup
    ON applicant_profiles (tenant_id, nisn)
    WHERE nisn IS NOT NULL;

CREATE INDEX idx_applicant_profiles_nik
    ON applicant_profiles (tenant_id, nik)
    WHERE nik IS NOT NULL;

CREATE TRIGGER trg_applicant_profiles_updated_at
    BEFORE UPDATE ON applicant_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Data Orang Tua / Wali (Dapodik & EMIS Ready)
CREATE TABLE applicant_guardians (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    application_id UUID NOT NULL,
    relation guardian_relation NOT NULL,
    full_name TEXT NOT NULL,
    nik TEXT,
    birth_year SMALLINT CHECK (birth_year IS NULL OR birth_year BETWEEN 1920 AND 2025),
    education_level TEXT,
    occupation TEXT,
    income_bracket TEXT,
    phone_whatsapp TEXT,
    is_primary_contact BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (application_id, relation),
    CONSTRAINT chk_guardian_nik_len CHECK (nik IS NULL OR nik ~ '^[0-9]{16}$')
);

CREATE INDEX idx_applicant_guardians_app
    ON applicant_guardians (tenant_id, application_id);

CREATE TRIGGER trg_applicant_guardians_updated_at
    BEFORE UPDATE ON applicant_guardians
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Riwayat Prestasi / Piagam (Jalur Prestasi SPMB/PMBM/Beasiswa)
CREATE TABLE applicant_achievements (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    application_id UUID NOT NULL,
    category TEXT NOT NULL,
    title TEXT NOT NULL,
    level TEXT NOT NULL,
    rank_achieved TEXT,
    achievement_year SMALLINT,
    organizer TEXT,
    raw_points NUMERIC(6, 2) NOT NULL DEFAULT 0,
    verified_points NUMERIC(6, 2),
    verified_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_applicant_achievements_app
    ON applicant_achievements (tenant_id, application_id);

CREATE TRIGGER trg_applicant_achievements_updated_at
    BEFORE UPDATE ON applicant_achievements
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Dokumen Unggahan Privat (R2 / MinIO Object Keys, bukan Public URL)
CREATE TABLE applicant_documents (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    application_id UUID NOT NULL,
    requirement_id UUID NOT NULL,
    storage_key TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    status document_review_status NOT NULL DEFAULT 'pending',
    reviewer_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    review_note TEXT,
    reviewed_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, application_id) REFERENCES applications(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, requirement_id) REFERENCES track_requirements(tenant_id, id) ON DELETE RESTRICT,
    UNIQUE (application_id, requirement_id)
);

CREATE INDEX idx_applicant_documents_app
    ON applicant_documents (tenant_id, application_id, status);

CREATE TRIGGER trg_applicant_documents_updated_at
    BEFORE UPDATE ON applicant_documents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS applicant_documents;
DROP TABLE IF EXISTS applicant_achievements;
DROP TABLE IF EXISTS applicant_guardians;
DROP TABLE IF EXISTS applicant_profiles;
DROP TABLE IF EXISTS applications;

DROP TYPE IF EXISTS document_review_status;
DROP TYPE IF EXISTS guardian_relation;
DROP TYPE IF EXISTS gender_type;
DROP TYPE IF EXISTS application_status;

-- +goose StatementEnd

