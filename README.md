# Platform Penerimaan Murid Baru (SPMB / PMBM / Mandiri)

> **Status Proyek:** Implementasi MVP Multi-Tenant End-to-End & Konsol Superadmin Platform (`/platform`)  
> **Tech Stack:** Go (`chi` + `pgx`) · PostgreSQL · React (TypeScript + Vite SPA + Vitest) · MinIO / S3 · Caddy

Platform SaaS penerimaan peserta didik / santri baru (*multi-tenant*, *multi-unit*, *multi-jalur*) yang mendukung **SPMB Sekolah**, **PMBM Madrasah**, dan **Mandiri / Yayasan / Pondok Pesantren**.

---

## Fitur Utama

1. **Multi-Tenant & Isolasi Lembaga**
   - Resolusi tenant berbasis host/subdomain maupun header `X-Tenant-Slug` (`demo-spmb`, `demo-pmbm`, `demo-mandiri`), lengkap dengan terminologi adaptif (`spmb`, `pmbm`, `mandiri`) dan tema warna instansi.
   - Verifikasi custom domain via endpoint internal Caddy On-Demand TLS (`/internal/tls/verify`).
2. **Konsol Superadmin Platform (`/platform`)**
   - Login khusus `is_platform_admin` (`/api/v1/platform/auth/login`) yang dapat diakses langsung di domain utama tanpa subdomain tenant.
   - Daftar seluruh tenant, onboarding instansi baru beserta unit pendidikan awal dan akun `tenant_admin` awal, pembaruan paket/branding/verifikasi custom domain, serta suspend/aktivasi tenant.
3. **Alur Pendaftar & Orang Tua (`/app`)**
   - Autentikasi OTP WhatsApp dan Email/Password dengan RBAC & sesi refresh token.
   - Wizard pendaftaran 6 langkah: Biodata Murid (standar Dapodik/EMIS), Orang Tua / Wali, Prestasi & Piagam, Alamat & Koordinat Domisili (peta interaktif Leaflet/OSM), Unggah Berkas (*presigned URL* S3/MinIO + kompresi klien), serta Tinjau & Kirim Final.
   - Cetak Kartu Tanda Pendaftaran dan Surat Keterangan Hasil Seleksi / Diterima langsung dari dashboard.
4. **Workspace Panitia, Bendahara, Penguji, Kepala Sekolah & Admin (`/admin`)**
   - Workbench Verifikasi Berkas & klaim antrean (`claim-next`), permintaan revisi berkas, pencatatan & verifikasi pembayaran manual/gateway (`treasurer`), input nilai tes/wawancara (`examiner`), simulasi pemeringkatan kuota-afirmasi/prestasi/domisili & publikasi hasil (`principal`), manajemen unit/periode/jalur (`tenant_admin`), serta ekspor `dapodik_xlsx`, `emis_xlsx`, `custom_csv` & audit log.

---

## Struktur Direktori

- `backend/` — Server API Go, skema migrasi SQL, penyimpanan *in-memory demo store* & *PostgreSQL store*, generator ekspor XLSX/CSV, dan distribusi SPA statis di `backend/web/dist/`.
- `frontend/` — Aplikasi SPA React + TypeScript + Vite dengan CSS Modules, `npm test` (Vitest + React Testing Library).
- `deploy/` — `docker-compose.yml` (PostgreSQL, MinIO, `minio-init` pembentuk bucket privat `pbdb-private-docs`, Backend, Caddy), `Dockerfile` *multi-stage*, dan `Caddyfile`.
- `docs/` — Dokumentasi rancangan skema database dan kontrak API.
- `.github/workflows/ci.yml` — Pipeline CI untuk build & test frontend, `go vet` & `go test` backend, serta verifikasi build image Docker.

---

## Akun Demo Bawaan

Semua akun demo di bawah memakai password yang sama: **`DemoPass123!`**

| Kredensial | Peran (`Role`) | Cakupan |
|---|---|---|
| `+6281234567890` / `wali@demo.id` | Wali Murid / Pendaftar | Portal pendaftar (`/app`) |
| `verifikator@smaharapan.sch.id` | `verifier` | `demo-spmb` |
| `admin@alhamid.sch.id` | `tenant_admin` | `demo-spmb`, `demo-pmbm`, `demo-mandiri` |
| `bendahara@alhamid.sch.id` | `treasurer` | `demo-spmb`, `demo-pmbm`, `demo-mandiri` |
| `penguji@smaharapan.sch.id` | `examiner` | `demo-spmb`, `demo-pmbm`, `demo-mandiri` |
| `kepsek@smaharapan.sch.id` | `principal` | `demo-spmb`, `demo-pmbm`, `demo-mandiri` |
| `platform@spmb-platform.id` | `is_platform_admin` | Konsol Superadmin (`/platform`) |

---

## Cara Menjalankan & Pengujian

### 1. Mode Development Lokal (Tanpa PostgreSQL Eksternal)

Saat `DATABASE_URL` kosong di mode development, backend otomatis menggunakan *demo store* in-memory beserta data awal 3 tenant (`demo-spmb`, `demo-pmbm`, `demo-mandiri`) dan akun platform superadmin.

```bash
# Terminal 1 — Backend API (port default :8080)
cd backend
go run ./cmd/server

# Terminal 2 — Frontend Vite Dev Server (port :5173, proxy /api ke :8080)
cd frontend
npm install
npm run dev
```

### 2. Build & Pengujian Otomatis

```bash
# Frontend: Vitest + React Testing Library & TypeScript + Vite build
cd frontend
npm test
npm run build

# Backend: Static check & seluruh test suite Go
cd ../backend
go vet ./...
go test -count=1 ./...
```

> Catatan: Ketika membangun frontend untuk disajikan langsung oleh binary Go (`backend/web/dist`), salin hasil `frontend/dist/*` ke `backend/web/dist/`.

### 3. Deployment Docker Compose (PostgreSQL + MinIO + Backend + Caddy)

1. Salin `.env.example` menjadi `.env` di root proyek dan atur secret produksi (`JWT_SECRET`, `OTP_SECRET`, `PAYMENT_WEBHOOK_SECRET`).
2. Pastikan konfigurasi endpoint S3/MinIO memisahkan jalur internal container dan URL publik browser:
   - `S3_ENDPOINT=http://minio:9000` (komunikasi worker/backend ke MinIO di jaringan Docker)
   - `S3_PUBLIC_ENDPOINT=http://localhost:9000` (atau origin publik S3/MinIO yang dapat diakses browser untuk *presigned URL* upload/download)
3. Atur `MINIO_API_CORS_ALLOW_ORIGIN`, `DB_AUTO_MIGRATE`, dan `DB_SEED_DEMO` sesuai kebutuhan lingkungan di `deploy/docker-compose.yml` atau `.env`.
4. Jalankan seluruh layanan:

```bash
docker compose -f deploy/docker-compose.yml up --build
```
