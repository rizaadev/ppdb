# 07. Skema Database PostgreSQL & Konvensi `sqlc`

Dokumen ini menjelaskan struktur tabel **PostgreSQL**, strategi isolasi *multi-tenant*, tipe data performa tinggi, serta cara pengelolaan migrasi **`goose`** dan query **`sqlc`**.

---

## 1. Keputusan Teknis Utama

| Aspek | Pilihan | Alasan |
| :--- | :--- | :--- |
| **Primary Key** | `UUID` (UUIDv7 dari Go) | Mencegah penebakan ID berurutan pada URL publik/pendaftar sekaligus menjaga indeks B-Tree tetap berurutan (minim fragmentasi). |
| **Isolasi Multi-Tenant** | Composite FK `(tenant_id, id)` + filter wajib di `sqlc` | Tabel anak merujuk `FOREIGN KEY (tenant_id, ...)` sehingga kebocoran relasi lintas sekolah mustahil terjadi di level database, sekaligus aman untuk `pgxpool` & River worker tanpa ketergantungan session variable RLS. |
| **Nominal Uang** | `BIGINT` (Rupiah utuh) | Bebas galat pembulatan *floating-point* pada transaksi pembayaran dan laporan keuangan. |
| **Koordinat Domisili** | `DOUBLE PRECISION` + Haversine | Hemat memori tanpa perlu memasang ekstensi PostGIS pada VPS berkapasitas kecil. |
| **Waktu** | `TIMESTAMPTZ` (UTC) | Konsisten untuk wilayah WIB/WITA/WIT melalui kolom `units.timezone`. |
| **Job Queue** | **River** (di PostgreSQL yang sama) | Mendukung *transactional enqueue* (status pendaftar + antrean notifikasi WA masuk dalam 1 transaksi DB yang sama). |

---

## 2. Daftar Migrasi SQL (`backend/sql/migrations/`)

Seluruh migrasi menggunakan format **`goose`** (`-- +goose Up` dan `-- +goose Down`):

1. **[00001_tenants_units_auth.sql](file:///d:/source/My%20Project/PBDB/backend/sql/migrations/00001_tenants_units_auth.sql)**
   * `tenants`: Pengaturan *white-label* (slug, *custom domain*, preset terminologi `spmb`/`pmbm`/`mandiri`, overrides label, tema warna, paket langganan).
   * `units`: Satuan jenjang di bawah tenant (`ra_tk`, `sd_mi`, `smp_mts`, `sma_ma`, `smk_mak`, `pesantren`), NPSN/NSM, koordinat sekolah, timezone.
   * `users`, `user_tenant_roles`: Akun pengguna (pendaftar & staf) beserta pembagian peran RBAC per tenant atau per unit (`tenant_admin` wajib `unit_id IS NULL`).
   * `otp_challenges`, `user_sessions`: Verifikasi OTP WhatsApp berbasis hash dan manajemen *refresh token*.

2. **[00002_admissions_and_tracks.sql](file:///d:/source/My%20Project/PBDB/backend/sql/migrations/00002_admissions_and_tracks.sql)**
   * `admission_periods`: Tahun ajaran & gelombang pendaftaran per unit, jadwal buka/tutup, jadwal pengumuman, batas daftar ulang, serta penghitung nomor urut `registration_seq`.
   * `admission_tracks`: Jalur pendaftaran, kuota, jalur tujuan limpahan sisa kuota (`spillover_target_track_id`), biaya formulir & daftar ulang (`BIGINT`), batas usia, penanda wajib koordinat/prestasi, bobot skor, dan skema formulir tambahan (`custom_form_schema` `JSONB`).
   * `track_requirements`: Syarat dokumen dinamis per jalur (kode berkas, mime-type, ukuran maksimal, dan aturan khusus seperti `min_kk_issue_months`).

3. **[00003_applications_profiles_docs.sql](file:///d:/source/My%20Project/PBDB/backend/sql/migrations/00003_applications_profiles_docs.sql)**
   * `applications`: Transaksi inti pendaftaran dengan enum `application_status` (17 status *state machine*), penghitung versi (*optimistic locking*), skor akhir, jarak meter, peringkat, dan `extra_answers` `JSONB`.
   * `applicant_profiles`: Biodata standar **Dapodik & EMIS** (NIK 16 digit, No. KK & tanggal terbit, NISN 10 digit, tempat/tanggal lahir, jenis kelamin `L`/`P`, kode kebutuhan khusus, alamat lengkap berbasis kode wilayah Kemendagri, koordinat, dan sekolah asal).
   * `applicant_guardians`: Data Ayah, Ibu, dan Wali sesuai kolom standar Dapodik/EMIS.
   * `applicant_achievements`: Daftar piagam/prestasi untuk jalur prestasi/beasiswa.
   * `applicant_documents`: Metadata berkas unggahan privat (menyimpan `storage_key` R2/MinIO, status verifikasi per berkas, dan catatan revisi panitia).

4. **[00004_payments_assessments_audit.sql](file:///d:/source/My%20Project/PBDB/backend/sql/migrations/00004_payments_assessments_audit.sql)**
   * `payment_transactions`: Tagihan formulir & daftar ulang (mendukung QRIS/VA/E-Wallet otomatis maupun unggah bukti transfer manual).
   * `assessment_schedules` & `assessment_scores`: Jadwal ujian/wawancara dan skor per komponen penilaian.
   * `status_audit_logs`: Log permanen (*append-only*) seluruh perubahan status, revisi berkas, verifikasi pembayaran, dan penetapan nilai.
   * `notification_logs` & `export_jobs`: Pelacakan pengiriman notifikasi WA/Email serta status pekerjaan ekspor Dapodik/EMIS.

---

## 3. Organisasi Query `sqlc` (`backend/sql/`)

Konfigurasi di **[sqlc.yaml](file:///d:/source/My%20Project/PBDB/backend/sqlc.yaml)** menggunakan `pgx/v5` dengan pemetaan tipe eksplisit (`uuid` ke `github.com/google/uuid.UUID`, `timestamptz`/`date` ke `time.Time`, `jsonb` ke `encoding/json.RawMessage`).

| File Query | Fokus Operasi |
| :--- | :--- |
| [tenants.sql](file:///d:/source/My%20Project/PBDB/backend/sql/queries/tenants.sql) | Resolusi tenant via slug/*custom domain*, verifikasi TLS Caddy, CRUD unit. |
| [auth.sql](file:///d:/source/My%20Project/PBDB/backend/sql/queries/auth.sql) | User CRUD, OTP WhatsApp, sesi refresh token, dan lookup peran RBAC. |
| [admissions.sql](file:///d:/source/My%20Project/PBDB/backend/sql/queries/admissions.sql) | Daftar periode & jalur publik, statistik kuota, atomic increment `registration_seq`, syarat jalur. |
| [applications.sql](file:///d:/source/My%20Project/PBDB/backend/sql/queries/applications.sql) | Buat pendaftaran, auto-save biodata/orang tua/prestasi/dokumen, dan transisi status *optimistic locking*. |
| [verifications.sql](file:///d:/source/My%20Project/PBDB/backend/sql/queries/verifications.sql) | Daftar pendaftar dengan *keyset cursor pagination*, klaim antrean atomik (`FOR UPDATE SKIP LOCKED`), review per berkas. |
| [payments.sql](file:///d:/source/My%20Project/PBDB/backend/sql/queries/payments.sql) | Pembuatan tagihan, lampiran bukti transfer manual, verifikasi bendahara/webhook, dan kedaluwarsa otomatis. |
| [assessments_ranking.sql](file:///d:/source/My%20Project/PBDB/backend/sql/queries/assessments_ranking.sql) | Jadwal tes, input nilai penguji, hitung ulang `final_score`, simulasi peringkat `ROW_NUMBER()`, dan kunci kelulusan. |
| [exports_audit.sql](file:///d:/source/My%20Project/PBDB/backend/sql/queries/exports_audit.sql) | Penulisan & pembacaan `status_audit_logs`, log notifikasi, antrean `export_jobs`, serta query gabung murid `enrolled` untuk ekspor **Dapodik & EMIS**. |

---

## 4. Perintah Pengelolaan Skema (Saat Tooling Terpasang)

```bash
# Menjalankan migrasi database (contoh dengan goose)
goose -dir backend/sql/migrations postgres "$DATABASE_URL" up

# Memvalidasi dan menghasilkan kode Go type-safe dari sqlc
cd backend
sqlc vet
sqlc generate
```

