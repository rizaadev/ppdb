import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  Check,
  ExternalLink,
  FileText,
  Inbox,
  RefreshCw,
  RotateCw,
  ShieldCheck,
  XCircle,
  ZoomIn,
  ZoomOut,
} from 'lucide-react';
import {
  apiRequest,
  extractErrorMessage,
  formatDateTime,
} from '@/core/api/client';
import { useAuth } from '@/core/auth/AuthContext';
import type {
  ApplicationDetail,
  ApplicationSummary,
  DocumentReviewStatus,
  PresignedDownloadResult,
} from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { StatusBadge, ToneBadge } from '@/components/ui/StatusBadge';
import adminStyles from './AdminShared.module.css';
import styles from './VerificationWorkbenchPage.module.css';

const QUICK_REASONS = [
  'Tanggal terbit KK terpotong',
  'Foto berkas buram / tidak terbaca',
  'Dokumen tidak sesuai jalur yang dipilih',
];

const STATUS_FILTER_OPTIONS = [
  { value: '', label: 'Semua Status' },
  { value: 'submitted', label: 'Menunggu Antrean (Submitted)' },
  { value: 'under_verification', label: 'Sedang Diperiksa' },
  { value: 'revision_required', label: 'Perlu Perbaikan' },
  { value: 'verified', label: 'Lolos Berkas' },
  { value: 'accepted', label: 'Diterima' },
  { value: 'enrolled', label: 'Terdaftar Resmi' },
];

export function VerificationWorkbenchPage({
  tenantSlug,
  activeUnitId,
}: {
  tenantSlug: string;
  activeUnitId: string;
}) {
  const { hasUnitAccess } = useAuth();
  const canVerify = hasUnitAccess(activeUnitId, 'tenant_admin', 'unit_admin', 'verifier');

  const [queue, setQueue] = useState<ApplicationSummary[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState('');
  const [searchInput, setSearchInput] = useState('');
  const [appliedSearch, setAppliedSearch] = useState('');

  const [selectedId, setSelectedId] = useState('');
  const [selectedDetail, setSelectedDetail] = useState<ApplicationDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const [activeDocIndex, setActiveDocIndex] = useState(0);
  const [previewUrl, setPreviewUrl] = useState('');
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState('');
  const [imgFailed, setImgFailed] = useState(false);

  const [mobileTab, setMobileTab] = useState<'preview' | 'decision'>('preview');
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);

  const [docNote, setDocNote] = useState('');
  const [summaryNote, setSummaryNote] = useState('');
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [bannerError, setBannerError] = useState('');
  const [bannerSuccess, setBannerSuccess] = useState('');

  const loadQueue = useCallback(
    async (preferredAppId?: string) => {
      if (!activeUnitId) return;
      setListLoading(true);
      setBannerError('');
      try {
        const params = new URLSearchParams({
          unit_id: activeUnitId,
          limit: '50',
        });
        if (statusFilter) params.set('status', statusFilter);
        if (appliedSearch) params.set('search', appliedSearch);

        const rows = await apiRequest<ApplicationSummary[]>(
          `/api/v1/admin/applications?${params.toString()}`,
          { tenantSlug },
        );
        const safeRows = rows ?? [];
        setQueue(safeRows);
        setSelectedId((prev) => {
          if (preferredAppId && safeRows.some((r) => r.id === preferredAppId)) {
            return preferredAppId;
          }
          if (prev && safeRows.some((r) => r.id === prev)) {
            return prev;
          }
          return safeRows[0]?.id ?? '';
        });
      } catch (err) {
        setQueue([]);
        setSelectedId('');
        setSelectedDetail(null);
        setBannerError(extractErrorMessage(err, 'Gagal memuat daftar pendaftar unit ini.'));
      } finally {
        setListLoading(false);
      }
    },
    [activeUnitId, appliedSearch, statusFilter, tenantSlug],
  );

  const loadDetail = useCallback(
    async (appId: string) => {
      if (!appId) {
        setSelectedDetail(null);
        return;
      }
      setDetailLoading(true);
      try {
        const detail = await apiRequest<ApplicationDetail>(
          `/api/v1/admin/applications/${appId}`,
          { tenantSlug },
        );
        setSelectedDetail(detail);
        setActiveDocIndex(0);
        setDocNote(detail.documents[0]?.review_note ?? '');
      } catch (err) {
        setSelectedDetail(null);
        setBannerError(extractErrorMessage(err, 'Gagal memuat detail berkas pendaftar.'));
      } finally {
        setDetailLoading(false);
      }
    },
    [tenantSlug],
  );

  useEffect(() => {
    void loadQueue();
  }, [loadQueue]);

  useEffect(() => {
    void loadDetail(selectedId);
  }, [selectedId, loadDetail]);

  const documents = selectedDetail?.documents ?? [];
  const currentDoc = documents[activeDocIndex] ?? documents[0] ?? null;

  const loadPresignedPreview = useCallback(
    async (appId: string, docId: string) => {
      setPreviewLoading(true);
      setPreviewError('');
      setPreviewUrl('');
      setImgFailed(false);
      try {
        const signed = await apiRequest<PresignedDownloadResult>(
          `/api/v1/admin/applications/${appId}/documents/${docId}/presign-download`,
          { tenantSlug },
        );
        setPreviewUrl(signed.url);
      } catch (err) {
        setPreviewError(extractErrorMessage(err, 'Gagal membuat URL pratinjau dokumen.'));
      } finally {
        setPreviewLoading(false);
      }
    },
    [tenantSlug],
  );

  useEffect(() => {
    if (!selectedDetail || !currentDoc) {
      setPreviewUrl('');
      setPreviewError('');
      return;
    }
    setZoom(1);
    setRotation(0);
    setDocNote(currentDoc.review_note ?? '');
    void loadPresignedPreview(selectedDetail.application.id, currentDoc.id);
  }, [selectedDetail?.application.id, currentDoc?.id, loadPresignedPreview]);

  async function handleClaimNext() {
    setBusyAction('claim');
    setBannerError('');
    setBannerSuccess('');
    try {
      const claimed = await apiRequest<ApplicationDetail>(
        '/api/v1/admin/applications/claim-next',
        {
          method: 'POST',
          tenantSlug,
          body: { unit_id: activeUnitId },
        },
      );
      setSelectedDetail(claimed);
      setSelectedId(claimed.application.id);
      setActiveDocIndex(0);
      setDocNote(claimed.documents[0]?.review_note ?? '');
      setBannerSuccess(
        `Antrean ${claimed.application.registration_no} berhasil diklaim untuk diperiksa.`,
      );
      await loadQueue(claimed.application.id);
    } catch (err) {
      setBannerError(
        extractErrorMessage(err, 'Tidak ada antrean baru atau Anda tidak memiliki izin klaim.'),
      );
    } finally {
      setBusyAction(null);
    }
  }

  async function handleReviewDoc(status: DocumentReviewStatus) {
    if (!selectedDetail || !currentDoc) return;
    if (status === 'rejected_for_revision' && !docNote.trim()) {
      setBannerError('Catatan pemeriksaan wajib diisi saat meminta perbaikan berkas.');
      return;
    }

    setBusyAction(status);
    setBannerError('');
    setBannerSuccess('');
    try {
      await apiRequest(
        `/api/v1/admin/applications/${selectedDetail.application.id}/documents/${currentDoc.id}/review`,
        {
          method: 'PATCH',
          tenantSlug,
          body: {
            status,
            review_note: docNote.trim(),
          },
        },
      );
      const refreshed = await apiRequest<ApplicationDetail>(
        `/api/v1/admin/applications/${selectedDetail.application.id}`,
        { tenantSlug },
      );
      setSelectedDetail(refreshed);
      setBannerSuccess(
        status === 'approved'
          ? `Berkas ${currentDoc.requirement_label} disetujui.`
          : `Berkas ${currentDoc.requirement_label} ditandai perlu perbaikan.`,
      );
      await loadQueue(refreshed.application.id);
    } catch (err) {
      setBannerError(extractErrorMessage(err, 'Gagal menyimpan hasil pemeriksaan dokumen.'));
    } finally {
      setBusyAction(null);
    }
  }

  async function handleCompleteVerification() {
    if (!selectedDetail) return;
    setBusyAction('complete');
    setBannerError('');
    setBannerSuccess('');
    try {
      const updated = await apiRequest<ApplicationDetail>(
        `/api/v1/admin/applications/${selectedDetail.application.id}/complete-verification`,
        {
          method: 'POST',
          tenantSlug,
          body: {
            version: selectedDetail.application.version,
            summary_note: summaryNote.trim(),
          },
        },
      );
      setSelectedDetail(updated);
      setSummaryNote('');
      setBannerSuccess(
        `Verifikasi ${updated.application.registration_no} selesai (status: ${updated.application.status}).`,
      );
      await loadQueue(updated.application.id);
    } catch (err) {
      setBannerError(extractErrorMessage(err, 'Gagal menyelesaikan verifikasi berkas.'));
    } finally {
      setBusyAction(null);
    }
  }

  function handleSearchSubmit(e: FormEvent) {
    e.preventDefault();
    setAppliedSearch(searchInput.trim());
  }

  const app = selectedDetail?.application;
  const profile = selectedDetail?.profile;
  const primaryGuardian =
    selectedDetail?.guardians.find((g) => g.is_primary_contact) ?? selectedDetail?.guardians[0];
  const canComplete =
    Boolean(app) &&
    canVerify &&
    (app?.status === 'submitted' || app?.status === 'under_verification');

  return (
    <div className={styles.stack}>
      <div className={styles.headerRow}>
        <div>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
            Verifikasi Berkas Pendaftar
          </h1>
          {app ? (
            <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-2)', marginTop: 4 }}>
              <span className="mono-code" style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>
                {app.registration_no}
              </span>
              <StatusBadge status={app.status} />
              <span className={adminStyles.metaMuted}>Versi v{app.version}</span>
            </div>
          ) : (
            <p className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
              Pilih pendaftar dari daftar atau klaim antrean berikutnya pada unit terpilih.
            </p>
          )}
        </div>

        <div className={adminStyles.filterBar}>
          <form onSubmit={handleSearchSubmit} className={adminStyles.filterBar}>
            <input
              type="search"
              className={adminStyles.searchInput}
              placeholder="Cari nama / no registrasi / NISN..."
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
            />
            <Button size="sm" variant="secondary" type="submit">
              Cari
            </Button>
          </form>

          <select
            aria-label="Filter status pendaftar"
            className={adminStyles.select}
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
          >
            {STATUS_FILTER_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>

          <Button size="sm" variant="secondary" onClick={() => void loadQueue(selectedId)}>
            <RefreshCw size={14} />
            Muat Ulang
          </Button>

          {canVerify && (
            <Button
              size="sm"
              loading={busyAction === 'claim'}
              onClick={() => void handleClaimNext()}
            >
              <Inbox size={15} />
              Klaim Antrean Berikutnya
            </Button>
          )}
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

      {!canVerify && (
        <div className={adminStyles.alertInfo}>
          Mode baca: peran Anda pada unit ini hanya dapat meninjau data pendaftar. Tindakan
          persetujuan/revisi berkas memerlukan peran Verifikator atau Admin Unit.
        </div>
      )}

      <Card compact>
        {listLoading ? (
          <div className={adminStyles.metaMuted}>Memuat antrean pendaftar unit...</div>
        ) : queue.length === 0 ? (
          <div className={adminStyles.emptyState}>
            Belum ada pendaftar yang cocok dengan filter unit saat ini.
          </div>
        ) : (
          <div className={adminStyles.queueStrip}>
            {queue.map((item) => {
              const active = item.id === selectedId;
              return (
                <button
                  key={item.id}
                  type="button"
                  className={`${adminStyles.queueButton} ${active ? adminStyles.queueButtonActive : ''}`}
                  onClick={() => setSelectedId(item.id)}
                >
                  <span>{item.student_name || item.registration_no}</span>
                  <StatusBadge status={item.status} />
                </button>
              );
            })}
          </div>
        )}
      </Card>

      {detailLoading && !selectedDetail && (
        <Card>
          <div className={adminStyles.emptyState}>Memuat berkas dan biodata pendaftar...</div>
        </Card>
      )}

      {selectedDetail && app && (
        <>
          <div className={styles.mobileTabs}>
            <Button
              size="sm"
              variant={mobileTab === 'preview' ? 'primary' : 'secondary'}
              onClick={() => setMobileTab('preview')}
            >
              1. Lihat Berkas
            </Button>
            <Button
              size="sm"
              variant={mobileTab === 'decision' ? 'primary' : 'secondary'}
              onClick={() => setMobileTab('decision')}
            >
              2. Data &amp; Keputusan
            </Button>
          </div>

          <div className={styles.splitGrid}>
            <Card className={mobileTab === 'decision' ? styles.hiddenOnMobile : ''}>
              {documents.length === 0 ? (
                <div className={adminStyles.emptyState}>
                  <FileText size={24} style={{ margin: '0 auto var(--space-2)' }} />
                  Pendaftar ini belum mengunggah berkas dokumen apapun.
                </div>
              ) : (
                <>
                  <div className={styles.docSwitcher}>
                    {documents.map((doc, idx) => (
                      <button
                        key={doc.id}
                        type="button"
                        className={`${styles.docTab} ${idx === activeDocIndex ? styles.docTabActive : ''}`}
                        onClick={() => {
                          setActiveDocIndex(idx);
                          setZoom(1);
                          setRotation(0);
                          setDocNote(doc.review_note ?? '');
                        }}
                      >
                        {doc.requirement_label}
                      </button>
                    ))}
                  </div>

                  {currentDoc && (
                    <>
                      <div className={styles.viewerToolbar}>
                        <span style={{ fontSize: 'var(--text-caption)', fontWeight: 600 }}>
                          {currentDoc.original_filename} ({Math.max(1, Math.round(currentDoc.size_bytes / 1024))} KB)
                        </span>
                        <div style={{ display: 'flex', gap: 6 }}>
                          <Button
                            size="sm"
                            variant="secondary"
                            aria-label="Perkecil dokumen"
                            onClick={() => setZoom((z) => Math.max(0.6, Number((z - 0.2).toFixed(1))))}
                          >
                            <ZoomOut size={15} />
                          </Button>
                          <Button
                            size="sm"
                            variant="secondary"
                            aria-label="Perbesar dokumen"
                            onClick={() => setZoom((z) => Math.min(2.2, Number((z + 0.2).toFixed(1))))}
                          >
                            <ZoomIn size={15} />
                          </Button>
                          <Button
                            size="sm"
                            variant="secondary"
                            aria-label="Putar dokumen 90 derajat"
                            onClick={() => setRotation((r) => (r + 90) % 360)}
                          >
                            <RotateCw size={15} />
                          </Button>
                          {previewUrl && (
                            <a
                              href={previewUrl}
                              target="_blank"
                              rel="noreferrer"
                              style={{
                                display: 'inline-flex',
                                alignItems: 'center',
                                gap: 4,
                                padding: '0 10px',
                                minHeight: 36,
                                borderRadius: 'var(--radius-md)',
                                border: '1px solid var(--border-strong)',
                                backgroundColor: 'var(--bg-surface)',
                                fontSize: 'var(--text-caption)',
                                fontWeight: 600,
                              }}
                            >
                              <ExternalLink size={14} />
                              Buka
                            </a>
                          )}
                        </div>
                      </div>

                      <div className={styles.viewerFrame}>
                        {previewLoading ? (
                          <span className={adminStyles.metaMuted}>Menyiapkan pratinjau dokumen...</span>
                        ) : previewError ? (
                          <div className={adminStyles.alertError}>{previewError}</div>
                        ) : currentDoc.mime_type === 'application/pdf' || imgFailed ? (
                          <div style={{ textAlign: 'center', maxWidth: 360 }}>
                            <FileText size={32} style={{ margin: '0 auto var(--space-2)', color: 'var(--text-secondary)' }} />
                            <p style={{ fontSize: 'var(--text-label)', fontWeight: 600 }}>
                              {currentDoc.mime_type === 'application/pdf'
                                ? 'Dokumen berformat PDF'
                                : 'Pratinjau gambar langsung tidak tersedia (MinIO dev belum aktif atau objek belum diunggah)'}
                            </p>
                            {previewUrl && (
                              <a
                                href={previewUrl}
                                target="_blank"
                                rel="noreferrer"
                                style={{
                                  display: 'inline-flex',
                                  alignItems: 'center',
                                  gap: 6,
                                  marginTop: 'var(--space-3)',
                                  color: 'var(--brand-primary)',
                                  fontWeight: 600,
                                  fontSize: 'var(--text-caption)',
                                }}
                              >
                                <ExternalLink size={14} />
                                Buka URL Presigned di Tab Baru
                              </a>
                            )}
                          </div>
                        ) : (
                          <img
                            src={previewUrl}
                            alt={`Pratinjau ${currentDoc.requirement_label}`}
                            className={styles.viewerImg}
                            style={{ transform: `scale(${zoom}) rotate(${rotation}deg)` }}
                            onError={() => setImgFailed(true)}
                          />
                        )}
                      </div>
                    </>
                  )}
                </>
              )}
            </Card>

            <Card className={mobileTab === 'preview' ? styles.hiddenOnMobile : ''}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-3)' }}>
                <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>
                  Ringkasan Biodata &amp; Keputusan
                </h2>
                {currentDoc && (
                  <ToneBadge
                    tone={
                      currentDoc.status === 'approved'
                        ? 'success'
                        : currentDoc.status === 'rejected_for_revision'
                          ? 'warning'
                          : 'neutral'
                    }
                    label={
                      currentDoc.status === 'approved'
                        ? 'Berkas Sah'
                        : currentDoc.status === 'rejected_for_revision'
                          ? 'Ditandai Revisi'
                          : 'Menunggu Review'
                    }
                  />
                )}
              </div>

              <table className={styles.dataTable}>
                <tbody>
                  <tr>
                    <th>Nama Lengkap</th>
                    <td>{profile?.full_name || app.student_name || '-'}</td>
                  </tr>
                  <tr>
                    <th>Unit &amp; Jalur</th>
                    <td>
                      {app.unit_name || '-'} · {app.track_name || '-'}
                    </td>
                  </tr>
                  <tr>
                    <th>NIK / NISN</th>
                    <td className="mono-code">
                      {profile?.nik || '-'} / {profile?.nisn || app.nisn || '-'}
                    </td>
                  </tr>
                  <tr>
                    <th>Nomor KK &amp; Tgl Terbit</th>
                    <td className="mono-code">
                      {profile?.kk_number || '-'} ({profile?.kk_issued_date || '-'})
                    </td>
                  </tr>
                  <tr>
                    <th>Alamat Domisili</th>
                    <td>
                      {[
                        profile?.address_street,
                        profile?.village_name,
                        profile?.district_name,
                        profile?.regency_name,
                      ]
                        .filter(Boolean)
                        .join(', ') || '-'}
                    </td>
                  </tr>
                  {app.distance_meters !== undefined && app.distance_meters !== null && (
                    <tr>
                      <th>Jarak Terhitung</th>
                      <td className="tabular-nums">{app.distance_meters} meter</td>
                    </tr>
                  )}
                  <tr>
                    <th>Kontak Wali Utama</th>
                    <td>
                      {primaryGuardian
                        ? `${primaryGuardian.full_name} (${primaryGuardian.phone_whatsapp || '-'})`
                        : '-'}
                    </td>
                  </tr>
                </tbody>
              </table>

              {currentDoc && (
                <div style={{ marginTop: 'var(--space-4)' }}>
                  <label htmlFor="review-note" style={{ fontSize: 'var(--text-label)', fontWeight: 600 }}>
                    Catatan Pemeriksaan ({currentDoc.requirement_label})
                  </label>
                  <div className={styles.chipRow}>
                    {QUICK_REASONS.map((reason) => (
                      <button
                        key={reason}
                        type="button"
                        className={styles.quickChip}
                        onClick={() => setDocNote(reason)}
                      >
                        + {reason}
                      </button>
                    ))}
                  </div>
                  <textarea
                    id="review-note"
                    className={styles.textarea}
                    placeholder="Wajib diisi bila menandai berkas untuk perbaikan..."
                    value={docNote}
                    onChange={(e) => setDocNote(e.target.value)}
                  />

                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-3)', marginTop: 'var(--space-3)' }}>
                    <Button
                      variant="secondary"
                      disabled={!canVerify}
                      loading={busyAction === 'rejected_for_revision'}
                      onClick={() => void handleReviewDoc('rejected_for_revision')}
                    >
                      <XCircle size={16} />
                      Minta Perbaikan
                    </Button>
                    <Button
                      disabled={!canVerify}
                      loading={busyAction === 'approved'}
                      onClick={() => void handleReviewDoc('approved')}
                    >
                      <Check size={16} />
                      Setujui Berkas
                    </Button>
                  </div>
                </div>
              )}

              <hr className={adminStyles.divider} />

              <div>
                <label htmlFor="summary-note" style={{ fontSize: 'var(--text-label)', fontWeight: 600 }}>
                  Finalisasi Verifikasi Pendaftar
                </label>
                <p className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
                  Mengunci keputusan akhir berkas: jika seluruh dokumen wajib disetujui, status
                  berubah ke <strong>Lolos Berkas</strong>; jika ada yang ditandai perbaikan,
                  pendaftar diminta mengunggah revisi.
                </p>
                <textarea
                  id="summary-note"
                  className={styles.textarea}
                  placeholder="Catatan ringkas panitia verifikasi (opsional)..."
                  value={summaryNote}
                  onChange={(e) => setSummaryNote(e.target.value)}
                />
                <div style={{ marginTop: 'var(--space-3)' }}>
                  <Button
                    fullWidth
                    disabled={!canComplete}
                    loading={busyAction === 'complete'}
                    onClick={() => void handleCompleteVerification()}
                  >
                    <ShieldCheck size={16} />
                    Selesaikan Verifikasi (v{app.version})
                  </Button>
                </div>
              </div>

              {selectedDetail.audit_logs.length > 0 && (
                <>
                  <hr className={adminStyles.divider} />
                  <div>
                    <h3 style={{ fontSize: 'var(--text-label)', fontWeight: 700, marginBottom: 'var(--space-2)' }}>
                      Riwayat Audit Terbaru
                    </h3>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                      {selectedDetail.audit_logs.slice(0, 5).map((log) => (
                        <div key={log.id} className={adminStyles.metaMuted}>
                          <strong>{log.action_type}</strong> · {log.actor_role} ·{' '}
                          {formatDateTime(log.created_at)}
                          {log.reason ? ` — "${log.reason}"` : ''}
                        </div>
                      ))}
                    </div>
                  </div>
                </>
              )}
            </Card>
          </div>
        </>
      )}
    </div>
  );
}
