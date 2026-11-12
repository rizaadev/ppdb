import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import {
  Building2,
  CheckCircle2,
  Globe,
  LogOut,
  Pencil,
  Plus,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react';
import { extractErrorMessage, formatDateTime } from '@/core/api/client';
import type {
  AuthTokenBundle,
  DomainVerificationResult,
  EducationLevel,
  InstitutionType,
  PlatformCreateTenantResult,
  PlatformTenantView,
  TerminologyPreset,
} from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { InputField } from '@/components/ui/InputField';
import { Modal } from '@/components/ui/Modal';
import { ToneBadge } from '@/components/ui/StatusBadge';
import adminStyles from '@/features/admin/AdminShared.module.css';
import styles from './PlatformAdminPage.module.css';

const PLATFORM_SESSION_KEY = 'pbdb.platform.session';

interface PlatformSession {
  accessToken: string;
  userEmail: string;
  fullName: string;
}

function readPlatformSession(): PlatformSession | null {
  try {
    const raw = window.localStorage.getItem(PLATFORM_SESSION_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<PlatformSession>;
    if (!parsed.accessToken) return null;
    return {
      accessToken: parsed.accessToken,
      userEmail: parsed.userEmail ?? '',
      fullName: parsed.fullName ?? 'Platform Admin',
    };
  } catch {
    return null;
  }
}

function writePlatformSession(session: PlatformSession | null): void {
  try {
    if (!session) {
      window.localStorage.removeItem(PLATFORM_SESSION_KEY);
    } else {
      window.localStorage.setItem(PLATFORM_SESSION_KEY, JSON.stringify(session));
    }
  } catch {
    // ignore storage errors
  }
}

async function platformFetch<T>(
  path: string,
  opts: { method?: string; token?: string; body?: unknown } = {},
): Promise<T> {
  const headers: Record<string, string> = {};
  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  if (opts.token) {
    headers.Authorization = `Bearer ${opts.token}`;
  }
  const res = await fetch(path, {
    method: opts.method ?? 'GET',
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  let parsed: { data?: T; error?: { message?: string } } | null = null;
  try {
    parsed = (await res.json()) as { data?: T; error?: { message?: string } };
  } catch {
    parsed = null;
  }
  if (!res.ok) {
    throw new Error(parsed?.error?.message ?? `Permintaan platform gagal (HTTP ${res.status}).`);
  }
  return parsed?.data as T;
}

function slugifyInstitution(raw: string): string {
  return raw
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48);
}

export function PlatformAdminPage() {
  const [session, setSession] = useState<PlatformSession | null>(() => readPlatformSession());
  const [loginEmail, setLoginEmail] = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [loginBusy, setLoginBusy] = useState(false);

  const [tenants, setTenants] = useState<PlatformTenantView[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  // Create Tenant Modal
  const [createOpen, setCreateOpen] = useState(false);
  const [slugTouched, setSlugTouched] = useState(false);
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [instType, setInstType] = useState<InstitutionType>('school');
  const [preset, setPreset] = useState<TerminologyPreset>('spmb');
  const [plan, setPlan] = useState('pro');
  const [primaryColor, setPrimaryColor] = useState('#0F766E');
  const [tagline, setTagline] = useState('');
  const [customDomain, setCustomDomain] = useState('');
  const [starterUnitCode, setStarterUnitCode] = useState('UNIT-1');
  const [starterUnitName, setStarterUnitName] = useState('');
  const [starterUnitLevel, setStarterUnitLevel] = useState<EducationLevel>('smp_mts');
  const [adminFullName, setAdminFullName] = useState('');
  const [adminEmail, setAdminEmail] = useState('');
  const [adminPassword, setAdminPassword] = useState('');
  const [savingCreate, setSavingCreate] = useState(false);

  // Edit Tenant Modal
  const [editing, setEditing] = useState<PlatformTenantView | null>(null);
  const [editName, setEditName] = useState('');
  const [editPreset, setEditPreset] = useState<TerminologyPreset>('spmb');
  const [editPlan, setEditPlan] = useState('starter');
  const [editColor, setEditColor] = useState('#1E3A5F');
  const [editTagline, setEditTagline] = useState('');
  const [editDomain, setEditDomain] = useState('');
  const [editDomainVerified, setEditDomainVerified] = useState(false);
  const [editActive, setEditActive] = useState(true);
  const [savingEdit, setSavingEdit] = useState(false);
  const [verifyingTenantId, setVerifyingTenantId] = useState<string | null>(null);

  const loadTenants = useCallback(async (token: string) => {
    setLoading(true);
    try {
      const items = await platformFetch<PlatformTenantView[]>('/api/v1/platform/tenants', { token });
      setTenants(items ?? []);
      setError('');
    } catch (err) {
      const msg = extractErrorMessage(err, 'Gagal memuat daftar instansi.');
      if (msg.toLowerCase().includes('token') || msg.toLowerCase().includes('autentikasi')) {
        writePlatformSession(null);
        setSession(null);
      }
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (session?.accessToken) {
      void loadTenants(session.accessToken);
    }
  }, [session?.accessToken, loadTenants]);

  async function handleLogin(e: FormEvent) {
    e.preventDefault();
    setLoginBusy(true);
    setError('');
    try {
      const bundle = await platformFetch<AuthTokenBundle>('/api/v1/platform/auth/login', {
        method: 'POST',
        body: { email: loginEmail.trim(), password: loginPassword },
      });
      const next: PlatformSession = {
        accessToken: bundle.access_token,
        userEmail: bundle.user.email ?? loginEmail.trim(),
        fullName: bundle.user.full_name,
      };
      writePlatformSession(next);
      setSession(next);
      setNotice('');
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal masuk sebagai Superadmin Platform.'));
    } finally {
      setLoginBusy(false);
    }
  }

  function handleLogout() {
    writePlatformSession(null);
    setSession(null);
    setTenants([]);
    setNotice('');
    setError('');
  }

  function openCreateModal() {
    setSlugTouched(false);
    setName('');
    setSlug('');
    setInstType('school');
    setPreset('spmb');
    setPlan('pro');
    setPrimaryColor('#0F766E');
    setTagline('');
    setCustomDomain('');
    setStarterUnitCode('UNIT-1');
    setStarterUnitName('');
    setStarterUnitLevel('smp_mts');
    setAdminFullName('');
    setAdminEmail('');
    setAdminPassword('');
    setCreateOpen(true);
  }

  async function handleCreateSubmit(e: FormEvent) {
    e.preventDefault();
    if (!session) return;
    setSavingCreate(true);
    setError('');
    setNotice('');
    try {
      const res = await platformFetch<PlatformCreateTenantResult>('/api/v1/platform/tenants', {
        method: 'POST',
        token: session.accessToken,
        body: {
          slug: slug.trim().toLowerCase(),
          name: name.trim(),
          institution_type: instType,
          terminology_preset: preset,
          subscription_plan: plan,
          primary_color: primaryColor.trim(),
          tagline: tagline.trim(),
          custom_domain: customDomain.trim() || null,
          starter_unit_code: starterUnitCode.trim().toUpperCase(),
          starter_unit_name: starterUnitName.trim(),
          starter_unit_level: starterUnitLevel,
          admin_full_name: adminFullName.trim(),
          admin_email: adminEmail.trim(),
          admin_password: adminPassword,
        },
      });
      setCreateOpen(false);
      setNotice(
        res.initial_admin
          ? `Tenant "${res.tenant.name}" (${res.tenant.slug}) berhasil dibuat beserta akun admin ${res.initial_admin.email}.`
          : `Tenant "${res.tenant.name}" (${res.tenant.slug}) berhasil dibuat.`,
      );
      await loadTenants(session.accessToken);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal mendaftarkan instansi baru.'));
    } finally {
      setSavingCreate(false);
    }
  }

  function openEditModal(t: PlatformTenantView) {
    setEditing(t);
    setEditName(t.name);
    setEditPreset(t.terminology_preset);
    setEditPlan(t.subscription_plan);
    setEditColor(t.theme_config.primary_color);
    setEditTagline(t.theme_config.tagline ?? '');
    setEditDomain(t.custom_domain ?? '');
    setEditDomainVerified(t.domain_verified);
    setEditActive(t.is_active);
  }

  async function handleEditSubmit(e: FormEvent) {
    e.preventDefault();
    if (!session || !editing) return;
    setSavingEdit(true);
    setError('');
    setNotice('');
    try {
      await platformFetch<PlatformTenantView>(`/api/v1/platform/tenants/${editing.id}`, {
        method: 'PATCH',
        token: session.accessToken,
        body: {
          name: editName.trim(),
          terminology_preset: editPreset,
          subscription_plan: editPlan,
          primary_color: editColor.trim(),
          tagline: editTagline.trim(),
          custom_domain: editDomain.trim(),
          domain_verified: editDomainVerified,
          is_active: editActive,
        },
      });
      setNotice(`Tenant "${editName.trim()}" berhasil diperbarui.`);
      setEditing(null);
      await loadTenants(session.accessToken);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memperbarui data instansi.'));
    } finally {
      setSavingEdit(false);
    }
  }

  async function toggleTenantActive(t: PlatformTenantView) {
    if (!session) return;
    setError('');
    try {
      await platformFetch<PlatformTenantView>(`/api/v1/platform/tenants/${t.id}`, {
        method: 'PATCH',
        token: session.accessToken,
        body: { is_active: !t.is_active },
      });
      setNotice(`Status tenant "${t.name}" diubah menjadi ${!t.is_active ? 'Aktif' : 'Nonaktif'}.`);
      await loadTenants(session.accessToken);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal mengubah status tenant.'));
    }
  }

  async function handleVerifyTenantDomain(t: PlatformTenantView) {
    if (!session) return;
    setVerifyingTenantId(t.id);
    setError('');
    setNotice('');
    try {
      const res = await platformFetch<{
        verification: DomainVerificationResult;
        tenant?: PlatformTenantView;
      }>(`/api/v1/platform/tenants/${t.id}/verify-domain`, {
        method: 'POST',
        token: session.accessToken,
      });
      if (res.verification.verified) {
        setNotice(`DNS ${res.verification.domain} terverifikasi: ${res.verification.message}`);
        await loadTenants(session.accessToken);
      } else {
        setError(
          `Verifikasi DNS ${res.verification.domain} belum cocok: ${res.verification.message} (TXT host: ${res.verification.expected_txt_host} = ${res.verification.expected_txt_value})`,
        );
      }
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memverifikasi DNS domain instansi.'));
    } finally {
      setVerifyingTenantId(null);
    }
  }

  if (!session) {
    return (
      <div className={styles.page}>
        <header className={styles.topbar}>
          <div className={styles.topbarInner}>
            <div className={styles.brandGroup}>
              <div className={styles.brandMark}>
                <ShieldCheck size={18} />
              </div>
              <div>
                <strong>Platform SaaS PBDB / SPMB</strong>
                <div className={adminStyles.metaMuted}>Konsol Superadmin &amp; Onboarding Instansi</div>
              </div>
            </div>
            <Link to="/">
              <Button variant="secondary" size="sm">
                Ke Portal Publik
              </Button>
            </Link>
          </div>
        </header>

        <div className={styles.loginWrap}>
          <Card>
            <h1 style={{ fontSize: 'var(--text-h2)', fontWeight: 700, marginBottom: 6 }}>
              Masuk Superadmin Platform
            </h1>
            <p className={adminStyles.metaMuted} style={{ marginBottom: 'var(--space-4)' }}>
              Kelola onboarding sekolah/madrasah/yayasan baru, paket langganan, dan verifikasi custom domain.
            </p>
            {error && (
              <div role="alert" className={adminStyles.alertError} style={{ marginBottom: 'var(--space-3)' }}>
                {error}
              </div>
            )}
            <form onSubmit={(e) => void handleLogin(e)} className={styles.formGrid}>
              <InputField
                label="Email Superadmin"
                type="email"
                value={loginEmail}
                onChange={(e) => setLoginEmail(e.target.value)}
                placeholder="platform@spmb-platform.id"
                required
              />
              <InputField
                label="Password"
                type="password"
                value={loginPassword}
                onChange={(e) => setLoginPassword(e.target.value)}
                required
              />
              <Button type="submit" disabled={loginBusy}>
                {loginBusy ? 'Memverifikasi...' : 'Masuk ke Konsol Platform'}
              </Button>
            </form>
            <div className={adminStyles.metaMuted} style={{ marginTop: 'var(--space-3)' }}>
              Masuk dengan akun Platform Admin yang dikonfigurasi pada server produksi.
            </div>
          </Card>
        </div>
      </div>
    );
  }

  const activeCount = tenants.filter((t) => t.is_active).length;
  const verifiedDomains = tenants.filter((t) => t.domain_verified).length;

  return (
    <div className={styles.page}>
      <header className={styles.topbar}>
        <div className={styles.topbarInner}>
          <div className={styles.brandGroup}>
            <div className={styles.brandMark}>
              <ShieldCheck size={18} />
            </div>
            <div>
              <strong>Konsol Superadmin Platform</strong>
              <div className={adminStyles.metaMuted}>
                {session.fullName} ({session.userEmail})
              </div>
            </div>
          </div>
          <div className={styles.actionsRow}>
            <Link to="/">
              <Button variant="secondary" size="sm">
                Ke Portal Publik
              </Button>
            </Link>
            <Button variant="ghost" size="sm" onClick={handleLogout}>
              <LogOut size={14} />
              Keluar
            </Button>
          </div>
        </div>
      </header>

      <main className={styles.container}>
        <div className={adminStyles.headerRow}>
          <div>
            <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
              Manajemen Instansi &amp; Onboarding SaaS
            </h1>
            <p className={adminStyles.metaMuted}>
              Buat portal sekolah/madrasah baru beserta admin instansi awal, atur paket, dan verifikasi SSL domain.
            </p>
          </div>
          <div className={styles.actionsRow}>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void loadTenants(session.accessToken)}
              disabled={loading}
            >
              <RefreshCw size={14} />
              Muat Ulang
            </Button>
            <Button size="sm" onClick={openCreateModal}>
              <Plus size={14} />
              Daftarkan Instansi Baru
            </Button>
          </div>
        </div>

        {error && (
          <div role="alert" className={adminStyles.alertError}>
            {error}
          </div>
        )}
        {notice && <div className={adminStyles.alertSuccess}>{notice}</div>}

        <div className={styles.kpiGrid}>
          <Card compact>
            <div className={adminStyles.metaMuted}>Total Tenant Terdaftar</div>
            <div className={`${styles.kpiValue} tabular-nums`}>{tenants.length}</div>
          </Card>
          <Card compact>
            <div className={adminStyles.metaMuted}>Tenant Aktif</div>
            <div className={`${styles.kpiValue} tabular-nums`}>{activeCount}</div>
          </Card>
          <Card compact>
            <div className={adminStyles.metaMuted}>Custom Domain Terverifikasi</div>
            <div className={`${styles.kpiValue} tabular-nums`}>{verifiedDomains}</div>
          </Card>
        </div>

        <Card>
          <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700, marginBottom: 'var(--space-3)', display: 'flex', alignItems: 'center', gap: 8 }}>
            <Building2 size={18} />
            Daftar Sekolah / Madrasah / Yayasan
          </h2>
          <div className={adminStyles.tableWrap}>
            <table className={adminStyles.table}>
              <thead>
                <tr>
                  <th>Instansi &amp; Slug</th>
                  <th>Jenis / Preset</th>
                  <th>Unit</th>
                  <th>Custom Domain</th>
                  <th>Paket</th>
                  <th>Status</th>
                  <th>Aksi</th>
                </tr>
              </thead>
              <tbody>
                {loading && tenants.length === 0 ? (
                  <tr>
                    <td colSpan={7} className={adminStyles.emptyState}>
                      Memuat daftar instansi...
                    </td>
                  </tr>
                ) : (
                  tenants.map((t) => (
                    <tr key={t.id}>
                      <td>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                          <span
                            style={{
                              width: 12,
                              height: 12,
                              borderRadius: 3,
                              backgroundColor: t.theme_config.primary_color,
                              display: 'inline-block',
                              flexShrink: 0,
                            }}
                          />
                          <div>
                            <div style={{ fontWeight: 700 }}>{t.name}</div>
                            <div className={`mono-code ${adminStyles.metaMuted}`}>
                              {t.slug} · dibuat {formatDateTime(t.created_at)}
                            </div>
                          </div>
                        </div>
                      </td>
                      <td>
                        <div>{t.institution_type}</div>
                        <div className={`mono-code ${adminStyles.metaMuted}`}>
                          {t.terminology_preset.toUpperCase()}
                        </div>
                      </td>
                      <td className="tabular-nums">{t.unit_count}</td>
                      <td>
                        {t.custom_domain ? (
                          <div>
                            <div className="mono-code" style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                              <Globe size={13} />
                              {t.custom_domain}
                            </div>
                            <ToneBadge
                              tone={t.domain_verified ? 'success' : 'warning'}
                              label={t.domain_verified ? 'SSL Terverifikasi' : 'Menunggu Verifikasi'}
                            />
                            {t.domain_verification_token && !t.domain_verified && (
                              <div className={`mono-code ${adminStyles.metaMuted}`} style={{ fontSize: 11, marginTop: 2 }}>
                                TXT: _spmb-verify.{t.custom_domain} = {t.domain_verification_token}
                              </div>
                            )}
                          </div>
                        ) : (
                          <span className={adminStyles.metaMuted}>Subdomain default</span>
                        )}
                      </td>
                      <td>
                        <ToneBadge tone="info" label={t.subscription_plan.toUpperCase()} />
                      </td>
                      <td>
                        <ToneBadge
                          tone={t.is_active ? 'success' : 'danger'}
                          label={t.is_active ? 'Aktif' : 'Suspended'}
                        />
                      </td>
                      <td>
                        <div className={styles.actionsRow}>
                          <Button size="sm" variant="ghost" onClick={() => openEditModal(t)}>
                            <Pencil size={13} />
                            Kelola
                          </Button>
                          {t.custom_domain && (
                            <Button
                              size="sm"
                              variant="ghost"
                              disabled={verifyingTenantId === t.id}
                              onClick={() => void handleVerifyTenantDomain(t)}
                            >
                              <Globe size={13} />
                              {verifyingTenantId === t.id ? 'Cek DNS...' : 'Cek DNS'}
                            </Button>
                          )}
                          <Button size="sm" variant="ghost" onClick={() => void toggleTenantActive(t)}>
                            <CheckCircle2 size={13} />
                            {t.is_active ? 'Suspend' : 'Aktifkan'}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </Card>
      </main>

      <Modal
        open={createOpen}
        title="Onboarding Sekolah / Yayasan Baru"
        onClose={() => setCreateOpen(false)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setCreateOpen(false)} disabled={savingCreate}>
              Batal
            </Button>
            <Button type="submit" form="platform-create-tenant-form" disabled={savingCreate}>
              {savingCreate ? 'Mendaftarkan...' : 'Buat Instansi'}
            </Button>
          </>
        }
      >
        <form id="platform-create-tenant-form" onSubmit={(e) => void handleCreateSubmit(e)} className={styles.formGrid}>
          <div className={`${styles.formGrid} ${styles.formGrid2}`}>
            <InputField
              label="Nama Lengkap Instansi"
              value={name}
              onChange={(e) => {
                const nextName = e.target.value;
                setName(nextName);
                if (!slugTouched) setSlug(slugifyInstitution(nextName));
              }}
              placeholder="Yayasan Pendidikan Al-Falah"
              required
            />
            <InputField
              label="Slug Subdomain"
              value={slug}
              onChange={(e) => {
                setSlugTouched(true);
                setSlug(e.target.value);
              }}
              placeholder="yayasan-alfalah"
              required
              mono
            />
            <div className={styles.fieldBlock}>
              <label htmlFor="pf-inst-type" className={styles.fieldLabel}>
                Jenis Lembaga
              </label>
              <select
                id="pf-inst-type"
                className={styles.input}
                value={instType}
                onChange={(e) => setInstType(e.target.value as InstitutionType)}
              >
                <option value="school">Sekolah</option>
                <option value="madrasah">Madrasah</option>
                <option value="foundation">Yayasan Multi-Unit</option>
                <option value="pesantren">Pondok Pesantren</option>
              </select>
            </div>
            <div className={styles.fieldBlock}>
              <label htmlFor="pf-preset" className={styles.fieldLabel}>
                Preset Terminologi
              </label>
              <select
                id="pf-preset"
                className={styles.input}
                value={preset}
                onChange={(e) => setPreset(e.target.value as TerminologyPreset)}
              >
                <option value="spmb">SPMB</option>
                <option value="pmbm">PMBM</option>
                <option value="mandiri">Mandiri</option>
              </select>
            </div>
            <div className={styles.fieldBlock}>
              <label htmlFor="pf-plan" className={styles.fieldLabel}>
                Paket Langganan
              </label>
              <select
                id="pf-plan"
                className={styles.input}
                value={plan}
                onChange={(e) => setPlan(e.target.value)}
              >
                <option value="starter">Starter</option>
                <option value="pro">Pro</option>
                <option value="enterprise">Enterprise</option>
              </select>
            </div>
            <InputField
              label="Warna Merek (Hex)"
              value={primaryColor}
              onChange={(e) => setPrimaryColor(e.target.value)}
              mono
            />
            <InputField
              label="Tagline Portal (Opsional)"
              value={tagline}
              onChange={(e) => setTagline(e.target.value)}
              placeholder="Penerimaan Murid Baru Terpadu"
            />
            <InputField
              label="Custom Domain (Opsional)"
              value={customDomain}
              onChange={(e) => setCustomDomain(e.target.value)}
              placeholder="ppdb.sekolah.sch.id"
              mono
            />
          </div>

          <div className={styles.sectionDivider}>
            <div className={styles.sectionHeading}>Unit Pendidikan Awal (Opsional)</div>
            <div className={`${styles.formGrid} ${styles.formGrid2}`}>
              <InputField
                label="Kode Unit Awal"
                value={starterUnitCode}
                onChange={(e) => setStarterUnitCode(e.target.value)}
                placeholder="SMP-1"
                mono
              />
              <InputField
                label="Nama Unit Awal"
                value={starterUnitName}
                onChange={(e) => setStarterUnitName(e.target.value)}
                placeholder="SMP Islam Al-Falah"
              />
              <div className={styles.fieldBlock}>
                <label htmlFor="pf-unit-lvl" className={styles.fieldLabel}>
                  Jenjang
                </label>
                <select
                  id="pf-unit-lvl"
                  className={styles.input}
                  value={starterUnitLevel}
                  onChange={(e) => setStarterUnitLevel(e.target.value as EducationLevel)}
                >
                  <option value="ra_tk">RA / TK</option>
                  <option value="sd_mi">SD / MI</option>
                  <option value="smp_mts">SMP / MTs</option>
                  <option value="sma_ma">SMA / MA</option>
                  <option value="smk_mak">SMK / MAK</option>
                  <option value="pesantren">Pesantren</option>
                </select>
              </div>
            </div>
          </div>

          <div className={styles.sectionDivider}>
            <div className={styles.sectionHeading}>Akun Admin Instansi Awal (Opsional)</div>
            <div className={`${styles.formGrid} ${styles.formGrid2}`}>
              <InputField
                label="Nama Lengkap Admin"
                value={adminFullName}
                onChange={(e) => setAdminFullName(e.target.value)}
                placeholder="Ketua Panitia PPDB"
              />
              <InputField
                label="Email Admin"
                type="email"
                value={adminEmail}
                onChange={(e) => setAdminEmail(e.target.value)}
                placeholder="admin@sekolah.sch.id"
              />
              <InputField
                label="Password Awal Admin"
                type="password"
                value={adminPassword}
                onChange={(e) => setAdminPassword(e.target.value)}
                placeholder="Minimal 6 karakter"
              />
            </div>
          </div>
        </form>
      </Modal>

      <Modal
        open={editing !== null}
        title={editing ? `Kelola Tenant: ${editing.slug}` : 'Kelola Tenant'}
        onClose={() => setEditing(null)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setEditing(null)} disabled={savingEdit}>
              Batal
            </Button>
            <Button type="submit" form="platform-edit-tenant-form" disabled={savingEdit}>
              {savingEdit ? 'Menyimpan...' : 'Simpan Perubahan'}
            </Button>
          </>
        }
      >
        <form id="platform-edit-tenant-form" onSubmit={(e) => void handleEditSubmit(e)} className={styles.formGrid}>
          <InputField
            label="Nama Instansi"
            value={editName}
            onChange={(e) => setEditName(e.target.value)}
            required
          />
          <div className={`${styles.formGrid} ${styles.formGrid2}`}>
            <div className={styles.fieldBlock}>
              <label htmlFor="edit-pf-preset" className={styles.fieldLabel}>
                Preset Terminologi
              </label>
              <select
                id="edit-pf-preset"
                className={styles.input}
                value={editPreset}
                onChange={(e) => setEditPreset(e.target.value as TerminologyPreset)}
              >
                <option value="spmb">SPMB</option>
                <option value="pmbm">PMBM</option>
                <option value="mandiri">Mandiri</option>
              </select>
            </div>
            <div className={styles.fieldBlock}>
              <label htmlFor="edit-pf-plan" className={styles.fieldLabel}>
                Paket Langganan
              </label>
              <select
                id="edit-pf-plan"
                className={styles.input}
                value={editPlan}
                onChange={(e) => setEditPlan(e.target.value)}
              >
                <option value="starter">Starter</option>
                <option value="pro">Pro</option>
                <option value="enterprise">Enterprise</option>
              </select>
            </div>
            <InputField
              label="Warna Merek (Hex)"
              value={editColor}
              onChange={(e) => setEditColor(e.target.value)}
              mono
            />
            <InputField
              label="Tagline"
              value={editTagline}
              onChange={(e) => setEditTagline(e.target.value)}
            />
          </div>
          <InputField
            label="Custom Domain"
            value={editDomain}
            onChange={(e) => setEditDomain(e.target.value)}
            placeholder="ppdb.sekolah.sch.id"
            mono
          />
          <label className={styles.checkboxLabel}>
            <input
              type="checkbox"
              checked={editDomainVerified}
              onChange={(e) => setEditDomainVerified(e.target.checked)}
              disabled={!editDomain.trim()}
            />
            <span>Domain terverifikasi &amp; izinkan penerbitan SSL otomatis (Caddy)</span>
          </label>
          <label className={styles.checkboxLabel}>
            <input
              type="checkbox"
              checked={editActive}
              onChange={(e) => setEditActive(e.target.checked)}
            />
            <span>Tenant aktif (nonaktifkan untuk suspend akses portal sekolah)</span>
          </label>
        </form>
      </Modal>
    </div>
  );
}

