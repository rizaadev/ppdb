import { useCallback, useEffect, useState } from 'react';
import {
  CheckCircle2,
  ExternalLink,
  Eye,
  RefreshCw,
  XCircle,
} from 'lucide-react';
import {
  apiRequest,
  extractErrorMessage,
  formatDateTime,
  formatRupiah,
} from '@/core/api/client';
import { useAuth } from '@/core/auth/AuthContext';
import type {
  PaymentStatus,
  PaymentTransaction,
  PaymentVerificationResult,
  PresignedDownloadResult,
} from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Modal } from '@/components/ui/Modal';
import { ToneBadge } from '@/components/ui/StatusBadge';
import adminStyles from './AdminShared.module.css';
import styles from './FinancePaymentsPanel.module.css';

function paymentTone(status: PaymentStatus): 'neutral' | 'info' | 'warning' | 'success' | 'danger' {
  switch (status) {
    case 'paid':
      return 'success';
    case 'pending':
      return 'warning';
    case 'failed':
      return 'danger';
    default:
      return 'neutral';
  }
}

function paymentStatusLabel(status: PaymentStatus): string {
  switch (status) {
    case 'paid':
      return 'Lunas';
    case 'pending':
      return 'Menunggu';
    case 'failed':
      return 'Ditolak';
    case 'expired':
      return 'Kedaluwarsa';
    case 'refunded':
      return 'Refund';
  }
}

function purposeLabel(purpose: string): string {
  return purpose === 're_enrollment_fee' ? 'Daftar Ulang' : 'Biaya Pendaftaran';
}

export function FinancePaymentsPanel({
  tenantSlug,
  activeUnitId,
}: {
  tenantSlug: string;
  activeUnitId: string;
}) {
  const { hasUnitAccess } = useAuth();
  const canRead = hasUnitAccess(
    activeUnitId,
    'tenant_admin',
    'unit_admin',
    'treasurer',
    'principal',
  );
  const canVerify = hasUnitAccess(activeUnitId, 'tenant_admin', 'unit_admin', 'treasurer');

  const [payments, setPayments] = useState<PaymentTransaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState('');
  const [methodFilter, setMethodFilter] = useState('');
  const [purposeFilter, setPurposeFilter] = useState('');

  const [bannerError, setBannerError] = useState('');
  const [bannerSuccess, setBannerSuccess] = useState('');

  const [activePayment, setActivePayment] = useState<PaymentTransaction | null>(null);
  const [proofUrl, setProofUrl] = useState('');
  const [proofLoading, setProofLoading] = useState(false);
  const [proofError, setProofError] = useState('');
  const [proofImgFailed, setProofImgFailed] = useState(false);
  const [verifyNote, setVerifyNote] = useState('');
  const [verifyBusy, setVerifyBusy] = useState<'paid' | 'failed' | 'sync' | null>(null);

  const loadPayments = useCallback(async () => {
    if (!activeUnitId || !canRead) {
      setPayments([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    setBannerError('');
    try {
      const params = new URLSearchParams({
        unit_id: activeUnitId,
        limit: '100',
      });
      if (statusFilter) params.set('status', statusFilter);
      if (methodFilter) params.set('method', methodFilter);
      if (purposeFilter) params.set('purpose', purposeFilter);

      const rows = await apiRequest<PaymentTransaction[]>(
        `/api/v1/admin/payments?${params.toString()}`,
        { tenantSlug },
      );
      setPayments(rows ?? []);
    } catch (err) {
      setPayments([]);
      setBannerError(extractErrorMessage(err, 'Gagal memuat transaksi pembayaran unit ini.'));
    } finally {
      setLoading(false);
    }
  }, [activeUnitId, canRead, methodFilter, purposeFilter, statusFilter, tenantSlug]);

  useEffect(() => {
    void loadPayments();
  }, [loadPayments]);

  async function openPaymentModal(payment: PaymentTransaction) {
    setActivePayment(payment);
    setVerifyNote(payment.verification_note ?? '');
    setProofUrl('');
    setProofError('');
    setProofImgFailed(false);

    if (payment.proof_storage_key) {
      setProofLoading(true);
      try {
        const res = await apiRequest<PresignedDownloadResult>(
          `/api/v1/admin/payments/${payment.id}/presign-proof`,
          { tenantSlug },
        );
        setProofUrl(res.url);
      } catch (err) {
        setProofError(extractErrorMessage(err, 'Gagal mengambil URL bukti pembayaran.'));
      } finally {
        setProofLoading(false);
      }
    }
  }

  async function handleVerifyManual(decision: 'paid' | 'failed') {
    if (!activePayment) return;
    if (decision === 'failed' && !verifyNote.trim()) {
      setProofError('Catatan penolakan wajib diisi untuk memberi tahu pendaftar.');
      return;
    }

    setVerifyBusy(decision);
    setProofError('');
    setBannerError('');
    setBannerSuccess('');
    try {
      const result = await apiRequest<PaymentVerificationResult>(
        `/api/v1/admin/payments/${activePayment.id}/verify-manual`,
        {
          method: 'POST',
          tenantSlug,
          body: {
            decision,
            note: verifyNote.trim(),
          },
        },
      );
      setBannerSuccess(
        `Transaksi ${result.payment.registration_no ?? result.payment.id} diperbarui ke ${paymentStatusLabel(result.payment.status)}.`,
      );
      setActivePayment(null);
      await loadPayments();
    } catch (err) {
      setProofError(extractErrorMessage(err, 'Gagal memverifikasi pembayaran manual.'));
    } finally {
      setVerifyBusy(null);
    }
  }

  async function handleSyncGateway() {
    if (!activePayment) return;
    setVerifyBusy('sync');
    setProofError('');
    setBannerError('');
    setBannerSuccess('');
    try {
      const result = await apiRequest<PaymentVerificationResult>(
        `/api/v1/admin/payments/${activePayment.id}/sync`,
        {
          method: 'POST',
          tenantSlug,
        },
      );
      setActivePayment(result.payment);
      setBannerSuccess(
        `Status transaksi ${result.payment.registration_no ?? result.payment.id} disinkronkan dari Midtrans: ${paymentStatusLabel(result.payment.status)}.`,
      );
      await loadPayments();
    } catch (err) {
      setProofError(extractErrorMessage(err, 'Gagal menyinkronkan status Midtrans.'));
    } finally {
      setVerifyBusy(null);
    }
  }

  const pendingCount = payments.filter((p) => p.status === 'pending').length;
  const paidItems = payments.filter((p) => p.status === 'paid');
  const paidTotal = paidItems.reduce((sum, p) => sum + p.amount_idr, 0);

  if (!canRead) {
    return (
      <Card>
        <div className={adminStyles.alertInfo}>
          Akses keuangan terbatas: modul ini hanya tersedia untuk peran{' '}
          <strong>Bendahara</strong>, <strong>Kepala Sekolah</strong>, atau{' '}
          <strong>Admin Unit/Instansi</strong> pada unit terpilih.
        </div>
      </Card>
    );
  }

  return (
    <div className={adminStyles.stack}>
      <div className={adminStyles.headerRow}>
        <div>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
            Rekonsiliasi Keuangan &amp; Pembayaran
          </h1>
          <p className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
            Pantau tagihan QRIS / VA / E-Wallet serta verifikasi bukti transfer manual dari pendaftar.
          </p>
        </div>

        <div className={adminStyles.filterBar}>
          <select
            aria-label="Filter status pembayaran"
            className={adminStyles.select}
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
          >
            <option value="">Semua Status</option>
            <option value="pending">Menunggu Verifikasi</option>
            <option value="paid">Lunas</option>
            <option value="failed">Ditolak</option>
            <option value="expired">Kedaluwarsa</option>
          </select>

          <select
            aria-label="Filter metode pembayaran"
            className={adminStyles.select}
            value={methodFilter}
            onChange={(e) => setMethodFilter(e.target.value)}
          >
            <option value="">Semua Metode</option>
            <option value="manual_transfer">Transfer Manual</option>
            <option value="qris">QRIS</option>
            <option value="va">Virtual Account</option>
            <option value="ewallet">E-Wallet</option>
          </select>

          <select
            aria-label="Filter jenis tagihan"
            className={adminStyles.select}
            value={purposeFilter}
            onChange={(e) => setPurposeFilter(e.target.value)}
          >
            <option value="">Semua Tagihan</option>
            <option value="registration_fee">Biaya Pendaftaran</option>
            <option value="re_enrollment_fee">Biaya Daftar Ulang</option>
          </select>

          <Button size="sm" variant="secondary" onClick={() => void loadPayments()}>
            <RefreshCw size={14} />
            Muat Ulang
          </Button>
        </div>
      </div>

      {bannerError && (
        <div role="alert" className={adminStyles.alertError}>
          {bannerError}
        </div>
      )}
      {bannerSuccess && (
        <div role="status" className={adminStyles.alertSuccess}>
          {bannerSuccess}
        </div>
      )}

      <div className={styles.kpiGrid}>
        <Card compact>
          <div className={styles.kpiLabel}>Total Transaksi Unit</div>
          <div className={`${styles.kpiValue} tabular-nums`}>{payments.length}</div>
        </Card>
        <Card compact>
          <div className={styles.kpiLabel}>Menunggu Verifikasi</div>
          <div className={`${styles.kpiValue} tabular-nums`}>{pendingCount}</div>
        </Card>
        <Card compact>
          <div className={styles.kpiLabel}>Total Penerimaan Lunas</div>
          <div className={`${styles.kpiValue} tabular-nums`}>
            {paidItems.length > 0 ? formatRupiah(paidTotal) : 'Rp 0'}
          </div>
        </Card>
      </div>

      <Card>
        {loading ? (
          <div className={adminStyles.emptyState}>Memuat daftar transaksi pembayaran...</div>
        ) : payments.length === 0 ? (
          <div className={adminStyles.emptyState}>
            Belum ada transaksi pembayaran yang sesuai filter unit ini.
          </div>
        ) : (
          <div className={adminStyles.tableWrap}>
            <table className={adminStyles.table}>
              <thead>
                <tr>
                  <th>Pendaftar</th>
                  <th>Jenis &amp; Jalur</th>
                  <th>Metode</th>
                  <th>Nominal</th>
                  <th>Status</th>
                  <th>Waktu</th>
                  <th>Aksi</th>
                </tr>
              </thead>
              <tbody>
                {payments.map((p) => (
                  <tr key={p.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>{p.student_name || '-'}</div>
                      <div className={`mono-code ${adminStyles.metaMuted}`}>
                        {p.registration_no || p.application_id.slice(0, 8)}
                      </div>
                    </td>
                    <td>
                      <div>{purposeLabel(p.purpose)}</div>
                      <div className={adminStyles.metaMuted}>{p.track_name || '-'}</div>
                    </td>
                    <td>
                      <span className="mono-code">{p.method}</span>
                      {p.proof_storage_key && (
                        <div className={adminStyles.metaMuted}>Bukti terunggah</div>
                      )}
                    </td>
                    <td className="tabular-nums" style={{ fontWeight: 600 }}>
                      {formatRupiah(p.amount_idr)}
                    </td>
                    <td>
                      <ToneBadge
                        tone={paymentTone(p.status)}
                        label={paymentStatusLabel(p.status)}
                      />
                    </td>
                    <td className={adminStyles.metaMuted}>
                      {formatDateTime(p.paid_at ?? p.created_at)}
                    </td>
                    <td>
                      <div className={styles.actionGroup}>
                        <Button
                          size="sm"
                          variant={p.status === 'pending' && canVerify ? 'primary' : 'secondary'}
                          onClick={() => void openPaymentModal(p)}
                        >
                          <Eye size={14} />
                          {p.status === 'pending' && canVerify ? 'Periksa / Sahkan' : 'Detail'}
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal
        open={Boolean(activePayment)}
        title="Detail & Verifikasi Pembayaran"
        onClose={() => {
          if (!verifyBusy) setActivePayment(null);
        }}
      >
        {activePayment && (
          <div className={adminStyles.stack}>
            <div style={{ fontSize: 'var(--text-label)' }}>
              <strong>{activePayment.student_name || activePayment.registration_no}</strong> ·{' '}
              <span className="mono-code">{activePayment.registration_no}</span>
              <div className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
                {purposeLabel(activePayment.purpose)} · Metode:{' '}
                <span className="mono-code">{activePayment.method}</span> · Nominal:{' '}
                <strong>{formatRupiah(activePayment.amount_idr)}</strong>
              </div>
            </div>

            {activePayment.proof_storage_key ? (
              <div className={styles.proofBox}>
                {proofLoading ? (
                  <span className={adminStyles.metaMuted}>Memuat URL bukti transfer...</span>
                ) : proofUrl ? (
                  <>
                    {!proofImgFailed ? (
                      <img
                        src={proofUrl}
                        alt="Bukti Transfer Manual"
                        className={styles.proofImg}
                        onError={() => setProofImgFailed(true)}
                      />
                    ) : (
                      <p className={adminStyles.metaMuted}>
                        Pratinjau gambar langsung belum termuat (pastikan object storage aktif dan gambar dapat diakses).
                      </p>
                    )}
                    <div style={{ marginTop: 'var(--space-2)' }}>
                      <a
                        href={proofUrl}
                        target="_blank"
                        rel="noreferrer"
                        style={{
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: 6,
                          color: 'var(--brand-primary)',
                          fontSize: 'var(--text-caption)',
                          fontWeight: 600,
                        }}
                      >
                        <ExternalLink size={14} />
                        Buka Bukti Transfer di Tab Baru
                      </a>
                    </div>
                  </>
                ) : null}
              </div>
            ) : (
              <div className={adminStyles.alertInfo}>
                {activePayment.method === 'manual_transfer'
                  ? 'Pendaftar belum mengunggah foto struk/bukti transfer manual.'
                  : `Tagihan gateway (${activePayment.method}) diselesaikan otomatis melalui webhook Midtrans atau tombol sinkronisasi aktif di bawah.`}
              </div>
            )}

            {activePayment.method !== 'manual_transfer' && activePayment.status === 'pending' && (
              <div style={{ display: 'flex', justifyContent: 'flex-start' }}>
                <Button
                  variant="secondary"
                  size="sm"
                  loading={verifyBusy === 'sync'}
                  disabled={verifyBusy !== null}
                  onClick={() => void handleSyncGateway()}
                >
                  <RefreshCw size={14} />
                  Sinkronkan Status Midtrans
                </Button>
              </div>
            )}

            {proofError && (
              <div role="alert" className={adminStyles.alertError}>
                {proofError}
              </div>
            )}

            {canVerify && activePayment.status === 'pending' && (
              <>
                <div>
                  <label htmlFor="verify-pay-note" style={{ fontSize: 'var(--text-label)', fontWeight: 600 }}>
                    Catatan Bendahara
                  </label>
                  <textarea
                    id="verify-pay-note"
                    className={styles.noteTextarea}
                    placeholder="Contoh: Mutasi BCA sesuai Rp 250.000 / Nominal belum sesuai..."
                    value={verifyNote}
                    onChange={(e) => setVerifyNote(e.target.value)}
                  />
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-3)' }}>
                  <Button
                    variant="secondary"
                    loading={verifyBusy === 'failed'}
                    disabled={verifyBusy !== null}
                    onClick={() => void handleVerifyManual('failed')}
                  >
                    <XCircle size={15} />
                    Tolak Bukti
                  </Button>
                  <Button
                    loading={verifyBusy === 'paid'}
                    disabled={verifyBusy !== null}
                    onClick={() => void handleVerifyManual('paid')}
                  >
                    <CheckCircle2 size={15} />
                    Sahkan Lunas
                  </Button>
                </div>
              </>
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}

