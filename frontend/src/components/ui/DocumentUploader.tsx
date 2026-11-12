import { useRef, useState } from 'react';
import { AlertTriangle, FileText, Upload } from 'lucide-react';
import type { DocumentReviewStatus } from '@/core/types/domain';
import { compressImageForUpload } from '@/lib/imageCompression';
import { ToneBadge } from './StatusBadge';
import { Button } from './Button';
import styles from './DocumentUploader.module.css';

export interface DocumentUploaderProps {
  label: string;
  required?: boolean;
  status?: DocumentReviewStatus;
  reviewNote?: string;
  initialFileName?: string;
  initialSizeKb?: number;
  initialPreviewUrl?: string;
  uploading?: boolean;
  uploadError?: string;
  onPreviewClick?: () => void;
  onFilePrepared?: (file: File, previewUrl: string) => void | Promise<void>;
}

const DOC_STATUS_MAP: Record<DocumentReviewStatus, { tone: 'neutral' | 'success' | 'warning'; label: string }> = {
  pending: { tone: 'neutral', label: 'Menunggu Pemeriksaan' },
  approved: { tone: 'success', label: 'Sah' },
  rejected_for_revision: { tone: 'warning', label: 'Perlu Perbaikan' },
};

export function DocumentUploader({
  label,
  required = true,
  status = 'pending',
  reviewNote,
  initialFileName,
  initialSizeKb,
  initialPreviewUrl,
  uploading = false,
  uploadError,
  onPreviewClick,
  onFilePrepared,
}: DocumentUploaderProps) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [fileName, setFileName] = useState(initialFileName ?? '');
  const [sizeInfo, setSizeInfo] = useState(
    initialSizeKb ? `${initialSizeKb} KB` : '',
  );
  const [previewUrl, setPreviewUrl] = useState(initialPreviewUrl ?? '');
  const [compressing, setCompressing] = useState(false);

  const badge = fileName ? DOC_STATUS_MAP[status] : { tone: 'neutral' as const, label: 'Belum Diunggah' };

  async function handleSelect(file: File | undefined) {
    if (!file) return;
    setCompressing(true);
    try {
      const result = await compressImageForUpload(file);
      const url = result.file.type.startsWith('image/') ? URL.createObjectURL(result.file) : '';
      const origKb = Math.max(1, Math.round(result.originalSizeBytes / 1024));
      const finalKb = Math.max(1, Math.round(result.compressedSizeBytes / 1024));

      setFileName(result.file.name);
      setSizeInfo(
        result.wasCompressed
          ? `${origKb} KB → ${finalKb} KB (dikompresi otomatis)`
          : `${finalKb} KB`,
      );
      setPreviewUrl(url);
      await onFilePrepared?.(result.file, url);
    } finally {
      setCompressing(false);
    }
  }

  return (
    <div className={`${styles.wrapper} ${status === 'rejected_for_revision' ? styles.rejected : ''}`}>
      <div className={styles.header}>
        <div className={styles.titleBlock}>
          <span className={styles.title}>
            {label}
            {required && <span style={{ color: 'var(--status-danger-fg)' }}> *</span>}
          </span>
        </div>
        <ToneBadge tone={badge.tone} label={badge.label} />
      </div>

      {status === 'rejected_for_revision' && reviewNote && (
        <div className={styles.noteBox} role="alert">
          <AlertTriangle size={16} style={{ flexShrink: 0, marginTop: 2 }} />
          <span>Catatan Panitia: {reviewNote}</span>
        </div>
      )}

      {uploadError && (
        <div className={styles.noteBox} role="alert" style={{ backgroundColor: 'var(--status-danger-bg)', color: 'var(--status-danger-fg)', borderColor: 'var(--status-danger-border)' }}>
          <AlertTriangle size={16} style={{ flexShrink: 0, marginTop: 2 }} />
          <span>{uploadError}</span>
        </div>
      )}

      {fileName ? (
        <div className={styles.fileRow}>
          <div className={styles.fileInfo}>
            {previewUrl ? (
              <img src={previewUrl} alt={`Pratinjau ${label}`} className={styles.thumb} />
            ) : (
              <FileText size={24} color="var(--text-secondary)" />
            )}
            <div style={{ minWidth: 0 }}>
              <div className={styles.fileName}>{fileName}</div>
              <div className={`${styles.meta} tabular-nums`}>
                {uploading ? 'Mengunggah ke penyimpanan privat...' : sizeInfo}
              </div>
            </div>
          </div>
          <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
            {onPreviewClick && (
              <Button variant="ghost" size="sm" onClick={onPreviewClick} disabled={uploading || compressing}>
                Lihat
              </Button>
            )}
            <Button
              variant="secondary"
              size="sm"
              onClick={() => inputRef.current?.click()}
              disabled={compressing || uploading}
            >
              {compressing ? 'Mengompres...' : uploading ? 'Mengunggah...' : 'Ganti Berkas'}
            </Button>
          </div>
        </div>
      ) : (
        <label className={styles.dropzone} onClick={() => !compressing && !uploading && inputRef.current?.click()}>
          <Upload size={20} color="var(--brand-primary)" />
          <span className={styles.title}>
            {compressing
              ? 'Mengompres foto di perangkat...'
              : uploading
                ? 'Mengunggah ke server aman...'
                : 'Pilih foto atau PDF berkas'}
          </span>
          <span className={styles.meta}>
            JPG, PNG, WebP, atau PDF · Foto kamera besar dikompresi otomatis di HP Anda
          </span>
        </label>
      )}

      <input
        ref={inputRef}
        type="file"
        className="sr-only"
        accept="image/jpeg,image/png,image/webp,application/pdf"
        onChange={(e) => {
          void handleSelect(e.target.files?.[0]);
          e.target.value = '';
        }}
      />
    </div>
  );
}

