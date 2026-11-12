# 08. API Contract & Integrasi Sistem

Spesifikasi formal seluruh endpoint REST API didokumentasikan dalam format **OpenAPI 3.1** pada **[openapi.yaml](file:///d:/source/My%20Project/PBDB/backend/api/openapi.yaml)** sebagai *single source of truth* antara backend Go (`chi`) dan frontend React TypeScript.

---

## 1. Konvensi Dasar API

* **Base Path:** `/api/v1` (ditambah `/healthz` dan `/internal/tls/verify` di root server).
* **Resolusi Tenant (*Multi-Tenant Host Routing*):**
  * Di **Production**, middleware Go membaca header `Host` (mis. `spmb.alhamid.sch.id` atau `alhamid.platform.id`) untuk menentukan `tenant_id` secara otomatis.
  * Di **Local Development**, pengembang dapat menyertakan header opsional `X-Tenant-Slug` saat mengakses `http://localhost:8080`.
* **Autentikasi & Otorisasi:**
  * Menggunakan `Authorization: Bearer <access_token>` (JWT berumur pendek + mekanisme `POST /api/v1/auth/refresh` memakai *refresh token* di `user_sessions`).
  * Pendaftar/orang tua masuk melalui **OTP WhatsApp** (`/auth/otp/request` + `/auth/otp/verify`) atau Password/PIN; staf & panitia masuk via Email + Password (`/auth/login`).
* **Format Amplop Respons:**
  * **Sukses:** `{ "data": <object|array>, "meta": { ... } }`
  * **Error:** `{ "error": { "code": "<ERROR_CODE>", "message": "<pesan ramah pengguna>", "fields": { "<field>": "<detail>" } } }`
* **Idempotensi & Konkurensi (*Optimistic Locking*):**
  * Endpoint mutasi krusial (`POST /applications`, `submit`, `payments`, `publish-results`) mendukung header `Idempotency-Key` (UUID) agar percobaan ulang saat jaringan HP putus tidak menghasilkan pendaftaran/tagihan ganda.
  * Perubahan status aplikasi memeriksa field `version` untuk mencegah benturan edit bersamaan (`409 Conflict`).

---

## 2. Alur Khusus Integrasi

### A. Verifikasi Custom Domain untuk Caddy (*On-Demand TLS*)
Sebelum Caddy menerbitkan sertifikat SSL Let's Encrypt untuk domain baru yang mengarah ke server, Caddy memanggil:
```text
GET /internal/tls/verify?domain=spmb.sekolahabc.sch.id
```
Backend mengecek tabel `tenants` (hanya mengembalikan `200 OK` jika domain terdaftar dan aktif; selain itu `403 Forbidden`) sehingga server terlindung dari penyalahgunaan kuota sertifikat SSL oleh domain liar.

### B. Unggah & Unduh Berkas Privat (*No Public URLs*)
1. Browser mengompresi foto KK/Akta/Ijazah di sisi client terlebih dahulu.
2. Frontend memanggil `POST /api/v1/applicant/applications/{id}/documents/presign-upload` untuk memperoleh URL unggah sementara ke **Cloudflare R2 / MinIO**.
3. Browser mengunggah file **langsung ke Object Storage** (bebas beban bandwidth server Go).
4. Frontend memanggil `POST .../documents/confirm` untuk mencatat metadata dan `storage_key` di PostgreSQL.
5. Untuk melihat dokumen (baik di portal wali maupun *Split-Screen Verification Workbench* panitia), frontend memanggil endpoint `presign-download` yang memverifikasi `tenant_id` dan peran RBAC sebelum menerbitkan URL sementara berumur pendek.

### C. Antrean Verifikasi Panitia (*Zero Double-Claim*)
Saat verifikator menekan tombol *"Ambil Pendaftar Berikutnya"*, endpoint `POST /api/v1/admin/applications/claim-next` mengeksekusi query PostgreSQL `FOR UPDATE SKIP LOCKED` agar dua verifikator yang bekerja bersamaan selalu mendapat pendaftar berbeda.

---

## 3. Ringkasan Kelompok Endpoint

| Kelompok | Prefix | Aktor Utama |
| :--- | :--- | :--- |
| **Infra & TLS** | `/healthz`, `/internal/tls/verify` | Caddy, Uptime Monitor |
| **Portal Publik** | `/api/v1/public/*` | Publik *(tanpa login, rate-limited)* |
| **Autentikasi** | `/api/v1/auth/*`, `/api/v1/me` | Semua Pengguna |
| **Portal Pendaftar** | `/api/v1/applicant/*` | Orang Tua / Wali / Calon Murid |
| **Pengaturan Lembaga** | `/api/v1/admin/tenant/*`, `/units`, `/periods`, `/tracks` | Admin Yayasan / Unit Admin |
| **Verifikasi Berkas** | `/api/v1/admin/applications/*` | Verifikator / Unit Admin |
| **Keuangan** | `/api/v1/admin/payments/*` | Bendahara / Unit Admin |
| **Seleksi & Kelulusan** | `/api/v1/admin/applications/{id}/scores`, `/simulate-ranking`, `/publish-results` | Tim Penguji / Unit Admin / Kepala Sekolah |
| **Ekspor & Audit** | `/api/v1/admin/exports/*`, `/api/v1/admin/audit-logs` | Operator / Pimpinan |
| **Payment Webhook** | `/api/v1/webhooks/payments/{provider}` | Payment Gateway *(Signature Verified)* |

Detail skema *request body*, *response*, parameter query, dan enumerasi lengkap tersedia pada **[openapi.yaml](file:///d:/source/My%20Project/PBDB/backend/api/openapi.yaml)**.

