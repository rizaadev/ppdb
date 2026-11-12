import type { HTMLAttributes, ReactNode } from 'react';
import styles from './Card.module.css';

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  interactive?: boolean;
  compact?: boolean;
  children: ReactNode;
}

export function Card({
  interactive = false,
  compact = false,
  className,
  children,
  ...rest
}: CardProps) {
  const classes = [
    styles.card,
    interactive ? styles.interactive : '',
    compact ? styles.compact : '',
    className ?? '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={classes} {...rest}>
      {children}
    </div>
  );
}

