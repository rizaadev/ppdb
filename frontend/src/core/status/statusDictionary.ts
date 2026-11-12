import type { ApplicationStatus } from '@/core/types/domain';

export type StatusTone = 'neutral' | 'info' | 'warning' | 'success' | 'danger';

export interface StatusMeta {
  label: string;
  tone: StatusTone;
  description: string;
}

export const STATUS_DICTIONARY: Record<ApplicationStatus, StatusMeta> = {
  account_created: {
    label: 'Akun Aktif',
    tone: 'neutral',
    description: 'Akun terverifikasi. Silakan pilih unit dan jalur pendaftaran.',
  },
  track_selected: {
    label: 'Jalur Dipilih',
    tone: 'neutral',
    description: 'Jalur dipilih. Lanjutkan ke langkah berikutnya.',
  },
  awaiting_payment: {
    label: 'Menunggu Pembayaran',
    tone: 'warning',
    description: 'Selesaikan pembayaran formulir agar pengisian berkas terbuka.',
  },
  payment_verified: {
    label: 'Pembayaran Sah',
    tone: 'success',
    description: 'Pembayaran terkonfirmasi. Formulir pendaftaran sudah terbuka.',
  },
  draft_form: {
    label: 'Draft Formulir',
    tone: 'neutral',
    description: 'Lengkapi seluruh biodata dan unggah dokumen persyaratan.',
  },
  submitted: {
    label: 'Terkirim',
    tone: 'info',
    description: 'Berkas terkirim dan sedang menunggu antrean pemeriksaan panitia.',
  },
  under_verification: {
    label: 'Sedang Diverifikasi',
    tone: 'info',
    description: 'Panitia sedang memeriksa kesesuaian biodata dan berkas Anda.',
  },
  revision_required: {
    label: 'Perlu Perbaikan',
    tone: 'warning',
    description: 'Ada berkas atau data yang perlu diperbaiki sesuai catatan panitia.',
  },
  verified: {
    label: 'Berkas Terverifikasi',
    tone: 'success',
    description: 'Seluruh berkas telah dinyatakan sah dan siap masuk tahap seleksi.',
  },
  selection_in_progress: {
    label: 'Proses Seleksi',
    tone: 'info',
    description: 'Sedang mengikuti tahapan tes, wawancara, atau penilaian jalur.',
  },
  ranked: {
    label: 'Pemeringkatan Selesai',
    tone: 'info',
    description: 'Menunggu penetapan resmi dan jadwal pengumuman kelulusan.',
  },
  accepted: {
    label: 'Diterima / Lulus',
    tone: 'success',
    description: 'Selamat! Segera lakukan konfirmasi daftar ulang sebelum batas waktu.',
  },
  waitlisted: {
    label: 'Cadangan',
    tone: 'warning',
    description: 'Berada pada daftar tunggu dan berpeluang naik jika tersedia kuota.',
  },
  rejected: {
    label: 'Belum Lolos',
    tone: 'danger',
    description: 'Belum memenuhi batas kuota atau kriteria seleksi jalur ini.',
  },
  re_enrollment: {
    label: 'Proses Daftar Ulang',
    tone: 'warning',
    description: 'Konfirmasi daftar ulang dan administrasi awal sedang diproses.',
  },
  enrolled: {
    label: 'Resmi Terdaftar',
    tone: 'success',
    description: 'Daftar ulang selesai dan data siap masuk sistem sekolah/madrasah.',
  },
  withdrawn: {
    label: 'Mengundurkan Diri / Batal',
    tone: 'danger',
    description: 'Pendaftaran dibatalkan atau melewati batas waktu yang ditentukan.',
  },
};

