import { STATUS_DICTIONARY, type StatusTone } from '@/core/status/statusDictionary';
import type { ApplicationStatus } from '@/core/types/domain';
import styles from './StatusBadge.module.css';

export function StatusBadge({ status }: { status: ApplicationStatus }) {
  const meta = STATUS_DICTIONARY[status];
  return <ToneBadge tone={meta.tone} label={meta.label} />;
}

export function ToneBadge({ tone, label }: { tone: StatusTone; label: string }) {
  return (
    <span className={`${styles.badge} ${styles[tone]}`}>
      <span className={styles.dot} aria-hidden="true" />
      {label}
    </span>
  );
}

