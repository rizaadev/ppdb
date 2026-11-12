import { useCallback, useEffect, useState } from 'react';
import {
  ArrowRight,
  Award,
  CheckCircle2,
  Download,
  FileCheck,
  FileSpreadsheet,
  RefreshCw,
  TrendingUp,
  Users,
  Wallet,
} from 'lucide-react';
import { apiRequest, extractErrorMessage, formatDateTime, formatRupiah } from '@/core/api/client';
import type { DashboardStats, UnitSummary } from '@/core/types/domain';
import type { AdminTabId } from '@/layouts/AdminShell';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import adminStyles from './AdminShared.module.css';
import styles from './AdminDashboardOverview.module.css';

interface AdminDashboardOverviewProps {
  tenantSlug: string;
  units: UnitSummary[];
  onNavigateTab: (tabId: AdminTabId) => void;
}

export function AdminDashboardOverview({
  tenantSlug,
  units,
  onNavigateTab,
}: AdminDashboardOverviewProps) {
  const [selectedUnitId, setSelectedUnitId] = useState('');
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const loadStats = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const query = selectedUnitId ? `?unit_id=${encodeURIComponent(selectedUnitId)}` : '';
      const data = await apiRequest<DashboardStats>(`/api/v1/admin/dashboard/stats${query}`, {
        tenantSlug,
      });
      setStats(data);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memuat ringkasan statistik dashboard.'));
    } finally {
      setLoading(false);
    }
  }, [selectedUnitId, tenantSlug]);

  useEffect(() => {
    void loadStats();
  }, [loadStats]);

  // Compute SVG Area Chart paths for 14-day daily trends
  const trends = stats?.daily_trends ?? [];
  const maxTrend = Math.max(1, ...trends.map((t) => t.count));
  const svgWidth = 600;
  const svgHeight = 180;
  const paddingX = 30;
  const paddingY = 25;
  const chartW = svgWidth - paddingX * 2;
  const chartH = svgHeight - paddingY * 2;

  const points = trends.map((t, i) => {
    const x = paddingX + (i / Math.max(1, trends.length - 1)) * chartW;
    const y = paddingY + chartH - (t.count / maxTrend) * chartH;
    return { x, y, count: t.count, date: t.date };
  });

  const polylineStr = points.map((p) => `${p.x},${p.y}`).join(' ');
  const areaPath = points.length > 0
    ? `M${points[0].x},${paddingY + chartH} L${points.map((p) => `${p.x},${p.y}`).join(' L')} L${points[points.length - 1].x},${paddingY + chartH} Z`
    : '';

  const totalQuota = (stats?.track_stats ?? []).reduce((acc, t) => acc + t.quota, 0);
  const totalAccepted = stats?.accepted_count ?? 0;
  const quotaPercent = totalQuota > 0 ? Math.min(100, Math.round((totalAccepted / totalQuota) * 100)) : 0;

  return (
    <div className={adminStyles.stack}>
      <div className={adminStyles.headerRow}>
        <div>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 800 }}>
            Dashboard Eksekutif Panitia
          </h1>
          <p className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
            Pemantauan langsung arus pendaftar, efektivitas verifikasi, kuota jalur, dan penerimaan kas.
          </p>
        </div>

        <div className={adminStyles.filterBar}>
          {units.length > 1 && (
            <select
              className={adminStyles.select}
              value={selectedUnitId}
              onChange={(e) => setSelectedUnitId(e.target.value)}
            >
              <option value="">Semua Satuan Pendidikan / Unit</option>
              {units.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.name}
                </option>
              ))}
            </select>
          )}

          <Button size="sm" variant="secondary" onClick={() => void loadStats()} disabled={loading}>
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            Perbarui Data
          </Button>
        </div>
      </div>

      {error && (
        <div role="alert" className={adminStyles.alertError}>
          {error}
        </div>
      )}

      {/* Top Metric Cards Ribbon */}
      <div className={styles.kpiGrid}>
        <div className={styles.kpiCard}>
          <div className={styles.kpiHeader}>
            <span className={styles.kpiLabel}>Total Pendaftar</span>
            <div className={styles.kpiIconWrap} style={{ backgroundColor: 'var(--brand-tint)', color: 'var(--brand-primary)' }}>
              <Users size={20} />
            </div>
          </div>
          <div className={styles.kpiValue} data-testid="kpi-total-applicants">
            {stats?.total_applicants ?? 0}
          </div>
          <div className={styles.kpiSub}>
            <span>{stats?.submitted_count ?? 0} formulir tersubmit</span>
            <span>·</span>
            <span>{stats?.draft_count ?? 0} draf</span>
          </div>
        </div>

        <div className={styles.kpiCard}>
          <div className={styles.kpiHeader}>
            <span className={styles.kpiLabel}>Butuh Verifikasi</span>
            <div className={styles.kpiIconWrap} style={{ backgroundColor: (stats?.verification_count ?? 0) > 0 ? 'var(--status-warning-bg)' : 'var(--bg-subtle)', color: 'var(--status-warning-fg)' }}>
              <FileCheck size={20} />
            </div>
          </div>
          <div className={styles.kpiValue} style={{ color: (stats?.verification_count ?? 0) > 0 ? 'var(--status-warning-fg)' : 'inherit' }}>
            {stats?.verification_count ?? 0}
          </div>
          <div className={styles.kpiSub}>
            <span style={{ color: (stats?.revision_required_count ?? 0) > 0 ? 'var(--status-danger-fg)' : 'inherit' }}>
              {stats?.revision_required_count ?? 0} revisi aktif
            </span>
          </div>
        </div>

        <div className={styles.kpiCard}>
          <div className={styles.kpiHeader}>
            <span className={styles.kpiLabel}>Lolos Seleksi</span>
            <div className={styles.kpiIconWrap} style={{ backgroundColor: 'var(--status-success-bg)', color: 'var(--status-success-fg)' }}>
              <Award size={20} />
            </div>
          </div>
          <div className={styles.kpiValue} style={{ color: 'var(--status-success-fg)' }}>
            {stats?.accepted_count ?? 0}
          </div>
          <div className={styles.kpiSub}>
            <span>{quotaPercent}% kuota terpenuhi ({totalAccepted}/{totalQuota || '-'})</span>
          </div>
        </div>

        <div className={styles.kpiCard}>
          <div className={styles.kpiHeader}>
            <span className={styles.kpiLabel}>Daftar Ulang Selesai</span>
            <div className={styles.kpiIconWrap} style={{ backgroundColor: 'var(--status-info-bg)', color: 'var(--status-info-fg)' }}>
              <CheckCircle2 size={20} />
            </div>
          </div>
          <div className={styles.kpiValue} style={{ color: 'var(--status-info-fg)' }}>
            {stats?.re_enrolled_count ?? 0}
          </div>
          <div className={styles.kpiSub}>
            <span>Santri/murid resmi terdaftar</span>
          </div>
        </div>

        <div className={styles.kpiCard}>
          <div className={styles.kpiHeader}>
            <span className={styles.kpiLabel}>Penerimaan Keuangan</span>
            <div className={styles.kpiIconWrap} style={{ backgroundColor: 'var(--brand-tint)', color: 'var(--brand-primary)' }}>
              <Wallet size={20} />
            </div>
          </div>
          <div className={styles.kpiValue} style={{ fontSize: '1.35rem' }}>
            {formatRupiah(stats?.total_revenue_idr ?? 0)}
          </div>
          <div className={styles.kpiSub}>
            {(stats?.pending_manual_payments ?? 0) > 0 ? (
              <span style={{ color: 'var(--status-warning-fg)', fontWeight: 700 }}>
                {stats?.pending_manual_payments} transfer manual menanti konfirmasi
              </span>
            ) : (
              <span>Tertunda: {formatRupiah(stats?.pending_revenue_idr ?? 0)}</span>
            )}
          </div>
        </div>
      </div>

      {/* Main Two-Column Analytics Grid */}
      <div className={styles.twoColGrid}>
        {/* Registration Daily Trend Graph */}
        <Card className={styles.chartCard}>
          <div className={styles.chartHeader}>
            <div>
              <div className={styles.chartTitle}>Tren Pendaftaran Harian (14 Hari Terakhir)</div>
              <div className={styles.chartSub}>Volume aplikasi masuk setiap hari secara berurutan</div>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: '0.8rem', color: 'var(--brand-primary)', fontWeight: 600 }}>
              <TrendingUp size={16} />
              <span>Real-time</span>
            </div>
          </div>

          <div className={styles.svgWrap}>
            <svg
              viewBox={`0 0 ${svgWidth} ${svgHeight}`}
              width="100%"
              height="100%"
              preserveAspectRatio="none"
              style={{ overflow: 'visible' }}
            >
              <defs>
                <linearGradient id="areaGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--brand-primary)" stopOpacity="0.32" />
                  <stop offset="100%" stopColor="var(--brand-primary)" stopOpacity="0.02" />
                </linearGradient>
              </defs>

              {/* Grid guide lines */}
              <line x1={paddingX} y1={paddingY} x2={svgWidth - paddingX} y2={paddingY} stroke="var(--border-default)" strokeDasharray="3 3" />
              <line x1={paddingX} y1={paddingY + chartH / 2} x2={svgWidth - paddingX} y2={paddingY + chartH / 2} stroke="var(--border-default)" strokeDasharray="3 3" />
              <line x1={paddingX} y1={paddingY + chartH} x2={svgWidth - paddingX} y2={paddingY + chartH} stroke="var(--border-strong)" />

              {/* Area & Stroke */}
              {areaPath && <path d={areaPath} fill="url(#areaGradient)" />}
              {polylineStr && (
                <polyline
                  points={polylineStr}
                  fill="none"
                  stroke="var(--brand-primary)"
                  strokeWidth="2.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              )}

              {/* Data points */}
              {points.map((p, idx) => (
                <g key={p.date}>
                  <circle cx={p.x} cy={p.y} r="3.5" fill="#ffffff" stroke="var(--brand-primary)" strokeWidth="2" />
                  {idx % 2 === 0 && (
                    <text
                      x={p.x}
                      y={svgHeight - 4}
                      fontSize="9"
                      fill="var(--text-muted)"
                      textAnchor="middle"
                    >
                      {p.date.slice(5)}
                    </text>
                  )}
                </g>
              ))}
            </svg>
          </div>

          {/* Quick Action Shortcuts */}
          <div style={{ marginTop: 'var(--space-2)' }}>
            <div style={{ fontSize: 'var(--text-label)', fontWeight: 700, marginBottom: 'var(--space-2)' }}>
              Aksi Cepat Panitia
            </div>
            <div className={styles.quickActionsGrid}>
              <button
                type="button"
                className={styles.actionBtn}
                onClick={() => onNavigateTab('verify')}
              >
                <FileCheck size={16} />
                <span>Periksa Berkas ({stats?.verification_count ?? 0})</span>
                <ArrowRight size={13} style={{ marginLeft: 'auto' }} />
              </button>

              <button
                type="button"
                className={styles.actionBtn}
                onClick={() => onNavigateTab('finance')}
              >
                <Wallet size={16} />
                <span>Konfirmasi Bayar ({stats?.pending_manual_payments ?? 0})</span>
                <ArrowRight size={13} style={{ marginLeft: 'auto' }} />
              </button>

              <button
                type="button"
                className={styles.actionBtn}
                onClick={() => onNavigateTab('ranking')}
              >
                <FileSpreadsheet size={16} />
                <span>Penilaian &amp; Ranking</span>
                <ArrowRight size={13} style={{ marginLeft: 'auto' }} />
              </button>

              <button
                type="button"
                className={styles.actionBtn}
                onClick={() => onNavigateTab('export')}
              >
                <Download size={16} />
                <span>Ekspor Dapodik / EMIS</span>
                <ArrowRight size={13} style={{ marginLeft: 'auto' }} />
              </button>
            </div>
          </div>
        </Card>

        {/* Quota Progress & Recent Activities */}
        <div className={adminStyles.stack}>
          {/* Quota distribution per track */}
          <Card>
            <div className={styles.chartHeader} style={{ marginBottom: 'var(--space-3)' }}>
              <div className={styles.chartTitle}>Kapasitas Jalur Pendaftaran</div>
              <span className={styles.chartSub}>Perbandingan pendaftar vs daya tampung</span>
            </div>

            <div className={styles.trackList}>
              {(stats?.track_stats ?? []).length === 0 ? (
                <div className={adminStyles.emptyState}>Belum ada jalur pendaftaran aktif.</div>
              ) : (
                stats?.track_stats.map((t) => {
                  const pct = t.quota > 0 ? Math.min(100, Math.round((t.applicant_count / t.quota) * 100)) : 0;
                  const isFull = pct >= 100;
                  return (
                    <div key={t.track_id} className={styles.trackItem}>
                      <div className={styles.trackItemHeader}>
                        <span className={styles.trackName}>{t.display_name}</span>
                        <span className="tabular-nums font-semibold" style={{ color: isFull ? 'var(--status-danger-fg)' : 'inherit' }}>
                          {t.applicant_count} / {t.quota} ({pct}%)
                        </span>
                      </div>
                      <div className={styles.trackProgressBg}>
                        <div
                          className={styles.trackProgressFill}
                          style={{
                            width: `${pct}%`,
                            backgroundColor: isFull ? 'var(--status-danger-fg)' : 'var(--brand-primary)',
                          }}
                        />
                      </div>
                      <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', display: 'flex', justifyContent: 'space-between' }}>
                        <span>Unit: {t.unit_name}</span>
                        <span>{t.accepted_count} diterima</span>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </Card>

          {/* Live Recent Committee Activities */}
          <Card>
            <div className={styles.chartHeader} style={{ marginBottom: 'var(--space-2)' }}>
              <div className={styles.chartTitle}>Aktivitas Panitia Terkini</div>
              <span className={styles.chartSub}>10 catatan log audit terbaru</span>
            </div>

            <div className={styles.activityList}>
              {(stats?.recent_activities ?? []).length === 0 ? (
                <div className={adminStyles.emptyState}>Belum ada riwayat aktivitas panitia.</div>
              ) : (
                stats?.recent_activities.map((act) => (
                  <div key={act.id} className={styles.activityItem}>
                    <div>
                      <div style={{ fontWeight: 600 }}>
                        {act.actor_name || 'Sistem'} · <span style={{ color: 'var(--text-secondary)' }}>{act.action_type.replace(/_/g, ' ')}</span>
                      </div>
                      <div className={styles.activityMeta}>
                        {act.registration_no ? `No. ${act.registration_no}` : act.entity_type}
                        {act.student_name ? ` (${act.student_name})` : ''}
                        {act.reason ? ` · "${act.reason}"` : ''}
                      </div>
                    </div>
                    <div style={{ textAlign: 'right', flexShrink: 0 }}>
                      <span className={styles.activityMeta}>{formatDateTime(act.created_at)}</span>
                    </div>
                  </div>
                ))
              )}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
