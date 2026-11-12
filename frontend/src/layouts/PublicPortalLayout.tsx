import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useAuth } from '@/core/auth/AuthContext';
import { useTerminology } from '@/core/terminology/terminology';
import type { TenantInfo } from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import styles from './PublicPortalLayout.module.css';

export function PublicPortalLayout({
  tenant,
  children,
}: {
  tenant: TenantInfo;
  children: ReactNode;
}) {
  const term = useTerminology();
  const { user, isStaff } = useAuth();
  const initials = tenant.name
    .split(' ')
    .slice(0, 2)
    .map((w) => w[0])
    .join('')
    .toUpperCase();

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div className={`container-public ${styles.headerInner}`}>
          <Link to="/" className={styles.brand}>
            <span className={styles.brandMark} aria-hidden="true">
              {tenant.theme_config.logo_url ? (
                <img
                  src={tenant.theme_config.logo_url}
                  alt={tenant.name}
                  style={{ width: '100%', height: '100%', objectFit: 'contain', borderRadius: 'inherit' }}
                />
              ) : (
                initials
              )}
            </span>
            <div style={{ minWidth: 0 }}>
              <div className={styles.brandTitle}>{tenant.name}</div>
              <div className={styles.brandSub}>{term.programTitle}</div>
            </div>
          </Link>

          <nav className={styles.navDesktop} aria-label="Navigasi Portal Publik">
            <a href="#jalur" className={styles.navLink}>
              {term.trackNoun}
            </a>
            <a href="#cek-status" className={styles.navLink}>
              Cek Status
            </a>
            <Link to="/verifikasi" className={styles.navLink}>
              Verifikasi Dokumen
            </Link>
            <Link to="/app" className={styles.navLink}>
              Portal {term.guardianNoun}
            </Link>
            <Link to="/admin" className={styles.navLink}>
              Panel Panitia
            </Link>
            <Link to={user ? (isStaff ? '/admin' : '/app') : '/masuk'}>
              <Button size="sm">{user ? `Dashboard (${user.full_name.split(' ')[0]})` : 'Masuk / Daftar'}</Button>
            </Link>
          </nav>
        </div>
      </header>

      <main className={styles.main}>{children}</main>

      <footer className={styles.footer}>
        <div className={`container-public ${styles.footerInner}`}>
          <span>
            &copy; {new Date().getFullYear()} {tenant.name} · Portal {term.programAcronym}
          </span>
          <span>Data kependudukan dilindungi &amp; terenkripsi (UU PDP).</span>
        </div>
      </footer>

      <div className={styles.mobileStickyBar}>
        <a href="#cek-status">
          <Button variant="secondary" fullWidth>
            Cek Status
          </Button>
        </a>
        <Link to="/app/daftar">
          <Button fullWidth>Daftar Sekarang</Button>
        </Link>
      </div>
    </div>
  );
}

