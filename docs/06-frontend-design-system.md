# 06. Sistem Desain Frontend, Tata Letak & Standar UI/UX

Dokumen ini menjadi panduan resmi desain antarmuka, arsitektur CSS, tipografi, tata letak responsif, dan pola interaksi untuk frontend **React + Vite + TypeScript (SPA)**. Seluruh aturan dirancang agar tampilan platform **konsisten, profesional, ringan di jaringan seluler, serta netral** untuk digunakan oleh sekolah umum (**SPMB**), madrasah (**PMBM**), maupun yayasan/pesantren (**Mandiri**).

---

## 1. Filosofi Visual: *"Civic Academic Clean"*

Tampilan platform mengusung karakter **"Civic Academic Clean"**: resmi dan menenangkan seperti portal pendidikan tepercaya, tetapi modern, bersih, dan jauh lebih mudah dipakai daripada portal web pemerintah konvensional.

### Prinsip Dasar
1. **Netral di Struktur, Fleksibel di Aksen (*90% Netral, 10% Brand*):** Latar dan elemen struktural menggunakan palet abu-abu netral (*Slate*) dengan garis tepi tipis (*hairline borders*), sementara warna identitas masing-masing sekolah/madrasah diterapkan secara terkendali pada aksi utama dan sorotan aktif.
2. **Mode Terang (*Light Mode*) Kontras Tinggi di Fase 1:** Seluruh permukaan di Fase 1 berfokus pada *Light Mode* untuk menjamin keterbacaan maksimal di ponsel saat berada di luar ruangan dan keakuratan visual panitia saat memeriksa foto dokumen KK, Akta, atau Ijazah. Struktur token tetap dipersiapkan agar ekspansi *Dark Mode* di Fase 2 dapat dilakukan tanpa mengubah komponen.
3. **Custom CSS Terstruktur Tanpa Framework Utility Berat:** Gaya dibangun menggunakan **CSS Custom Properties (Design Tokens) + CSS Modules** tanpa Tailwind CSS. Pendekatan ini menjaga ukuran bundle JS/CSS sekecil mungkin, memudahkan audit gaya per komponen, dan bebas risiko konflik kelas.
4. **Satu Sistem, Dua Orientasi Ergonomi:**
   * **Portal Publik & Pendaftar:** *Mobile-First* (dirancang untuk kenyamanan ibu jari di layar HP 360–412px).
   * **Workspace Panitia & Yayasan:** *Desktop-First Responsive* (dirancang untuk produktivitas verifikasi massal, tabel padat, dan multi-kolom).

---

## 2. Sistem Warna & *White-Label Theme Engine*

### A. Token Permukaan & Teks (*Surface & Neutral Tokens* — Tetap)

```css
:root,
[data-theme="light"] {
  /* Permukaan (Surfaces) */
  --bg-canvas: #f8fafc;        /* Latar halaman utama */
  --bg-surface: #ffffff;       /* Kartu, modal, panel form */
  --bg-subtle: #f1f5f9;        /* Header tabel, area input readonly, hover ringan */
  --bg-muted: #e2e8f0;         /* Pembatas kuat, track progress bar */

  /* Garis Tepi (Borders) */
  --border-default: #e2e8f0;
  --border-strong: #cbd5e1;

  /* Teks */
  --text-primary: #0f172a;     /* Judul, isi utama, nilai input */
  --text-secondary: #475569;   /* Label form, deskripsi */
  --text-muted: #64748b;       /* Placeholder, timestamp, metadata */
  --text-disabled: #94a3b8;
}
```

### B. Token Brand Dinamis per Tenant (*White-Label Brand Tokens*)

Saat portal dimuat berdasarkan domain/slug sekolah, *Theme Provider* menyuntikkan variabel warna instansi ke `:root`:

```css
:root {
  --brand-primary: #1e3a5f;         /* Default Netral: Academic Navy */
  --brand-primary-hover: #172e4d;   /* Lebih gelap 10-12% untuk state hover */
  --brand-tint: #eff4fa;            /* Latar sorotan lembut (~8% opasitas) */
  --brand-border: #bfdbfe;          /* Border fokus/kartu terpilih */
  --brand-foreground: #ffffff;      /* Teks di atas --brand-primary */
}
```

* **Contoh Preset Referensi per Segmen:**
  * **Netral Default:** `#1E3A5F` (*Academic Navy*)
  * **Preset SPMB (Sekolah Umum):** `#1D4ED8` (*Civic Blue*)
  * **Preset PMBM (Madrasah):** `#047857` (*Madrasah Emerald*)
  * **Preset Mandiri / Yayasan:** `#4338CA` (*Deep Indigo*) atau `#7C2D12` (*Warm Terracotta*)
* **Aturan Kontras WCAG AA:** Pada halaman pengaturan tema sekolah, sistem menghitung rasio kontras warna brand terhadap putih/hitam. `--brand-foreground` otomatis memilih `#FFFFFF` atau `#0F172A`, dan warna yang terlalu pucat (rasio kontras < `4.5:1`) ditolak atau diberi peringatan di pengaturan tenant.

### C. Token Status Semantik (*Dikunci — Tidak Mengikuti Warna Sekolah*)

Warna status *state machine* **tidak boleh diubah** oleh warna brand tenant agar arti visual tetap seragam bagi siapa pun:

| Kelompok Status | Token Teks / Border | Token Latar | Status *State Machine* Terkait |
| :--- | :--- | :--- | :--- |
| **Netral / Draft** | `--status-neutral-fg: #475569` | `--status-neutral-bg: #f1f5f9` | `ACCOUNT_CREATED`, `TRACK_SELECTED`, `DRAFT_FORM` |
| **Dalam Proses** | `--status-info-fg: #0369a1` | `--status-info-bg: #e0f2fe` | `SUBMITTED`, `UNDER_VERIFICATION`, `SELECTION_IN_PROGRESS`, `RANKED` |
| **Perlu Tindakan** | `--status-warning-fg: #b45309` | `--status-warning-bg: #fef3c7` | `AWAITING_PAYMENT`, `REVISION_REQUIRED`, `WAITLISTED`, `RE_ENROLLMENT` |
| **Berhasil / Lulus** | `--status-success-fg: #047857` | `--status-success-bg: #d1fae5` | `PAYMENT_VERIFIED`, `VERIFIED`, `ACCEPTED`, `ENROLLED` |
| **Gagal / Batal** | `--status-danger-fg: #be123c` | `--status-danger-bg: #ffe4e6` | `REJECTED`, `WITHDRAWN` |

---

## 3. Tipografi & Keterbacaan Data

### A. Pasangan Font
* **Font Utama (`--font-sans`):** `'Plus Jakarta Sans', 'Inter', system-ui, -apple-system, sans-serif` — bentuk huruf terbuka dan sangat mudah dibaca pada formulir panjang berbahasa Indonesia.
* **Font Data & Kode (`--font-mono`):** `'JetBrains Mono', ui-monospace, SFMono-Regular, monospace` — digunakan khusus untuk **Nomor Pendaftaran, NIK, No. KK, NISN, Kode Bayar/VA, dan Token OTP** agar karakter `0`/`O` dan `1`/`I`/`l` tidak tertukar.
* **Self-Hosted Subset:** File font `.woff2` (latin subset) disajikan lokal bersama aset SPA dengan `font-display: swap` agar tampilan langsung terbaca tanpa menunggu CDN eksternal.

### B. Skala Tipografi (`tokens.css`)

| Token | Ukuran | Line Height | Bobot | Penggunaan |
| :--- | :--- | :--- | :--- | :--- |
| `--text-hero` | `clamp(1.75rem, 4vw, 2.5rem)` | `1.15` | `700` | Judul utama halaman portal publik sekolah |
| `--text-h1` | `1.5rem` (`24px`) | `1.25` | `700` | Judul halaman dashboard / form wizard |
| `--text-h2` | `1.25rem` (`20px`) | `1.3` | `600` | Judul section & modal utama |
| `--text-h3` | `1.0625rem` (`17px`) | `1.35` | `600` | Judul kartu, sub-bagian formulir |
| `--text-body` | `0.9375rem` (`15px`) | `1.55` | `400` | Teks umum, paragraf, isi catatan verifikator |
| `--text-label` | `0.875rem` (`14px`) | `1.4` | `500` | Label field input, item menu navigasi |
| `--text-caption` | `0.8125rem` (`13px`) | `1.4` | `400` | Teks bantuan (*helper text*), metadata tabel |
| `--text-badge` | `0.75rem` (`12px`) | `1.2` | `600` | Badge status, tag jalur *(ukuran minimum teks)* |

### C. Aturan Wajib Tipografi
1. **Pencegahan Auto-Zoom Mobile:** Seluruh elemen `<input>`, `<select>`, dan `<textarea>` pada layar `< 768px` wajib memiliki ukuran font minimal **`16px` (`1rem`)** agar iOS Safari dan browser mobile tidak melakukan zoom paksa saat pengguna mengetik.
2. **Angka Sejajar (`tabular-nums`):** Kelas utilitas `.tabular-nums` (`font-variant-numeric: tabular-nums`) wajib diterapkan pada kuota daya tampung, jarak koordinat, skor/nilai, hitung mundur timer, dan nominal Rupiah.
3. **Batas Lebar Teks Bacaan:** Paragraf pengumuman, syarat ketentuan, dan FAQ dibatasi maksimal `65ch` agar mata tidak lelah membaca baris terlalu panjang di monitor lebar.

---

## 4. Arsitektur Custom CSS & Sistem Spasi

### A. Struktur File CSS di `frontend/src/`

```text
src/
├── styles/
│   ├── reset.css        # Modern box-sizing, margin reset, media defaults
│   ├── tokens.css       # Variabel warna, tipografi, spasi, radius, z-index, shadow
│   ├── base.css         # Gaya elemen dasar, :focus-visible, .sr-only, .tabular-nums
│   └── layouts.css      # Aturan grid & kontainer 3 permukaan utama
└── components/
    └── ui/
        ├── Button.tsx
        └── Button.module.css   # CSS Modules terisolasi per komponen
```

> **Aturan Konsistensi Gaya:** Komponen tidak boleh menulis nilai warna hex, ukuran radius, atau shadow secara *hardcoded* di dalam `*.module.css`. Semua nilai wajib mengacu ke CSS Custom Properties dari `tokens.css`.

### B. Skala Spasi, Radius & Elevasi
* **Spasi (Kelipatan `4px`):** `--space-1` (`4px`), `--space-2` (`8px`), `--space-3` (`12px`), `--space-4` (`16px`), `--space-6` (`24px`), `--space-8` (`32px`), `--space-12` (`48px`).
* **Radius Sudut:** `--radius-sm: 6px` (badge/tag), `--radius-md: 8px` (input/button), `--radius-lg: 12px` (card/panel), `--radius-xl: 16px` (modal/sheet), `--radius-pill: 9999px`.
* **Elevasi (*Subtle Depth*):** Mengutamakan `1px solid var(--border-default)` daripada bayangan tebal:
  * `--shadow-card`: `0 1px 2px rgba(15, 23, 42, 0.05)`
  * `--shadow-popover`: `0 8px 24px -4px rgba(15, 23, 42, 0.10)`
  * `--shadow-modal`: `0 20px 40px -12px rgba(15, 23, 42, 0.16)`

---

## 5. Arsitektur 3 Permukaan & Layout Responsif (Mobile vs Desktop)

### A. Breakpoint Standar

```css
/* Mobile default: < 640px (single-column, thumb-friendly) */
/* Tablet kecil: >= 640px */
/* Tablet / Laptop kecil: >= 768px */
/* Desktop / Admin Workbench: >= 1024px */
/* Layar Lebar: >= 1280px */
```

### B. Permukaan 1: Portal Publik Sekolah (`/` atau Custom Domain)
Dirancang agar orang tua langsung memahami **apakah pendaftaran sedang buka, jalur apa saja yang tersedia, berkas apa yang perlu disiapkan, dan cara cek status**.

* **Header:** Logo instansi + nama sekolah, tautan loncat ke *Jadwal*, *Jalur & Kuota*, *Cek Status*, dan tombol **Masuk / Daftar**.
* **Hero Ringkas:** Status penerimaan real-time (mis. `Pendaftaran Gelombang 1 Dibuka`), rentang tanggal, serta dua tombol utama (*Mulai Mendaftar* & *Cek Status*).
* **Kartu Jalur & Syarat Dinamis:** Grid 1 kolom (mobile) → 2–4 kolom (desktop) menampilkan jalur sesuai preset aktif (**SPMB / PMBM / Mandiri**) lengkap dengan ringkasan kuota dan daftar dokumen wajib.
* **Sticky Mobile Action Bar (`< 768px`):** Di layar HP, bar bawah tetap menempel (*fixed bottom*) berisi tombol cepat **Cek Status** dan **Daftar Sekarang** agar pengguna tidak perlu menggulir balik ke atas.

### C. Permukaan 2: Portal Pendaftar / Orang Tua (`/app/*`)
Ergonomi fokus pada **minimnya kesalahan pengisian** dan **kemudahan akses satu tangan di ponsel** (`max-width: 720px` terpusat):

```text
Mobile (< 768px)                       Desktop (>= 768px)
┌───────────────────────────┐          ┌──────────────────────────────────────────┐
│ [Logo] Anak: Ahmad ▼ [?]  │          │ [Logo] Portal Pendaftar    Ahmad ▼ [Akun]│
├───────────────────────────┤          ├──────────────┬───────────────────────────┤
│ ●──●──○──○──○             │          │ Langkah:     │                           │
│ Tahap 2/5: Upload Berkas  │          │ 1. Biodata ✔ │  [Form Card max-w 720px]  │
│ [Disimpan otomatis 14:02] │          │ 2. Ortu    ✔ │  Grid 2 kolom utk field   │
├───────────────────────────┤          │ 3. Alamat  ● │  pendek (TTL, RT/RW)      │
│                           │          │ 4. Berkas  ○ │                           │
│ [Form 1 Kolom Penuh]      │          │ 5. Review  ○ │  [Peta / Upload Berkas]   │
│ Input min 16px, tinggi    │          └──────────────┴───────────────────────────┤
│ tombol/field >= 44px      │                         [Kembali] [Simpan] [Lanjut] │
├───────────────────────────┤          └──────────────────────────────────────────┘
│ [Kembali] [Lanjut →]      │ (Sticky Bottom Bar)
└───────────────────────────┘
```

* **Multi-Child Switcher:** Satu akun orang tua bisa mendaftarkan beberapa anak (misal satu ke MI, satu ke MTs dalam satu yayasan) dan berpindah aplikasi lewat *dropdown* di header.
* **Banner Aksi Mendesak:** Jika status aplikasi `REVISION_REQUIRED` atau `AWAITING_PAYMENT`, kartu berwarna amber muncul paling atas di dashboard dengan tombol langsung menuju berkas/tagihan terkait.
* **Touch Target Minimal:** Seluruh tombol aksi, radio card pemilihan jalur, dan tombol unggah memiliki tinggi minimal `44px` (`--touch-target: 44px`).

### D. Permukaan 3: Workspace Panitia, Penguji, Keuangan & Yayasan (`/admin/*`)
Dioptimalkan untuk layar desktop (`>= 1024px`, kontainer fluida hingga `1440px`), tetap berfungsi rapi di tablet/mobile melalui *Slide-over Drawer* dan pergantian ke *Tab View*:

* **Shell Utama:**
  * **Sidebar Kiri:** Dapat diciutkan (`240px` ↔ `68px`). Di bagian atas terdapat **Unit Switcher** (khusus Admin Yayasan untuk berpindah antara jenjang SD/MI, SMP/MTs, SMA/MA). Pada layar `< 1024px`, sidebar tersembunyi di balik tombol menu dan tampil sebagai *Drawer*.
  * **Header Utilitas:** Breadcrumb lokasi halaman, kolom pencarian cepat (cari Nama / NISN / No. Pendaftaran), dan badge peran aktif (*Operator*, *Verifikator*, *Bendahara*, *Penguji*, *Pimpinan*).
* **Split-Screen Document Verification Workbench (`>= 1024px`):**

```text
┌───────────────────────────────┬───────────────────────────────┐
│ PANEL KIRI (55%): DOKUMEN     │ PANEL KANAN (45%): KEPUTUSAN  │
│ [KK] [Akta] [KIP] [Rapor]     │ Nama: Ahmad Fauzi (Jalur: ...)│
│ ┌───────────────────────────┐ │ ┌───────────────────────────┐ │
│ │                           │ │ │ Data Terketik vs Dokumen  │ │
│ │   Preview Gambar / PDF    │ │ │ NIK: 327501...            │ │
│ │   [Zoom +] [Zoom -] [↻]   │ │ │ Tgl Terbit KK: 10-02-2023 │ │
│ │                           │ │ └───────────────────────────┘ │
│ └───────────────────────────┘ │ Status KK: [✓ Sah] [✕ Revisi] │
│                               │ Catatan cepat:                │
│                               │ [Foto buram] [Terpotong]      │
│                               │ [Simpan & Lanjut Antrean →]   │
└───────────────────────────────┴───────────────────────────────┘
```

* Pada layar `< 1024px`, layout otomatis berubah menjadi dua tab penuh (**Tab 1: Lihat Berkas**, **Tab 2: Periksa & Keputusan**) agar panitia di tablet/HP tetap dapat memverifikasi tanpa himpitan kolom.
* **Pola Data Responsif:** Tabel pendaftar, tabel nilai tes, dan rekap transaksi keuangan memakai **Dense Sticky-Header Table** di desktop dan berubah menjadi **Stacked Cards** di layar `< 768px`.

---

## 6. Peta Domisili: Integrasi Leaflet + OpenStreetMap

Untuk mendukung **Jalur Domisili (SPMB)** maupun pemetaan jarak tempat tinggal tanpa biaya lisensi API pihak ketiga:

* **Stack Peta:** **Leaflet** dengan *tile layer* **OpenStreetMap**.
* **Optimasi Bundle (*Lazy Load*):** Pustaka `leaflet` dan file CSS-nya dimuat secara dinamis (`React.lazy` / dynamic `import()`) **hanya** saat pendaftar membuka langkah alamat pada jalur yang mengaktifkan koordinat, atau saat panitia membuka panel lokasi pendaftar. Portal pendaftaran jalur non-koordinat tidak terbebani ukuran library peta sama sekali.
* **Alur UX untuk Orang Tua di Ponsel:**
  1. Menyediakan tombol utama **"Deteksi Lokasi Rumah Saya (GPS)"** dengan pesan izin lokasi yang jelas.
  2. Menyediakan kotak pencarian nama jalan/kelurahan (geocoding opsional) + penanda pin yang dapat digeser (*draggable marker*) untuk koreksi manual.
  3. Menampilkan indikator estimasi jarak garis lurus (*Haversine*) ke titik koordinat sekolah secara informatif beserta catatan bahwa **perhitungan jarak resmi ditetapkan oleh sistem server**.
  4. Jika GPS gagal atau izin ditolak browser, form tidak macet—pengguna tetap dapat mencari wilayah atau menggeser pin secara manual di peta.

---

## 7. Pola Interaksi, Konsistensi Komponen & Performa

### A. Kamus Istilah Dinamis (`useTerminology`)
Seluruh komponen UI dilarang menulis istilah spesifik seperti *"PPDB"*, *"Peserta Didik"*, *"Madrasah"*, atau *"Santri"* secara statis di teks tombol/judul. Komponen wajib mengambil label dari *Terminology Context* yang memetakan preset **SPMB / PMBM / Mandiri** (mengacu pada tabel pemetaan di [docs/02-neutral-architecture-and-domain-model.md](file:///d:/source/My%20Project/PBDB/docs/02-neutral-architecture-and-domain-model.md)).

### B. Komponen *DocumentUploader* & Kompresi Client-Side
1. Menerima foto kamera HP ukuran besar (`JPG/PNG/WebP`) maupun `PDF`.
2. Foto otomatis dikompresi & disesuaikan dimensinya di browser (Canvas/OffscreenCanvas) sebelum diunggah ke **Pre-signed URL** Object Storage—target ukuran `< 1 MB` dengan keterbacaan teks NIK/KK tetap terjaga.
3. Menampilkan pratinjau thumbnail, nama file, ukuran sebelum → sesudah kompresi, *progress bar* upload, serta badge status verifikasi per berkas (`Menunggu`, `Sah`, atau kotak merah berisi alasan revisi dari panitia).

### C. Validasi Formulir, Umpan Balik & Penjagaan Aksi Kritis
* **Validasi Inline:** Pesan error tampil persis di bawah input yang salah segera setelah field kehilangan fokus (*onBlur*) atau saat tombol *Lanjut* ditekan. Jika pengguna klik *Submit* dengan error di tengah halaman, muncul ringkasan error yang bisa diklik untuk menggulir langsung ke field bermasalah.
* **Auto-Save Non-Intrusif:** Menyimpan draft bertahap dengan label status kecil di header langkah (`Menyimpan...` → `Tersimpan pukul 14:02`).
* **Skeleton Loading:** Memakai placeholder berbentuk kartu/baris tabel saat memuat data (mencegah layar berkedip atau *layout shift*).
* **Modal Konfirmasi Eksplisit:** Wajib muncul untuk aksi yang berdampak permanen atau massal: *Submit Pendaftaran Final*, *Tolak Berkas untuk Revisi*, *Verifikasi Pembayaran Manual*, *Kunci Perankingan*, dan *Publikasikan Pengumuman Kelulusan*.

### D. Aksesibilitas & Target Performa
* **Navigasi Keyboard & Fokus:** Seluruh elemen interaktif wajib memiliki gaya `:focus-visible` yang tegas (outline `2px solid var(--brand-primary)` + offset `2px`). Modal & Drawer wajib mendukung penutupan via tombol `Esc` dan *focus trap*.
* **Pengumuman Status Layar (*Screen Reader*):** Indikator *auto-save* dan error formulir menggunakan atribut `aria-live="polite"` / `role="alert"`.
* **Animasi Hemat Resource:** Transisi hanya menganimasikan `opacity` dan `transform` (`150ms–200ms ease-out`) serta mematuhi `@media (prefers-reduced-motion: reduce)`.
* **Code Splitting Ketat:** Vite memisahkan bundel berdasarkan 3 permukaan + modul peta Leaflet + komponen tabel/ekspor admin, sehingga bundel awal untuk orang tua di Portal Publik maupun Portal Pendaftar tetap sangat ringan dan cepat dibuka di koneksi seluler menengah ke bawah.

