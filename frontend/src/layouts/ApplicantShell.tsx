import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, CheckCircle2, LogOut, Plus } from 'lucide-react';
import { useAuth } from '@/core/auth/AuthContext';
import type { ApplicationSummary } from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import styles from './ApplicantShell.module.css';

export function ApplicantShell({
  tenantName,
  childrenApps,
  activeChildId,
  onSelectChild,
  onNewApplication,
  autoSaveText,
  children,
}: {
  tenantName: string;
  childrenApps: ApplicationSummary[];
  activeChildId: string;
  onSelectChild: (id: string) => void;
  onNewApplication?: () => void;
  autoSaveText?: string;
  children: ReactNode;
}) {
  const { user, logout } = useAuth();

  return (
    <div className={styles.shell}>
      <header className={`${styles.topbar} no-print`}>
        <div className={styles.topbarInner}>
          <div className={styles.left}>
            <Link to="/" className={styles.backLink}>
              <ArrowLeft size={16} />
              <span>{tenantName}</span>
            </Link>
          </div>

          <div className={styles.right}>
            {autoSaveText && (
              <span className={styles.saveIndicator} aria-live="polite">
                <CheckCircle2 size={14} />
                {autoSaveText}
              </span>
            )}
            {childrenApps.length > 0 && (
              <>
                <label htmlFor="child-switcher" className="sr-only">
                  Pilih pendaftaran anak
                </label>
                <select
                  id="child-switcher"
                  className={styles.childSelect}
                  value={activeChildId}
                  onChange={(e) => onSelectChild(e.target.value)}
                >
                  {childrenApps.map((app) => (
                    <option key={app.id} value={app.id}>
                      {app.student_name ?? 'Calon Murid'} ({app.registration_no})
                    </option>
                  ))}
                </select>
              </>
            )}
            {onNewApplication && (
              <Button size="sm" variant="secondary" onClick={onNewApplication}>
                <Plus size={14} />
                Daftar Baru
              </Button>
            )}
            {user && (
              <Button
                size="sm"
                variant="ghost"
                aria-label="Keluar"
                title={`Keluar (${user.full_name})`}
                onClick={() => void logout()}
              >
                <LogOut size={15} />
              </Button>
            )}
          </div>
        </div>
      </header>

      <main className={styles.body}>{children}</main>
    </div>
  );
}
