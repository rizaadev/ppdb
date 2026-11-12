import { useMemo } from 'react';
import QRCode from 'qrcode';

export function QrCodeSvg({
  value,
  size = 96,
  label = 'QR Code Verifikasi Dokumen',
}: {
  value: string;
  size?: number;
  label?: string;
}) {
  const rendered = useMemo(() => {
    const cleaned = value.trim();
    if (!cleaned) return null;
    try {
      const qr = QRCode.create(cleaned, { errorCorrectionLevel: 'M' });
      const count = qr.modules.size;
      const parts: string[] = [];
      for (let r = 0; r < count; r += 1) {
        for (let c = 0; c < count; c += 1) {
          if (qr.modules.get(r, c)) {
            parts.push(`M${c},${r}h1v1h-1z`);
          }
        }
      }
      return { count, path: parts.join('') };
    } catch {
      return null;
    }
  }, [value]);

  if (!rendered) return null;

  const viewSize = rendered.count + 2;
  return (
    <svg
      role="img"
      aria-label={label}
      width={size}
      height={size}
      viewBox={`-1 -1 ${viewSize} ${viewSize}`}
      shapeRendering="crispEdges"
      style={{
        display: 'block',
        backgroundColor: '#ffffff',
        border: '1px solid #d1d5db',
        borderRadius: 4,
      }}
    >
      <rect x={-1} y={-1} width={viewSize} height={viewSize} fill="#ffffff" />
      <path d={rendered.path} fill="#111827" />
    </svg>
  );
}

