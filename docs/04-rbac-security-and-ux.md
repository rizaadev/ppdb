# 04. Pembagian Peran (RBAC), Keamanan & Standar UX

## 1. Matriks Hak Akses (*Role-Based Access Control*)

Untuk menjaga akuntabilitas operasional di sekolah maupun yayasan multi-unit, hak akses dipisahkan secara ketat per peran dan dibatasi pada lingkup `tenant_id` (dan `unit_id` jika berlaku):

| Peran (*Role*) | Lingkup Akses | Wewenang Utama |
| :--- | :--- | :--- |
| **Platform Super Admin** | Lintas seluruh tenant | Mengelola langganan tenant, preset regulasi global, konfigurasi gateway sistem, monitoring kesehatan layanan (tidak mengakses data pribadi pendaftar kecuali izin dukungan teknis). |
| **Admin Yayasan / Tenant** | Seluruh unit dalam 1 tenant | Mengatur profil yayasan, custom domain, membuat unit jenjang, melihat rekap lintas jenjang, mengelola akun staf/panitia. |
| **Admin Unit / Operator Sekolah** | 1 unit jenjang tertentu | Membuka periode/gelombang, mengatur jalur & kuota, mengelola persyaratan berkas, melakukan perankingan, mempublikasikan pengumuman, dan ekspor Dapodik/EMIS. |
| **Verifikator Berkas** | Antrean unit/jalur yang ditugaskan | Memeriksa biodata & dokumen unggahan pendaftar, menyetujui atau mengembalikan berkas untuk revisi dengan catatan. Tidak bisa mengubah nilai tes atau status kelulusan akhir. |
| **Tim Penguji / Pewawancara** | Sesi ujian/wawancara yang ditugaskan | Melihat daftar peserta ujian, menginput nilai tes/wawancara dan catatan pengamatan. Tidak bisa memodifikasi biodata atau pembayaran. |
| **Bendahara / Keuangan** | Transaksi unit/tenant | Memverifikasi bukti transfer manual, memantau status pembayaran otomatis, menerbitkan kuitansi, dan merekap laporan keuangan formulir/daftar ulang. |
| **Pimpinan / Kepala Sekolah** | Unit/Tenant (*Read-Only* + Approval) | Melihat dashboard statistik real-time (pendaftar, rasio keterisian kuota, laporan keuangan) dan memberikan persetujuan (*sign-off*) penetapan hasil kelulusan. |
| **Pendaftar / Orang Tua / Wali** | Hanya aplikasi milik sendiri | Mendaftar, melengkapi formulir, mengunggah/memperbaiki berkas, melakukan pembayaran, mencetak kartu ujian/kelulusan, dan melakukan daftar ulang. |

---

## 2. *Audit Trail* & Akuntabilitas

Sebagai sistem seleksi yang sensitif terhadap keadilan dan keuangan, setiap aksi kritis wajib mencatat log permanen (*append-only audit log*):

* **Data yang dicatat:** `tenant_id`, `actor_user_id`, `actor_role`, `target_entity` (`application`, `document`, `payment`, `score`), `action_type`, `previous_value`, `new_value`, `reason/notes`, `timestamp`, dan `ip_address`.
* **Aksi wajib tercatat:**
  * Perubahan status verifikasi berkas atau pembatalan verifikasi.
  * Perubahan data biodata krusial pendaftar oleh panitia (misalnya perbaikan NIK/koordinat setelah verifikasi).
  * Input maupun revisi nilai tes/wawancara oleh penguji.
  * Verifikasi pembayaran manual atau perubahan status tagihan.
  * Penguncian perankingan, pengalihan sisa kuota, dan penetapan status `ACCEPTED`/`WAITLISTED`/`REJECTED`.
  * Aktivitas ekspor data massal (Dapodik/EMIS/Excel) oleh operator.

---

## 3. Keamanan Data & Kepatuhan Privasi (UU PDP)

Sistem penerimaan murid memproses **data pribadi spesifik dan data anak di bawah umur** (NIK, Nomor KK, Akta Lahir, riwayat disabilitas/afirmasi, alamat rumah). Standar keamanan minimum yang wajib diterapkan:

1. **Isolasi *Multi-Tenant* Ketat:**
   * Setiap query di lapisan data dan otorisasi wajib menegakkan filter `tenant_id` (dan `unit_id`) agar tidak terjadi kebocoran data lintas sekolah.
2. **Penyimpanan Dokumen Privat (*No Public URLs*):**
   * Seluruh berkas unggahan (KK, Akta, KIP, Ijazah, Bukti Bayar) disimpan di *private bucket/storage* yang tidak dapat diakses publik secara langsung.
   * Akses tampilan dokumen hanya diberikan melalui **Signed URL berumur pendek** (*short-lived pre-signed URL*) atau *proxy endpoint* yang memvalidasi peran & kepemilikan tenant.
3. **Prinsip *Least Privilege* & *Data Masking*:**
   * Peran yang tidak relevan (misalnya Tim Pewawancara atau Penguji) tidak perlu melihat dokumen finansial atau detail NIK/No. KK lengkap bila tidak dibutuhkan untuk penilaian.
4. **Keamanan Transport & Integrasi:**
   * Wajib HTTPS/TLS untuk seluruh endpoint, validasi tanda tangan (*signature verification*) pada setiap *webhook* Payment Gateway, serta *rate limiting* pada endpoint OTP, login, dan cek status publik.
5. **Kebijakan Retensi & Penghapusan Data:**
   * Menyediakan fitur bagi Admin Sekolah/Yayasan untuk mengarsipkan atau menganonimkan/menghapus berkas pendaftar yang tidak lolos atau mengundurkan diri setelah periode tahun ajaran berakhir sesuai ketentuan perlindungan data pribadi.

---

## 4. Standar UX *Mobile-First* & Reliabilitas Lapangan

Mayoritas orang tua calon murid di Indonesia mengakses portal melalui ponsel pintar dengan kondisi jaringan yang beragam:

* **Kompresi Gambar di Sisi Browser (*Client-Side Compression*):**
  * Foto Kartu Keluarga atau Akta dari kamera HP modern sering berukuran 5–12 MB. Aplikasi harus otomatis mengecilkan resolusi/ukuran file (misal ke target < 1 MB dengan teks tetap terbaca jelas) sebelum dikirim ke server. Ini mempercepat upload bagi pengguna sekaligus menekan biaya storage dan bandwidth.
* **Notifikasi Berbasis WhatsApp sebagai Kanal Utama:**
  * Selain notifikasi di dashboard dan email, setiap kejadian penting (OTP registrasi, konfirmasi pembayaran, pengingat perbaikan berkas, jadwal tes, pengumuman kelulusan) diprioritaskan terkirim lewat WhatsApp.
* **Formulir Bertahap & Simpan Otomatis (*Draft Saving*):**
  * Memecah form panjang menjadi beberapa bagian ringkas dengan indikator progres yang jelas dan penyimpanan bertahap.
* **Kesiapan Lonjakan Beban (*Traffic Spike*) Saat Pengumuman:**
  * Halaman beranda publik dan pengecekan kelulusan harus ringan (mendukung *caching* agresif dan indeks database yang optimal) karena seluruh pendaftar cenderung mengakses sistem secara serentak pada jam pembukaan pengumuman.

