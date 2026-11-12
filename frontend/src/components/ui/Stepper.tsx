import { Check } from 'lucide-react';
import styles from './Stepper.module.css';

export interface StepItem {
  id: string;
  title: string;
  subtitle?: string;
}

export function Stepper({
  steps,
  currentIndex,
  onSelectStep,
  compactMobile = false,
}: {
  steps: StepItem[];
  currentIndex: number;
  onSelectStep?: (index: number) => void;
  compactMobile?: boolean;
}) {
  if (compactMobile) {
    const active = steps[currentIndex];
    return (
      <div className={styles.mobileCompact}>
        <div>
          <div className={styles.subtitle}>
            Langkah {currentIndex + 1} dari {steps.length}
          </div>
          <div className={styles.label}>{active?.title}</div>
        </div>
        <div className={styles.mobileDots} aria-hidden="true">
          {steps.map((s, idx) => (
            <span
              key={s.id}
              className={[
                styles.mobileDot,
                idx < currentIndex ? styles.mobileDotDone : '',
                idx === currentIndex ? styles.mobileDotActive : '',
              ]
                .filter(Boolean)
                .join(' ')}
            />
          ))}
        </div>
      </div>
    );
  }

  return (
    <nav className={styles.stepper} aria-label="Tahapan Pendaftaran">
      {steps.map((step, idx) => {
        const isDone = idx < currentIndex;
        const isActive = idx === currentIndex;
        return (
          <button
            key={step.id}
            type="button"
            className={[
              styles.step,
              isDone ? styles.done : '',
              isActive ? styles.active : '',
            ]
              .filter(Boolean)
              .join(' ')}
            onClick={() => onSelectStep?.(idx)}
          >
            <span className={styles.circle}>
              {isDone ? <Check size={14} strokeWidth={2.6} /> : idx + 1}
            </span>
            <div>
              <div className={styles.label}>{step.title}</div>
              {step.subtitle && <div className={styles.subtitle}>{step.subtitle}</div>}
            </div>
          </button>
        );
      })}
    </nav>
  );
}

