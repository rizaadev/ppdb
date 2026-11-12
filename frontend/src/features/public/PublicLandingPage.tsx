import { useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import {
  Award,
  BookOpen,
  Building2,
  Calendar,
  CheckCircle2,
  Clock,
  FileCheck,
  MapPin,
  MessageCircle,
  Search,
  ShieldCheck,
  Sparkles,
  Users,
} from 'lucide-react';
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

interface TimeRemaining {
  days: number;
  hours: number;
  minutes: number;
  seconds: number;
  isPast: boolean;
}

function calculateRemaining(targetIso?: string): TimeRemaining {
  if (!targetIso) {
    return { days: 0, hours: 0, minutes: 0, seconds: 0, isPast: true };
  }
  const diff = new Date(targetIso).getTime() - Date.now();
  if (diff <= 0) {
    return { days: 0, hours: 0, minutes: 0, seconds: 0, isPast: true };
  }
  return {
    days: Math.floor(diff / (1000 * 60 * 60 * 24)),
    hours: Math.floor((diff / (1000 * 60 * 60)) % 24),
    minutes: Math.floor((diff / 1000 / 60) % 60),
    seconds: Math.floor((diff / 1000) % 60),
    isPast: false,
  };
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

  const [remaining, setRemaining] = useState<TimeRemaining>(() =>
    calculateRemaining(activePeriod?.closes_at),
  );

  useEffect(() => {
    if (!activePeriod?.closes_at) return;
    const timer = setInterval(() => {
      setRemaining(calculateRemaining(activePeriod.closes_at));
    }, 1000);
    return () => clearInterval(timer);
  }, [activePeriod?.closes_at]);

  const totalCapacity = (activePeriod?.tracks ?? []).reduce((acc, t) => acc + (t.quota || 0), 0);

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

  const helpdeskWa = '6281234567890';
  const waHelpdeskUrl = `https://wa.me/${helpdeskWa}?text=${encodeURIComponent(
    `Halo Panitia PPDB ${portal.tenant.name}, saya ingin menanyakan informasi pendaftaran santri baru.`,
  )}`;

  return (
    <div>
      {/* HERO SECTION */}
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
              {portal.tenant.theme_config.tagline ||
                'Pendidikan Islam Berkarakter, Unggul dalam Ilmu & Akhlakul Karimah'}
              . Pendaftaran {term.applicantNoun} dilakukan secara daring terpadu dengan pemantauan
              status real-time oleh {term.guardianNoun}.
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

            {/* STATS RIBBON */}
            <div className={styles.statsRibbon}>
              <div className={styles.statItem}>
                <span className={styles.statNumber}>{portal.units.length} Unit</span>
                <span className={styles.statLabel}>Jenjang Pendidikan</span>
              </div>
              <div className={styles.statItem}>
                <span className={styles.statNumber}>{totalCapacity} Kuota</span>
                <span className={styles.statLabel}>Total Daya Tampung</span>
              </div>
              <div className={styles.statItem}>
                <span className={styles.statNumber}>{activePeriod?.tracks.length ?? 0} Jalur</span>
                <span className={styles.statLabel}>Pendaftaran Aktif</span>
              </div>
              <div className={styles.statItem}>
                <span className={styles.statNumber}>Terakreditasi</span>
                <span className={styles.statLabel}>Berasrama Penuh</span>
              </div>
            </div>
          </div>

          {/* COUNTDOWN & LOCATION CARD */}
          <div className={styles.countdownCard}>
            <div className={styles.countdownHeader}>
              <Clock size={18} style={{ color: 'var(--brand-primary)' }} />
              <span>Hitung Mundur Penutupan Pendaftaran</span>
            </div>

            <div className={styles.countdownGrid}>
              <div className={styles.countdownBlock}>
                <div className={`${styles.countdownNumber} tabular-nums`}>
                  {String(remaining.days).padStart(2, '0')}
                </div>
                <div className={styles.countdownLabel}>Hari</div>
              </div>
              <div className={styles.countdownBlock}>
                <div className={`${styles.countdownNumber} tabular-nums`}>
                  {String(remaining.hours).padStart(2, '0')}
                </div>
                <div className={styles.countdownLabel}>Jam</div>
              </div>
              <div className={styles.countdownBlock}>
                <div className={`${styles.countdownNumber} tabular-nums`}>
                  {String(remaining.minutes).padStart(2, '0')}
                </div>
                <div className={styles.countdownLabel}>Menit</div>
              </div>
              <div className={styles.countdownBlock}>
                <div className={`${styles.countdownNumber} tabular-nums`}>
                  {String(remaining.seconds).padStart(2, '0')}
                </div>
                <div className={styles.countdownLabel}>Detik</div>
              </div>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)', paddingTop: 'var(--space-2)', borderTop: '1px solid var(--border-default)' }}>
              <div style={{ display: 'flex', gap: 'var(--space-2)', color: 'var(--text-secondary)', fontSize: 'var(--text-caption)' }}>
                <Calendar size={16} style={{ flexShrink: 0, marginTop: 2, color: 'var(--brand-primary)' }} />
                <span>
                  Batas Akhir: <strong className="tabular-nums">{formatCloseDate(activePeriod?.closes_at)}</strong>
                </span>
              </div>
              <div style={{ display: 'flex', gap: 'var(--space-2)', color: 'var(--text-secondary)', fontSize: 'var(--text-caption)' }}>
                <MapPin size={16} style={{ flexShrink: 0, marginTop: 2, color: 'var(--brand-primary)' }} />
                <span>{portal.units[0]?.address || 'Pondok Pesantren Al-Hamid, Way Kanan, Lampung'}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* 7-STEP ADMISSION FLOW TIMELINE */}
      <section id="alur" className={`container-public ${styles.section}`}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionPill}>Panduan Registrasi</span>
          <h2 className={styles.sectionTitle}>Alur Pendaftaran 7 Langkah</h2>
          <p className={styles.sectionSub}>
            Proses seleksi penerimaan santri baru berlangsung transparan, mandiri, dan dapat dipantau langsung kapan saja.
          </p>
        </div>

        <div className={styles.timelineGrid}>
          <div className={styles.stepCard}>
            <div className={styles.stepBadge}>1</div>
            <h3 className={styles.stepTitle}>Buat Akun Wali</h3>
            <p className={styles.stepDesc}>
              Daftar akun menggunakan nomor WhatsApp dan kata sandi untuk mengelola pendaftaran calon santri secara terpusat.
            </p>
          </div>
          <div className={styles.stepCard}>
            <div className={styles.stepBadge}>2</div>
            <h3 className={styles.stepTitle}>Pilih Jalur &amp; Jenjang</h3>
            <p className={styles.stepDesc}>
              Pilih unit pendidikan tujuan serta jalur seleksi (Reguler, Prestasi, atau Tahfidz Al-Qur'an).
            </p>
          </div>
          <div className={styles.stepCard}>
            <div className={styles.stepBadge}>3</div>
            <h3 className={styles.stepTitle}>Pengisian Biodata</h3>
            <p className={styles.stepDesc}>
              Lengkapi data kependudukan (NIK, NISN, Kartu Keluarga), data orang tua/wali, serta riwayat pendidikan formal.
            </p>
          </div>
          <div className={styles.stepCard}>
            <div className={styles.stepBadge}>4</div>
            <h3 className={styles.stepTitle}>Unggah Berkas</h3>
            <p className={styles.stepDesc}>
              Unggah foto dokumen resmi (Kartu Keluarga, Akta Kelahiran, Pas Foto formal, dan Surat Keterangan Lulus/Rapor).
            </p>
          </div>
          <div className={styles.stepCard}>
            <div className={styles.stepBadge}>5</div>
            <h3 className={styles.stepTitle}>Pembayaran Biaya</h3>
            <p className={styles.stepDesc}>
              Selesaikan biaya pendaftaran melalui QRIS instan, Virtual Account semua bank nasional, atau transfer manual.
            </p>
          </div>
          <div className={styles.stepCard}>
            <div className={styles.stepBadge}>6</div>
            <h3 className={styles.stepTitle}>Verifikasi &amp; Seleksi</h3>
            <p className={styles.stepDesc}>
              Panitia memvalidasi berkas dan calon santri mengikuti jadwal tes seleksi (baca kitab, tahsin, atau tes potensi).
            </p>
          </div>
          <div className={styles.stepCard}>
            <div className={styles.stepBadge}>7</div>
            <h3 className={styles.stepTitle}>SK Kelulusan &amp; Daftar Ulang</h3>
            <p className={styles.stepDesc}>
              Cek pengumuman resmi kelulusan, cetak Surat Keterangan Lulus (SKL) berstempel digital, dan konfirmasi daftar ulang.
            </p>
          </div>
        </div>
      </section>

      {/* FASILITAS & KEUNGGULAN LEMBAGA */}
      <section id="fasilitas" className={`${styles.section} ${styles.sectionAlt}`}>
        <div className={`container-public`}>
          <div className={styles.sectionHeader}>
            <span className={styles.sectionPill}>Keunggulan Lembaga</span>
            <h2 className={styles.sectionTitle}>Fasilitas &amp; Lingkungan Belajar</h2>
            <p className={styles.sectionSub}>
              Sarana dan prasarana terpadu untuk membentuk santri yang beriman kokoh, berwawasan luas, dan mandiri.
            </p>
          </div>

          <div className={styles.facilityGrid}>
            <div className={styles.facilityCard}>
              <div className={styles.facilityIcon}>
                <Building2 size={24} />
              </div>
              <h3 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>Asrama Representatif &amp; Terpisah</h3>
              <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                Kompleks asrama putra dan putri yang bersih, aman, dan dibimbing oleh dewan asatidz pembina selama 24 jam penuh.
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityIcon}>
                <Sparkles size={24} />
              </div>
              <h3 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>Masjid Jami' Pusat Ubudiyah</h3>
              <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                Pusat peribadatan shalat berjamaah 5 waktu, kajian halaqah kitab kuning, serta setoran hafalan Al-Qur'an.
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityIcon}>
                <BookOpen size={24} />
              </div>
              <h3 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>Maktabah Kitab &amp; Perpustakaan</h3>
              <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                Koleksi referensi ribuan jilid kitab turats klasik pesantren dipadukan dengan literatur ilmu pengetahuan modern.
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityIcon}>
                <ShieldCheck size={24} />
              </div>
              <h3 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>Lab Komputer &amp; Riset Sains</h3>
              <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                Fasilitas pembelajaran teknologi informasi, multimedia digital, dan eksperimen sains dasar penunjang akademik.
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityIcon}>
                <Users size={24} />
              </div>
              <h3 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>Pengembangan Bakat &amp; Olahraga</h3>
              <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                Lapangan olahraga, seni hadrah, pencak silat Pagar Nusa, pramuka, dan pembinaan kepemimpinan santri (OSIS/IPNU).
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityIcon}>
                <Award size={24} />
              </div>
              <h3 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>Pos Kesehatan Pesantren (Poskestren)</h3>
              <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                Layanan medis pertolongan pertama, pemantauan gizi santri, kerja sama rujukan puskesmas, dan dokter jaga berkala.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* JALUR & TRANSPARANSI BIAYA */}
      <section id="jalur" className={`container-public ${styles.section}`}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionPill}>Pilihan Penerimaan</span>
          <h2 className={styles.sectionTitle}>
            {term.trackNoun} &amp; Transparansi Biaya
          </h2>
          <p className={styles.sectionSub}>
            Tahun Ajaran {activePeriod?.academic_year} · Periode {activePeriod?.name}. Biaya pendaftaran jelas tanpa pungutan tersembunyi.
          </p>
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
                      Biaya Formulir: {formatRupiah(track.registration_fee_idr)}
                    </span>
                    {track.requires_coordinates && <span className={styles.tag}>Wajib Titik Peta</span>}
                    {track.requires_achievements && <span className={styles.tag}>Wajib Piagam Prestasi</span>}
                  </div>
                </div>

                <div>
                  <div className={`${styles.quotaMeta} tabular-nums`}>
                    <span>Terisi {track.active_applicant_count} santri</span>
                    <span>Daya tampung {track.quota}</span>
                  </div>
                  <div className={styles.progressTrack} aria-hidden="true">
                    <div className={styles.progressFill} style={{ width: `${pct}%` }} />
                  </div>

                  <div className={styles.paymentBadgesRow}>
                    <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)', fontWeight: 600 }}>Metode:</span>
                    <span className={styles.payPill}>QRIS</span>
                    <span className={styles.payPill}>BSI VA</span>
                    <span className={styles.payPill}>Mandiri</span>
                    <span className={styles.payPill}>BRI</span>
                    <span className={styles.payPill}>BCA</span>
                    <span className={styles.payPill}>Manual</span>
                  </div>

                  <div style={{ marginTop: 'var(--space-3)' }}>
                    <Link to={`/app/daftar?track=${track.id}`}>
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

      {/* CHECKLIST PERSYARATAN BERKAS */}
      <section id="syarat" className={`${styles.section} ${styles.sectionAlt}`}>
        <div className={`container-public`}>
          <div className={styles.sectionHeader}>
            <span className={styles.sectionPill}>Kelengkapan Dokumen</span>
            <h2 className={styles.sectionTitle}>Persyaratan Berkas Pendaftaran</h2>
            <p className={styles.sectionSub}>
              Pastikan seluruh berkas telah disiapkan dalam bentuk file digital (foto/scan jelas format PDF, JPG, atau PNG).
            </p>
          </div>

          <div className={styles.reqCard}>
            <div className={styles.reqList}>
              <div className={styles.reqItem}>
                <FileCheck size={20} style={{ color: 'var(--brand-primary)', flexShrink: 0, marginTop: 2 }} />
                <div>
                  <div className={styles.reqItemTitle}>Kartu Keluarga (KK)</div>
                  <div className={styles.reqItemSub}>Scan Kartu Keluarga terbaru yang mencantumkan nama calon santri &amp; orang tua.</div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <FileCheck size={20} style={{ color: 'var(--brand-primary)', flexShrink: 0, marginTop: 2 }} />
                <div>
                  <div className={styles.reqItemTitle}>Akta Kelahiran</div>
                  <div className={styles.reqItemSub}>Scan Akta Kelahiran resmi dari Dinas Kependudukan dan Pencatatan Sipil.</div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <FileCheck size={20} style={{ color: 'var(--brand-primary)', flexShrink: 0, marginTop: 2 }} />
                <div>
                  <div className={styles.reqItemTitle}>Pas Foto Formal Berwarna</div>
                  <div className={styles.reqItemSub}>Ukuran 3x4 dengan latar belakang merah atau biru, berpakaian rapi dan sopan.</div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <FileCheck size={20} style={{ color: 'var(--brand-primary)', flexShrink: 0, marginTop: 2 }} />
                <div>
                  <div className={styles.reqItemTitle}>Ijazah / SKL / Surat Keterangan Aktif</div>
                  <div className={styles.reqItemSub}>Surat keterangan dari madrasah/sekolah asal jika ijazah resmi belum terbit.</div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <FileCheck size={20} style={{ color: 'var(--brand-primary)', flexShrink: 0, marginTop: 2 }} />
                <div>
                  <div className={styles.reqItemTitle}>Surat Keterangan Sehat</div>
                  <div className={styles.reqItemSub}>Surat dokter atau Puskesmas yang menyatakan santri sehat untuk belajar di asrama.</div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <FileCheck size={20} style={{ color: 'var(--brand-primary)', flexShrink: 0, marginTop: 2 }} />
                <div>
                  <div className={styles.reqItemTitle}>Piagam Prestasi / Syahadah Tahfidz</div>
                  <div className={styles.reqItemSub}>Khusus bagi pendaftar jalur Prestasi atau Beasiswa Tahfidz Al-Qur'an.</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* FAST STATUS CHECK */}
      <section id="cek-status" className={`container-public ${styles.section}`}>
        <Card>
          <div className={styles.sectionHeader} style={{ marginBottom: 'var(--space-4)', textAlign: 'left' }}>
            <span className={styles.sectionPill}>Layanan Mandiri</span>
            <h2 className={styles.sectionTitle} style={{ fontSize: '1.4rem' }}>
              Cek Status Pendaftaran Cepat
            </h2>
            <p className={styles.sectionSub} style={{ fontSize: '0.875rem' }}>
              Masukkan nomor pendaftaran untuk melihat progres berkas dan seleksi secara instan (data pribadi disamarkan sesuai UU PDP).
            </p>
          </div>

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
                <div className="mono-code" style={{ fontWeight: 700, fontSize: '0.95rem' }}>
                  {statusResult.registration_no} · {statusResult.masked_name}
                </div>
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', marginTop: 4 }}>
                  Unit: <strong>{statusResult.unit_name}</strong> · Jalur: <strong>{statusResult.track_name}</strong>
                  {statusResult.rank_position ? ` · Peringkat #${statusResult.rank_position}` : ''}
                </div>
              </div>
              <StatusBadge status={statusResult.status} />
            </div>
          )}
        </Card>
      </section>

      {/* FAQ SECTION */}
      <section id="faq" className={`${styles.section} ${styles.sectionAlt}`}>
        <div className={`container-public`}>
          <div className={styles.sectionHeader}>
            <span className={styles.sectionPill}>Tanya Jawab</span>
            <h2 className={styles.sectionTitle}>Pertanyaan yang Sering Diajukan</h2>
            <p className={styles.sectionSub}>
              Temukan jawaban cepat seputar tata cara pendaftaran, tes seleksi, dan biaya operasional pesantren.
            </p>
          </div>

          <div className={styles.faqList}>
            <details className={styles.faqItem} open>
              <summary className={styles.faqSummary}>
                Bagaimana jika ijazah sekolah asal belum terbit saat mendaftar?
              </summary>
              <div className={styles.faqBody}>
                Calon santri dapat menggunakan Surat Keterangan Lulus (SKL) sementara atau surat keterangan aktif dari kepala madrasah/sekolah asal yang memuat NISN. Ijazah asli dapat disusulkan saat proses verifikasi berkas fisik atau saat kedatangan santri baru di pondok.
              </div>
            </details>

            <details className={styles.faqItem}>
              <summary className={styles.faqSummary}>
                Apakah pembayaran pendaftaran dapat dilakukan melalui transfer manual ke rekening bank?
              </summary>
              <div className={styles.faqBody}>
                Ya, sistem kami mendukung pembayaran instan otomatis (QRIS dan Virtual Account Bank Syariah Indonesia, Mandiri, BRI, BCA, BNI) serta transfer manual ke rekening panitia. Untuk transfer manual, cukup unggah foto bukti transfer di portal wali dan panitia keuangan akan memverifikasinya dalam 1x24 jam.
              </div>
            </details>

            <details className={styles.faqItem}>
              <summary className={styles.faqSummary}>
                Bagaimana mekanisme pelaksanaan tes seleksi penerimaan santri baru?
              </summary>
              <div className={styles.faqBody}>
                Materi seleksi meliputi tes baca Al-Qur'an (tajwid &amp; makharijul huruf), tes praktik ibadah dasar, tes potensi akademik, serta wawancara motivasi santri dan komitmen orang tua. Seleksi dapat dilaksanakan secara tatap muka langsung di pesantren atau daring (video call) bagi pendaftar luar pulau.
              </div>
            </details>

            <details className={styles.faqItem}>
              <summary className={styles.faqSummary}>
                Apakah tersedia beasiswa untuk calon santri penghafal Al-Qur'an (Tahfidz)?
              </summary>
              <div className={styles.faqBody}>
                Ya, Pondok Pesantren Al-Hamid menyediakan kuota beasiswa khusus bagi santri jalur Tahfidz yang memiliki hafalan minimal 3 Juz mutqin bersyahadah. Silakan pilih Jalur Prestasi/Tahfidz saat mendaftar dan unggah piagam atau surat keterangan dari pondok/ustadz sebelumnya.
              </div>
            </details>

            <details className={styles.faqItem}>
              <summary className={styles.faqSummary}>
                Bagaimana cara mencetak Kartu Ujian dan Surat Keterangan Kelulusan (SKL)?
              </summary>
              <div className={styles.faqBody}>
                Setelah berkas pendaftaran diverifikasi panitia, tombol Cetak Kartu Peserta Ujian akan aktif otomatis di Dashboard Wali Santri. Setelah pengumuman kelulusan resmi, Surat Keputusan Kelulusan resmi lengkap dengan tanda tangan digital dan QR verifikasi dinas dapat langsung diunduh dan dicetak dalam format PDF.
              </div>
            </details>
          </div>
        </div>
      </section>

      {/* FLOATING WHATSAPP HELPDESK BUTTON */}
      <a
        href={waHelpdeskUrl}
        target="_blank"
        rel="noopener noreferrer"
        className={styles.floatingWa}
        aria-label="Konsultasi Panitia PPDB via WhatsApp"
      >
        <MessageCircle size={20} />
        <span>Bantuan Panitia PPDB</span>
      </a>
    </div>
  );
}
