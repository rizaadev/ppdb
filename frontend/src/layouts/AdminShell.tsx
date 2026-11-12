import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  ClipboardCheck,
  Download,
  FileSpreadsheet,
  LogOut,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Users,
  Wallet,
  X,
} from 'lucide-react';
import { useAuth } from '@/core/auth/AuthContext';
import type { CurrentUser, TenantRole, UnitSummary } from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import styles from './AdminShell.module.css';

export type AdminTabId = 'verify' | 'finance' | 'ranking' | 'export' | 'staff' | 'settings';

const ROLE_LABELS: Record<TenantRole, string> = {
  tenant_admin: 'Admin Instansi',
  unit_admin: 'Admin Unit',
  verifier: 'Verifikator Berkas',
  examiner: 'Penguji Seleksi',
  treasurer: 'Bendahara',
  principal: 'Kepala Sekolah',
};

const ADMIN_NAV: Array<{
  id: AdminTabId;
  label: string;
  topbarTitle: string;
  icon: typeof ClipboardCheck;
}> = [
  {
    id: 'verify',
    label: 'Verifikasi & Pendaftar',
    topbarTitle: 'Meja Kerja Verifikasi & Pendaftar',
    icon: ClipboardCheck,
  },
  {
    id: 'finance',
    label: 'Keuangan & Transfer',
    topbarTitle: 'Rekonsiliasi Keuangan & Pembayaran',
    icon: Wallet,
  },
  {
    id: 'ranking',
    label: 'Nilai, Ranking & Hasil',
    topbarTitle: 'Penilaian Seleksi, Ranking & Rilis Hasil',
    icon: FileSpreadsheet,
  },
  {
    id: 'export',
    label: 'Ekspor & Audit Log',
    topbarTitle: 'Ekspor Data Nasional & Audit Log',
    icon: Download,
  },
  {
    id: 'staff',
    label: 'Staf & Hak Akses',
    topbarTitle: 'Manajemen Staf & Hak Akses RBAC',
    icon: Users,
  },
  {
    id: 'settings',
    label: 'Pengaturan Instansi',
    topbarTitle: 'Pengaturan Instansi & Jadwal Periode',
    icon: Settings,
  },
];

function resolveStaffRoleLabel(user: CurrentUser | null, activeUnitId: string): string {
  if (!user) return 'Panitia PPDB';
  if (user.is_platform_admin) return 'Platform Admin';
  const unitScoped = user.roles.find((r) => r.unit_id === activeUnitId);
  const picked = unitScoped ?? user.roles[0];
  return picked ? ROLE_LABELS[picked.role] ?? picked.role : 'Panitia';
}

export function AdminShell({
  tenantName,
  units,
  activeUnitId,
  onSelectUnit,
  activeTab,
  onSelectTab,
  children,
}: {
  tenantName: string;
  units: UnitSummary[];
  activeUnitId: string;
  onSelectUnit: (id: string) => void;
  activeTab: AdminTabId;
  onSelectTab: (tab: AdminTabId) => void;
  children: ReactNode;
}) {
  const { user, logout } = useAuth();
  const [collapsed, setCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const currentNav = ADMIN_NAV.find((n) => n.id === activeTab) ?? ADMIN_NAV[0];
  const roleLabel = resolveStaffRoleLabel(user, activeUnitId);

  const navContent = (isCompact: boolean) => (
    <>
      {!isCompact && (
        <div>
          <label htmlFor="admin-unit-select" className="sr-only">
            Pilih unit jenjang
          </label>
          <select
            id="admin-unit-select"
            className={styles.unitSelect}
            value={activeUnitId}
            onChange={(e) => onSelectUnit(e.target.value)}
          >
            {units.map((u) => (
              <option key={u.id} value={u.id}>
                {u.name}
              </option>
            ))}
          </select>
        </div>
      )}

      <nav className={styles.nav} aria-label="Navigasi Panitia">
        {ADMIN_NAV.map((item) => {
          const Icon = item.icon;
          const isActive = item.id === activeTab;
          return (
            <button
              key={item.id}
              type="button"
              className={`${styles.navItem} ${isActive ? styles.navItemActive : ''}`}
              title={item.label}
              aria-current={isActive ? 'page' : undefined}
              onClick={() => {
                onSelectTab(item.id);
                setDrawerOpen(false);
              }}
            >
              <Icon size={18} style={{ flexShrink: 0 }} />
              {!isCompact && <span>{item.label}</span>}
            </button>
          );
        })}
      </nav>
    </>
  );

  return (
    <div className={styles.shell}>
      <aside className={`${styles.sidebar} ${collapsed ? styles.sidebarCollapsed : ''}`}>
        <div className={styles.sidebarTop}>
          {!collapsed && <span className={styles.workspaceName}>{tenantName}</span>}
          <button
            type="button"
            className={styles.collapseButton}
            aria-label={collapsed ? 'Perluas sidebar' : 'Ciutkan sidebar'}
            onClick={() => setCollapsed((v) => !v)}
          >
            {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
          </button>
        </div>
        {navContent(collapsed)}
      </aside>

      {drawerOpen && (
        <div
          className={styles.drawerBackdrop}
          onClick={(e) => {
            if (e.target === e.currentTarget) setDrawerOpen(false);
          }}
        >
          <div className={styles.drawerPanel}>
            <div className={styles.sidebarTop}>
              <span className={styles.workspaceName}>{tenantName}</span>
              <button
                type="button"
                className={styles.collapseButton}
                aria-label="Tutup menu"
                onClick={() => setDrawerOpen(false)}
              >
                <X size={16} />
              </button>
            </div>
            {navContent(false)}
          </div>
        </div>
      )}

      <div className={styles.contentWrap}>
        <header className={styles.topbar}>
          <div className={styles.topbarLeft}>
            <button
              type="button"
              className={styles.menuButton}
              aria-label="Buka menu panitia"
              onClick={() => setDrawerOpen(true)}
            >
              <Menu size={18} />
            </button>
            <strong style={{ fontSize: 'var(--text-body)' }}>{currentNav.topbarTitle}</strong>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', flexWrap: 'wrap' }}>
            {user && (
              <span style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', fontWeight: 600 }}>
                {user.full_name}
              </span>
            )}
            <span className={styles.roleTag}>{roleLabel}</span>
            <Link
              to="/"
              style={{ fontSize: 'var(--text-caption)', color: 'var(--brand-primary)', fontWeight: 600 }}
            >
              Portal Publik
            </Link>
            <Button size="sm" variant="secondary" onClick={() => void logout()}>
              <LogOut size={14} />
              Keluar
            </Button>
          </div>
        </header>

        <main className={`container-admin ${styles.main}`}>{children}</main>
      </div>
    </div>
  );
}
