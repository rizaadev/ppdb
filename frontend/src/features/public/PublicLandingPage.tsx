import { useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { Calendar, CheckCircle2, MapPin, Search } from 'lucide-react';
import { ApiError, apiRequest, formatRupiah } from '@/core/api/client';
import { useTerminology } from '@/core/terminology/terminology';
import type { PublicCheckStatusResult, PublicPortalPayload } from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { InputField } from '@/components/ui/InputField';
import { StatusBadge } from '@/components/ui/StatusBadge';
import styles from './PublicLandingPage.module.css';

function formatCloseDate(iso?: string): string {
  if (!iso) return 'Menyesuaikan jadwal resmi';
  const dt = new Date(iso);
  if (Number.isNaN(dt.getTime())) return iso;
  return new Intl.DateTimeFormat('id-ID', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  }).format(dt);
}

export function PublicLandingPage({
  portal,
  tenantSlug,
}: {
  portal: PublicPortalPayload;
  tenantSlug: string;
}) {
  const term = useTerminology();
  const activePeriod = portal.periods[0];
  const [regNo, setRegNo] = useState('');
  const [statusResult, setStatusResult] = useState<PublicCheckStatusResult | null>(null);
  const [statusError, setStatusError] = useState('');
  const [checking, setChecking] = useState(false);

  async function handleCheckStatus(e: FormEvent) {
    e.preventDefault();
    const cleaned = regNo.trim().toUpperCase();
    if (!cleaned) {
      setStatusError('Masukkan nomor pendaftaran terlebih dahulu.');
      setStatusResult(null);
      return;
    }

    setStatusError('');
    setChecking(true);
    try {
      const res = await apiRequest<PublicCheckStatusResult>('/api/v1/public/check-status', {
        method: 'POST',
        tenantSlug,
        skipAuth: true,
        body: { registration_no: cleaned },
      });
      setStatusResult(res);
    } catch (err) {
      setStatusResult(null);
      if (err instanceof ApiError) {
        setStatusError(err.message);
      } else {
        setStatusError('Nomor pendaftaran tidak ditemukan atau server tidak terjangkau.');
      }
    } finally {
      setChecking(false);
    }
  }

  return (
    <div>
      <section className={styles.hero}>
        <div className={`container-public ${styles.heroGrid}`}>
          <div>
            <div className={styles.pill}>
              <CheckCircle2 size={14} />
              <span>
                {term.programAcronym} {activePeriod?.academic_year} · {activePeriod?.name} Dibuka
              </span>
            </div>
            <h1 className={styles.heroTitle}>{portal.tenant.name}</h1>
            <p className={`${styles.heroSub} prose-limit`}>
              {portal.tenant.theme_config.tagline} — Pendaftaran {term.applicantNoun} dilakukan
              secara daring dengan pemantauan status real-time oleh {term.guardianNoun}.
            </p>
            <div className={styles.heroActions}>
              <Link to="/app/daftar">
                <Button size="lg">Daftar {term.applicantNoun}</Button>
              </Link>
              <Link to="/app">
                <Button variant="secondary" size="lg">
                  Masuk Portal {term.guardianNoun}
                </Button>
              </Link>
            </div>
          </div>

          <Card>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
              <div style={{ fontWeight: 700, fontSize: 'var(--text-h3)' }}>
                Ringkasan Jadwal &amp; Lokasi
              </div>
              <div style={{ display: 'flex', gap: 'var(--space-2)', color: 'var(--text-secondary)', fontSize: 'var(--text-caption)' }}>
                <Calendar size={16} style={{ flexShrink: 0, marginTop: 2 }} />
                <span>
                  Pendaftaran ditutup: <strong className="tabular-nums">{formatCloseDate(activePeriod?.closes_at)}</strong>
                </span>
              </div>
              <div style={{ display: 'flex', gap: 'var(--space-2)', color: 'var(--text-secondary)', fontSize: 'var(--text-caption)' }}>
                <MapPin size={16} style={{ flexShrink: 0, marginTop: 2 }} />
                <span>{portal.units[0]?.address}</span>
              </div>
              <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>
                Total {portal.units.length} unit jenjang aktif siap menerima pendaftaran.
              </div>
            </div>
          </Card>
        </div>
      </section>

      <section id="jalur" className={`container-public ${styles.section}`}>
        <div className={styles.sectionHeader}>
          <h2 className={styles.sectionTitle}>
            {term.trackNoun} ({activePeriod?.name})
          </h2>
          <span style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>
            Kuota &amp; persyaratan menyesuaikan jalur terpilih
          </span>
        </div>

        <div className="grid-auto-cards grid-auto-cards-3">
          {activePeriod?.tracks.map((track) => {
            const pct = Math.min(
              100,
              Math.round((track.active_applicant_count / Math.max(1, track.quota)) * 100),
            );
            return (
              <Card key={track.id} interactive className={styles.trackCard}>
                <div>
                  <h3 className={styles.trackTitle}>{track.display_name}</h3>
                  <p className={styles.trackDesc}>{track.description}</p>
                  <div className={styles.trackTags}>
                    <span className={`${styles.tag} tabular-nums`}>
                      Biaya: {formatRupiah(track.registration_fee_idr)}
                    </span>
                    {track.requires_coordinates && <span className={styles.tag}>Wajib Titik Peta</span>}
                    {track.requires_achievements && <span className={styles.tag}>Unggah Piagam</span>}
                  </div>
                </div>

                <div>
                  <div className={`${styles.quotaMeta} tabular-nums`}>
                    <span>Terisi {track.active_applicant_count} pendaftar</span>
                    <span>Daya tampung {track.quota}</span>
                  </div>
                  <div className={styles.progressTrack} aria-hidden="true">
                    <div className={styles.progressFill} style={{ width: `${pct}%` }} />
                  </div>
                  <div style={{ marginTop: 'var(--space-3)' }}>
                    <Link to={`/app?track=${track.id}`}>
                      <Button variant="secondary" size="sm" fullWidth>
                        Daftar Jalur Ini
                      </Button>
                    </Link>
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      </section>

      <section id="cek-status" className={`container-public ${styles.section}`}>
        <Card>
          <h2 className={styles.sectionTitle} style={{ marginBottom: 'var(--space-2)' }}>
            Cek Status Pendaftaran Cepat
          </h2>
          <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', marginBottom: 'var(--space-4)' }}>
            Masukkan nomor pendaftaran untuk melihat progres verifikasi secara aman (data kependudukan disamarkan).
          </p>
          <form onSubmit={handleCheckStatus} className={styles.statusForm}>
            <InputField
              label="Nomor Pendaftaran"
              placeholder="Contoh: SPMB-2601-0001"
              value={regNo}
              onChange={(e) => setRegNo(e.target.value)}
              error={statusError}
              mono
            />
            <Button type="submit" disabled={checking}>
              <Search size={16} />
              {checking ? 'Memeriksa...' : 'Periksa Status'}
            </Button>
          </form>

          {statusResult && (
            <div className={styles.statusResult}>
              <div>
                <div className="mono-code" style={{ fontWeight: 700 }}>
                  {statusResult.registration_no} · {statusResult.masked_name}
                </div>
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)' }}>
                  {statusResult.unit_name} · {statusResult.track_name}
                  {statusResult.rank_position ? ` · Peringkat #${statusResult.rank_position}` : ''}
                </div>
              </div>
              <StatusBadge status={statusResult.status} />
            </div>
          )}
        </Card>
      </section>
    </div>
  );
}

