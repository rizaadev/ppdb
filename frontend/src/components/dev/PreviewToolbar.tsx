import { Link } from 'react-router-dom';
import type { TerminologyPreset } from '@/core/types/domain';
import styles from './PreviewToolbar.module.css';

export function PreviewToolbar({
  preset,
  onChangePreset,
}: {
  preset: TerminologyPreset;
  onChangePreset: (next: TerminologyPreset) => void;
}) {
  return (
    <aside className={`${styles.bar} no-print`} aria-label="Pengalih Pratinjau Preset">
      <span style={{ opacity: 0.75 }}>Preview:</span>
      <select
        aria-label="Pilih preset regulasi"
        className={styles.select}
        value={preset}
        onChange={(e) => onChangePreset(e.target.value as TerminologyPreset)}
      >
        <option value="spmb">Preset SPMB</option>
        <option value="pmbm">Preset PMBM</option>
        <option value="mandiri">Preset Mandiri/Pesantren</option>
      </select>
      <Link to="/" className={styles.pillLink}>
        Publik
      </Link>
      <Link to="/app" className={styles.pillLink}>
        Pendaftar
      </Link>
      <Link to="/app/daftar" className={styles.pillLink}>
        Wizard
      </Link>
      <Link to="/admin" className={styles.pillLink}>
        Panitia
      </Link>
      <Link to="/platform" className={styles.pillLink}>
        Platform
      </Link>
    </aside>
  );
}

