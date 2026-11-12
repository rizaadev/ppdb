import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  ArrowUpCircle,
  Award,
  Calculator,
  CalendarPlus,
  Megaphone,
  Plus,
  RefreshCw,
  Trash2,
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
  AssessmentSchedule,
  PublicPortalPayload,
  PublishResultsResult,
  StaffMemberView,
  TrackRankingSimulation,
  UpsertScoresResult,
} from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { InputField } from '@/components/ui/InputField';
import { Modal } from '@/components/ui/Modal';
import { StatusBadge } from '@/components/ui/StatusBadge';
import adminStyles from './AdminShared.module.css';
import styles from './SelectionRankingPanel.module.css';

interface EditableScoreRow {
  component_code: string;
  raw_score: string;
  weight_pct: string;
  notes: string;
}

const DEFAULT_SCORE_TEMPLATE: EditableScoreRow[] = [
  { component_code: 'TPA', raw_score: '', weight_pct: '60', notes: '' },
  { component_code: 'WAWANCARA', raw_score: '', weight_pct: '40', notes: '' },
];

function weightsToTemplate(weights?: Record<string, number>): EditableScoreRow[] {
  const entries = weights ? Object.entries(weights) : [];
  if (entries.length === 0) return DEFAULT_SCORE_TEMPLATE;
  return entries.map(([code, pct]) => ({
    component_code: code.toUpperCase(),
    raw_score: '',
    weight_pct: String(pct),
    notes: '',
  }));
}

function defaultScheduleDateTimeLocal(): string {
  const d = new Date(Date.now() + 2 * 24 * 60 * 60 * 1000);
  d.setHours(9, 0, 0, 0);
  const offset = d.getTimezoneOffset() * 60000;
  return new Date(d.getTime() - offset).toISOString().slice(0, 16);
}

export function SelectionRankingPanel({
  tenantSlug,
  portal,
  activeUnitId,
  onPortalChanged,
}: {
  tenantSlug: string;
  portal: PublicPortalPayload;
  activeUnitId: string;
  onPortalChanged?: () => void;
}) {
  const { hasAnyRole, hasUnitAccess } = useAuth();
  const canScore = hasUnitAccess(activeUnitId, 'tenant_admin', 'unit_admin', 'examiner');
  const canRankPublish = hasUnitAccess(activeUnitId, 'tenant_admin', 'unit_admin', 'principal');
  const canManageSchedules = canScore || canRankPublish;
  const isTenantAdmin = hasAnyRole('tenant_admin');

  const unitPeriods = portal.periods.filter((p) => p.unit_id === activeUnitId);
  const [periodId, setPeriodId] = useState(unitPeriods[0]?.id ?? '');
  const activePeriod = unitPeriods.find((p) => p.id === periodId) ?? unitPeriods[0];
  const [trackId, setTrackId] = useState(activePeriod?.tracks[0]?.id ?? '');
  const activeTrack =
    activePeriod?.tracks.find((t) => t.id === trackId) ?? activePeriod?.tracks[0];

  useEffect(() => {
    const nextPeriod = portal.periods.find((p) => p.unit_id === activeUnitId);
    setPeriodId(nextPeriod?.id ?? '');
    setTrackId(nextPeriod?.tracks[0]?.id ?? '');
    setSimulation(null);
    setPublishedSummary(null);
  }, [activeUnitId, portal.periods]);

  useEffect(() => {
    if (activePeriod && !activePeriod.tracks.some((t) => t.id === trackId)) {
      setTrackId(activePeriod.tracks[0]?.id ?? '');
    }
  }, [activePeriod, trackId]);

  const [candidates, setCandidates] = useState<ApplicationSummary[]>([]);
  const [loadingList, setLoadingList] = useState(true);
  const [selectedAppId, setSelectedAppId] = useState('');
  const [selectedDetail, setSelectedDetail] = useState<ApplicationDetail | null>(null);
  const [scoreRows, setScoreRows] = useState<EditableScoreRow[]>(DEFAULT_SCORE_TEMPLATE);

  const [simulation, setSimulation] = useState<TrackRankingSimulation | null>(null);
  const [publishedSummary, setPublishedSummary] = useState<PublishResultsResult | null>(null);
  const [publishConfirmOpen, setPublishConfirmOpen] = useState(false);

  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [bannerError, setBannerError] = useState('');
  const [bannerSuccess, setBannerSuccess] = useState('');

  // Assessment schedules state
  const [schedules, setSchedules] = useState<AssessmentSchedule[]>([]);
  const [loadingSchedules, setLoadingSchedules] = useState(false);
  const [examiners, setExaminers] = useState<StaffMemberView[]>([]);
  const [scheduleModalOpen, setScheduleModalOpen] = useState(false);
  const [schedTargetMode, setSchedTargetMode] = useState<'single' | 'all_track'>('single');
  const [schedAppId, setSchedAppId] = useState('');
  const [schedSessionName, setSchedSessionName] = useState('Tes Akademik & Wawancara');
  const [schedRoomOrLink, setSchedRoomOrLink] = useState('');
  const [schedDateTime, setSchedDateTime] = useState(defaultScheduleDateTimeLocal);
  const [schedExaminerId, setSchedExaminerId] = useState('');

  const loadCandidates = useCallback(
    async (preferredId?: string) => {
      if (!activeUnitId) return;
      setLoadingList(true);
      try {
        const params = new URLSearchParams({
          unit_id: activeUnitId,
          limit: '100',
        });
        if (activeTrack?.id) params.set('track_id', activeTrack.id);

        const rows = await apiRequest<ApplicationSummary[]>(
          `/api/v1/admin/applications?${params.toString()}`,
          { tenantSlug },
        );
        const safe = rows ?? [];
        setCandidates(safe);
        setSelectedAppId((prev) => {
          if (preferredId && safe.some((c) => c.id === preferredId)) return preferredId;
          if (prev && safe.some((c) => c.id === prev)) return prev;
          return safe[0]?.id ?? '';
        });
      } catch (err) {
        setCandidates([]);
        setBannerError(extractErrorMessage(err, 'Gagal memuat daftar kandidat unit.'));
      } finally {
        setLoadingList(false);
      }
    },
    [activeTrack?.id, activeUnitId, tenantSlug],
  );

  const loadSchedules = useCallback(async () => {
    if (!activeUnitId) {
      setSchedules([]);
      return;
    }
    setLoadingSchedules(true);
    try {
      const params = new URLSearchParams({ unit_id: activeUnitId, limit: '100' });
      if (activeTrack?.id) params.set('track_id', activeTrack.id);
      const list = await apiRequest<AssessmentSchedule[]>(
        `/api/v1/admin/schedules?${params.toString()}`,
        { tenantSlug },
      );
      setSchedules(list ?? []);
    } catch {
      setSchedules([]);
    } finally {
      setLoadingSchedules(false);
    }
  }, [activeTrack?.id, activeUnitId, tenantSlug]);

  useEffect(() => {
    void loadSchedules();
  }, [loadSchedules]);

  useEffect(() => {
    if (!isTenantAdmin) {
      setExaminers([]);
      return;
    }
    apiRequest<StaffMemberView[]>('/api/v1/admin/staff', { tenantSlug })
      .then((list) => setExaminers(list ?? []))
      .catch(() => setExaminers([]));
  }, [isTenantAdmin, tenantSlug]);

  const loadCandidateDetail = useCallback(
    async (appId: string) => {
      if (!appId) {
        setSelectedDetail(null);
        setScoreRows(weightsToTemplate(activeTrack?.scoring_weights));
        return;
      }
      try {
        const detail = await apiRequest<ApplicationDetail>(
          `/api/v1/admin/applications/${appId}`,
          { tenantSlug },
        );
        setSelectedDetail(detail);
        if (detail.scores.length > 0) {
          setScoreRows(
            detail.scores.map((s) => ({
              component_code: s.component_code,
              raw_score: String(s.raw_score),
              weight_pct: String(s.weight_pct),
              notes: s.notes ?? '',
            })),
          );
        } else {
          setScoreRows(
            weightsToTemplate(detail.track_scoring_weights ?? activeTrack?.scoring_weights),
          );
        }
      } catch (err) {
        setBannerError(extractErrorMessage(err, 'Gagal memuat komponen nilai pendaftar.'));
      }
    },
    [activeTrack?.scoring_weights, tenantSlug],
  );

  useEffect(() => {
    void loadCandidates();
  }, [loadCandidates]);

  useEffect(() => {
    void loadCandidateDetail(selectedAppId);
  }, [selectedAppId, loadCandidateDetail]);

  const projectedScore = scoreRows.reduce((sum, row) => {
    const raw = Number(row.raw_score) || 0;
    const weight = Number(row.weight_pct) || 0;
    return sum + (raw * weight) / 100;
  }, 0);

  async function handleSaveScores() {
    if (!selectedDetail) return;
    const cleaned = scoreRows
      .filter((r) => r.component_code.trim() !== '')
      .map((r) => ({
        component_code: r.component_code.trim().toUpperCase(),
        raw_score: Number(r.raw_score) || 0,
        weight_pct: Number(r.weight_pct) || 0,
        notes: r.notes.trim(),
      }));
    if (cleaned.length === 0) {
      setBannerError('Minimal 1 komponen nilai harus diisi.');
      return;
    }

    setBusyAction('scores');
    setBannerError('');
    setBannerSuccess('');
    try {
      const res = await apiRequest<UpsertScoresResult>(
        `/api/v1/admin/applications/${selectedDetail.application.id}/scores`,
        {
          method: 'PUT',
          tenantSlug,
          body: { scores: cleaned },
        },
      );
      setSelectedDetail(res.application);
      setBannerSuccess(
        `Nilai ${res.application.application.registration_no} disimpan. Skor akhir: ${res.final_score.toFixed(2)}.`,
      );
      await loadCandidates(res.application.application.id);
    } catch (err) {
      setBannerError(extractErrorMessage(err, 'Gagal menyimpan nilai seleksi.'));
    } finally {
      setBusyAction(null);
    }
  }

  async function handleSimulateRanking() {
    if (!activeTrack) return;
    setBusyAction('simulate');
    setBannerError('');
    setBannerSuccess('');
    try {
      const res = await apiRequest<TrackRankingSimulation>(
        `/api/v1/admin/tracks/${activeTrack.id}/simulate-ranking`,
        { method: 'POST', tenantSlug },
      );
      setSimulation(res);
      setBannerSuccess(
        `Simulasi jalur ${res.track_name} selesai: ${res.accepted_count} diterima, ${res.waitlisted_count} cadangan, ${res.rejected_count} tidak lolos.`,
      );
    } catch (err) {
      setBannerError(extractErrorMessage(err, 'Gagal menjalankan simulasi perankingan jalur.'));
    } finally {
      setBusyAction(null);
    }
  }

  async function handlePublishResults() {
    if (!activePeriod) return;
    setBusyAction('publish');
    setBannerError('');
    setBannerSuccess('');
    try {
      const res = await apiRequest<PublishResultsResult>(
        `/api/v1/admin/periods/${activePeriod.id}/publish-results`,
        {
          method: 'POST',
          tenantSlug,
          idempotencyKey: `publish-${activePeriod.id}-${crypto.randomUUID()}`,
        },
      );
      setPublishedSummary(res);
      setPublishConfirmOpen(false);
      setBannerSuccess(
        `Pengumuman hasil periode ${activePeriod.name} diterbitkan (${res.total_accepted} diterima, ${res.total_waitlisted} cadangan).`,
      );
      await loadCandidates(selectedAppId);
      onPortalChanged?.();
      if (activeTrack) {
        const matchedTrack = res.tracks.find((t) => t.track_id === activeTrack.id);
        if (matchedTrack) setSimulation(matchedTrack);
      }
    } catch (err) {
      setBannerError(extractErrorMessage(err, 'Gagal menerbitkan pengumuman hasil periode.'));
    } finally {
      setBusyAction(null);
    }
  }

  async function handlePromoteWaitlist(appId: string, label: string) {
    setBusyAction(`promote-${appId}`);
    setBannerError('');
    setBannerSuccess('');
    try {
      await apiRequest<ApplicationDetail>(
        `/api/v1/admin/applications/${appId}/promote-waitlist`,
        { method: 'POST', tenantSlug },
      );
      setBannerSuccess(`Kandidat cadangan ${label} dinaikkan menjadi Diterima.`);
      await loadCandidates(appId);
      if (activeTrack && canRankPublish) {
        const sim = await apiRequest<TrackRankingSimulation>(
          `/api/v1/admin/tracks/${activeTrack.id}/simulate-ranking`,
          { method: 'POST', tenantSlug },
        );
        setSimulation(sim);
      }
    } catch (err) {
      setBannerError(extractErrorMessage(err, 'Gagal menaikkan status cadangan pendaftar.'));
    } finally {
      setBusyAction(null);
    }
  }

  function openCreateSchedule() {
    setSchedTargetMode(selectedAppId ? 'single' : 'all_track');
    setSchedAppId(selectedAppId || candidates[0]?.id || '');
    setSchedSessionName('Tes Akademik & Wawancara');
    setSchedRoomOrLink('');
    setSchedDateTime(defaultScheduleDateTimeLocal());
    setSchedExaminerId('');
    setScheduleModalOpen(true);
  }

  async function handleCreateSchedule(e: FormEvent) {
    e.preventDefault();
    const targetIds =
      schedTargetMode === 'all_track'
        ? candidates.map((c) => c.id)
        : schedAppId
          ? [schedAppId]
          : [];
    if (targetIds.length === 0) {
      setBannerError('Pilih minimal satu kandidat pendaftar untuk dijadwalkan.');
      return;
    }
    const iso = new Date(schedDateTime).toISOString();
    setBusyAction('schedule-create');
    setBannerError('');
    setBannerSuccess('');
    try {
      const created = await apiRequest<AssessmentSchedule[]>('/api/v1/admin/schedules', {
        method: 'POST',
        tenantSlug,
        body: {
          application_ids: targetIds,
          session_name: schedSessionName.trim(),
          room_or_link: schedRoomOrLink.trim() || null,
          scheduled_at: iso,
          examiner_user_id: schedExaminerId || null,
        },
      });
      setScheduleModalOpen(false);
      setBannerSuccess(
        `${created?.length ?? targetIds.length} jadwal seleksi berhasil dibuat dan notifikasi WhatsApp dikirim.`,
      );
      await loadSchedules();
      if (selectedAppId) {
        await loadCandidateDetail(selectedAppId);
      }
    } catch (err) {
      setBannerError(extractErrorMessage(err, 'Gagal menyimpan jadwal seleksi.'));
    } finally {
      setBusyAction(null);
    }
  }

  async function handleDeleteSchedule(scheduleId: string) {
    setBusyAction(`schedule-del-${scheduleId}`);
    setBannerError('');
    setBannerSuccess('');
    try {
      await apiRequest(`/api/v1/admin/schedules/${scheduleId}`, {
        method: 'DELETE',
        tenantSlug,
      });
      setBannerSuccess('Jadwal asesmen berhasil dihapus.');
      await loadSchedules();
    } catch (err) {
      setBannerError(extractErrorMessage(err, 'Gagal menghapus jadwal asesmen.'));
    } finally {
      setBusyAction(null);
    }
  }

  return (
    <div className={adminStyles.stack}>
      <div className={adminStyles.headerRow}>
        <div>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
            Penilaian, Simulasi Ranking &amp; Pengumuman
          </h1>
          <p className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
            Input komponen nilai oleh Penguji serta simulasi kuota, terbit hasil, dan promosi cadangan oleh Kepala Sekolah/Admin.
          </p>
        </div>

        <div className={adminStyles.filterBar}>
          <select
            aria-label="Pilih periode pendaftaran"
            className={adminStyles.select}
            value={activePeriod?.id ?? ''}
            onChange={(e) => {
              setPeriodId(e.target.value);
              const p = unitPeriods.find((x) => x.id === e.target.value);
              setTrackId(p?.tracks[0]?.id ?? '');
              setSimulation(null);
            }}
          >
            {unitPeriods.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} ({p.academic_year})
              </option>
            ))}
          </select>

          <select
            aria-label="Pilih jalur seleksi"
            className={adminStyles.select}
            value={activeTrack?.id ?? ''}
            onChange={(e) => {
              setTrackId(e.target.value);
              setSimulation(null);
            }}
          >
            {(activePeriod?.tracks ?? []).map((t) => (
              <option key={t.id} value={t.id}>
                {t.display_name} (Kuota {t.quota})
              </option>
            ))}
          </select>

          <Button size="sm" variant="secondary" onClick={() => void loadCandidates(selectedAppId)}>
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

      <Card>
        <div className={styles.controlPanel}>
          <div>
            <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>
              Kontrol Ranking Jalur &amp; Pengumuman Periode
            </h2>
            <p className={adminStyles.metaMuted} style={{ marginTop: 2 }}>
              Jalur aktif: <strong>{activeTrack?.display_name || '-'}</strong> · Kuota:{' '}
              <strong>{activeTrack?.quota ?? 0} kursi</strong> · Mode:{' '}
              <span className="mono-code">{activeTrack?.selection_mode || '-'}</span>
              {activePeriod?.results_published_at && (
                <> · Diterbitkan pada {formatDateTime(activePeriod.results_published_at)}</>
              )}
            </p>
          </div>

          <div className={adminStyles.filterBar}>
            <Button
              variant="secondary"
              disabled={!activeTrack || !canRankPublish}
              loading={busyAction === 'simulate'}
              onClick={() => void handleSimulateRanking()}
            >
              <Calculator size={15} />
              Simulasi Perankingan
            </Button>
            <Button
              disabled={!activePeriod || !canRankPublish}
              loading={busyAction === 'publish'}
              onClick={() => setPublishConfirmOpen(true)}
            >
              <Megaphone size={15} />
              Terbitkan Pengumuman
            </Button>
          </div>
        </div>

        {!canRankPublish && (
          <div className={adminStyles.alertInfo} style={{ marginTop: 'var(--space-3)' }}>
            Simulasi perankingan, penerbitan hasil, dan promosi cadangan memerlukan peran{' '}
            <strong>Kepala Sekolah</strong> atau <strong>Admin Unit/Instansi</strong>.
          </div>
        )}
      </Card>

      {publishedSummary && (
        <Card>
          <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700, marginBottom: 'var(--space-2)' }}>
            Ringkasan Pengumuman Diterbitkan ({formatDateTime(publishedSummary.published_at)})
          </h2>
          <div className={styles.summaryGrid} style={{ marginBottom: 0 }}>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>Total Diterima</div>
              <div className={`${styles.statValue} tabular-nums`}>{publishedSummary.total_accepted}</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>Total Cadangan</div>
              <div className={`${styles.statValue} tabular-nums`}>{publishedSummary.total_waitlisted}</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>Tidak Lolos</div>
              <div className={`${styles.statValue} tabular-nums`}>{publishedSummary.total_rejected}</div>
            </div>
          </div>
        </Card>
      )}

      <div className={styles.layoutGrid}>
        <Card>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-3)' }}>
            <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>
              Kandidat Jalur ({candidates.length})
            </h2>
          </div>

          {loadingList ? (
            <div className={adminStyles.emptyState}>Memuat daftar kandidat...</div>
          ) : candidates.length === 0 ? (
            <div className={adminStyles.emptyState}>Belum ada kandidat pada jalur ini.</div>
          ) : (
            <div className={adminStyles.tableWrap}>
              <table className={adminStyles.table}>
                <thead>
                  <tr>
                    <th>Pendaftar</th>
                    <th>Status</th>
                    <th>Skor</th>
                    <th>Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {candidates.map((c) => {
                    const isSelected = c.id === selectedAppId;
                    return (
                      <tr
                        key={c.id}
                        style={{
                          backgroundColor: isSelected ? 'var(--brand-tint)' : undefined,
                        }}
                      >
                        <td>
                          <div style={{ fontWeight: 600 }}>
                            {c.student_name || c.registration_no}
                          </div>
                          <div className={`mono-code ${adminStyles.metaMuted}`}>
                            {c.registration_no}
                            {c.rank_position ? ` · Rank #${c.rank_position}` : ''}
                          </div>
                        </td>
                        <td>
                          <StatusBadge status={c.status} />
                        </td>
                        <td className="tabular-nums" style={{ fontWeight: 600 }}>
                          {c.final_score ?? '-'}
                        </td>
                        <td>
                          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                            <Button
                              size="sm"
                              variant={isSelected ? 'primary' : 'secondary'}
                              onClick={() => setSelectedAppId(c.id)}
                            >
                              <Award size={14} />
                              Nilai
                            </Button>
                            {c.status === 'waitlisted' && canRankPublish && (
                              <Button
                                size="sm"
                                variant="secondary"
                                loading={busyAction === `promote-${c.id}`}
                                onClick={() =>
                                  void handlePromoteWaitlist(
                                    c.id,
                                    c.student_name || c.registration_no,
                                  )
                                }
                              >
                                <ArrowUpCircle size={14} />
                                Angkat
                              </Button>
                            )}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>

        <Card>
          <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700, marginBottom: 'var(--space-2)' }}>
            Input Skor Komponen Seleksi
          </h2>
          {!selectedDetail ? (
            <div className={adminStyles.emptyState}>
              Pilih salah satu kandidat di samping untuk mengisi atau meninjau nilai tes.
            </div>
          ) : (
            <div className={adminStyles.stack}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
                <div>
                  <strong>
                    {selectedDetail.profile?.full_name ||
                      selectedDetail.application.student_name ||
                      selectedDetail.application.registration_no}
                  </strong>
                  <div className={`mono-code ${adminStyles.metaMuted}`}>
                    {selectedDetail.application.registration_no}
                  </div>
                </div>
                <StatusBadge status={selectedDetail.application.status} />
              </div>

              {!canScore && (
                <div className={adminStyles.alertInfo}>
                  Mode baca: penyimpanan nilai memerlukan peran <strong>Penguji</strong> atau{' '}
                  <strong>Admin Unit/Instansi</strong>.
                </div>
              )}

              <div>
                {scoreRows.map((row, idx) => (
                  <div key={idx} className={styles.scoreRow}>
                    <input
                      aria-label={`Kode komponen ${idx + 1}`}
                      className={styles.input}
                      placeholder="Kode (mis. TPA)"
                      value={row.component_code}
                      onChange={(e) => {
                        const val = e.target.value;
                        setScoreRows((prev) =>
                          prev.map((item, i) => (i === idx ? { ...item, component_code: val } : item)),
                        );
                      }}
                    />
                    <input
                      aria-label={`Skor mentah ${idx + 1}`}
                      type="number"
                      min={0}
                      max={100}
                      step="0.1"
                      className={styles.input}
                      placeholder="Nilai (0-100)"
                      value={row.raw_score}
                      onChange={(e) => {
                        const val = e.target.value;
                        setScoreRows((prev) =>
                          prev.map((item, i) => (i === idx ? { ...item, raw_score: val } : item)),
                        );
                      }}
                    />
                    <input
                      aria-label={`Bobot persen ${idx + 1}`}
                      type="number"
                      min={0}
                      max={100}
                      step="1"
                      className={styles.input}
                      placeholder="Bobot %"
                      value={row.weight_pct}
                      onChange={(e) => {
                        const val = e.target.value;
                        setScoreRows((prev) =>
                          prev.map((item, i) => (i === idx ? { ...item, weight_pct: val } : item)),
                        );
                      }}
                    />
                    <input
                      aria-label={`Catatan komponen ${idx + 1}`}
                      className={styles.input}
                      placeholder="Catatan penguji"
                      value={row.notes}
                      onChange={(e) => {
                        const val = e.target.value;
                        setScoreRows((prev) =>
                          prev.map((item, i) => (i === idx ? { ...item, notes: val } : item)),
                        );
                      }}
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      aria-label="Hapus komponen nilai"
                      disabled={scoreRows.length <= 1}
                      onClick={() =>
                        setScoreRows((prev) => prev.filter((_, i) => i !== idx))
                      }
                    >
                      <Trash2 size={15} />
                    </Button>
                  </div>
                ))}

                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 'var(--space-2)' }}>
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() =>
                      setScoreRows((prev) => [
                        ...prev,
                        { component_code: '', raw_score: '80', weight_pct: '20', notes: '' },
                      ])
                    }
                  >
                    <Plus size={14} />
                    Tambah Komponen
                  </Button>
                  <span className="tabular-nums" style={{ fontSize: 'var(--text-label)', fontWeight: 700 }}>
                    Proyeksi Skor Akhir: {projectedScore.toFixed(2)}
                  </span>
                </div>
              </div>

              <Button
                fullWidth
                disabled={!canScore}
                loading={busyAction === 'scores'}
                onClick={() => void handleSaveScores()}
              >
                <Award size={16} />
                Simpan Komponen Nilai
              </Button>
            </div>
          )}
        </Card>
      </div>

      <Card>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-3)', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
          <div>
            <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>
              Jadwal Tes, Observasi &amp; Wawancara ({schedules.length})
            </h2>
            <p className={adminStyles.metaMuted}>
              Penjadwalan sesi mengirim notifikasi WhatsApp otomatis ke orang tua/wali dan tampil di dashboard pendaftar.
            </p>
          </div>
          {canManageSchedules && (
            <Button
              size="sm"
              variant="secondary"
              disabled={candidates.length === 0}
              onClick={openCreateSchedule}
            >
              <CalendarPlus size={14} />
              Jadwalkan Seleksi
            </Button>
          )}
        </div>

        {loadingSchedules ? (
          <div className={adminStyles.emptyState}>Memuat jadwal asesmen...</div>
        ) : schedules.length === 0 ? (
          <div className={adminStyles.emptyState}>
            Belum ada jadwal tes/wawancara untuk jalur ini.
          </div>
        ) : (
          <div className={adminStyles.tableWrap}>
            <table className={adminStyles.table}>
              <thead>
                <tr>
                  <th>Pendaftar</th>
                  <th>Sesi / Tahapan</th>
                  <th>Waktu</th>
                  <th>Ruangan / Tautan</th>
                  <th>Penguji</th>
                  {canManageSchedules && <th>Aksi</th>}
                </tr>
              </thead>
              <tbody>
                {schedules.map((sched) => (
                  <tr key={sched.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>
                        {sched.student_name || sched.registration_no}
                      </div>
                      <div className={`mono-code ${adminStyles.metaMuted}`}>
                        {sched.registration_no}
                      </div>
                    </td>
                    <td style={{ fontWeight: 600 }}>{sched.session_name}</td>
                    <td className="tabular-nums">{formatDateTime(sched.scheduled_at)}</td>
                    <td>{sched.room_or_link || '-'}</td>
                    <td>{sched.examiner_name || '-'}</td>
                    {canManageSchedules && (
                      <td>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={busyAction === `schedule-del-${sched.id}`}
                          onClick={() => void handleDeleteSchedule(sched.id)}
                        >
                          <Trash2 size={14} />
                          Hapus
                        </Button>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {simulation && (
        <Card>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-3)', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
            <div>
              <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>
                Hasil Simulasi Ranking — {simulation.track_name}
              </h2>
              <p className={adminStyles.metaMuted}>
                Kuota {simulation.quota} kursi · Slot cadangan {simulation.waitlist_slots} · Sisa kuota{' '}
                {simulation.remaining_quota}
              </p>
            </div>
          </div>

          <div className={styles.summaryGrid}>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>Lolos Kuota</div>
              <div className={`${styles.statValue} tabular-nums`}>{simulation.accepted_count}</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>Masuk Cadangan</div>
              <div className={`${styles.statValue} tabular-nums`}>{simulation.waitlisted_count}</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>Tidak Lolos</div>
              <div className={`${styles.statValue} tabular-nums`}>{simulation.rejected_count}</div>
            </div>
            <div className={styles.statCard}>
              <div className={styles.statLabel}>Sisa Kuota</div>
              <div className={`${styles.statValue} tabular-nums`}>{simulation.remaining_quota}</div>
            </div>
          </div>

          {simulation.candidates.length === 0 ? (
            <div className={adminStyles.emptyState}>
              Belum ada pendaftar berstatus lolos berkas / seleksi pada jalur ini.
            </div>
          ) : (
            <div className={adminStyles.tableWrap}>
              <table className={adminStyles.table}>
                <thead>
                  <tr>
                    <th>Peringkat</th>
                    <th>Pendaftar</th>
                    <th>Skor Akhir</th>
                    <th>Jarak</th>
                    <th>Status Saat Ini</th>
                    <th>Proyeksi Keputusan</th>
                    <th>Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {simulation.candidates.map((cand) => (
                    <tr key={cand.application_id}>
                      <td className="tabular-nums" style={{ fontWeight: 700 }}>
                        #{cand.rank_position}
                      </td>
                      <td>
                        <div style={{ fontWeight: 600 }}>
                          {cand.student_name || cand.registration_no}
                        </div>
                        <div className={`mono-code ${adminStyles.metaMuted}`}>
                          {cand.registration_no}
                        </div>
                      </td>
                      <td className="tabular-nums" style={{ fontWeight: 600 }}>
                        {cand.final_score.toFixed(2)}
                      </td>
                      <td className="tabular-nums">
                        {cand.distance_meters !== undefined && cand.distance_meters !== null
                          ? `${cand.distance_meters} m`
                          : '-'}
                      </td>
                      <td>
                        <StatusBadge status={cand.current_status} />
                      </td>
                      <td>
                        <StatusBadge status={cand.simulated_decision} />
                      </td>
                      <td>
                        {cand.current_status === 'waitlisted' && canRankPublish ? (
                          <Button
                            size="sm"
                            variant="secondary"
                            loading={busyAction === `promote-${cand.application_id}`}
                            onClick={() =>
                              void handlePromoteWaitlist(
                                cand.application_id,
                                cand.student_name || cand.registration_no,
                              )
                            }
                          >
                            <ArrowUpCircle size={14} />
                            Angkat ke Diterima
                          </Button>
                        ) : (
                          <span className={adminStyles.metaMuted}>-</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      <Modal
        open={scheduleModalOpen}
        title="Jadwalkan Sesi Tes / Wawancara Seleksi"
        onClose={() => {
          if (busyAction !== 'schedule-create') setScheduleModalOpen(false);
        }}
        footer={
          <>
            <Button
              variant="secondary"
              disabled={busyAction === 'schedule-create'}
              onClick={() => setScheduleModalOpen(false)}
            >
              Batal
            </Button>
            <Button
              type="submit"
              form="schedule-create-form"
              loading={busyAction === 'schedule-create'}
            >
              <CalendarPlus size={14} />
              Simpan &amp; Kirim WA
            </Button>
          </>
        }
      >
        <form id="schedule-create-form" onSubmit={(e) => void handleCreateSchedule(e)} className={adminStyles.stack}>
          <div>
            <label htmlFor="sched-target-mode" style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
              Target Peserta
            </label>
            <select
              id="sched-target-mode"
              className={adminStyles.select}
              style={{ width: '100%' }}
              value={schedTargetMode}
              onChange={(e) => setSchedTargetMode(e.target.value as 'single' | 'all_track')}
            >
              <option value="single">Satu Pendaftar Tertentu</option>
              <option value="all_track">Semua Kandidat Terdaftar Jalur Ini ({candidates.length})</option>
            </select>
          </div>
          {schedTargetMode === 'single' && (
            <div>
              <label htmlFor="sched-app-select" style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
                Pilih Pendaftar
              </label>
              <select
                id="sched-app-select"
                className={adminStyles.select}
                style={{ width: '100%' }}
                value={schedAppId}
                onChange={(e) => setSchedAppId(e.target.value)}
              >
                {candidates.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.registration_no} — {c.student_name || 'Tanpa Nama'}
                  </option>
                ))}
              </select>
            </div>
          )}
          <InputField
            label="Nama Sesi / Tahapan"
            value={schedSessionName}
            onChange={(e) => setSchedSessionName(e.target.value)}
            placeholder="Tes Akademik & Wawancara Gelombang 1"
            required
          />
          <InputField
            label="Ruangan Fisik atau Tautan Meeting"
            value={schedRoomOrLink}
            onChange={(e) => setSchedRoomOrLink(e.target.value)}
            placeholder="Gedung A Ruang 102 / https://meet.jit.si/..."
          />
          <InputField
            label="Tanggal & Jam Pelaksanaan"
            type="datetime-local"
            value={schedDateTime}
            onChange={(e) => setSchedDateTime(e.target.value)}
            required
          />
          {examiners.length > 0 && (
            <div>
              <label htmlFor="sched-examiner-select" style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
                Penguji Bertugas (Opsional)
              </label>
              <select
                id="sched-examiner-select"
                className={adminStyles.select}
                style={{ width: '100%' }}
                value={schedExaminerId}
                onChange={(e) => setSchedExaminerId(e.target.value)}
              >
                <option value="">-- Otomatis / Ditentukan Panitia --</option>
                {examiners.map((st) => (
                  <option key={st.user_id} value={st.user_id}>
                    {st.full_name} ({st.roles.map((r) => r.role).join(', ')})
                  </option>
                ))}
              </select>
            </div>
          )}
        </form>
      </Modal>

      <Modal
        open={publishConfirmOpen}
        title="Konfirmasi Terbitkan Pengumuman"
        onClose={() => {
          if (busyAction !== 'publish') setPublishConfirmOpen(false);
        }}
      >
        <div className={adminStyles.stack}>
          <p style={{ fontSize: 'var(--text-label)', color: 'var(--text-secondary)', lineHeight: 1.55 }}>
            Anda akan menghitung perankingan final seluruh jalur pada periode{' '}
            <strong>{activePeriod?.name}</strong>, mengunci hasil ke status{' '}
            <strong>Diterima / Cadangan / Tidak Diterima</strong>, serta menandai hasil periode
            diumumkan kepada pendaftar.
          </p>
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-2)' }}>
            <Button
              variant="secondary"
              disabled={busyAction === 'publish'}
              onClick={() => setPublishConfirmOpen(false)}
            >
              Batal
            </Button>
            <Button
              loading={busyAction === 'publish'}
              onClick={() => void handlePublishResults()}
            >
              <Megaphone size={15} />
              Ya, Terbitkan Sekarang
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
