# 01. Visi Produk, Strategi Pasar & Model Bisnis

## 1. Visi Produk

Menyediakan platform penerimaan murid baru berbasis SaaS (*Software as a Service*) yang **netral, terstruktur, dan siap pakai** bagi satuan pendidikan maupun yayasan di Indonesia—baik yang mengacu pada alur **SPMB** (Kemendikdasmen), **PMBM** (Kemenag), maupun mekanisme **Mandiri** (sekolah swasta dan pondok pesantren).

---

## 2. Mengapa Harus Netral?

Dalam konteks regulasi pendidikan Indonesia, ketergantungan pada satu istilah atau satu aturan baku akan membatasi pasar secara drastis:

| Segmen | Acuan Utama | Karakteristik Pendaftaran |
| :--- | :--- | :--- |
| **Sekolah Umum (SD–SMA/SMK)** | **SPMB** (Permendikdasmen No. 3/2025) | Jalur Domisili, Afirmasi, Prestasi (SMP/SMA), Mutasi. Sekolah negeri umumnya gratis & terpusat di Pemda, namun sekolah swasta sering mengadopsi format pelaporan yang selaras. |
| **Madrasah (RA, MI, MTs, MA/MAK)** | **PMBM** (Juknis Dirjen Pendis Kemenag) | Jalur Reguler, Prestasi (termasuk keagamaan/tahfidz), Afirmasi, serta seleksi tes/wawancara pada madrasah unggulan atau swasta. |
| **Sekolah Swasta, Yayasan & Pesantren** | **Mandiri / Kebijakan Yayasan** | Multi-gelombang (Early Bird, Gelombang 1–3), biaya formulir & daftar ulang, tes akademik/BTQ, wawancara orang tua, serta opsi asrama/non-asrama. |

Dengan memisahkan **mesin alur inti** dari **konfigurasi istilah dan aturan**, satu platform dapat melayani ketiga segmen tersebut tanpa membuat cabang kode (*codebase fork*) terpisah.

---

## 3. Segmentasi Pasar & Strategi Go-to-Market

### Target Pasar Utama (*Primary Market*)
1. **Sekolah & Madrasah Swasta Mandiri (SD/MI hingga SMA/MA/SMK)**
   * Bebas menentukan jadwal, gelombang, dan biaya pendaftaran sendiri.
   * Membutuhkan tampilan portal resmi yang meningkatkan kepercayaan orang tua calon murid.
2. **Yayasan Pendidikan Multi-Unit & Pondok Pesantren**
   * Mengelola lebih dari satu jenjang dalam satu atap (misalnya TK/RA, SD/MI, SMP/MTs, SMA/MA/SMK).
   * Membutuhkan satu akun terpusat untuk manajemen yayasan, namun tetap memberi fleksibilitas syarat & biaya per unit jenjang.

### Target Pasar Sekunder (*Secondary Market*)
3. **Satuan Pendidikan Negeri / Daerah dengan Kebutuhan Portal Pendukung atau Mandiri**
   * Membutuhkan sistem pengumpulan data awal, penjadwalan tes/wawancara internal, atau peladen *white-label* di daerah/instansi yang belum terfasilitasi penuh.

---

## 4. Model Komersialisasi & Monetisasi

Karena siklus penerimaan murid bersifat musiman dengan puncak penggunaan 3–7 bulan per tahun ajaran, skema harga dirancang fleksibel agar ramah terhadap anggaran operasional sekolah:

### A. Paket Berlangganan Tahunan (*Annual / Seasonal Pass*)
* **Paket Starter (Sekolah/Madrasah Tunggal):** 1 unit jenjang, subdomain platform, kuota pendaftar terbatas, export Excel/CSV Dapodik/EMIS.
* **Paket Pro (Sekolah/Madrasah Unggulan):** Custom domain, kuota pendaftar lebih besar, integrasi notifikasi WhatsApp & Payment Gateway, modul ujian/penjadwalan wawancara, *audit trail*.
* **Paket Yayasan / Enterprise:** Multi-unit dalam satu yayasan, dashboard eksekutif lintas jenjang, RBAC lengkap, dukungan SLA prioritas.

### B. Model *Pay-as-You-Grow* (Biaya per Pendaftar / Transaksi)
* Biaya awal sangat rendah atau gratis di muka untuk sekolah kecil/rintisan.
* Platform mengenakan biaya kecil per pendaftar yang lolos tahap verifikasi/pembayaran (atau biaya layanan tambahan per transaksi payment gateway).
* Mengurangi hambatan psikologis sekolah yang khawatir berinvestasi sebelum jumlah pendaftar pasti.

### C. Lisensi *White-Label / Dedicated Deployment*
* Ditujukan untuk yayasan besar atau konsorsium sekolah yang mewajibkan isolasi infrastruktur khusus (*dedicated database/server*) atau *branding* penuh tanpa atribusi vendor.

### D. *Add-on* Layanan
* Paket kuota pesan **WhatsApp Gateway** (OTP, pengingat revisi berkas, pengumuman).
* Modul **CBT / Ujian Seleksi Online** terintegrasi.
* Jasa migrasi data awal atau pendampingan setup gelombang perdana.

---

## 5. Ruang Lingkup Produk: MVP vs Fase Lanjutan

Agar cepat rilis dan teruji di pasar (*time-to-market*), pengembangan dibagi bertahap:

### Fase 1 — MVP (*Minimum Viable Product*)
* Arsitektur *multi-tenant* dasar + branding sekolah (logo, warna, informasi profil).
* Preset konfigurasi istilah (**SPMB**, **PMBM**, **Mandiri**).
* Manajemen Periode/Gelombang, Unit Jenjang, dan Jalur Pendaftaran (termasuk toggle gratis/berbayar).
* Registrasi pendaftar, pengisian formulir biodata standar (Dapodik/EMIS compatible), dan unggah berkas dinamis per jalur.
* Pembayaran manual (upload bukti transfer + verifikasi bendahara).
* Panel verifikasi panitia dengan status **Perlu Revisi per berkas**.
* Penetapan kelulusan (manual/ranking sederhana) & halaman pengumuman.
* Daftar ulang sederhana & **Export Excel/CSV** standar Dapodik/EMIS.

### Fase 2 — Pertumbuhan & Otomatisasi (*Growth*)
* Integrasi **Payment Gateway** otomatis (QRIS, Virtual Account, E-Wallet).
* Integrasi **WhatsApp Gateway** untuk OTP & notifikasi perubahan status otomatis.
* **Custom Domain** per tenant (`spmb.sekolah.sch.id` / `pmbm.madrasah.sch.id`).
* Penjadwalan tes & wawancara + input skor tim penguji + perankingan berbobot otomatis (jarak koordinat peta, usia, nilai rapor/prestasi, tes) beserta aturan limpahan kuota.
* RBAC detail per divisi + *Audit Trail* komprehensif.

### Fase 3 — Ekosistem Lanjutan (*Scale*)
* Modul **CBT / Tes Seleksi Online** bawaan.
* Manajemen asrama/kamar awal (khusus pesantren & *boarding school*).
* API / Webhook untuk integrasi dengan SIAKAD/SIM Sekolah eksisting.
* Dashboard analitik multi-tahun (tren pendaftar, rasio konversi daftar ulang, sebaran wilayah asal sekolah).

