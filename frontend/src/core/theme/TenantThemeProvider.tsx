import { useEffect, type ReactNode } from 'react';

function hexToRgb(hex: string): [number, number, number] {
  const clean = hex.replace('#', '').trim();
  const full =
    clean.length === 3
      ? clean.split('').map((c) => c + c).join('')
      : clean.padEnd(6, '0').slice(0, 6);
  const num = Number.parseInt(full, 16);
  if (Number.isNaN(num)) return [4, 120, 87]; // Default Deep Emerald
  return [(num >> 16) & 255, (num >> 8) & 255, num & 255];
}

function relativeLuminance([r, g, b]: [number, number, number]): number {
  const toLinear = (c: number) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b);
}

function darken([r, g, b]: [number, number, number], factor = 0.85): string {
  const clamp = (v: number) => Math.max(0, Math.min(255, Math.round(v * factor)));
  return `rgb(${clamp(r)}, ${clamp(g)}, ${clamp(b)})`;
}

export function TenantThemeProvider({
  primaryColor,
  children,
}: {
  primaryColor?: string;
  children: ReactNode;
}) {
  useEffect(() => {
    const root = document.documentElement;
    const baseHex = primaryColor || '#047857';
    const rgb = hexToRgb(baseHex);
    const lum = relativeLuminance(rgb);
    const foreground = lum > 0.45 ? '#0f172a' : '#ffffff';

    root.style.setProperty('--brand-primary', baseHex);
    root.style.setProperty('--brand-primary-hover', darken(rgb, 0.82));
    root.style.setProperty('--brand-tint', `rgba(${rgb[0]}, ${rgb[1]}, ${rgb[2]}, 0.08)`);
    root.style.setProperty('--brand-border', `rgba(${rgb[0]}, ${rgb[1]}, ${rgb[2]}, 0.28)`);
    root.style.setProperty('--brand-foreground', foreground);
  }, [primaryColor]);

  return children;
}
