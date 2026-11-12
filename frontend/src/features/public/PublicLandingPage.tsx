import { useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowRight,
  Award,
  BookOpen,
  Building2,
  Calendar,
  Check,
  CheckCircle2,
  Clock,
  Compass,
  CreditCard,
  FileCheck,
  FileText,
  MapPin,
  MessageCircle,
  Search,
  ShieldCheck,
  Sparkles,
  UploadCloud,
  UserCheck,
  Users,
} from 'lucide-react';
import { ApiError, apiRequest, formatRupiah } from '@/core/api/client';
import { useTerminology } from '@/core/terminology/terminology';
import type { PublicCheckStatusResult, PublicPortalPayload } from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { InputField } from '@/components/ui/InputField';
import { StatusBadge } from '@/components/ui/StatusBadge';
import styles from './PublicLandingPage.module.css';

function formatCloseDate(iso?: string): string {
  if (!iso) return 'Menyesuaikan jadwal resmi panitia';
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
  const totalRegistered = (activePeriod?.tracks ?? []).reduce(
    (acc, t) => acc + (t.active_applicant_count || 0),
    0,
  );

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
    <div className={styles.landingWrapper}>
      {/* =========================================================================
          HERO SECTION
          ========================================================================= */}
      <section className={styles.hero}>
        <div className={`container-public ${styles.heroGrid}`}>
          {/* Left Column: Headlines & Actions */}
          <div className={styles.heroLeft}>
            <div className={styles.heroBadge}>
              <span className={styles.pulseDotWrapper}>
                <span className={styles.pulseDotPing} />
                <span className={styles.pulseDotCore} />
              </span>
              <span>
                {term.programAcronym} T.A. {activePeriod?.academic_year || '2026/2027'} · {activePeriod?.name || 'Gelombang 1'} Dibuka
              </span>
            </div>

            <h1 className={styles.heroTitle}>
              Pondok Pesantren <br />
              <span className={styles.heroTitleGradient}>
                {portal.tenant.name.replace(/Pondok Pesantren\s*/i, '') || 'Al-Hamid Cinta Mulya'}
              </span>
            </h1>

            <p className={styles.heroSub}>
              {portal.tenant.theme_config.tagline ||
                'Pendidikan Islam Berkarakter, Unggul dalam Ilmu & Akhlakul Karimah'}
              . Portal pendaftaran daring terpadu untuk calon {term.applicantNoun}. Pantau proses seleksi, pembayaran multi-bank, hingga cetak SK kelulusan resmi dalam satu tempat.
            </p>

            <div className={styles.heroActions}>
              <Link to="/app/daftar">
                <Button size="lg" className={styles.primaryCtaBtn}>
                  <span>Daftar {term.applicantNoun} Sekarang</span>
                  <ArrowRight size={18} />
                </Button>
              </Link>
              <Link to="/app">
                <Button variant="secondary" size="lg" className={styles.secondaryCtaBtn}>
                  Masuk Portal {term.guardianNoun}
                </Button>
              </Link>
            </div>

            <div className={styles.heroTrustBadges}>
              <div className={styles.trustItem}>
                <CheckCircle2 size={16} className={styles.trustIcon} />
                <span>Terakreditasi Kemenag RI</span>
              </div>
              <div className={styles.trustItem}>
                <CheckCircle2 size={16} className={styles.trustIcon} />
                <span>100% Berasrama Penuh</span>
              </div>
              <div className={styles.trustItem}>
                <CheckCircle2 size={16} className={styles.trustIcon} />
                <span>Beasiswa Tahfidz Al-Qur'an</span>
              </div>
            </div>

            {/* Quick Stats Grid */}
            <div className={styles.statsRibbon}>
              <div className={styles.statCard}>
                <div className={styles.statIconWrap}>
                  <Building2 size={20} />
                </div>
                <div>
                  <span className={styles.statNumber}>{portal.units.length} Unit</span>
                  <span className={styles.statLabel}>Jenjang Pendidikan</span>
                </div>
              </div>
              <div className={styles.statCard}>
                <div className={styles.statIconWrap}>
                  <Users size={20} />
                </div>
                <div>
                  <span className={styles.statNumber}>{totalCapacity} Kuota</span>
                  <span className={styles.statLabel}>Daya Tampung Total</span>
                </div>
              </div>
              <div className={styles.statCard}>
                <div className={styles.statIconWrap}>
                  <Award size={20} />
                </div>
                <div>
                  <span className={styles.statNumber}>{activePeriod?.tracks.length ?? 0} Jalur</span>
                  <span className={styles.statLabel}>Pendaftaran Aktif</span>
                </div>
              </div>
              <div className={styles.statCard}>
                <div className={styles.statIconWrap}>
                  <ShieldCheck size={20} />
                </div>
                <div>
                  <span className={styles.statNumber}>24 Jam</span>
                  <span className={styles.statLabel}>Bimbingan Asatidz</span>
                </div>
              </div>
            </div>
          </div>

          {/* Right Column: Registration Hub Card */}
          <div className={styles.heroRight}>
            <div className={styles.hubCard}>
              <div className={styles.hubHeader}>
                <div className={styles.hubStatusLive}>
                  <span className={styles.liveIndicatorDot} />
                  <span>Pendaftaran Sedang Berlangsung</span>
                </div>
                <div className={styles.hubWaveTag}>
                  {activePeriod?.name || 'Gelombang 1'}
                </div>
              </div>

              <div className={styles.countdownSection}>
                <div className={styles.countdownTitle}>
                  <Clock size={16} />
                  <span>Sisa Waktu Penutupan Gelombang:</span>
                </div>
                <div className={styles.countdownGrid}>
                  <div className={styles.countdownBlock}>
                    <div className={styles.countdownNumber}>
                      {String(remaining.days).padStart(2, '0')}
                    </div>
                    <div className={styles.countdownLabel}>Hari</div>
                  </div>
                  <div className={styles.countdownBlock}>
                    <div className={styles.countdownNumber}>
                      {String(remaining.hours).padStart(2, '0')}
                    </div>
                    <div className={styles.countdownLabel}>Jam</div>
                  </div>
                  <div className={styles.countdownBlock}>
                    <div className={styles.countdownNumber}>
                      {String(remaining.minutes).padStart(2, '0')}
                    </div>
                    <div className={styles.countdownLabel}>Menit</div>
                  </div>
                  <div className={styles.countdownBlock}>
                    <div className={styles.countdownNumber}>
                      {String(remaining.seconds).padStart(2, '0')}
                    </div>
                    <div className={styles.countdownLabel}>Detik</div>
                  </div>
                </div>
              </div>

              {/* Quota Progress */}
              <div className={styles.hubQuotaBox}>
                <div className={styles.hubQuotaHeader}>
                  <span>Keterisian Kuota</span>
                  <strong className="tabular-nums">
                    {totalRegistered} dari {totalCapacity} Santri
                  </strong>
                </div>
                <div className={styles.hubProgressBar}>
                  <div
                    className={styles.hubProgressFill}
                    style={{
                      width: `${Math.min(100, Math.round((totalRegistered / Math.max(1, totalCapacity)) * 100))}%`,
                    }}
                  />
                </div>
              </div>

              {/* Meta Info */}
              <div className={styles.hubMetaList}>
                <div className={styles.hubMetaItem}>
                  <Calendar size={16} className={styles.hubMetaIcon} />
                  <span>
                    Batas Akhir: <strong>{formatCloseDate(activePeriod?.closes_at)}</strong>
                  </span>
                </div>
                <div className={styles.hubMetaItem}>
                  <MapPin size={16} className={styles.hubMetaIcon} />
                  <span>{portal.units[0]?.address || 'Kampus Al-Hamid, Way Pengubuan, Lampung'}</span>
                </div>
              </div>

              <div className={styles.hubActionArea}>
                <Link to="/app/daftar">
                  <Button fullWidth size="lg">
                    <span>Mulai Pendaftaran Online</span>
                    <ArrowRight size={16} />
                  </Button>
                </Link>
                <div className={styles.hubGuaranteeText}>
                  Proses cepat ± 10 menit · Siapkan NIK &amp; Kartu Keluarga
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* =========================================================================
          PILLARS / VALUE PROPOSITION RIBBON
          ========================================================================= */}
      <section className={styles.pillarsRibbon}>
        <div className={`container-public ${styles.pillarsGrid}`}>
          <div className={styles.pillarCard}>
            <div className={styles.pillarIcon}>
              <BookOpen size={22} />
            </div>
            <div>
              <h3 className={styles.pillarTitle}>Kurikulum Terintegrasi</h3>
              <p className={styles.pillarDesc}>
                Memadukan kajian kitab kuning salafiyah klasik dengan pendidikan kurikulum nasional berakreditasi.
              </p>
            </div>
          </div>

          <div className={styles.pillarCard}>
            <div className={styles.pillarIcon}>
              <Sparkles size={22} />
            </div>
            <div>
              <h3 className={styles.pillarTitle}>Tahfidzul Qur'an Bersanad</h3>
              <p className={styles.pillarDesc}>
                Program intensif hafalan Al-Qur'an 30 Juz mutqin dengan bimbingan asatidz muhaffidz berpengalaman.
              </p>
            </div>
          </div>

          <div className={styles.pillarCard}>
            <div className={styles.pillarIcon}>
              <ShieldCheck size={22} />
            </div>
            <div>
              <h3 className={styles.pillarTitle}>Karakter &amp; Kemandirian</h3>
              <p className={styles.pillarDesc}>
                Kompleks asrama representatif dengan pembinaan akhlak mulia dan pengawasan 24 jam penuh.
              </p>
            </div>
          </div>

          <div className={styles.pillarCard}>
            <div className={styles.pillarIcon}>
              <CreditCard size={22} />
            </div>
            <div>
              <h3 className={styles.pillarTitle}>Pembayaran Transparan</h3>
              <p className={styles.pillarDesc}>
                Konfirmasi instan otomatis via QRIS dan Virtual Account seluruh bank nasional serta transfer manual.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* =========================================================================
          7-STEP ADMISSION FLOW TIMELINE
          ========================================================================= */}
      <section id="alur" className={`container-public ${styles.section}`}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionPill}>Panduan Registrasi</span>
          <h2 className={styles.sectionTitle}>Alur Pendaftaran 7 Langkah</h2>
          <p className={styles.sectionSub}>
            Proses seleksi penerimaan santri baru berlangsung transparan, mandiri, dan dapat dipantau langsung setiap saat.
          </p>
        </div>

        <div className={styles.timelineGrid}>
          <div className={styles.stepCard}>
            <div className={styles.stepHeader}>
              <div className={styles.stepBadge}>01</div>
              <UserCheck size={20} className={styles.stepIcon} />
            </div>
            <h3 className={styles.stepTitle}>Buat Akun Wali</h3>
            <p className={styles.stepDesc}>
              Daftar menggunakan nomor WhatsApp aktif untuk mengelola pendaftaran calon santri secara terpusat.
            </p>
          </div>

          <div className={styles.stepCard}>
            <div className={styles.stepHeader}>
              <div className={styles.stepBadge}>02</div>
              <Compass size={20} className={styles.stepIcon} />
            </div>
            <h3 className={styles.stepTitle}>Pilih Jalur &amp; Jenjang</h3>
            <p className={styles.stepDesc}>
              Tentukan unit pendidikan tujuan serta jalur masuk yang sesuai (Reguler, Prestasi, atau Beasiswa Tahfidz).
            </p>
          </div>

          <div className={styles.stepCard}>
            <div className={styles.stepHeader}>
              <div className={styles.stepBadge}>03</div>
              <FileText size={20} className={styles.stepIcon} />
            </div>
            <h3 className={styles.stepTitle}>Pengisian Biodata</h3>
            <p className={styles.stepDesc}>
              Lengkapi identitas calon santri (NIK, NISN, KK), riwayat kesehatan, dan data orang tua/wali secara lengkap.
            </p>
          </div>

          <div className={styles.stepCard}>
            <div className={styles.stepHeader}>
              <div className={styles.stepBadge}>04</div>
              <UploadCloud size={20} className={styles.stepIcon} />
            </div>
            <h3 className={styles.stepTitle}>Unggah Berkas</h3>
            <p className={styles.stepDesc}>
              Unggah foto digital dokumen resmi: Kartu Keluarga, Akta Kelahiran, Pas Foto, dan SKL/Rapor sekolah asal.
            </p>
          </div>

          <div className={styles.stepCard}>
            <div className={styles.stepHeader}>
              <div className={styles.stepBadge}>05</div>
              <CreditCard size={20} className={styles.stepIcon} />
            </div>
            <h3 className={styles.stepTitle}>Pembayaran Biaya</h3>
            <p className={styles.stepDesc}>
              Selesaikan biaya pendaftaran via QRIS instan, Virtual Account semua bank nasional, atau transfer manual.
            </p>
          </div>

          <div className={styles.stepCard}>
            <div className={styles.stepHeader}>
              <div className={styles.stepBadge}>06</div>
              <CheckCircle2 size={20} className={styles.stepIcon} />
            </div>
            <h3 className={styles.stepTitle}>Verifikasi &amp; Seleksi</h3>
            <p className={styles.stepDesc}>
              Panitia memvalidasi berkas, dan calon santri mengikuti jadwal tes seleksi (baca Al-Qur'an &amp; wawancara).
            </p>
          </div>

          <div className={styles.stepCard}>
            <div className={styles.stepHeader}>
              <div className={styles.stepBadge}>07</div>
              <Award size={20} className={styles.stepIcon} />
            </div>
            <h3 className={styles.stepTitle}>Kelulusan &amp; Cetak SKL</h3>
            <p className={styles.stepDesc}>
              Pantau pengumuman kelulusan resmi, cetak Surat Keputusan bertanda tangan digital, dan selesaikan daftar ulang.
            </p>
          </div>
        </div>
      </section>

      {/* =========================================================================
          FASILITAS & KEUNGGULAN LEMBAGA
          ========================================================================= */}
      <section id="fasilitas" className={`${styles.section} ${styles.sectionAlt}`}>
        <div className="container-public">
          <div className={styles.sectionHeader}>
            <span className={styles.sectionPill}>Keunggulan Lembaga</span>
            <h2 className={styles.sectionTitle}>Fasilitas &amp; Lingkungan Belajar</h2>
            <p className={styles.sectionSub}>
              Sarana dan prasarana terpadu untuk membentuk santri yang beriman kokoh, berwawasan luas, mandiri, dan berakhlakul karimah.
            </p>
          </div>

          <div className={styles.facilityGrid}>
            <div className={styles.facilityCard}>
              <div className={styles.facilityCardTop}>
                <div className={styles.facilityIcon}>
                  <Building2 size={24} />
                </div>
                <span className={styles.facilityTag}>Keasramaan</span>
              </div>
              <h3 className={styles.facilityTitle}>Asrama Representatif &amp; Terpisah</h3>
              <p className={styles.facilityDesc}>
                Kompleks asrama putra dan putri yang bersih, higienis, dan dibimbing oleh dewan asatidz pembina asrama selama 24 jam penuh.
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityCardTop}>
                <div className={styles.facilityIcon}>
                  <Sparkles size={24} />
                </div>
                <span className={styles.facilityTag}>Ubudiyah</span>
              </div>
              <h3 className={styles.facilityTitle}>Masjid Jami' Pusat Peribadatan</h3>
              <p className={styles.facilityDesc}>
                Pusat kegiatan shalat berjamaah 5 waktu, kajian halaqah kitab kuning ba'da Shubuh &amp; Maghrib, serta setoran tahfidz.
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityCardTop}>
                <div className={styles.facilityIcon}>
                  <BookOpen size={24} />
                </div>
                <span className={styles.facilityTag}>Literasi Salaf</span>
              </div>
              <h3 className={styles.facilityTitle}>Maktabah Kitab &amp; Perpustakaan</h3>
              <p className={styles.facilityDesc}>
                Koleksi ribuan jilid kitab turats klasik pesantren dipadukan dengan buku teks sains modern dan ruang baca nyaman.
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityCardTop}>
                <div className={styles.facilityIcon}>
                  <ShieldCheck size={24} />
                </div>
                <span className={styles.facilityTag}>Teknologi &amp; Sains</span>
              </div>
              <h3 className={styles.facilityTitle}>Lab Komputer &amp; Riset Sains</h3>
              <p className={styles.facilityDesc}>
                Fasilitas pembelajaran teknologi informasi, multimedia digital, CBT ujian, dan laboratorium sains penunjang akademik.
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityCardTop}>
                <div className={styles.facilityIcon}>
                  <Users size={24} />
                </div>
                <span className={styles.facilityTag}>Karakter</span>
              </div>
              <h3 className={styles.facilityTitle}>Pengembangan Bakat &amp; Seni</h3>
              <p className={styles.facilityDesc}>
                Lapangan olahraga multifungsi, seni hadrah &amp; shalawat, pencak silat Pagar Nusa, pramuka, dan organisasi santri (OSIS/IPNU).
              </p>
            </div>

            <div className={styles.facilityCard}>
              <div className={styles.facilityCardTop}>
                <div className={styles.facilityIcon}>
                  <Award size={24} />
                </div>
                <span className={styles.facilityTag}>Kesehatan</span>
              </div>
              <h3 className={styles.facilityTitle}>Pos Kesehatan Pesantren (Poskestren)</h3>
              <p className={styles.facilityDesc}>
                Layanan medis pertolongan pertama, pemantauan gizi &amp; higienitas makanan, kerja sama rujukan puskesmas, dan tenaga medis siaga.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* =========================================================================
          ISLAMIC WISDOM BANNER / PESANTREN QUOTE
          ========================================================================= */}
      <section className={styles.quoteSection}>
        <div className="container-public">
          <div className={styles.quoteCard}>
            <div className={styles.quoteArabic} dir="rtl">
              مَنْ يُرِدِ اللَّهُ بِهِ خَيْرًا يُفَقِّهْهُ فِي الدِّينِ
            </div>
            <blockquote className={styles.quoteText}>
              "Barangsiapa yang Allah kehendaki kebaikan baginya, niscaya Allah pahamkan ia secara mendalam dalam urusan agamanya."
            </blockquote>
            <div className={styles.quoteAuthor}>
              — Komitmen Pengasuh Pondok Pesantren Al-Hamid Cinta Mulya dalam Membina Generasi Qur'ani yang Beradab &amp; Mandiri
            </div>
          </div>
        </div>
      </section>

      {/* =========================================================================
          JALUR & TRANSPARANSI BIAYA (TIER CARDS)
          ========================================================================= */}
      <section id="jalur" className={`container-public ${styles.section}`}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionPill}>Pilihan Penerimaan</span>
          <h2 className={styles.sectionTitle}>
            {term.trackNoun} &amp; Transparansi Biaya
          </h2>
          <p className={styles.sectionSub}>
            Tahun Ajaran {activePeriod?.academic_year} · Periode {activePeriod?.name}. Rincian biaya transparan dan terverifikasi tanpa biaya tersembunyi.
          </p>
        </div>

        <div className={styles.tracksGrid}>
          {activePeriod?.tracks.map((track) => {
            const pct = Math.min(
              100,
              Math.round((track.active_applicant_count / Math.max(1, track.quota)) * 100),
            );
            return (
              <div key={track.id} className={styles.trackCardEnhanced}>
                <div className={styles.trackCardTop}>
                  <div className={styles.trackBadge}>Jalur Pilihan Utama</div>
                  <h3 className={styles.trackName}>{track.display_name}</h3>
                  <p className={styles.trackDescription}>{track.description}</p>
                </div>

                <div className={styles.trackPricingBlock}>
                  <div className={styles.trackFeeLabel}>Biaya Formulir Pendaftaran</div>
                  <div className={styles.trackFeeAmount}>
                    {formatRupiah(track.registration_fee_idr)}
                    <span className={styles.trackFeePer}> / calon santri</span>
                  </div>
                </div>

                <div className={styles.trackFeaturesList}>
                  <div className={styles.trackFeatureItem}>
                    <Check size={16} className={styles.checkIcon} />
                    <span>Daya tampung: <strong>{track.quota} Santri</strong></span>
                  </div>
                  <div className={styles.trackFeatureItem}>
                    <Check size={16} className={styles.checkIcon} />
                    <span>Ujian Seleksi Potensi &amp; Baca Al-Qur'an</span>
                  </div>
                  <div className={styles.trackFeatureItem}>
                    <Check size={16} className={styles.checkIcon} />
                    <span>Verifikasi Berkas Daring Tanpa Antri</span>
                  </div>
                  <div className={styles.trackFeatureItem}>
                    <Check size={16} className={styles.checkIcon} />
                    <span>Notifikasi Real-time WhatsApp Panitia</span>
                  </div>
                </div>

                <div className={styles.trackQuotaSection}>
                  <div className={styles.trackQuotaMeta}>
                    <span>Terisi: {track.active_applicant_count} santri</span>
                    <span>Sisa: {Math.max(0, track.quota - track.active_applicant_count)} kuota</span>
                  </div>
                  <div className={styles.trackProgressBar}>
                    <div className={styles.trackProgressFill} style={{ width: `${pct}%` }} />
                  </div>
                </div>

                <div className={styles.trackPaymentMethods}>
                  <span className={styles.paymentMethodLabel}>Metode Resmi:</span>
                  <div className={styles.paymentPillsWrap}>
                    <span className={styles.payPill}>QRIS</span>
                    <span className={styles.payPill}>BSI VA</span>
                    <span className={styles.payPill}>Mandiri</span>
                    <span className={styles.payPill}>BRI</span>
                    <span className={styles.payPill}>BCA</span>
                    <span className={styles.payPill}>Manual</span>
                  </div>
                </div>

                <div className={styles.trackActionArea}>
                  <Link to={`/app/daftar?track=${track.id}`}>
                    <Button fullWidth size="lg">
                      <span>Pilih Jalur &amp; Mulai Daftar</span>
                      <ArrowRight size={16} />
                    </Button>
                  </Link>
                </div>
              </div>
            );
          })}
        </div>
      </section>

      {/* =========================================================================
          CHECKLIST PERSYARATAN BERKAS
          ========================================================================= */}
      <section id="syarat" className={`${styles.section} ${styles.sectionAlt}`}>
        <div className="container-public">
          <div className={styles.sectionHeader}>
            <span className={styles.sectionPill}>Kelengkapan Dokumen</span>
            <h2 className={styles.sectionTitle}>Persyaratan Berkas Pendaftaran</h2>
            <p className={styles.sectionSub}>
              Pastikan berkas telah disiapkan dalam bentuk file digital yang jelas (format PDF, JPG, atau PNG maksimal 2MB per dokumen).
            </p>
          </div>

          <div className={styles.reqCard}>
            <div className={styles.reqList}>
              <div className={styles.reqItem}>
                <div className={styles.reqIconWrap}>
                  <FileCheck size={22} />
                </div>
                <div>
                  <div className={styles.reqItemTitle}>Kartu Keluarga (KK)</div>
                  <div className={styles.reqItemSub}>
                    Foto/scan Kartu Keluarga terbaru yang memuat nama calon santri &amp; orang tua secara jelas.
                  </div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <div className={styles.reqIconWrap}>
                  <FileCheck size={22} />
                </div>
                <div>
                  <div className={styles.reqItemTitle}>Akta Kelahiran</div>
                  <div className={styles.reqItemSub}>
                    Foto/scan Akta Kelahiran resmi dari Dinas Kependudukan dan Pencatatan Sipil.
                  </div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <div className={styles.reqIconWrap}>
                  <FileCheck size={22} />
                </div>
                <div>
                  <div className={styles.reqItemTitle}>Pas Foto Formal Berwarna</div>
                  <div className={styles.reqItemSub}>
                    Ukuran 3x4 berlatar belakang merah atau biru, berpakaian kemeja/busana muslim sopan dan rapi.
                  </div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <div className={styles.reqIconWrap}>
                  <FileCheck size={22} />
                </div>
                <div>
                  <div className={styles.reqItemTitle}>Ijazah / SKL / Rapor Terakhir</div>
                  <div className={styles.reqItemSub}>
                    Surat Keterangan Lulus atau surat keterangan aktif dari kepala madrasah/sekolah asal jika ijazah belum terbit.
                  </div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <div className={styles.reqIconWrap}>
                  <FileCheck size={22} />
                </div>
                <div>
                  <div className={styles.reqItemTitle}>Surat Keterangan Sehat</div>
                  <div className={styles.reqItemSub}>
                    Surat dari dokter atau Puskesmas yang menyatakan santri sehat jasmani untuk belajar di asrama.
                  </div>
                </div>
              </div>

              <div className={styles.reqItem}>
                <div className={styles.reqIconWrap}>
                  <FileCheck size={22} />
                </div>
                <div>
                  <div className={styles.reqItemTitle}>Piagam Prestasi / Syahadah Tahfidz</div>
                  <div className={styles.reqItemSub}>
                    Khusus bagi pendaftar Jalur Prestasi atau Beasiswa Tahfidz Al-Qur'an minimal 3 Juz mutqin.
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* =========================================================================
          FAST STATUS CHECK WIDGET
          ========================================================================= */}
      <section id="cek-status" className={`container-public ${styles.section}`}>
        <div className={styles.statusCardWrap}>
          <div className={styles.statusCardHeader}>
            <span className={styles.sectionPill}>Layanan Mandiri</span>
            <h2 className={styles.statusCardTitle}>
              Cek Status Pendaftaran Cepat
            </h2>
            <p className={styles.statusCardSub}>
              Pantau progres verifikasi berkas, jadwal tes, dan pengumuman seleksi secara instan. Data pribadi disamarkan sesuai UU Pelindungan Data Pribadi (UU PDP).
            </p>
          </div>

          <form onSubmit={handleCheckStatus} className={styles.statusForm}>
            <div style={{ flex: 1 }}>
              <InputField
                label="Nomor Pendaftaran Resmi"
                placeholder="Contoh: SPMB-2601-0001"
                value={regNo}
                onChange={(e) => setRegNo(e.target.value)}
                error={statusError}
                mono
              />
            </div>
            <div className={styles.statusSubmitWrap}>
              <Button type="submit" size="lg" disabled={checking} fullWidth>
                <Search size={18} />
                <span>{checking ? 'Memeriksa Data...' : 'Periksa Status'}</span>
              </Button>
            </div>
          </form>

          {statusResult && (
            <div className={styles.statusResultCard}>
              <div className={styles.statusResultLeft}>
                <div className={styles.statusRegNo}>
                  {statusResult.registration_no}
                  <span className={styles.statusMaskedName}>· {statusResult.masked_name}</span>
                </div>
                <div className={styles.statusDetailMeta}>
                  Unit: <strong>{statusResult.unit_name}</strong> · Jalur: <strong>{statusResult.track_name}</strong>
                  {statusResult.rank_position ? ` · Peringkat #${statusResult.rank_position}` : ''}
                </div>
              </div>
              <div className={styles.statusResultRight}>
                <StatusBadge status={statusResult.status} />
                <Link to="/app">
                  <Button variant="secondary" size="sm">
                    Masuk Portal Wali
                  </Button>
                </Link>
              </div>
            </div>
          )}
        </div>
      </section>

      {/* =========================================================================
          FAQ ACCORDION SECTION
          ========================================================================= */}
      <section id="faq" className={`${styles.section} ${styles.sectionAlt}`}>
        <div className="container-public">
          <div className={styles.sectionHeader}>
            <span className={styles.sectionPill}>Tanya Jawab</span>
            <h2 className={styles.sectionTitle}>Pertanyaan yang Sering Diajukan</h2>
            <p className={styles.sectionSub}>
              Temukan jawaban cepat seputar mekanisme seleksi, syarat asrama, beasiswa tahfidz, dan biaya pendaftaran.
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

      {/* =========================================================================
          FLOATING WHATSAPP HELPDESK BUTTON
          ========================================================================= */}
      <a
        href={waHelpdeskUrl}
        target="_blank"
        rel="noopener noreferrer"
        className={styles.floatingWa}
        aria-label="Konsultasi Panitia PPDB via WhatsApp"
      >
        <MessageCircle size={22} />
        <span>Bantuan Panitia PPDB</span>
      </a>
    </div>
  );
}
