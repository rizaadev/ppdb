import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  FileCheck2,
  Lock,
  Mail,
  MapPin,
  Phone,
  Shield,
} from 'lucide-react';
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
            <a href="#alur" className={styles.navLink}>
              Alur
            </a>
            <a href="#fasilitas" className={styles.navLink}>
              Fasilitas
            </a>
            <a href="#jalur" className={styles.navLink}>
              {term.trackNoun}
            </a>
            <a href="#syarat" className={styles.navLink}>
              Persyaratan
            </a>
            <a href="#cek-status" className={styles.navLink}>
              Cek Status
            </a>
            <a href="#faq" className={styles.navLink}>
              FAQ
            </a>
            <Link to="/verifikasi" className={styles.navLink}>
              Validasi Dokumen
            </Link>
            <Link to={user ? (isStaff ? '/admin' : '/app') : '/masuk'}>
              <Button size="sm">
                {user ? `Dashboard (${user.full_name.split(' ')[0]})` : 'Masuk / Daftar'}
              </Button>
            </Link>
          </nav>
        </div>
      </header>

      <main className={styles.main}>{children}</main>

      <footer className={styles.footer}>
        <div className="container-public">
          <div className={styles.footerGrid}>
            {/* COLUMN 1: IDENTITAS LEMBAGA */}
            <div className={styles.footerCol}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <span className={styles.brandMark} style={{ width: 32, height: 32, fontSize: '0.85rem' }}>
                  {initials}
                </span>
                <span className={styles.footerTitle}>{tenant.name}</span>
              </div>
              <p style={{ lineHeight: 1.6, color: 'var(--text-secondary)' }}>
                {tenant.theme_config.tagline ||
                  'Lembaga Pendidikan Islam Berkarakter, Memadukan Ilmu Salaf & Sains Modern Menuju Generasi Rabbani.'}
              </p>
              <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start', color: 'var(--text-secondary)' }}>
                <MapPin size={16} style={{ flexShrink: 0, marginTop: 3, color: 'var(--brand-primary)' }} />
                <span>Jl. Lintas Sumatera, Cinta Mulya, Kec. Way Pengubuan, Lampung</span>
              </div>
            </div>

            {/* COLUMN 2: NAVIGASI CEPAT */}
            <div className={styles.footerCol}>
              <div className={styles.footerTitle}>Navigasi Cepat</div>
              <ul className={styles.footerLinkList}>
                <li>
                  <a href="#alur" className={styles.footerLink}>
                    Alur 7 Langkah Pendaftaran
                  </a>
                </li>
                <li>
                  <a href="#jalur" className={styles.footerLink}>
                    Pilihan {term.trackNoun} &amp; Kuota
                  </a>
                </li>
                <li>
                  <a href="#syarat" className={styles.footerLink}>
                    Checklist Dokumen Berkas
                  </a>
                </li>
                <li>
                  <a href="#cek-status" className={styles.footerLink}>
                    Cek Status Pendaftaran Cepat
                  </a>
                </li>
                <li>
                  <Link to="/verifikasi" className={styles.footerLink}>
                    Verifikasi Keaslian Dokumen Digital
                  </Link>
                </li>
              </ul>
            </div>

            {/* COLUMN 3: KONTAK PANITIA */}
            <div className={styles.footerCol}>
              <div className={styles.footerTitle}>Sekretariat Panitia</div>
              <ul className={styles.footerLinkList}>
                <li style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <Phone size={15} style={{ color: 'var(--brand-primary)' }} />
                  <span>WhatsApp: +62 812-3456-7890</span>
                </li>
                <li style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <Mail size={15} style={{ color: 'var(--brand-primary)' }} />
                  <span>Email: ppdb@alhamidcintamulya.fun</span>
                </li>
                <li style={{ marginTop: 4, color: 'var(--text-muted)' }}>
                  Jam Layanan Panitia:<br />
                  Senin – Ahad: 08.00 – 16.30 WIB
                </li>
              </ul>
            </div>

            {/* COLUMN 4: KEPATUHAN & PRIVASI */}
            <div className={styles.footerCol}>
              <div className={styles.footerTitle}>Keamanan &amp; Kepatuhan</div>
              <p style={{ lineHeight: 1.55, color: 'var(--text-secondary)' }}>
                Sistem pendaftaran ini menerapkan keamanan bertingkat:
              </p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <div style={{ display: 'flex', gap: 6, alignItems: 'center', fontSize: '0.78rem' }}>
                  <Shield size={14} style={{ color: 'var(--brand-primary)' }} />
                  <span>UU Pelindungan Data Pribadi (UU PDP)</span>
                </div>
                <div style={{ display: 'flex', gap: 6, alignItems: 'center', fontSize: '0.78rem' }}>
                  <Lock size={14} style={{ color: 'var(--brand-primary)' }} />
                  <span>Enkripsi Database PostgreSQL RLS</span>
                </div>
                <div style={{ display: 'flex', gap: 6, alignItems: 'center', fontSize: '0.78rem' }}>
                  <FileCheck2 size={14} style={{ color: 'var(--brand-primary)' }} />
                  <span>Validasi QR Dokumen Berintegritas</span>
                </div>
              </div>
            </div>
          </div>

          <div className={styles.footerBottom}>
            <div>
              &copy; {new Date().getFullYear()} {tenant.name} · Portal Penerimaan Santri Baru (PSB). All rights reserved.
            </div>
            <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
              <span>Status Sistem: <strong style={{ color: '#16a34a' }}>● Operasional Produksi</strong></span>
              <span>v2.0 Enterprise</span>
            </div>
          </div>
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
