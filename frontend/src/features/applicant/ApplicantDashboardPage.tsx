import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import {
  AlertTriangle,
  ArrowRight,
  Calendar,
  CheckCircle2,
  CreditCard,
  ExternalLink,
  FileCheck,
  Plus,
  Printer,
  RefreshCw,
  Upload,
} from 'lucide-react';
import {
  ApiError,
  apiRequest,
  formatDateTime,
  formatRupiah,
  uploadToPresignedUrl,
} from '@/core/api/client';
import { STATUS_DICTIONARY } from '@/core/status/statusDictionary';
import { useTerminology } from '@/core/terminology/terminology';
import type {
  ApplicationDetail,
  ApplicationStatus,
  PaymentMethod,
  PaymentTransaction,
  PaymentVerificationResult,
  PresignUploadResult,
  PresignedDownloadResult,
  PublicPortalPayload,
} from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { InputField } from '@/components/ui/InputField';
import { Modal } from '@/components/ui/Modal';
import { QrCodeSvg } from '@/components/ui/QrCodeSvg';
import { StatusBadge, ToneBadge } from '@/components/ui/StatusBadge';
import { Stepper } from '@/components/ui/Stepper';
import styles from './ApplicantDashboardPage.module.css';

const PIPELINE_STEPS = [
  { id: 'track', title: 'Pilih Jalur & Tagihan', subtitle: 'Akun aktif dan jalur dipilih' },
  { id: 'form', title: 'Biodata & Berkas', subtitle: 'Lengkapi data Dapodik/EMIS' },
  { id: 'verify', title: 'Verifikasi Panitia', subtitle: 'Pemeriksaan dokumen oleh panitia' },
  { id: 'select', title: 'Seleksi & Pengumuman', subtitle: 'Pemeringkatan / tes & hasil' },
  { id: 'enroll', title: 'Daftar Ulang', subtitle: 'Resmi menjadi murid terdaftar' },
];

function terbilangAngka(n: number): string {
  const satuan = ['', 'Satu', 'Dua', 'Tiga', 'Empat', 'Lima', 'Enam', 'Tujuh', 'Delapan', 'Sembilan', 'Sepuluh', 'Sebelas'];
  if (n < 12) return satuan[n];
  if (n < 20) return `${terbilangAngka(n - 10)} Belas`;
  if (n < 100) return `${terbilangAngka(Math.floor(n / 10))} Puluh ${terbilangAngka(n % 10)}`.trim();
  if (n < 200) return `Seratus ${terbilangAngka(n - 100)}`.trim();
  if (n < 1000) return `${terbilangAngka(Math.floor(n / 100))} Ratus ${terbilangAngka(n % 100)}`.trim();
  if (n < 2000) return `Seribu ${terbilangAngka(n - 1000)}`.trim();
  if (n < 1000000) return `${terbilangAngka(Math.floor(n / 1000))} Ribu ${terbilangAngka(n % 1000)}`.trim();
  if (n < 1000000000) return `${terbilangAngka(Math.floor(n / 1000000))} Juta ${terbilangAngka(n % 1000000)}`.trim();
  return `${terbilangAngka(Math.floor(n / 1000000000))} Miliar ${terbilangAngka(n % 1000000000)}`.trim();
}

function terbilangRupiah(amount: number): string {
  if (amount <= 0) return 'Nol Rupiah';
  return `${terbilangAngka(Math.floor(amount))} Rupiah`;
}

function pipelineStepIndex(status: ApplicationStatus): number {
  switch (status) {
    case 'account_created':
    case 'track_selected':
    case 'awaiting_payment':
    case 'payment_verified':
      return 0;
    case 'draft_form':
    case 'revision_required':
      return 1;
    case 'submitted':
    case 'under_verification':
    case 'verified':
      return 2;
    case 'selection_in_progress':
    case 'ranked':
    case 'accepted':
    case 'waitlisted':
    case 'rejected':
      return 3;
    case 're_enrollment':
    case 'enrolled':
    case 'withdrawn':
      return 4;
  }
}

export function ApplicantDashboardPage({
  tenantSlug,
  portal,
  activeApp,
  loading,
  createOpen,
  onOpenCreate,
  onCloseCreate,
  onApplicationUpdated,
  onApplicationCreated,
}: {
  tenantSlug: string;
  portal: PublicPortalPayload;
  activeApp: ApplicationDetail | null;
  loading: boolean;
  createOpen: boolean;
  onOpenCreate: () => void;
  onCloseCreate: () => void;
  onApplicationUpdated: (detail: ApplicationDetail) => void;
  onApplicationCreated: (detail: ApplicationDetail) => void;
}) {
  const term = useTerminology();
  const [selectedPeriodId, setSelectedPeriodId] = useState(portal.periods[0]?.id ?? '');
  const activePeriod = portal.periods.find((p) => p.id === selectedPeriodId) ?? portal.periods[0];
  const [selectedTrackId, setSelectedTrackId] = useState(activePeriod?.tracks[0]?.id ?? '');
  const [newStudentName, setNewStudentName] = useState('');
  const [createError, setCreateError] = useState('');
  const [creating, setCreating] = useState(false);

  const [payMethod, setPayMethod] = useState<PaymentMethod>('qris');
  const [payBusy, setPayBusy] = useState(false);
  const [payNotice, setPayNotice] = useState('');
  const [payError, setPayError] = useState('');
  const [syncingPaymentId, setSyncingPaymentId] = useState<string | null>(null);

  const [agreeReEnroll, setAgreeReEnroll] = useState(false);
  const [reEnrollBusy, setReEnrollBusy] = useState(false);
  const [printMode, setPrintMode] = useState<'card' | 'letter' | 'receipt' | null>(null);
  const [selectedReceiptPay, setSelectedReceiptPay] = useState<PaymentTransaction | null>(null);

  async function handleCreateSubmit(e: FormEvent) {
    e.preventDefault();
    if (!activePeriod) return;
    const trackId = selectedTrackId || activePeriod.tracks[0]?.id;
    if (!trackId) return;

    setCreateError('');
    setCreating(true);
    try {
      const created = await apiRequest<ApplicationDetail>('/api/v1/applicant/applications', {
        method: 'POST',
        tenantSlug,
        idempotencyKey: `create-${crypto.randomUUID()}`,
        body: {
          unit_id: activePeriod.unit_id,
          period_id: activePeriod.id,
          track_id: trackId,
          student_full_name: newStudentName,
        },
      });
      setNewStudentName('');
      onCloseCreate();
      onApplicationCreated(created);
    } catch (err) {
      setCreateError(err instanceof ApiError ? err.message : 'Gagal membuat pendaftaran baru.');
    } finally {
      setCreating(false);
    }
  }

  async function refreshActiveApp(appId: string) {
    const fresh = await apiRequest<ApplicationDetail>(`/api/v1/applicant/applications/${appId}`, { tenantSlug });
    onApplicationUpdated(fresh);
  }

  async function handleCreateInvoice(purpose: 'registration_fee' | 're_enrollment_fee') {
    if (!activeApp) return;
    setPayBusy(true);
    setPayError('');
    setPayNotice('');
    try {
      const inv = await apiRequest<PaymentTransaction>(
        `/api/v1/applicant/applications/${activeApp.application.id}/payments`,
        {
          method: 'POST',
          tenantSlug,
          idempotencyKey: `pay-${activeApp.application.id}-${purpose}-${payMethod}-${Date.now()}`,
          body: { purpose, method: payMethod },
        },
      );
      setPayNotice(`Tagihan ${inv.external_ref ?? inv.id} berhasil dibuat (${formatRupiah(inv.amount_idr)}).`);
      await refreshActiveApp(activeApp.application.id);
    } catch (err) {
      setPayError(err instanceof ApiError ? err.message : 'Gagal membuat tagihan.');
    } finally {
      setPayBusy(false);
    }
  }

  async function handleUploadManualProof(paymentId: string, file: File | undefined) {
    if (!activeApp || !file) return;
    setPayBusy(true);
    setPayError('');
    setPayNotice('');
    try {
      const presigned = await apiRequest<PresignUploadResult>(
        `/api/v1/applicant/applications/${activeApp.application.id}/payments/${paymentId}/presign-proof`,
        {
          method: 'POST',
          tenantSlug,
          body: {
            filename: file.name,
            mime_type: file.type || 'image/jpeg',
            size_bytes: file.size,
          },
        },
      );
      await uploadToPresignedUrl(presigned.upload_url, file, presigned.required_headers);
      await apiRequest(
        `/api/v1/applicant/applications/${activeApp.application.id}/payments/${paymentId}/manual-proof`,
        {
          method: 'POST',
          tenantSlug,
          body: { proof_storage_key: presigned.storage_key },
        },
      );
      setPayNotice('Bukti transfer berhasil diunggah. Menunggu verifikasi Bendahara.');
      await refreshActiveApp(activeApp.application.id);
    } catch (err) {
      setPayError(err instanceof Error ? err.message : 'Gagal mengunggah bukti transfer.');
    } finally {
      setPayBusy(false);
    }
  }

  async function handleSyncPayment(paymentId: string) {
    if (!activeApp) return;
    setSyncingPaymentId(paymentId);
    setPayError('');
    setPayNotice('');
    try {
      const res = await apiRequest<PaymentVerificationResult>(
        `/api/v1/applicant/applications/${activeApp.application.id}/payments/${paymentId}/sync`,
        {
          method: 'POST',
          tenantSlug,
        },
      );
      onApplicationUpdated(res.application);
      setPayNotice(`Status pembayaran diperbarui: ${res.payment.status.toUpperCase()}.`);
    } catch (err) {
      setPayError(
        err instanceof ApiError ? err.message : 'Gagal memeriksa status terbaru pembayaran.',
      );
    } finally {
      setSyncingPaymentId(null);
    }
  }

  async function handleConfirmReEnroll() {
    if (!activeApp) return;
    setReEnrollBusy(true);
    setPayError('');
    try {
      const updated = await apiRequest<ApplicationDetail>(
        `/api/v1/applicant/applications/${activeApp.application.id}/re-enroll`,
        {
          method: 'POST',
          tenantSlug,
          body: {
            version: activeApp.application.version,
            agreement_accepted: agreeReEnroll,
          },
        },
      );
      onApplicationUpdated(updated);
    } catch (err) {
      setPayError(err instanceof ApiError ? err.message : 'Gagal memproses daftar ulang.');
    } finally {
      setReEnrollBusy(false);
    }
  }

  async function handlePreviewDoc(docId: string) {
    if (!activeApp) return;
    try {
      const signed = await apiRequest<PresignedDownloadResult>(
        `/api/v1/applicant/applications/${activeApp.application.id}/documents/${docId}/presign-download`,
        { tenantSlug },
      );
      window.open(signed.url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      setPayError(err instanceof ApiError ? err.message : 'Gagal membuka pratinjau dokumen.');
    }
  }

  if (loading && !activeApp) {
    return (
      <div className="container-wizard">
        <Card>
          <p style={{ color: 'var(--text-secondary)' }}>Memuat data pendaftaran {term.guardianNoun}...</p>
        </Card>
      </div>
    );
  }

  const createModal = (
    <Modal
      open={createOpen}
      title={`Pendaftaran ${term.applicantNoun} Baru`}
      onClose={onCloseCreate}
      footer={
        <>
          <Button variant="secondary" onClick={onCloseCreate}>
            Batal
          </Button>
          <Button type="submit" form="create-app-form" disabled={creating}>
            {creating ? 'Menyimpan...' : 'Mulai Pendaftaran'}
          </Button>
        </>
      }
    >
      <form id="create-app-form" onSubmit={handleCreateSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        {createError && (
          <div style={{ color: 'var(--status-danger-fg)', fontSize: 'var(--text-caption)', fontWeight: 600 }}>
            {createError}
          </div>
        )}
        <div>
          <label htmlFor="new-period-select" style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
            Unit &amp; Periode
          </label>
          <select
            id="new-period-select"
            value={activePeriod?.id ?? ''}
            onChange={(e) => {
              const pId = e.target.value;
              setSelectedPeriodId(pId);
              const nextP = portal.periods.find((p) => p.id === pId);
              setSelectedTrackId(nextP?.tracks[0]?.id ?? '');
            }}
            style={{ width: '100%', padding: '0.6rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-strong)' }}
          >
            {portal.periods.map((p) => {
              const u = portal.units.find((un) => un.id === p.unit_id);
              return (
                <option key={p.id} value={p.id}>
                  {u?.name ?? 'Unit'} — {p.name} ({p.academic_year})
                </option>
              );
            })}
          </select>
        </div>

        <div>
          <label htmlFor="new-track-select" style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
            Pilih {term.trackNoun}
          </label>
          <select
            id="new-track-select"
            value={selectedTrackId || activePeriod?.tracks[0]?.id || ''}
            onChange={(e) => setSelectedTrackId(e.target.value)}
            style={{ width: '100%', padding: '0.6rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-strong)' }}
          >
            {activePeriod?.tracks.map((tr) => (
              <option key={tr.id} value={tr.id}>
                {tr.display_name} ({formatRupiah(tr.registration_fee_idr)})
              </option>
            ))}
          </select>
        </div>

        <InputField
          label={`Nama Lengkap ${term.applicantNoun}`}
          value={newStudentName}
          onChange={(e) => setNewStudentName(e.target.value)}
          placeholder="Contoh: Ahmad Fadhil"
          required
        />
      </form>
    </Modal>
  );

  if (!activeApp) {
    return (
      <div className={`container-wizard ${styles.stack}`}>
        <Card>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700, marginBottom: 'var(--space-2)' }}>
            Belum Ada Pendaftaran
          </h1>
          <p style={{ color: 'var(--text-secondary)', fontSize: 'var(--text-caption)', marginBottom: 'var(--space-4)' }}>
            Anda belum memiliki formulir pendaftaran aktif pada {portal.tenant.name}.
          </p>
          <Button onClick={onOpenCreate}>
            <Plus size={16} />
            Mulai Pendaftaran {term.applicantNoun}
          </Button>
        </Card>
        {createModal}
      </div>
    );
  }

  const app = activeApp.application;
  const statusMeta = STATUS_DICTIONARY[app.status];
  const currentStep = pipelineStepIndex(app.status);
  const canFillWizard = app.status === 'draft_form' || app.status === 'revision_required';
  const canPrintAcceptanceLetter =
    app.status === 'accepted' || app.status === 're_enrollment' || app.status === 'enrolled';
  const needsRegPayment = app.status === 'awaiting_payment';
  const needsReEnrollPayment = app.status === 're_enrollment' && activeApp.re_enrollment_fee_idr > 0;
  const pendingPurpose: 'registration_fee' | 're_enrollment_fee' | null = needsRegPayment
    ? 'registration_fee'
    : needsReEnrollPayment
      ? 're_enrollment_fee'
      : null;

  const primaryGuardian =
    activeApp.guardians.find((g) => g.is_primary_contact) ?? activeApp.guardians[0];

  const verifyToken = activeApp.verification_token ?? '';
  const verifyUrl =
    verifyToken && typeof window !== 'undefined'
      ? `${window.location.origin}/verifikasi?token=${encodeURIComponent(verifyToken)}`
      : verifyToken;

  const printSheetNode = printMode ? (
    <div className={styles.printSheet}>
      {/* KOP SURAT DINAS FORMAL */}
      <div className={styles.kopSurat}>
        {portal.tenant.theme_config.logo_url ? (
          <img
            src={portal.tenant.theme_config.logo_url}
            alt={portal.tenant.name}
            className={styles.kopLogo}
          />
        ) : (
          <div
            className={styles.kopLogo}
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              backgroundColor: '#1e3a8a',
              color: '#fff',
              fontWeight: 900,
              borderRadius: 8,
              fontSize: '1.25rem',
            }}
          >
            AH
          </div>
        )}
        <div className={styles.kopText}>
          <div className={styles.kopYayasan}>YAYASAN PONDOK PESANTREN AL-HAMID CINTA MULYA</div>
          <div className={styles.kopLembaga}>{portal.tenant.name}</div>
          <div className={styles.kopPanitia}>
            PANITIA PENERIMAAN SANTRI BARU (PSB) · TAHUN AJARAN {app.academic_year ?? '2026/2027'}
          </div>
          <div className={styles.kopAlamat}>
            {portal.units[0]?.address ??
              'Jl. Lintas Sumatera, Cinta Mulya, Kec. Way Pengubuan, Kab. Lampung Tengah, Lampung'}{' '}
            · Website: https://alhamidcintamulya.fun · Email: ppdb@alhamidcintamulya.fun
          </div>
        </div>
      </div>

      {/* 1. KARTU TANDA PESERTA UJIAN & SELEKSI */}
      {printMode === 'card' && (
        <>
          <div className={styles.printDocMeta}>
            <span>
              JENJANG: <strong>{app.unit_name ?? '-'}</strong>
            </span>
            <span>
              NO. REGISTRASI:{' '}
              <strong className="mono-code">{app.registration_no}</strong>
            </span>
            <span>
              JALUR: <strong>{app.track_name ?? '-'}</strong>
            </span>
          </div>

          <div className={styles.printTitle}>
            Kartu Tanda Pendaftaran {term.programAcronym} &amp; Ujian Seleksi
          </div>

          <div className={styles.examCardBody}>
            <table className={styles.printTable}>
              <tbody>
                <tr>
                  <th>Nomor Peserta</th>
                  <td className="mono-code">
                    <strong>{app.registration_no}</strong>
                  </td>
                </tr>
                <tr>
                  <th>Nama Calon Santri</th>
                  <td>
                    <strong>{activeApp.profile?.full_name ?? app.student_name ?? '-'}</strong>
                  </td>
                </tr>
                <tr>
                  <th>NIK / NISN</th>
                  <td className="mono-code">
                    {activeApp.profile?.nik || '-'} / {activeApp.profile?.nisn || '-'}
                  </td>
                </tr>
                <tr>
                  <th>Tempat, Tanggal Lahir</th>
                  <td>
                    {activeApp.profile?.birth_place || '-'}
                    {activeApp.profile?.birth_date ? `, ${activeApp.profile.birth_date}` : ''}
                  </td>
                </tr>
                <tr>
                  <th>Asal Madrasah/Sekolah</th>
                  <td>{activeApp.profile?.prev_school_name || '-'}</td>
                </tr>
                <tr>
                  <th>Nama Orang Tua / Wali</th>
                  <td>
                    {primaryGuardian?.full_name ?? '-'}
                    {primaryGuardian?.phone_whatsapp ? ` (${primaryGuardian.phone_whatsapp})` : ''}
                  </td>
                </tr>
                <tr>
                  <th>Status Administrasi</th>
                  <td>
                    <strong>{statusMeta.label}</strong> (Berkas Diverifikasi)
                  </td>
                </tr>
              </tbody>
            </table>

            <div className={styles.pasFotoBox}>
              <span>PAS FOTO</span>
              <span style={{ fontSize: '0.68rem', marginTop: 2 }}>3 × 4 CM</span>
            </div>
          </div>

          {/* JADWAL UJIAN & SELEKSI */}
          <div className={styles.noticeBox}>
            <div className={styles.noticeTitle}>Jadwal &amp; Ruang Seleksi Calon Santri:</div>
            <div>
              {activeApp.schedules && activeApp.schedules.length > 0 ? (
                activeApp.schedules.map((s) => (
                  <div key={s.id} style={{ marginBottom: 4 }}>
                    • <strong>{s.session_name}</strong>: {formatDateTime(s.scheduled_at)}
                    {s.room_or_link ? ` | Ruang/Tautan: ${s.room_or_link}` : ''}
                    {s.examiner_name ? ` | Penguji: ${s.examiner_name}` : ''}
                  </div>
                ))
              ) : (
                <div>
                  • <strong>Sesi Utama</strong>: Sesuai jadwal pengumuman gelombang (08.00 - 11.30 WIB) |
                  Lokasi: Ruang Asrama Terpadu / Daring Video Call | Materi: Baca Tulis Al-Qur'an (Tahsin),
                  Praktik Shalat Dasar, Wawancara Wali &amp; Tes Potensi Akademik.
                </div>
              )}
            </div>
          </div>

          {/* TATA TERTIB */}
          <div className={styles.noticeBox} style={{ backgroundColor: '#fff', borderColor: '#e5e7eb' }}>
            <div className={styles.noticeTitle}>Tata Tertib Peserta Seleksi:</div>
            <ol style={{ margin: '4px 0 0 16px', padding: 0, lineHeight: 1.5 }}>
              <li>Peserta wajib hadir 15 menit sebelum ujian dimulai dengan membawa kartu ini.</li>
              <li>Berbusana muslim/muslimah rapi dan sopan (baju putih, bawahan gelap, berkopiah/berjilbab).</li>
              <li>Membawa alat tulis mandiri serta fotokopi kartu identitas / Kartu Keluarga.</li>
              <li>Menjaga adab, kejujuran, dan ketertiban selama rangkaian ujian seleksi berlangsung.</li>
            </ol>
          </div>

          {/* TANDA TANGAN */}
          <div className={styles.printSignatures}>
            <div className={styles.signatureCol}>
              <div>Peserta Seleksi,</div>
              <div className={styles.signatureSpace} />
              <div className={styles.signatureLine}>
                ( {activeApp.profile?.full_name ?? app.student_name ?? '....................'} )
              </div>
            </div>
            <div className={styles.signatureCol}>
              <div>Panitia Seleksi Penerimaan,</div>
              <div className={styles.signatureSpace} />
              <div className={styles.signatureLine}>
                ( Panitia PSB {app.unit_name ?? portal.tenant.name} )
              </div>
            </div>
          </div>
        </>
      )}

      {/* 2. SURAT KEPUTUSAN KELULUSAN (SKL RESMI) */}
      {printMode === 'letter' && (
        <>
          <div className={styles.printDocMeta}>
            <span>
              Nomor: <strong>045/PAN-PSB/PP.AH/V/2026</strong>
            </span>
            <span>Lampiran: -</span>
            <span>
              Perihal: <strong>Pengumuman Penetapan Kelulusan Santri Baru</strong>
            </span>
          </div>

          <div className={styles.printTitle}>
            Surat Keterangan Hasil Seleksi — Diterima (SKL Resmi)
          </div>

          <p style={{ fontSize: '0.85rem', lineHeight: 1.6, margin: 0 }}>
            <em>Assalamu'alaikum Warahmatullahi Wabarakatuh,</em>
            <br />
            Berdasarkan hasil verifikasi dokumen administrasi, penilaian tes seleksi Al-Qur'an dan wawancara,
            serta hasil sidang pleno Panitia Penerimaan Santri Baru Pondok Pesantren Al-Hamid, dengan ini
            menerangkan bahwa calon santri di bawah ini:
          </p>

          <table className={styles.printTable}>
            <tbody>
              <tr>
                <th>Nomor Registrasi</th>
                <td className="mono-code">
                  <strong>{app.registration_no}</strong>
                </td>
              </tr>
              <tr>
                <th>Nama Lengkap Santri</th>
                <td>
                  <strong>{activeApp.profile?.full_name ?? app.student_name ?? '-'}</strong>
                </td>
              </tr>
              <tr>
                <th>NIK / NISN</th>
                <td className="mono-code">
                  {activeApp.profile?.nik || '-'} / {activeApp.profile?.nisn || '-'}
                </td>
              </tr>
              <tr>
                <th>Asal Satuan Pendidikan</th>
                <td>{activeApp.profile?.prev_school_name || '-'}</td>
              </tr>
              <tr>
                <th>Jenjang / Unit Tujuan</th>
                <td>
                  <strong>{app.unit_name ?? '-'}</strong>
                </td>
              </tr>
              <tr>
                <th>Jalur Penerimaan</th>
                <td>
                  {app.track_name ?? '-'} ({app.period_name ?? '-'})
                </td>
              </tr>
            </tbody>
          </table>

          {/* DECISION CALLOUT */}
          <div className={styles.sklDecisionBox}>
            <div className={styles.sklDecisionTitle}>D I T E R I M A</div>
            <div className={styles.sklDecisionSub}>
              Sebagai Santri Baru {portal.tenant.name} Tahun Ajaran {app.academic_year ?? '2026/2027'}
              {app.rank_position ? ` · Peringkat #${app.rank_position}` : ''}
              {app.final_score ? ` · Nilai Akhir: ${app.final_score}` : ''}
            </div>
          </div>

          {/* INSTRUKSI DAFTAR ULANG */}
          <div className={styles.noticeBox}>
            <div className={styles.noticeTitle}>Petunjuk Penting Daftar Ulang:</div>
            <ol style={{ margin: '4px 0 0 16px', padding: 0, lineHeight: 1.5 }}>
              <li>
                Lakukan konfirmasi kesediaan daftar ulang melalui Portal Wali Santri paling lambat 7 hari kerja
                setelah surat ini diterbitkan.
              </li>
              <li>
                Kewajiban biaya daftar ulang:{' '}
                <strong>
                  {activeApp.re_enrollment_fee_idr > 0
                    ? formatRupiah(activeApp.re_enrollment_fee_idr)
                    : 'Bebas Biaya Daftar Ulang (Beasiswa Penuh)'}
                </strong>
                .
              </li>
              <li>
                Membawa berkas dokumen asli (Kartu Keluarga, Akta Kelahiran, dan Ijazah/SKL) pada saat penyerahan
                santri di pondok pesantren.
              </li>
            </ol>
          </div>

          {/* TANDA TANGAN KEPALA / PANITIA */}
          <div className={styles.printSignatures}>
            <div className={styles.signatureCol}>
              <div>Wali Santri yang Bersangkutan,</div>
              <div className={styles.signatureSpace} />
              <div className={styles.signatureLine}>
                ( {primaryGuardian?.full_name ?? '....................'} )
              </div>
            </div>
            <div className={styles.signatureCol}>
              <div>
                Ditetapkan di: Cinta Mulya
                <br />
                Ketua Panitia Penerimaan Santri Baru,
              </div>
              <div className={styles.signatureSpace} />
              <div className={styles.signatureLine}>
                ( Ust. H. Muhammad Rijal, M.Pd. )
              </div>
            </div>
          </div>
        </>
      )}

      {/* 3. KWITANSI RESMI PEMBAYARAN LUNAS */}
      {printMode === 'receipt' && selectedReceiptPay && (
        <>
          <div className={styles.printDocMeta}>
            <span>
              NO. KWITANSI:{' '}
              <strong className="mono-code">
                KWIT/PSB/{selectedReceiptPay.external_ref ?? selectedReceiptPay.id.slice(0, 8).toUpperCase()}
              </strong>
            </span>
            <span>
              TANGGAL BAYAR:{' '}
              <strong>{formatDateTime(selectedReceiptPay.paid_at ?? new Date().toISOString())}</strong>
            </span>
            <span>STATUS: <strong>LUNAS</strong></span>
          </div>

          <div className={styles.printTitle}>KWITANSI BUKTI PEMBAYARAN RESMI</div>

          <table className={styles.printTable}>
            <tbody>
              <tr>
                <th>Telah Diterima Dari</th>
                <td>
                  <strong>
                    {primaryGuardian?.full_name ??
                      activeApp.profile?.full_name ??
                      app.student_name ??
                      '-'}
                  </strong>
                </td>
              </tr>
              <tr>
                <th>Untuk Pembayaran</th>
                <td>
                  <strong>
                    {selectedReceiptPay.purpose === 'registration_fee'
                      ? 'Biaya Formulir Pendaftaran Santri Baru'
                      : 'Biaya Daftar Ulang Santri Baru'}
                  </strong>{' '}
                  — Calon Santri: <strong>{app.student_name ?? '-'}</strong> (No. Reg:{' '}
                  <span className="mono-code">{app.registration_no}</span>)
                </td>
              </tr>
              <tr>
                <th>Satuan Pendidikan &amp; Jalur</th>
                <td>
                  {app.unit_name ?? '-'} · Jalur {app.track_name ?? '-'} (T.A.{' '}
                  {app.academic_year ?? '2026/2027'})
                </td>
              </tr>
              <tr>
                <th>Metode Transaksi</th>
                <td>
                  {selectedReceiptPay.method.toUpperCase()}
                  {selectedReceiptPay.external_ref ? ` (Ref: ${selectedReceiptPay.external_ref})` : ''}
                </td>
              </tr>
            </tbody>
          </table>

          {/* JUMLAH & STAMP LUNAS */}
          <div className={styles.receiptBox}>
            <div>
              <div style={{ fontSize: '0.8rem', color: '#4b5563', fontWeight: 600 }}>
                Jumlah Pembayaran Diterima:
              </div>
              <div
                style={{
                  fontSize: '1.45rem',
                  fontWeight: 900,
                  color: '#1e40af',
                  marginTop: 2,
                }}
              >
                {formatRupiah(selectedReceiptPay.amount_idr)}
              </div>
              <div className={styles.terbilangBox}>
                Terbilang: {terbilangRupiah(selectedReceiptPay.amount_idr)}
              </div>
            </div>

            <div className={styles.lunasStamp}>
              <div className={styles.lunasStampText}>L U N A S</div>
              <div className={styles.lunasStampDate}>
                {selectedReceiptPay.paid_at ? selectedReceiptPay.paid_at.slice(0, 10) : 'VERIFIED'}
              </div>
            </div>
          </div>

          {/* TANDA TANGAN */}
          <div className={styles.printSignatures}>
            <div className={styles.signatureCol}>
              <div>Penyetor / Wali Santri,</div>
              <div className={styles.signatureSpace} />
              <div className={styles.signatureLine}>
                ( {primaryGuardian?.full_name ?? '....................'} )
              </div>
            </div>
            <div className={styles.signatureCol}>
              <div>
                Cinta Mulya, {new Date().toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' })}
                <br />
                Bendahara / Bagian Keuangan PSB,
              </div>
              <div className={styles.signatureSpace} />
              <div className={styles.signatureLine}>( Bendahara Penerimaan PPDB )</div>
            </div>
          </div>
        </>
      )}

      {/* FOOTER VERIFIKASI QR DOKUMEN */}
      <div className={styles.printFooter}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, maxWidth: '68%' }}>
          {verifyUrl && (
            <div style={{ flexShrink: 0 }}>
              <QrCodeSvg
                value={verifyUrl}
                size={70}
                label={`QR Verifikasi ${app.registration_no}`}
              />
            </div>
          )}
          <div style={{ color: '#4b5563', fontSize: '0.73rem', lineHeight: 1.45 }}>
            Dokumen resmi ini dicetak langsung dari Sistem Informasi PSB {portal.tenant.name}.
            Keaslian dokumen ini dilindungi kode otentikasi digital. Pindai QR Code atau kunjungi
            laman <strong>/verifikasi</strong> untuk memverifikasi keabsahan data secara publik.
          </div>
        </div>
        <div style={{ textAlign: 'center', minWidth: 160 }}>
          <div style={{ fontWeight: 700, fontSize: '0.78rem' }}>
            SEKRETARIAT PSB AL-HAMID
          </div>
          <div style={{ fontSize: '0.72rem', color: '#6b7280', marginTop: 2 }}>
            Sistem Dokumen Terverifikasi
          </div>
        </div>
      </div>
    </div>
  ) : null;

  return (
    <div className="container-wizard">
      <div className={`${styles.stack} no-print`}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', flexWrap: 'wrap', gap: 'var(--space-3)' }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700, marginBottom: '4px' }}>
            Dashboard {term.guardianNoun}
          </h1>
          <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)' }}>
            Pantau perkembangan pendaftaran {term.applicantNoun} secara real-time.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={onOpenCreate}>
          <Plus size={15} />
          Daftarkan Anak / Jalur Lain
        </Button>
      </div>

      {payNotice && (
        <div style={{ padding: 'var(--space-3)', borderRadius: 'var(--radius-md)', backgroundColor: 'var(--status-success-bg)', color: 'var(--status-success-fg)', fontWeight: 600 }}>
          {payNotice}
        </div>
      )}
      {payError && (
        <div style={{ padding: 'var(--space-3)', borderRadius: 'var(--radius-md)', backgroundColor: 'var(--status-danger-bg)', color: 'var(--status-danger-fg)', fontWeight: 600 }} role="alert">
          {payError}
        </div>
      )}

      {app.status === 'revision_required' && (
        <div className={styles.urgentCard} role="alert">
          <div className={styles.urgentContent}>
            <AlertTriangle size={20} style={{ flexShrink: 0, marginTop: 2 }} />
            <div>
              <strong style={{ display: 'block', marginBottom: 2 }}>
                Perbaikan Berkas Diperlukan
              </strong>
              <span style={{ fontSize: 'var(--text-caption)' }}>
                Panitia meminta unggah ulang atau koreksi berkas bertanda kuning sebelum verifikasi dapat dilanjutkan.
              </span>
            </div>
          </div>
          <Link to="/app/daftar">
            <Button size="sm">
              Perbaiki Sekarang <ArrowRight size={15} />
            </Button>
          </Link>
        </div>
      )}

      <Card>
        <div className={styles.summaryHeader}>
          <div>
            <div className="mono-code" style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>
              {app.registration_no}
            </div>
            <h2 style={{ fontSize: 'var(--text-h2)', fontWeight: 700, marginTop: 2 }}>
              {app.student_name ?? 'Calon Murid'}
            </h2>
            <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', marginTop: 4 }}>
              {app.unit_name} · {app.track_name}
              {app.distance_meters != null ? ` · Jarak: ${app.distance_meters} m` : ''}
              {app.final_score ? ` · Skor Akhir: ${app.final_score}` : ''}
              {app.rank_position ? ` · Peringkat #${app.rank_position}` : ''}
            </div>
          </div>
          <StatusBadge status={app.status} />
        </div>

        <p style={{ marginTop: 'var(--space-4)', fontSize: 'var(--text-caption)', color: 'var(--text-secondary)' }}>
          {statusMeta.description}
        </p>

        <div style={{ marginTop: 'var(--space-5)' }}>
          <Stepper steps={PIPELINE_STEPS} currentIndex={currentStep} />
        </div>

        <div className={styles.printActions}>
          <Button variant="secondary" size="sm" onClick={() => setPrintMode('card')}>
            <Printer size={14} />
            Cetak Kartu Pendaftaran
          </Button>
          {canPrintAcceptanceLetter && (
            <Button variant="secondary" size="sm" onClick={() => setPrintMode('letter')}>
              <FileCheck size={14} />
              Cetak Surat Keterangan Diterima
            </Button>
          )}
        </div>
      </Card>

      {app.status === 'accepted' && (
        <Card>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', color: 'var(--status-success-fg)', fontWeight: 700 }}>
              <CheckCircle2 size={18} />
              Selamat! Calon murid dinyatakan Diterima
            </div>
            <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)' }}>
              Konfirmasi daftar ulang untuk mengunci kursi. Biaya daftar ulang jalur ini:{' '}
              <strong className="tabular-nums">{formatRupiah(activeApp.re_enrollment_fee_idr)}</strong>.
            </p>
            <label style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', fontSize: 'var(--text-label)' }}>
              <input
                type="checkbox"
                checked={agreeReEnroll}
                onChange={(e) => setAgreeReEnroll(e.target.checked)}
              />
              <span>Saya menyatakan bersedia daftar ulang dan mematuhi tata tertib {app.unit_name}.</span>
            </label>
            <div>
              <Button disabled={!agreeReEnroll || reEnrollBusy} onClick={() => void handleConfirmReEnroll()}>
                {reEnrollBusy ? 'Memproses...' : 'Konfirmasi Daftar Ulang Sekarang'}
              </Button>
            </div>
          </div>
        </Card>
      )}

      {(activeApp.schedules?.length ?? 0) > 0 && (
        <Card>
          <h3
            style={{
              fontSize: 'var(--text-h3)',
              fontWeight: 700,
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--space-2)',
              marginBottom: 'var(--space-3)',
            }}
          >
            <Calendar size={18} />
            Jadwal Tes &amp; Wawancara Seleksi
          </h3>
          <div className={styles.docList}>
            {activeApp.schedules?.map((sched) => (
              <div key={sched.id} className={styles.docItem}>
                <div>
                  <div style={{ fontWeight: 700, fontSize: 'var(--text-label)' }}>
                    {sched.session_name}
                  </div>
                  <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)' }}>
                    Waktu: <strong className="tabular-nums">{formatDateTime(sched.scheduled_at)}</strong>
                    {sched.room_or_link ? ` · Tempat/Tautan: ${sched.room_or_link}` : ''}
                    {sched.examiner_name ? ` · Penguji: ${sched.examiner_name}` : ''}
                  </div>
                </div>
                <ToneBadge tone="info" label="Terjadwal" />
              </div>
            ))}
          </div>
        </Card>
      )}

      {(pendingPurpose !== null || activeApp.payments.length > 0) && (
        <Card>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
            <h3 style={{ fontSize: 'var(--text-h3)', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
              <CreditCard size={18} />
              Pembayaran &amp; Tagihan
            </h3>
            {pendingPurpose && (
              <ToneBadge
                tone="warning"
                label={
                  pendingPurpose === 'registration_fee'
                    ? `Tagihan Formulir: ${formatRupiah(activeApp.registration_fee_idr)}`
                    : `Tagihan Daftar Ulang: ${formatRupiah(activeApp.re_enrollment_fee_idr)}`
                }
              />
            )}
          </div>

          {pendingPurpose && (
            <div style={{ marginTop: 'var(--space-4)', display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)', alignItems: 'center' }}>
              <select
                aria-label="Pilih metode pembayaran"
                value={payMethod}
                onChange={(e) => setPayMethod(e.target.value as PaymentMethod)}
                style={{ padding: '0.55rem 0.75rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-strong)' }}
              >
                <option value="qris">QRIS Otomatis</option>
                <option value="va">Virtual Account Bank</option>
                <option value="ewallet">E-Wallet</option>
                <option value="manual_transfer">Transfer Bank Manual (Verifikasi Bendahara)</option>
              </select>
              <Button disabled={payBusy} onClick={() => void handleCreateInvoice(pendingPurpose)}>
                {payBusy ? 'Membuat Tagihan...' : 'Buat Tagihan Pembayaran'}
              </Button>
            </div>
          )}

          {activeApp.payments.length > 0 && (
            <div className={styles.docList}>
              {activeApp.payments.map((pay) => (
                <div key={pay.id} className={styles.docItem}>
                  <div>
                    <div className="mono-code" style={{ fontWeight: 700, fontSize: 'var(--text-label)' }}>
                      {pay.external_ref ?? pay.id.slice(0, 8)} · {formatRupiah(pay.amount_idr)}
                    </div>
                    <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>
                      {pay.purpose === 'registration_fee' ? 'Biaya Formulir' : 'Biaya Daftar Ulang'} · Metode:{' '}
                      {pay.method.toUpperCase()}
                    </div>
                    {pay.verification_note && (
                      <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', marginTop: 2 }}>
                        Catatan: {pay.verification_note}
                      </div>
                    )}
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                    <ToneBadge
                      tone={
                        pay.status === 'paid'
                          ? 'success'
                          : pay.status === 'pending'
                            ? 'warning'
                            : 'danger'
                      }
                      label={pay.status.toUpperCase()}
                    />
                    {pay.status === 'paid' && (
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => {
                          setSelectedReceiptPay(pay);
                          setPrintMode('receipt');
                        }}
                      >
                        <Printer size={14} />
                        Cetak Kwitansi
                      </Button>
                    )}
                    {pay.status === 'pending' && pay.payment_url && (
                      <a href={pay.payment_url} target="_blank" rel="noreferrer">
                        <Button size="sm" variant="secondary">
                          Bayar <ExternalLink size={14} />
                        </Button>
                      </a>
                    )}
                    {pay.status === 'pending' && pay.method !== 'manual_transfer' && (
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={syncingPaymentId === pay.id}
                        onClick={() => void handleSyncPayment(pay.id)}
                      >
                        <RefreshCw size={14} />
                        {syncingPaymentId === pay.id ? 'Memeriksa...' : 'Cek Status'}
                      </Button>
                    )}
                    {pay.status === 'pending' && pay.method === 'manual_transfer' && (
                      <label style={{ cursor: 'pointer' }}>
                        <input
                          type="file"
                          className="sr-only"
                          accept="image/jpeg,image/png,image/webp,application/pdf"
                          disabled={payBusy}
                          onChange={(e) => {
                            void handleUploadManualProof(pay.id, e.target.files?.[0]);
                            e.target.value = '';
                          }}
                        />
                        <span
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 6,
                            padding: '0.45rem 0.75rem',
                            borderRadius: 'var(--radius-md)',
                            border: '1px solid var(--border-strong)',
                            backgroundColor: 'var(--bg-surface)',
                            fontSize: 'var(--text-caption)',
                            fontWeight: 600,
                          }}
                        >
                          <Upload size={14} />
                          {pay.proof_storage_key ? 'Ganti Struk Transfer' : 'Unggah Struk'}
                        </span>
                      </label>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}

      <Card>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
          <h3 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>Kelengkapan Berkas Persyaratan</h3>
          {canFillWizard && (
            <Link to="/app/daftar">
              <Button size="sm">
                Buka Formulir &amp; Berkas <ArrowRight size={15} />
              </Button>
            </Link>
          )}
        </div>

        <div className={styles.docList}>
          {activeApp.requirements.map((req) => {
            const uploaded = activeApp.documents.find((d) => d.requirement_id === req.id);
            const tone =
              uploaded?.status === 'approved'
                ? 'success'
                : uploaded?.status === 'rejected_for_revision'
                  ? 'warning'
                  : 'neutral';
            const label = !uploaded
              ? 'Belum Diunggah'
              : uploaded.status === 'approved'
                ? 'Sah'
                : uploaded.status === 'rejected_for_revision'
                  ? 'Perlu Revisi'
                  : 'Menunggu Review';

            return (
              <div key={req.id} className={styles.docItem}>
                <div>
                  <div style={{ fontWeight: 600, fontSize: 'var(--text-label)' }}>
                    {req.label} {req.is_required ? '*' : '(Opsional)'}
                  </div>
                  <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>
                    {uploaded
                      ? `${uploaded.original_filename} (${Math.max(1, Math.round(uploaded.size_bytes / 1024))} KB)`
                      : req.description ?? `Maks ${req.max_size_kb} KB`}
                  </div>
                  {uploaded?.review_note && (
                    <div style={{ fontSize: 'var(--text-caption)', color: 'var(--status-warning-fg)', marginTop: 2 }}>
                      Catatan: {uploaded.review_note}
                    </div>
                  )}
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                  <ToneBadge tone={tone} label={label} />
                  {uploaded && (
                    <Button variant="ghost" size="sm" onClick={() => void handlePreviewDoc(uploaded.id)}>
                      Unduh
                    </Button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </Card>

      {createModal}
      <Modal
        open={printMode !== null}
        title={
          printMode === 'letter'
            ? 'Pratinjau Surat Keputusan Kelulusan Resmi'
            : printMode === 'receipt'
              ? 'Pratinjau Kwitansi Pembayaran Lunas Resmi'
              : 'Pratinjau Kartu Peserta Seleksi Masuk'
        }
        onClose={() => {
          setPrintMode(null);
          setSelectedReceiptPay(null);
        }}
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                setPrintMode(null);
                setSelectedReceiptPay(null);
              }}
            >
              Tutup
            </Button>
            <Button onClick={() => window.print()}>
              <Printer size={15} />
              Cetak / Simpan PDF
            </Button>
          </>
        }
      >
        {printSheetNode}
      </Modal>
      </div>
      {printSheetNode && <div className="print-only-root">{printSheetNode}</div>}
    </div>
  );
}
