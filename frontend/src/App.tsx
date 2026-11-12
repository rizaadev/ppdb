import { lazy, Suspense, useCallback, useEffect, useState } from 'react';
import {
  BrowserRouter,
  Link,
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from 'react-router-dom';
import { apiRequest } from '@/core/api/client';
import { AuthProvider, useAuth } from '@/core/auth/AuthContext';
import { TerminologyProvider } from '@/core/terminology/terminology';
import { TenantThemeProvider } from '@/core/theme/TenantThemeProvider';
import type {
  ApplicationDetail,
  ApplicationSummary,
  PublicPortalPayload,
  TerminologyPreset,
} from '@/core/types/domain';
import { AdminShell, type AdminTabId } from '@/layouts/AdminShell';
import { ApplicantShell } from '@/layouts/ApplicantShell';
import { PublicPortalLayout } from '@/layouts/PublicPortalLayout';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';

const ENABLE_DEV_PREVIEW =
  import.meta.env.DEV && import.meta.env.VITE_ENABLE_PREVIEW_MOCK === 'true';

const PreviewToolbar = ENABLE_DEV_PREVIEW
  ? lazy(() =>
      import('@/components/dev/PreviewToolbar').then((m) => ({
        default: m.PreviewToolbar,
      })),
    )
  : null;

const PublicLandingPage = lazy(() =>
  import('@/features/public/PublicLandingPage').then((m) => ({ default: m.PublicLandingPage })),
);
const PublicVerifyPage = lazy(() =>
  import('@/features/public/PublicVerifyPage').then((m) => ({ default: m.PublicVerifyPage })),
);
const AuthPage = lazy(() =>
  import('@/features/auth/AuthPage').then((m) => ({ default: m.AuthPage })),
);
const ApplicantDashboardPage = lazy(() =>
  import('@/features/applicant/ApplicantDashboardPage').then((m) => ({
    default: m.ApplicantDashboardPage,
  })),
);
const RegistrationWizardPage = lazy(() =>
  import('@/features/applicant/RegistrationWizardPage').then((m) => ({
    default: m.RegistrationWizardPage,
  })),
);
const VerificationWorkbenchPage = lazy(() =>
  import('@/features/admin/VerificationWorkbenchPage').then((m) => ({
    default: m.VerificationWorkbenchPage,
  })),
);
const AdminDashboardOverview = lazy(() =>
  import('@/features/admin/AdminDashboardOverview').then((m) => ({
    default: m.AdminDashboardOverview,
  })),
);
const FinancePaymentsPanel = lazy(() =>
  import('@/features/admin/FinancePaymentsPanel').then((m) => ({
    default: m.FinancePaymentsPanel,
  })),
);
const SelectionRankingPanel = lazy(() =>
  import('@/features/admin/SelectionRankingPanel').then((m) => ({
    default: m.SelectionRankingPanel,
  })),
);
const ExportsAuditPanel = lazy(() =>
  import('@/features/admin/ExportsAuditPanel').then((m) => ({
    default: m.ExportsAuditPanel,
  })),
);
const StaffManagementPanel = lazy(() =>
  import('@/features/admin/StaffManagementPanel').then((m) => ({
    default: m.StaffManagementPanel,
  })),
);
const TenantSettingsPanel = lazy(() =>
  import('@/features/admin/TenantSettingsPanel').then((m) => ({
    default: m.TenantSettingsPanel,
  })),
);
const PlatformAdminPage = lazy(() =>
  import('@/features/platform/PlatformAdminPage').then((m) => ({
    default: m.PlatformAdminPage,
  })),
);

const PRESET_TO_SLUG: Record<TerminologyPreset, string> = {
  spmb: 'demo-spmb',
  pmbm: 'demo-pmbm',
  mandiri: 'demo-mandiri',
};

function isLoopbackHost(): boolean {
  if (typeof window === 'undefined') return true;
  const host = window.location.hostname.toLowerCase();
  return host === '' || host === 'localhost' || host === '127.0.0.1' || host === '[::1]';
}

function initialPresetFromQuery(): TerminologyPreset {
  if (typeof window === 'undefined') return 'spmb';
  const raw = new URLSearchParams(window.location.search).get('preset');
  if (raw === 'pmbm' || raw === 'mandiri' || raw === 'spmb') {
    return raw;
  }
  return 'spmb';
}

function CenterMessageCard({
  title,
  message,
  children,
}: {
  title: string;
  message: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="container-public" style={{ padding: 'var(--space-8) var(--space-4)' }}>
      <Card style={{ maxWidth: 520, margin: '0 auto', textAlign: 'center' }}>
        <h1 style={{ fontSize: 'var(--text-h2)', fontWeight: 700, marginBottom: 'var(--space-2)' }}>
          {title}
        </h1>
        <p style={{ fontSize: 'var(--text-label)', color: 'var(--text-secondary)', lineHeight: 1.55 }}>
          {message}
        </p>
        {children && (
          <div style={{ marginTop: 'var(--space-4)', display: 'flex', justifyContent: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
            {children}
          </div>
        )}
      </Card>
    </div>
  );
}

function AppRoutes({
  portal,
  tenantSlug,
  preset,
  showDevBar,
  onChangePreset,
  onReloadPortal,
}: {
  portal: PublicPortalPayload;
  tenantSlug: string;
  preset: TerminologyPreset;
  showDevBar: boolean;
  onChangePreset: (next: TerminologyPreset) => void;
  onReloadPortal: () => void;
}) {
  const auth = useAuth();
  const location = useLocation();
  const navigate = useNavigate();

  const [childrenApps, setChildrenApps] = useState<ApplicationSummary[]>([]);
  const [activeChildId, setActiveChildId] = useState('');
  const [activeApp, setActiveApp] = useState<ApplicationDetail | null>(null);
  const [appsLoading, setAppsLoading] = useState(false);
  const [autoSaveText, setAutoSaveText] = useState('Tersimpan otomatis');
  const [createOpen, setCreateOpen] = useState(false);

  const [activeUnitId, setActiveUnitId] = useState(portal.units[0]?.id ?? '');
  const [activeAdminTab, setActiveAdminTab] = useState<AdminTabId>('overview');

  const resolvedUnitId = (() => {
    if (portal.units.some((u) => u.id === activeUnitId)) {
      return activeUnitId;
    }
    const scopedUnit = portal.units.find((u) => auth.hasUnitAccess(u.id));
    return scopedUnit?.id ?? portal.units[0]?.id ?? '';
  })();

  useEffect(() => {
    if (!auth.user) return;
    if (auth.hasAnyRole('tenant_admin')) {
      setActiveAdminTab('overview');
    } else if (auth.hasAnyRole('verifier', 'unit_admin')) {
      setActiveAdminTab('verify');
    } else if (auth.hasAnyRole('treasurer')) {
      setActiveAdminTab('finance');
    } else if (auth.hasAnyRole('examiner', 'principal')) {
      setActiveAdminTab('ranking');
    }
  }, [auth.user?.id, auth.hasAnyRole]);

  useEffect(() => {
    if (location.pathname !== '/app') return;
    const params = new URLSearchParams(location.search);
    if (params.has('track') || params.get('create') === '1') {
      setCreateOpen(true);
    }
  }, [location.pathname, location.search]);

  useEffect(() => {
    if (!auth.user) {
      setChildrenApps([]);
      setActiveChildId('');
      setActiveApp(null);
      setAppsLoading(false);
      return;
    }

    let cancelled = false;
    setAppsLoading(true);
    apiRequest<ApplicationSummary[]>('/api/v1/applicant/applications', { tenantSlug })
      .then((rows) => {
        if (cancelled) return;
        const safe = rows ?? [];
        setChildrenApps(safe);
        setActiveChildId((prev) =>
          prev && safe.some((item) => item.id === prev) ? prev : safe[0]?.id ?? '',
        );
        if (safe.length === 0) {
          setActiveApp(null);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setChildrenApps([]);
          setActiveChildId('');
          setActiveApp(null);
        }
      })
      .finally(() => {
        if (!cancelled) setAppsLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [auth.user?.id, tenantSlug]);

  useEffect(() => {
    if (!auth.user || !activeChildId) {
      if (!activeChildId) setActiveApp(null);
      return;
    }

    let cancelled = false;
    setAppsLoading(true);
    apiRequest<ApplicationDetail>(`/api/v1/applicant/applications/${activeChildId}`, {
      tenantSlug,
    })
      .then((detail) => {
        if (!cancelled) setActiveApp(detail);
      })
      .catch(() => {
        if (!cancelled) setActiveApp(null);
      })
      .finally(() => {
        if (!cancelled) setAppsLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [auth.user?.id, activeChildId, tenantSlug]);

  function handleApplicationUpdated(detail: ApplicationDetail) {
    setActiveApp(detail);
    setChildrenApps((prev) => {
      const exists = prev.some((item) => item.id === detail.application.id);
      if (!exists) return [detail.application, ...prev];
      return prev.map((item) =>
        item.id === detail.application.id ? detail.application : item,
      );
    });
  }

  function handleApplicationCreated(detail: ApplicationDetail) {
    setActiveChildId(detail.application.id);
    handleApplicationUpdated(detail);
    onReloadPortal();
  }

  function protectApplicant(element: React.ReactNode) {
    if (auth.loading) {
      return (
        <PublicPortalLayout tenant={portal.tenant}>
          <CenterMessageCard
            title="Memeriksa Sesi Akun"
            message="Mohon tunggu sebentar selagi sesi pendaftaran Anda diverifikasi..."
          />
        </PublicPortalLayout>
      );
    }
    if (!auth.user) {
      const next = encodeURIComponent(location.pathname + location.search);
      return <Navigate to={`/masuk?next=${next}`} replace />;
    }
    return element;
  }

  function protectAdmin() {
    if (auth.loading) {
      return (
        <PublicPortalLayout tenant={portal.tenant}>
          <CenterMessageCard
            title="Memuat Ruang Kerja Panitia"
            message="Memverifikasi hak akses peran pada unit pendidikan..."
          />
        </PublicPortalLayout>
      );
    }
    if (!auth.user) {
      return <Navigate to="/masuk?next=/admin" replace />;
    }
    if (!auth.isStaff) {
      return (
        <PublicPortalLayout tenant={portal.tenant}>
          <CenterMessageCard
            title="Akses Terbatas untuk Panitia"
            message={`Akun ${auth.user.full_name} terdaftar sebagai Pendaftar / Wali dan tidak memiliki peran Panitia PPDB.`}
          >
            <Link to="/app">
              <Button>Ke Portal Pendaftar</Button>
            </Link>
            <Link to="/masuk?next=/admin">
              <Button variant="secondary">Masuk dengan Akun Panitia</Button>
            </Link>
          </CenterMessageCard>
        </PublicPortalLayout>
      );
    }

    return (
      <AdminShell
        tenantName={portal.tenant.name}
        units={portal.units}
        activeUnitId={resolvedUnitId}
        onSelectUnit={setActiveUnitId}
        activeTab={activeAdminTab}
        onSelectTab={setActiveAdminTab}
      >
        {activeAdminTab === 'overview' && (
          <AdminDashboardOverview
            tenantSlug={tenantSlug}
            units={portal.units}
            onNavigateTab={setActiveAdminTab}
          />
        )}
        {activeAdminTab === 'verify' && (
          <VerificationWorkbenchPage tenantSlug={tenantSlug} activeUnitId={resolvedUnitId} />
        )}
        {activeAdminTab === 'finance' && (
          <FinancePaymentsPanel tenantSlug={tenantSlug} activeUnitId={resolvedUnitId} />
        )}
        {activeAdminTab === 'ranking' && (
          <SelectionRankingPanel
            tenantSlug={tenantSlug}
            portal={portal}
            activeUnitId={resolvedUnitId}
            onPortalChanged={onReloadPortal}
          />
        )}
        {activeAdminTab === 'export' && (
          <ExportsAuditPanel
            tenantSlug={tenantSlug}
            portal={portal}
            activeUnitId={resolvedUnitId}
          />
        )}
        {activeAdminTab === 'staff' && (
          <StaffManagementPanel tenantSlug={tenantSlug} units={portal.units} />
        )}
        {activeAdminTab === 'settings' && (
          <TenantSettingsPanel
            tenantSlug={tenantSlug}
            portal={portal}
            activeUnitId={resolvedUnitId}
            onPortalChanged={onReloadPortal}
          />
        )}
      </AdminShell>
    );
  }

  return (
    <>
      <Suspense
        fallback={
          <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--text-muted)' }}>
            Memuat tampilan portal...
          </div>
        }
      >
        <Routes>
          <Route
            path="/"
            element={
              <PublicPortalLayout tenant={portal.tenant}>
                <PublicLandingPage portal={portal} tenantSlug={tenantSlug} />
              </PublicPortalLayout>
            }
          />
          <Route
            path="/verifikasi"
            element={
              <PublicPortalLayout tenant={portal.tenant}>
                <PublicVerifyPage portal={portal} tenantSlug={tenantSlug} />
              </PublicPortalLayout>
            }
          />
          <Route
            path="/masuk"
            element={
              <PublicPortalLayout tenant={portal.tenant}>
                <AuthPage portal={portal} />
              </PublicPortalLayout>
            }
          />
          <Route path="/login" element={<Navigate to="/masuk" replace />} />
          <Route
            path="/app"
            element={protectApplicant(
              <ApplicantShell
                tenantName={portal.tenant.name}
                childrenApps={childrenApps}
                activeChildId={activeChildId}
                onSelectChild={setActiveChildId}
                onNewApplication={() => setCreateOpen(true)}
              >
                <ApplicantDashboardPage
                  tenantSlug={tenantSlug}
                  portal={portal}
                  activeApp={activeApp}
                  loading={appsLoading}
                  createOpen={createOpen}
                  onOpenCreate={() => setCreateOpen(true)}
                  onCloseCreate={() => setCreateOpen(false)}
                  onApplicationUpdated={handleApplicationUpdated}
                  onApplicationCreated={handleApplicationCreated}
                />
              </ApplicantShell>,
            )}
          />
          <Route
            path="/app/daftar"
            element={protectApplicant(
              <ApplicantShell
                tenantName={portal.tenant.name}
                childrenApps={childrenApps}
                activeChildId={activeChildId}
                onSelectChild={setActiveChildId}
                onNewApplication={() => {
                  setCreateOpen(true);
                  navigate('/app');
                }}
                autoSaveText={autoSaveText}
              >
                <RegistrationWizardPage
                  tenantSlug={tenantSlug}
                  portal={portal}
                  activeApp={activeApp}
                  onSaveTick={setAutoSaveText}
                  onApplicationUpdated={handleApplicationUpdated}
                />
              </ApplicantShell>,
            )}
          />
          <Route path="/admin" element={protectAdmin()} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>

      {PreviewToolbar && showDevBar && (
        <Suspense fallback={null}>
          <PreviewToolbar preset={preset} onChangePreset={onChangePreset} />
        </Suspense>
      )}
    </>
  );
}

function initialTenantSlugFromQuery(): string {
  if (typeof window === 'undefined') return '';
  return new URLSearchParams(window.location.search).get('tenant')?.trim() ?? '';
}

function PortalRoot() {
  const location = useLocation();
  const [preset, setPreset] = useState<TerminologyPreset>(initialPresetFromQuery);
  const [portalVersion, setPortalVersion] = useState(0);
  const [portal, setPortal] = useState<PublicPortalPayload | null>(null);
  const [portalLoading, setPortalLoading] = useState(true);
  const [portalError, setPortalError] = useState('');
  const loopback = isLoopbackHost();
  const queryTenant = initialTenantSlugFromQuery();
  const envDefaultTenant = (import.meta.env.VITE_DEFAULT_TENANT_SLUG ?? '').trim();
  const requestTenantSlug =
    queryTenant || (loopback ? envDefaultTenant || PRESET_TO_SLUG[preset] : '');
  const isPlatformRoute = location.pathname.startsWith('/platform');

  const reloadPortal = useCallback(() => {
    setPortalVersion((v) => v + 1);
  }, []);

  useEffect(() => {
    if (isPlatformRoute) return;
    let cancelled = false;
    setPortalLoading(true);
    setPortalError('');

    apiRequest<PublicPortalPayload>('/api/v1/public/portal', {
      tenantSlug: requestTenantSlug || undefined,
      skipAuth: true,
    })
      .then((res) => {
        if (!cancelled) {
          setPortal(res);
          setPreset(res.tenant.terminology_preset);
          setPortalError('');
          setPortalLoading(false);
        }
      })
      .catch(async (err) => {
        if (cancelled) return;
        if (ENABLE_DEV_PREVIEW && loopback) {
          const mockMod = await import('@/core/data/mockPortalData');
          if (!cancelled) {
            setPortal(mockMod.PRESET_PORTALS[preset]);
            setPortalError('');
            setPortalLoading(false);
          }
          return;
        }
        setPortal(null);
        setPortalError(
          err instanceof Error
            ? err.message
            : 'Gagal memuat konfigurasi portal instansi dari server.',
        );
        setPortalLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [isPlatformRoute, loopback, portalVersion, preset, requestTenantSlug]);

  if (isPlatformRoute) {
    return (
      <TenantThemeProvider primaryColor="#1E3A5F">
        <Suspense
          fallback={
            <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--text-muted)' }}>
              Memuat konsol platform...
            </div>
          }
        >
          <PlatformAdminPage />
        </Suspense>
        {PreviewToolbar && ENABLE_DEV_PREVIEW && (
          <Suspense fallback={null}>
            <PreviewToolbar
              preset={preset}
              onChangePreset={(next) => {
                setPreset(next);
              }}
            />
          </Suspense>
        )}
      </TenantThemeProvider>
    );
  }

  if (portalLoading && !portal) {
    return (
      <TenantThemeProvider primaryColor="#1E3A5F">
        <CenterMessageCard
          title="Memuat Portal Pendaftaran"
          message="Sedang mengambil data resmi jalur, jadwal, dan unit sekolah dari server..."
        />
      </TenantThemeProvider>
    );
  }

  if (!portal) {
    return (
      <TenantThemeProvider primaryColor="#1E3A5F">
        <CenterMessageCard
          title="Portal Instansi Tidak Tersedia"
          message={portalError || 'Konfigurasi instansi tidak ditemukan atau server belum merespons.'}
        >
          <Button type="button" onClick={reloadPortal}>
            Coba Muat Ulang
          </Button>
        </CenterMessageCard>
      </TenantThemeProvider>
    );
  }

  const effectiveSlug = portal.tenant.slug || requestTenantSlug;

  function handleChangePreset(next: TerminologyPreset) {
    setPreset(next);
    reloadPortal();
  }

  return (
    <TenantThemeProvider primaryColor={portal.tenant.theme_config.primary_color}>
      <TerminologyProvider
        preset={portal.tenant.terminology_preset}
        overrides={portal.tenant.terminology_overrides}
      >
        <AuthProvider tenantSlug={effectiveSlug}>
          <AppRoutes
            portal={portal}
            tenantSlug={effectiveSlug}
            preset={preset}
            showDevBar={ENABLE_DEV_PREVIEW && loopback}
            onChangePreset={handleChangePreset}
            onReloadPortal={reloadPortal}
          />
        </AuthProvider>
      </TerminologyProvider>
    </TenantThemeProvider>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <PortalRoot />
    </BrowserRouter>
  );
}
export default App;
