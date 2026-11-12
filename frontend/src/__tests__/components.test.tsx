import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writeStoredSession } from '@/core/api/client';
import { AuthProvider } from '@/core/auth/AuthContext';
import { PRESET_PORTALS } from '@/core/data/mockPortalData';
import { TerminologyProvider } from '@/core/terminology/terminology';
import type {
  ApplicationDetail,
  ExportJob,
  PlatformTenantView,
  PublicDocumentVerifyResult,
  StaffMemberView,
} from '@/core/types/domain';
import { ExportsAuditPanel } from '@/features/admin/ExportsAuditPanel';
import { StaffManagementPanel } from '@/features/admin/StaffManagementPanel';
import { ApplicantDashboardPage } from '@/features/applicant/ApplicantDashboardPage';
import { RegistrationWizardPage } from '@/features/applicant/RegistrationWizardPage';
import { PlatformAdminPage } from '@/features/platform/PlatformAdminPage';
import { PublicVerifyPage } from '@/features/public/PublicVerifyPage';

function makeJsonResponse(payload: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'Content-Type': 'application/json' }),
    json: async () => payload,
  } as unknown as Response;
}

function makeApplicationDetail(overrides: Partial<ApplicationDetail> = {}): ApplicationDetail {
  const base: ApplicationDetail = {
    application: {
      id: 'app-1',
      registration_no: 'SPMB-2026-0001',
      status: 'draft_form',
      unit_id: 'unit-sma',
      unit_name: 'SMA Harapan Nusantara',
      period_id: 'period-2026',
      period_name: 'Tahap Utama',
      academic_year: '2026/2027',
      track_id: 'trk-prestasi',
      track_name: 'Jalur Prestasi',
      student_name: 'Bima Pratama',
      nisn: '0098765432',
      distance_meters: 420,
      final_score: '91.50',
      rank_position: 3,
      version: 1,
      created_at: '2026-06-01T08:00:00Z',
    },
    guardian_user_id: 'user-wali-1',
    requires_coordinates: true,
    requires_achievements: true,
    registration_fee_idr: 0,
    re_enrollment_fee_idr: 500000,
    extra_answers: {},
    profile: {
      full_name: 'Bima Pratama',
      nik: '3175010101100001',
      nisn: '0098765432',
      birth_place: 'Jakarta',
      birth_date: '2011-05-14',
      citizenship: 'WNI',
      prev_school_name: 'SMPN 1 Jakarta',
    },
    guardians: [
      {
        relation: 'father',
        full_name: 'Hendra Pratama',
        phone_whatsapp: '+6281234567890',
        is_primary_contact: true,
      },
    ],
    achievements: [],
    requirements: [
      {
        id: 'req-kk',
        doc_code: 'kk',
        label: 'Kartu Keluarga',
        is_required: true,
        accepted_mimes: ['image/jpeg', 'application/pdf'],
        max_size_kb: 2048,
      },
    ],
    documents: [],
    payments: [],
    scores: [],
    audit_logs: [],
  };
  return {
    ...base,
    ...overrides,
    application: {
      ...base.application,
      ...(overrides.application ?? {}),
    },
  };
}

describe('PBDB frontend feature components', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
  });

  it('RegistrationWizardPage renders 6 steps and saves achievements via PUT /achievements', async () => {
    const user = userEvent.setup();
    writeStoredSession('demo-spmb', { accessToken: 'tok-wali', refreshToken: 'ref-wali' });

    const savedAchievements = [
      {
        id: 'ach-1',
        category: 'akademik',
        title: 'Juara 1 OSN Matematika',
        level: 'nasional',
        rank_achieved: 'Juara 1',
        achievement_year: 2025,
        organizer: 'Kemendikbud',
        raw_points: 10,
      },
    ];

    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      makeJsonResponse({ data: { achievements: savedAchievements } }),
    );

    const onUpdated = vi.fn();
    const onSaveTick = vi.fn();

    render(
      <MemoryRouter>
        <TerminologyProvider preset="spmb">
          <RegistrationWizardPage
            tenantSlug="demo-spmb"
            portal={PRESET_PORTALS.spmb}
            activeApp={makeApplicationDetail()}
            onSaveTick={onSaveTick}
            onApplicationUpdated={onUpdated}
          />
        </TerminologyProvider>
      </MemoryRouter>,
    );

    expect(screen.getAllByText('Prestasi & Piagam').length).toBeGreaterThan(0);

    await user.click(screen.getByRole('button', { name: /Prestasi & Piagam/i }));
    expect(screen.getByText('Daftar Prestasi & Piagam')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Tambah Prestasi/i }));
    await user.type(screen.getByLabelText(/Nama Kejuaraan \/ Capaian/i), 'Juara 1 OSN Matematika');
    await user.click(screen.getByRole('button', { name: /Simpan & Lanjut/i }));

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledTimes(1);
    });

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/v1/applicant/applications/app-1/achievements');
    expect(init?.method).toBe('PUT');
    expect(JSON.parse(String(init?.body))).toMatchObject({
      achievements: [
        expect.objectContaining({
          category: 'akademik',
          title: 'Juara 1 OSN Matematika',
        }),
      ],
    });
    expect(onUpdated).toHaveBeenCalledWith(
      expect.objectContaining({ achievements: savedAchievements }),
    );
  });

  it('ApplicantDashboardPage renders Kartu Pendaftaran and Surat Keterangan Diterima modals', async () => {
    const user = userEvent.setup();
    vi.spyOn(window, 'print').mockImplementation(() => {});

    const acceptedApp = makeApplicationDetail({
      application: {
        ...makeApplicationDetail().application,
        status: 'accepted',
      },
    });

    render(
      <MemoryRouter>
        <TerminologyProvider preset="spmb">
          <ApplicantDashboardPage
            tenantSlug="demo-spmb"
            portal={PRESET_PORTALS.spmb}
            activeApp={acceptedApp}
            loading={false}
            createOpen={false}
            onOpenCreate={() => {}}
            onCloseCreate={() => {}}
            onApplicationUpdated={() => {}}
            onApplicationCreated={() => {}}
          />
        </TerminologyProvider>
      </MemoryRouter>,
    );

    await user.click(screen.getByRole('button', { name: /Cetak Kartu Pendaftaran/i }));
    let dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText(/Kartu Tanda Pendaftaran SPMB/i)).toBeInTheDocument();
    expect(within(dialog).getAllByText('SPMB-2026-0001').length).toBeGreaterThanOrEqual(1);

    await user.click(within(dialog).getByRole('button', { name: 'Tutup' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Cetak Surat Keterangan Diterima/i }));
    dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText(/Surat Keterangan Hasil Seleksi — Diterima/i)).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: /Cetak \/ Simpan PDF/i }));
    expect(window.print).toHaveBeenCalledTimes(1);
  });

  it('ExportsAuditPanel triggers POST /exports and downloads completed files', async () => {
    const user = userEvent.setup();
    writeStoredSession('demo-spmb', { accessToken: 'tok-admin', refreshToken: 'ref-admin' });

    const job: ExportJob = {
      id: 'exp-1',
      unit_id: 'unit-sma',
      unit_name: 'SMA Harapan Nusantara',
      period_id: 'period-2026',
      period_name: 'Tahap Utama',
      requested_by_user_id: 'u-admin-1',
      format: 'dapodik_xlsx',
      status: 'completed',
      row_count: 18,
      filename: 'dapodik-2026.xlsx',
      download_url: '/api/v1/admin/exports/exp-1/download',
      created_at: '2026-06-15T10:00:00Z',
    };

    const jobsQueue: ExportJob[] = [];
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url.startsWith('/api/v1/admin/exports?') && method === 'GET') {
        return makeJsonResponse({ data: [...jobsQueue] });
      }
      if (url.startsWith('/api/v1/admin/audit-logs?') && method === 'GET') {
        return makeJsonResponse({ data: [] });
      }
      if (url === '/api/v1/admin/exports' && method === 'POST') {
        jobsQueue.splice(0, jobsQueue.length, job);
        return makeJsonResponse({ data: job }, 201);
      }
      if (url === '/api/v1/admin/exports/exp-1/download' && method === 'GET') {
        return {
          ok: true,
          status: 200,
          headers: new Headers({
            'Content-Disposition': 'attachment; filename="dapodik-2026.xlsx"',
          }),
          blob: async () => new Blob(['PK\x03\x04']),
        } as unknown as Response;
      }
      return makeJsonResponse({ error: { message: `Unhandled ${method} ${url}` } }, 404);
    });

    const createObjectURL = vi.fn(() => 'blob:export-url');
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() });
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

    render(
      <MemoryRouter>
        <ExportsAuditPanel
          tenantSlug="demo-spmb"
          portal={PRESET_PORTALS.spmb}
          activeUnitId="unit-sma"
        />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText(/Belum ada riwayat ekspor untuk unit ini/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /Buat Ekspor/i }));

    await waitFor(() => {
      expect(screen.getByText(/siap diunduh \(18 baris\)/i)).toBeInTheDocument();
    });
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/v1/admin/exports',
      expect.objectContaining({ method: 'POST' }),
    );

    await user.click(screen.getByRole('button', { name: /Unduh/i }));

    await waitFor(() => {
      expect(clickSpy).toHaveBeenCalledTimes(1);
    });
  });

  it('PlatformAdminPage authenticates superadmin, lists tenants, and onboards a new institution', async () => {
    const user = userEvent.setup();

    const tenantList: PlatformTenantView[] = [
      {
        id: 't-1',
        slug: 'demo-spmb',
        name: 'SMA Harapan Nusantara',
        institution_type: 'school',
        terminology_preset: 'spmb',
        theme_config: { primary_color: '#1D4ED8', tagline: 'Portal SPMB' },
        domain_verified: false,
        subscription_plan: 'pro',
        is_active: true,
        unit_count: 1,
        created_at: '2026-06-01T00:00:00Z',
      },
    ];

    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/v1/platform/auth/login' && method === 'POST') {
        return makeJsonResponse({
          data: {
            access_token: 'pf-token-1',
            refresh_token: 'pf-ref-1',
            expires_in: 900,
            user: {
              id: 'u-platform',
              full_name: 'Superadmin SaaS',
              email: 'platform@spmb-platform.id',
              is_platform_admin: true,
              roles: [],
            },
          },
        });
      }
      if (url === '/api/v1/platform/tenants' && method === 'GET') {
        return makeJsonResponse({ data: [...tenantList] });
      }
      if (url === '/api/v1/platform/tenants' && method === 'POST') {
        const body = JSON.parse(String(init?.body)) as {
          slug: string;
          name: string;
          institution_type: PlatformTenantView['institution_type'];
          terminology_preset: PlatformTenantView['terminology_preset'];
          subscription_plan: string;
          primary_color: string;
          tagline: string;
          admin_email?: string;
          admin_full_name?: string;
        };
        const created: PlatformTenantView = {
          id: 't-2',
          slug: body.slug,
          name: body.name,
          institution_type: body.institution_type,
          terminology_preset: body.terminology_preset,
          theme_config: { primary_color: body.primary_color, tagline: body.tagline },
          domain_verified: false,
          subscription_plan: body.subscription_plan,
          is_active: true,
          unit_count: 1,
          created_at: '2026-06-16T09:00:00Z',
        };
        tenantList.push(created);
        return makeJsonResponse(
          {
            data: {
              tenant: created,
              initial_admin: {
                id: 'u-admin-2',
                email: body.admin_email ?? 'admin@yayasan.id',
                full_name: body.admin_full_name ?? 'Admin Yayasan',
              },
            },
          },
          201,
        );
      }
      return makeJsonResponse({ error: { message: `Unhandled ${method} ${url}` } }, 404);
    });

    render(
      <MemoryRouter>
        <PlatformAdminPage />
      </MemoryRouter>,
    );

    expect(screen.getByText(/Masuk Superadmin Platform/i)).toBeInTheDocument();

    await user.type(screen.getByLabelText(/Email Superadmin/i), 'platform@spmb-platform.id');
    await user.type(screen.getByLabelText(/Password/i), 'DemoPass123!');
    await user.click(screen.getByRole('button', { name: /Masuk ke Konsol Platform/i }));

    await waitFor(() => {
      expect(screen.getByText('SMA Harapan Nusantara')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /Daftarkan Instansi Baru/i }));
    const dialog = screen.getByRole('dialog');
    await user.type(within(dialog).getByLabelText(/Nama Lengkap Instansi/i), 'Yayasan Al-Falah');
    expect(within(dialog).getByLabelText(/Slug Subdomain/i)).toHaveValue('yayasan-al-falah');

    await user.type(within(dialog).getByLabelText(/Nama Unit Awal/i), 'SMP Islam Al-Falah');
    await user.type(within(dialog).getByLabelText(/Nama Lengkap Admin/i), 'Admin Al-Falah');
    await user.type(within(dialog).getByLabelText(/Email Admin/i), 'admin@alfalah.sch.id');
    await user.type(within(dialog).getByLabelText(/Password Awal Admin/i), 'Rahasia123');

    await user.click(within(dialog).getByRole('button', { name: /Buat Instansi/i }));

    await waitFor(() => {
      expect(screen.getByText('Yayasan Al-Falah')).toBeInTheDocument();
    });
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/v1/platform/tenants',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('PublicVerifyPage verifies QR token against /api/v1/public/verify-document', async () => {
    const user = userEvent.setup();
    const verifyPayload: PublicDocumentVerifyResult = {
      valid: true,
      document_type: 'Surat Keterangan Diterima',
      registration_no: 'SPMB-2026-0001',
      masked_name: 'Bi*** Pr*****',
      tenant_name: 'SMA Harapan Nusantara',
      unit_name: 'SMA Harapan Nusantara',
      track_name: 'Jalur Prestasi',
      academic_year: '2026/2027',
      status: 'accepted',
      final_score: 91.5,
      verified_at: '2026-06-16T10:00:00Z',
      message: 'Tanda tangan digital dokumen sah dan cocok dengan catatan server.',
    };

    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(makeJsonResponse({ data: verifyPayload }));

    render(
      <MemoryRouter initialEntries={['/verifikasi']}>
        <PublicVerifyPage portal={PRESET_PORTALS.spmb} tenantSlug="demo-spmb" />
      </MemoryRouter>,
    );

    await user.type(screen.getByLabelText(/Token Verifikasi QR/i), 'tok-qr-123');
    await user.click(screen.getByRole('button', { name: /Verifikasi Token/i }));

    await waitFor(() => {
      expect(screen.getByText('Dokumen Resmi Terverifikasi')).toBeInTheDocument();
    });
    expect(screen.getByText('SPMB-2026-0001')).toBeInTheDocument();
    expect(screen.getByText('91.50')).toBeInTheDocument();
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/v1/public/verify-document?token=tok-qr-123',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('StaffManagementPanel lists staff and submits new RBAC account via POST /admin/staff', async () => {
    const user = userEvent.setup();
    writeStoredSession('demo-spmb', { accessToken: 'tok-tadmin', refreshToken: 'ref-tadmin' });

    const staffList: StaffMemberView[] = [
      {
        user_id: 'u-staff-1',
        full_name: 'Admin Utama',
        email: 'admin@sekolah.sch.id',
        whatsapp_phone: '+628111111111',
        is_platform_admin: false,
        roles: [
          {
            id: 'r-1',
            role: 'tenant_admin',
            unit_id: null,
            unit_name: null,
            is_active: true,
          },
        ],
        created_at: '2026-06-01T00:00:00Z',
      },
    ];

    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/v1/me' && method === 'GET') {
        return makeJsonResponse({
          data: {
            id: 'u-tadmin',
            full_name: 'Admin Utama',
            email: 'admin@sekolah.sch.id',
            is_platform_admin: false,
            roles: [{ role: 'tenant_admin', unit_id: null, is_active: true }],
          },
        });
      }
      if (url === '/api/v1/admin/staff' && method === 'GET') {
        return makeJsonResponse({ data: [...staffList] });
      }
      if (url === '/api/v1/admin/staff' && method === 'POST') {
        const body = JSON.parse(String(init?.body)) as {
          full_name: string;
          email: string;
          role: 'verifier';
        };
        const created: StaffMemberView = {
          user_id: 'u-staff-2',
          full_name: body.full_name,
          email: body.email,
          is_platform_admin: false,
          roles: [{ id: 'r-2', role: body.role, is_active: true }],
          created_at: '2026-06-16T11:00:00Z',
        };
        staffList.push(created);
        return makeJsonResponse({ data: created }, 201);
      }
      return makeJsonResponse({ error: { message: `Unhandled ${method} ${url}` } }, 404);
    });

    render(
      <MemoryRouter>
        <AuthProvider tenantSlug="demo-spmb">
          <StaffManagementPanel tenantSlug="demo-spmb" units={PRESET_PORTALS.spmb.units} />
        </AuthProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText('Admin Utama')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /Tambah Staf Baru/i }));
    const dialog = screen.getByRole('dialog');
    await user.type(within(dialog).getByLabelText(/Nama Lengkap/i), 'Verifikator Berkas Satu');
    await user.type(within(dialog).getByLabelText(/Email Kerja/i), 'verif@sekolah.sch.id');
    await user.type(within(dialog).getByLabelText(/Kata Sandi Awal/i), 'RahasiaPanitia88');
    await user.click(within(dialog).getByRole('button', { name: /Simpan Staf/i }));

    await waitFor(() => {
      expect(screen.getByText('Verifikator Berkas Satu')).toBeInTheDocument();
    });
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/v1/admin/staff',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('ApplicantDashboardPage renders assessment schedules, QR code on print sheet, and Cek Status Midtrans sync', async () => {
    const user = userEvent.setup();
    writeStoredSession('demo-spmb', { accessToken: 'tok-wali', refreshToken: 'ref-wali' });

    const scheduledApp = makeApplicationDetail({
      verification_token: 'v1.app-1.sig-demo',
      schedules: [
        {
          id: 'sch-1',
          application_id: 'app-1',
          registration_no: 'SPMB-2026-0001',
          unit_id: 'unit-sma',
          session_name: 'Wawancara Akademik Gelombang 1',
          scheduled_at: '2026-06-22T09:00:00Z',
          room_or_link: 'Ruang A-101',
          examiner_name: 'Dr. Budi Santoso',
          created_at: '2026-06-15T00:00:00Z',
          updated_at: '2026-06-15T00:00:00Z',
        },
      ],
      payments: [
        {
          id: 'pay-1',
          application_id: 'app-1',
          purpose: 'registration_fee',
          method: 'va',
          provider: 'midtrans_snap',
          status: 'pending',
          amount_idr: 250000,
          external_ref: 'SPMB-2026-0001-1',
          created_at: '2026-06-15T00:00:00Z',
        },
      ],
    });

    const paidSyncResponse: ApplicationDetail = {
      ...scheduledApp,
      payments: [
        {
          ...scheduledApp.payments[0],
          status: 'paid',
        },
      ],
    };

    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      makeJsonResponse({
        data: {
          application: paidSyncResponse,
          payment: paidSyncResponse.payments[0],
        },
      }),
    );

    const onUpdated = vi.fn();
    render(
      <MemoryRouter>
        <TerminologyProvider preset="spmb">
          <ApplicantDashboardPage
            tenantSlug="demo-spmb"
            portal={PRESET_PORTALS.spmb}
            activeApp={scheduledApp}
            loading={false}
            createOpen={false}
            onOpenCreate={() => {}}
            onCloseCreate={() => {}}
            onApplicationUpdated={onUpdated}
            onApplicationCreated={() => {}}
          />
        </TerminologyProvider>
      </MemoryRouter>,
    );

    expect(screen.getByText('Jadwal Tes & Wawancara Seleksi')).toBeInTheDocument();
    expect(screen.getByText('Wawancara Akademik Gelombang 1')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Cek Status/i }));
    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/v1/applicant/applications/app-1/payments/pay-1/sync',
        expect.objectContaining({ method: 'POST' }),
      );
    });
    expect(onUpdated).toHaveBeenCalledWith(
      expect.objectContaining({ payments: paidSyncResponse.payments }),
    );

    await user.click(screen.getByRole('button', { name: /Cetak Kartu Pendaftaran/i }));
    const dialog = screen.getByRole('dialog');
    expect(
      within(dialog).getByRole('img', { name: /QR Verifikasi SPMB-2026-0001/i }),
    ).toBeInTheDocument();
  });

  it('RegistrationWizardPage renders custom_form_schema fields and sends extra_answers on profile save', async () => {
    const user = userEvent.setup();
    writeStoredSession('demo-spmb', { accessToken: 'tok-wali', refreshToken: 'ref-wali' });

    const appWithCustomForm = makeApplicationDetail({
      custom_form_schema: [
        {
          key: 'ukuran_seragam',
          label: 'Ukuran Seragam',
          type: 'select',
          required: true,
          options: ['M', 'L', 'XL'],
        },
        {
          key: 'hafalan_juz',
          label: 'Jumlah Hafalan Juz',
          type: 'text',
          required: false,
        },
      ],
    });

    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      makeJsonResponse({
        data: {
          ...appWithCustomForm,
          extra_answers: { ukuran_seragam: 'L', hafalan_juz: '3 Juz' },
        },
      }),
    );

    const onUpdated = vi.fn();
    render(
      <MemoryRouter>
        <TerminologyProvider preset="spmb">
          <RegistrationWizardPage
            tenantSlug="demo-spmb"
            portal={PRESET_PORTALS.spmb}
            activeApp={appWithCustomForm}
            onSaveTick={() => {}}
            onApplicationUpdated={onUpdated}
          />
        </TerminologyProvider>
      </MemoryRouter>,
    );

    await user.selectOptions(screen.getByLabelText(/Ukuran Seragam/i), 'L');
    await user.type(screen.getByLabelText(/Jumlah Hafalan Juz/i), '3 Juz');
    await user.click(screen.getByRole('button', { name: /Simpan & Lanjut/i }));

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledTimes(1);
    });

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/v1/applicant/applications/app-1/profile');
    expect(init?.method).toBe('PATCH');
    expect(JSON.parse(String(init?.body))).toMatchObject({
      extra_answers: {
        ukuran_seragam: 'L',
        hafalan_juz: '3 Juz',
      },
    });
  });
});
