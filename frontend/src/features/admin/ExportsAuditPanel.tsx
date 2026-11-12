import { useCallback, useEffect, useMemo, useState } from 'react';
import { Download, FileSpreadsheet, History, RefreshCw } from 'lucide-react';
import {
  apiRequest,
  downloadProtectedFile,
  extractErrorMessage,
  formatDateTime,
} from '@/core/api/client';
import type {
  ExportFormat,
  ExportJob,
  PublicPortalPayload,
  TenantAuditLogItem,
} from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { ToneBadge } from '@/components/ui/StatusBadge';
import adminStyles from './AdminShared.module.css';

const FORMAT_LABELS: Record<ExportFormat, string> = {
  dapodik_xlsx: 'Dapodik (.xlsx)',
  emis_xlsx: 'EMIS (.xlsx)',
  custom_csv: 'CSV Lengkap (.csv)',
};

function jobStatusTone(status: ExportJob['status']): 'success' | 'danger' | 'info' | 'warning' {
  switch (status) {
    case 'completed':
      return 'success';
    case 'failed':
      return 'danger';
    case 'processing':
      return 'info';
    default:
      return 'warning';
  }
}

export function ExportsAuditPanel({
  tenantSlug,
  portal,
  activeUnitId,
}: {
  tenantSlug: string;
  portal: PublicPortalPayload;
  activeUnitId: string;
}) {
  const [unitId, setUnitId] = useState(activeUnitId || portal.units[0]?.id || '');
  const unitPeriods = useMemo(
    () => portal.periods.filter((p) => !unitId || p.unit_id === unitId),
    [portal.periods, unitId],
  );
  const [periodId, setPeriodId] = useState(unitPeriods[0]?.id || portal.periods[0]?.id || '');
  const [format, setFormat] = useState<ExportFormat>('dapodik_xlsx');

  const [jobs, setJobs] = useState<ExportJob[]>([]);
  const [jobsLoading, setJobsLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [downloadingId, setDownloadingId] = useState('');

  const [auditLogs, setAuditLogs] = useState<TenantAuditLogItem[]>([]);
  const [knownActions, setKnownActions] = useState<string[]>([]);
  const [auditLoading, setAuditLoading] = useState(false);
  const [actionFilter, setActionFilter] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [limit, setLimit] = useState(50);

  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  useEffect(() => {
    if (activeUnitId) {
      setUnitId(activeUnitId);
    }
  }, [activeUnitId]);

  useEffect(() => {
    if (!unitPeriods.some((p) => p.id === periodId)) {
      setPeriodId(unitPeriods[0]?.id ?? '');
    }
  }, [unitPeriods, periodId]);

  const loadJobs = useCallback(async () => {
    setJobsLoading(true);
    try {
      const qs = new URLSearchParams({ limit: '25' });
      if (unitId) qs.set('unit_id', unitId);
      const data = await apiRequest<ExportJob[]>(`/api/v1/admin/exports?${qs.toString()}`, {
        tenantSlug,
      });
      setJobs(data);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memuat riwayat ekspor.'));
    } finally {
      setJobsLoading(false);
    }
  }, [tenantSlug, unitId]);

  const loadAuditLogs = useCallback(async () => {
    setAuditLoading(true);
    try {
      const qs = new URLSearchParams({ limit: String(limit) });
      if (unitId) qs.set('unit_id', unitId);
      if (actionFilter.trim()) qs.set('action_type', actionFilter.trim());
      const data = await apiRequest<TenantAuditLogItem[]>(
        `/api/v1/admin/audit-logs?${qs.toString()}`,
        { tenantSlug },
      );
      setAuditLogs(data);
      setKnownActions((prev) => {
        const set = new Set(prev);
        data.forEach((l) => {
          if (l.action_type) set.add(l.action_type);
        });
        return Array.from(set).sort();
      });
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memuat audit log.'));
    } finally {
      setAuditLoading(false);
    }
  }, [tenantSlug, unitId, actionFilter, limit]);

  useEffect(() => {
    setError('');
    void Promise.all([loadJobs(), loadAuditLogs()]);
  }, [loadJobs, loadAuditLogs]);

  const hasActiveJob = jobs.some((j) => j.status === 'pending' || j.status === 'processing');
  useEffect(() => {
    if (!hasActiveJob) return;
    const timer = window.setInterval(() => {
      void loadJobs();
    }, 2500);
    return () => window.clearInterval(timer);
  }, [hasActiveJob, loadJobs]);

  async function handleCreateExport() {
    if (!unitId || !periodId) {
      setError('Pilih unit dan periode terlebih dahulu sebelum membuat ekspor.');
      return;
    }
    setCreating(true);
    setError('');
    setNotice('');
    try {
      const created = await apiRequest<ExportJob>('/api/v1/admin/exports', {
        method: 'POST',
        tenantSlug,
        body: {
          unit_id: unitId,
          period_id: periodId,
          format,
        },
      });
      setNotice(
        created.status === 'completed'
          ? `Berkas ${FORMAT_LABELS[created.format]} siap diunduh (${created.row_count ?? 0} baris).`
          : `Permintaan ekspor ${FORMAT_LABELS[created.format]} diterima (status: ${created.status}).`,
      );
      await Promise.all([loadJobs(), loadAuditLogs()]);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal membuat pekerjaan ekspor.'));
    } finally {
      setCreating(false);
    }
  }

  async function handleDownload(job: ExportJob) {
    const targetUrl = job.download_url || `/api/v1/admin/exports/${job.id}/download`;
    const fallbackName =
      job.filename ||
      `${job.format}-${job.id.slice(0, 8)}.${job.format === 'custom_csv' ? 'csv' : 'xlsx'}`;
    setDownloadingId(job.id);
    setError('');
    try {
      await downloadProtectedFile(targetUrl, fallbackName, tenantSlug);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal mengunduh file ekspor.'));
    } finally {
      setDownloadingId('');
    }
  }

  const filteredLogs = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return auditLogs;
    return auditLogs.filter((log) => {
      const hay = [
        log.registration_no,
        log.student_name,
        log.actor_name,
        log.actor_role,
        log.action_type,
        log.entity_type,
        log.from_status,
        log.to_status,
        log.reason,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();
      return hay.includes(q);
    });
  }, [auditLogs, searchQuery]);

  return (
    <div className={adminStyles.stack}>
      <div className={adminStyles.headerRow}>
        <div>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
            Ekspor Data Nasional &amp; Audit Log
          </h1>
          <p className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
            Unduh data murid terdaftar sesuai format Dapodik / EMIS / CSV dan telusuri jejak audit perubahan status.
          </p>
        </div>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => {
            setError('');
            void Promise.all([loadJobs(), loadAuditLogs()]);
          }}
        >
          <RefreshCw size={14} />
          Muat Ulang
        </Button>
      </div>

      {error && (
        <div role="alert" className={adminStyles.alertError}>
          {error}
        </div>
      )}
      {notice && <div className={adminStyles.alertSuccess}>{notice}</div>}

      <Card>
        <div className={adminStyles.headerRow} style={{ marginBottom: 'var(--space-3)' }}>
          <div>
            <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>
              <FileSpreadsheet size={18} />
              Ekspor Data Murid Diterima / Terdaftar
            </h2>
            <p className={adminStyles.metaMuted}>
              Menggunakan format OOXML <span className="mono-code">.xlsx</span> atau UTF-8 BOM <span className="mono-code">.csv</span>.
            </p>
          </div>
        </div>

        <div className={adminStyles.filterBar} style={{ marginBottom: 'var(--space-4)' }}>
          <select
            aria-label="Pilih unit ekspor"
            className={adminStyles.select}
            value={unitId}
            onChange={(e) => setUnitId(e.target.value)}
          >
            {portal.units.map((u) => (
              <option key={u.id} value={u.id}>
                {u.name}
              </option>
            ))}
          </select>

          <select
            aria-label="Pilih periode ekspor"
            className={adminStyles.select}
            value={periodId}
            onChange={(e) => setPeriodId(e.target.value)}
          >
            {unitPeriods.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} ({p.academic_year})
              </option>
            ))}
          </select>

          <select
            aria-label="Pilih format ekspor"
            className={adminStyles.select}
            value={format}
            onChange={(e) => setFormat(e.target.value as ExportFormat)}
          >
            <option value="dapodik_xlsx">Dapodik (.xlsx)</option>
            <option value="emis_xlsx">EMIS (.xlsx)</option>
            <option value="custom_csv">Kustom Lengkap (.csv)</option>
          </select>

          <Button onClick={() => void handleCreateExport()} disabled={creating || !periodId}>
            <Download size={14} />
            {creating ? 'Memproses...' : 'Buat Ekspor'}
          </Button>
        </div>

        <div className={adminStyles.tableWrap}>
          <table className={adminStyles.table}>
            <thead>
              <tr>
                <th>Waktu</th>
                <th>Unit &amp; Periode</th>
                <th>Format</th>
                <th>Baris</th>
                <th>Status</th>
                <th>Aksi</th>
              </tr>
            </thead>
            <tbody>
              {jobsLoading && jobs.length === 0 ? (
                <tr>
                  <td colSpan={6} className={adminStyles.emptyState}>
                    Memuat riwayat pekerjaan ekspor...
                  </td>
                </tr>
              ) : jobs.length === 0 ? (
                <tr>
                  <td colSpan={6} className={adminStyles.emptyState}>
                    Belum ada riwayat ekspor untuk unit ini. Pilih format lalu klik Buat Ekspor.
                  </td>
                </tr>
              ) : (
                jobs.map((job) => (
                  <tr key={job.id}>
                    <td style={{ whiteSpace: 'nowrap' }}>{formatDateTime(job.created_at)}</td>
                    <td>
                      <div style={{ fontWeight: 600 }}>{job.unit_name ?? job.unit_id}</div>
                      <div className={adminStyles.metaMuted}>{job.period_name ?? job.period_id}</div>
                    </td>
                    <td>
                      <ToneBadge tone="info" label={FORMAT_LABELS[job.format] ?? job.format} />
                    </td>
                    <td className="tabular-nums">{job.row_count ?? '-'}</td>
                    <td>
                      <ToneBadge tone={jobStatusTone(job.status)} label={job.status.toUpperCase()} />
                      {job.error_message && (
                        <div style={{ fontSize: 'var(--text-caption)', color: 'var(--status-danger-fg)', marginTop: 2 }}>
                          {job.error_message}
                        </div>
                      )}
                    </td>
                    <td>
                      {job.status === 'completed' ? (
                        <Button
                          size="sm"
                          variant="secondary"
                          disabled={downloadingId === job.id}
                          onClick={() => void handleDownload(job)}
                        >
                          <Download size={14} />
                          {downloadingId === job.id ? 'Mengunduh...' : 'Unduh'}
                        </Button>
                      ) : (
                        <span className={adminStyles.metaMuted}>Menunggu selesai</span>
                      )}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>

      <Card>
        <div className={adminStyles.headerRow} style={{ marginBottom: 'var(--space-3)' }}>
          <div>
            <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>
              <History size={18} />
              Audit Log Perubahan &amp; Aktivitas Panitia
            </h2>
            <p className={adminStyles.metaMuted}>
              Seluruh perubahan status, verifikasi pembayaran/berkas, penilaian, dan ekspor dicatat kronologis.
            </p>
          </div>
          <div className={adminStyles.filterBar}>
            <input
              type="search"
              aria-label="Cari di audit log"
              className={adminStyles.searchInput}
              placeholder="Cari no. daftar, nama, atau aktor..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
            <select
              aria-label="Filter jenis aksi"
              className={adminStyles.select}
              value={actionFilter}
              onChange={(e) => setActionFilter(e.target.value)}
            >
              <option value="">Semua Aksi</option>
              {knownActions.map((act) => (
                <option key={act} value={act}>
                  {act}
                </option>
              ))}
            </select>
            <select
              aria-label="Batas jumlah audit log"
              className={adminStyles.select}
              value={limit}
              onChange={(e) => setLimit(Number(e.target.value))}
            >
              <option value={50}>50 terbaru</option>
              <option value={100}>100 terbaru</option>
              <option value={200}>200 terbaru</option>
            </select>
          </div>
        </div>

        <div className={adminStyles.tableWrap}>
          <table className={adminStyles.table}>
            <thead>
              <tr>
                <th>Waktu</th>
                <th>Pendaftar / Entitas</th>
                <th>Aktor</th>
                <th>Aksi</th>
                <th>Perubahan Status</th>
                <th>Catatan</th>
              </tr>
            </thead>
            <tbody>
              {auditLoading && filteredLogs.length === 0 ? (
                <tr>
                  <td colSpan={6} className={adminStyles.emptyState}>
                    Memuat audit log...
                  </td>
                </tr>
              ) : filteredLogs.length === 0 ? (
                <tr>
                  <td colSpan={6} className={adminStyles.emptyState}>
                    Tidak ada catatan audit log sesuai filter saat ini.
                  </td>
                </tr>
              ) : (
                filteredLogs.map((log) => (
                  <tr key={log.id}>
                    <td style={{ whiteSpace: 'nowrap' }}>{formatDateTime(log.created_at)}</td>
                    <td>
                      {log.registration_no ? (
                        <>
                          <div className="mono-code" style={{ fontWeight: 600 }}>
                            {log.registration_no}
                          </div>
                          <div className={adminStyles.metaMuted}>{log.student_name ?? '-'}</div>
                        </>
                      ) : (
                        <div className="mono-code">
                          {log.entity_type} · {log.entity_id.slice(0, 8)}
                        </div>
                      )}
                    </td>
                    <td>
                      <div style={{ fontWeight: 600 }}>{log.actor_name ?? 'Sistem'}</div>
                      <div className={`mono-code ${adminStyles.metaMuted}`}>{log.actor_role}</div>
                    </td>
                    <td>
                      <span className="mono-code">{log.action_type}</span>
                    </td>
                    <td>
                      {log.from_status || log.to_status ? (
                        <span className="mono-code">
                          {log.from_status ?? '-'} &rarr; {log.to_status ?? '-'}
                        </span>
                      ) : (
                        <span className={adminStyles.metaMuted}>-</span>
                      )}
                    </td>
                    <td>{log.reason || <span className={adminStyles.metaMuted}>-</span>}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
