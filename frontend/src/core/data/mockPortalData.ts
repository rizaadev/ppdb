import type {
  ChildApplicationPreview,
  PublicPortalPayload,
  TerminologyPreset,
} from '@/core/types/domain';

export const PRESET_PORTALS: Record<TerminologyPreset, PublicPortalPayload> = {
  spmb: {
    tenant: {
      id: '01960000-0000-7000-8000-000000000001',
      slug: 'demo-spmb',
      name: 'SMA Harapan Nusantara',
      institution_type: 'school',
      terminology_preset: 'spmb',
      terminology_overrides: {},
      theme_config: {
        primary_color: '#1D4ED8',
        tagline: 'Portal Resmi Sistem Penerimaan Murid Baru (SPMB) TA 2026/2027',
      },
    },
    units: [
      {
        id: 'unit-sma',
        code: 'SMA',
        name: 'SMA Harapan Nusantara',
        education_level: 'sma_ma',
        address: 'Jl. Pendidikan Raya No. 10, Jakarta Timur',
        latitude: -6.2415,
        longitude: 106.8832,
      },
    ],
    periods: [
      {
        id: 'period-2026',
        unit_id: 'unit-sma',
        academic_year: '2026/2027',
        name: 'Tahap Utama',
        opens_at: '2026-06-01T00:00:00Z',
        closes_at: '2026-06-24T16:59:00Z',
        announcement_at: '2026-06-28T03:00:00Z',
        tracks: [
          {
            id: 'trk-domisili',
            code: 'domisili',
            display_name: 'Jalur Domisili',
            description: 'Seleksi berbasis jarak domisili ke sekolah dengan KK minimal terbit 1 tahun.',
            selection_mode: 'rule_based',
            quota: 140,
            active_applicant_count: 112,
            registration_fee_idr: 0,
            requires_coordinates: true,
            requires_achievements: false,
          },
          {
            id: 'trk-afirmasi',
            code: 'afirmasi',
            display_name: 'Jalur Afirmasi',
            description: 'Kuota khusus keluarga ekonomi tidak mampu (KIP/PKH) dan penyandang disabilitas.',
            selection_mode: 'rule_based',
            quota: 60,
            active_applicant_count: 42,
            registration_fee_idr: 0,
            requires_coordinates: true,
            requires_achievements: false,
          },
          {
            id: 'trk-prestasi',
            code: 'prestasi',
            display_name: 'Jalur Prestasi',
            description: 'Seleksi rapor dan capaian kejuaraan akademik maupun non-akademik.',
            selection_mode: 'rule_based',
            quota: 80,
            active_applicant_count: 68,
            registration_fee_idr: 0,
            requires_coordinates: false,
            requires_achievements: true,
          },
        ],
      },
    ],
  },
  pmbm: {
    tenant: {
      id: '01960000-0000-7000-8000-000000000002',
      slug: 'demo-pmbm',
      name: 'Madrasah Aliyah Al-Hamid',
      institution_type: 'madrasah',
      terminology_preset: 'pmbm',
      terminology_overrides: {},
      theme_config: {
        primary_color: '#047857',
        tagline: 'Portal Resmi Penerimaan Murid Baru Madrasah (PMBM) TA 2026/2027',
      },
    },
    units: [
      {
        id: 'unit-ma',
        code: 'MA',
        name: 'Madrasah Aliyah Al-Hamid',
        education_level: 'sma_ma',
        address: 'Jl. Cilangkap Baru No. 1, Cipayung, Jakarta Timur',
        latitude: -6.3228,
        longitude: 106.8994,
      },
    ],
    periods: [
      {
        id: 'period-pmbm-2026',
        unit_id: 'unit-ma',
        academic_year: '2026/2027',
        name: 'Gelombang 1',
        opens_at: '2026-02-01T00:00:00Z',
        closes_at: '2026-04-30T16:59:00Z',
        announcement_at: '2026-05-08T03:00:00Z',
        tracks: [
          {
            id: 'trk-reguler',
            code: 'reguler',
            display_name: 'Jalur Reguler Madrasah',
            description: 'Seleksi administrasi, tes literasi akademik, serta baca tulis Al-Quran (BTQ).',
            selection_mode: 'hybrid',
            quota: 120,
            active_applicant_count: 84,
            registration_fee_idr: 150000,
            requires_coordinates: false,
            requires_achievements: false,
          },
          {
            id: 'trk-tahfidz',
            code: 'prestasi',
            display_name: 'Jalur Prestasi & Tahfidz',
            description: 'Jalur khusus hafidz Al-Quran serta peraih juara KSM/kompetisi resmi.',
            selection_mode: 'hybrid',
            quota: 40,
            active_applicant_count: 31,
            registration_fee_idr: 0,
            requires_coordinates: false,
            requires_achievements: true,
          },
        ],
      },
    ],
  },
  mandiri: {
    tenant: {
      id: '01960000-0000-7000-8000-000000000003',
      slug: 'demo-mandiri',
      name: 'Yayasan Pendidikan & Pesantren Al-Ikhlas',
      institution_type: 'pesantren',
      terminology_preset: 'mandiri',
      terminology_overrides: {},
      theme_config: {
        primary_color: '#4338CA',
        tagline: 'Portal Pendaftaran Terpadu SDIT, SMPIT, SMAIT & Pondok Pesantren',
      },
    },
    units: [
      {
        id: 'unit-smpit',
        code: 'SMPIT',
        name: 'SMPIT & Pondok Putra/Putri',
        education_level: 'smp_mts',
        address: 'Jl. Raya Pendidikan KM 4, Depok',
        latitude: -6.3982,
        longitude: 106.8247,
      },
      {
        id: 'unit-smait',
        code: 'SMAIT',
        name: 'SMAIT Boarding School',
        education_level: 'sma_ma',
        address: 'Jl. Raya Pendidikan KM 4, Depok',
        latitude: -6.3982,
        longitude: 106.8247,
      },
    ],
    periods: [
      {
        id: 'period-gel1',
        unit_id: 'unit-smpit',
        academic_year: '2026/2027',
        name: 'Gelombang 1 (Early Bird)',
        opens_at: '2025-11-01T00:00:00Z',
        closes_at: '2026-01-31T16:59:00Z',
        announcement_at: '2026-02-10T03:00:00Z',
        tracks: [
          {
            id: 'trk-asrama',
            code: 'early_bird',
            display_name: 'Program Reguler & Asrama',
            description: 'Mencakup tes masuk akademik, wawancara orang tua, serta penempatan asrama.',
            selection_mode: 'test_based',
            quota: 150,
            active_applicant_count: 92,
            registration_fee_idr: 250000,
            requires_coordinates: false,
            requires_achievements: false,
          },
          {
            id: 'trk-beasiswa',
            code: 'beasiswa',
            display_name: 'Jalur Beasiswa Yatim & Prestasi',
            description: 'Bebas biaya formulir dengan verifikasi berkas sosial atau piagam kejuaraan.',
            selection_mode: 'hybrid',
            quota: 30,
            active_applicant_count: 19,
            registration_fee_idr: 0,
            requires_coordinates: false,
            requires_achievements: true,
          },
        ],
      },
    ],
  },
};

const SAMPLE_KK_SVG =
  'data:image/svg+xml;utf8,' +
  encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="800" height="520" viewBox="0 0 800 520">
      <rect width="800" height="520" fill="#f8fafc"/>
      <rect x="24" y="24" width="752" height="472" rx="12" fill="#ffffff" stroke="#cbd5e1" stroke-width="2"/>
      <rect x="48" y="48" width="704" height="56" rx="6" fill="#e2e8f0"/>
      <text x="400" y="83" text-anchor="middle" font-family="sans-serif" font-size="22" font-weight="bold" fill="#0f172a">KARTU KELUARGA (PRATINJAU DOKUMEN CONTOH)</text>
      <text x="64" y="145" font-family="monospace" font-size="16" fill="#334155">No. KK: 3175011203090004</text>
      <text x="64" y="175" font-family="sans-serif" font-size="16" fill="#334155">Kepala Keluarga: H. Ahmad Fauzan</text>
      <text x="64" y="205" font-family="sans-serif" font-size="15" fill="#475569">Alamat: Jl. Cipayung Jaya RT 04 / RW 02, Jakarta Timur</text>
      <rect x="64" y="235" width="672" height="170" rx="6" fill="#f1f5f9" stroke="#cbd5e1"/>
      <text x="84" y="275" font-family="monospace" font-size="15" fill="#0f172a">1. Ahmad Fauzan (Ayah) - 3175010101800002</text>
      <text x="84" y="310" font-family="monospace" font-size="15" fill="#0f172a">2. Siti Rahmawati (Ibu) - 3175010506830007</text>
      <text x="84" y="345" font-family="monospace" font-size="15" fill="#0f172a">3. Zaidan Al-Farizi (Anak) - 3175011408110011</text>
      <text x="530" y="455" font-family="monospace" font-size="14" fill="#047857">Diterbitkan: 12-02-2023</text>
    </svg>`,
  );

const SAMPLE_AKTA_SVG =
  'data:image/svg+xml;utf8,' +
  encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="600" height="760" viewBox="0 0 600 760">
      <rect width="600" height="760" fill="#f8fafc"/>
      <rect x="28" y="28" width="544" height="704" rx="12" fill="#ffffff" stroke="#cbd5e1" stroke-width="2"/>
      <text x="300" y="96" text-anchor="middle" font-family="sans-serif" font-size="22" font-weight="bold" fill="#0f172a">KUTIPAN AKTA KELAHIRAN</text>
      <text x="300" y="128" text-anchor="middle" font-family="monospace" font-size="15" fill="#475569">Nomor: 3175-LU-18082011-0042</text>
      <line x1="64" y1="160" x2="536" y2="160" stroke="#e2e8f0" stroke-width="2"/>
      <text x="64" y="220" font-family="sans-serif" font-size="17" fill="#0f172a">Nama: Zaidan Al-Farizi</text>
      <text x="64" y="260" font-family="sans-serif" font-size="16" fill="#334155">Tempat, Tanggal Lahir: Jakarta, 14 Agustus 2011</text>
      <text x="64" y="300" font-family="sans-serif" font-size="16" fill="#334155">Anak ke-2 dari pasangan Ahmad Fauzan &amp; Siti Rahmawati</text>
    </svg>`,
  );

export const MOCK_CHILD_APPLICATIONS: ChildApplicationPreview[] = [
  {
    id: 'app-001',
    registration_no: 'SPMB-2601-0001',
    student_name: 'Zaidan Al-Farizi',
    unit_name: 'SMA Harapan Nusantara',
    track_name: 'Jalur Domisili',
    status: 'revision_required',
    submitted_at: '2026-06-03T07:15:00Z',
    distance_meters: 840,
    documents: [
      {
        id: 'doc-kk',
        doc_code: 'kk',
        label: 'Kartu Keluarga (KK)',
        file_name: 'kk-keluarga-fauzan.jpg',
        size_kb: 420,
        status: 'rejected_for_revision',
        review_note: 'Tanggal terbit di pojok kanan bawah kartu keluarga sedikit terpotong. Mohon unggah ulang foto penuh.',
        preview_url: SAMPLE_KK_SVG,
      },
      {
        id: 'doc-akta',
        doc_code: 'akta_lahir',
        label: 'Akta Kelahiran',
        file_name: 'akta-zaidan.jpg',
        size_kb: 380,
        status: 'approved',
        preview_url: SAMPLE_AKTA_SVG,
      },
    ],
  },
  {
    id: 'app-002',
    registration_no: 'PMBM-2601-0018',
    student_name: 'Nadia Azzahra',
    unit_name: 'Madrasah Aliyah Al-Hamid',
    track_name: 'Jalur Prestasi & Tahfidz',
    status: 'under_verification',
    submitted_at: '2026-03-11T09:30:00Z',
    documents: [
      {
        id: 'doc-kk-2',
        doc_code: 'kk',
        label: 'Kartu Keluarga (KK)',
        file_name: 'kk-nadia.jpg',
        size_kb: 510,
        status: 'pending',
        preview_url: SAMPLE_KK_SVG,
      },
    ],
  },
];

