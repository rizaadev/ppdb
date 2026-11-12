import type { InputHTMLAttributes } from 'react';
import styles from './InputField.module.css';

export interface InputFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  helperText?: string;
  error?: string;
  mono?: boolean;
}

export function InputField({
  label,
  helperText,
  error,
  mono = false,
  required,
  id,
  className,
  ...rest
}: InputFieldProps) {
  const inputId = id ?? label.toLowerCase().replace(/[^a-z0-9]+/g, '-');
  const classes = [
    styles.input,
    error ? styles.inputError : '',
    mono ? styles.inputMono : '',
    className ?? '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={styles.field}>
      <div className={styles.labelRow}>
        <label htmlFor={inputId} className={styles.label}>
          {label}
          {required && <span className={styles.required}>*</span>}
        </label>
      </div>
      <input id={inputId} className={classes} aria-invalid={Boolean(error)} required={required} {...rest} />
      {error ? (
        <span className={styles.error} role="alert">
          {error}
        </span>
      ) : (
        helperText && <span className={styles.hint}>{helperText}</span>
      )}
    </div>
  );
}

