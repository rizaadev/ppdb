import { createElement } from 'react';
import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ApiError,
  downloadProtectedFile,
  extractErrorMessage,
  formatRupiah,
  sessionStorageKey,
  writeStoredSession,
} from '@/core/api/client';
import { TerminologyProvider, useTerminology } from '@/core/terminology/terminology';

function TerminologyProbe() {
  const t = useTerminology();
  return createElement(
    'div',
    null,
    createElement('span', { 'data-testid': 'acronym' }, t.programAcronym),
    createElement('span', { 'data-testid': 'institution' }, t.institutionNoun),
    createElement('span', { 'data-testid': 'applicant' }, t.applicantNoun),
    createElement('span', { 'data-testid': 'period' }, t.periodNoun),
  );
}

describe('client utilities & terminology', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
  });

  it('formats IDR currency and treats zero or negative as Gratis', () => {
    expect(formatRupiah(0)).toBe('Gratis');
    expect(formatRupiah(-5000)).toBe('Gratis');
    const formatted = formatRupiah(250000);
    expect(formatted).toContain('Rp');
    expect(formatted).toContain('250.000');
  });

  it('extracts error messages cleanly from ApiError, Error, and unknown values', () => {
    expect(extractErrorMessage(new ApiError(400, 'BAD_INPUT', 'NIK wajib 16 digit'), 'Fallback')).toBe(
      'NIK wajib 16 digit',
    );
    expect(extractErrorMessage(new Error('Koneksi terputus'), 'Fallback')).toBe('Koneksi terputus');
    expect(extractErrorMessage('unexpected', 'Fallback pesan')).toBe('Fallback pesan');
  });

  it('opens external URLs in a new tab without calling fetch', async () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
    const fetchSpy = vi.spyOn(globalThis, 'fetch');

    await downloadProtectedFile('https://cdn.example.com/file.pdf', 'berkas.pdf', 'demo-spmb');

    expect(openSpy).toHaveBeenCalledWith(
      'https://cdn.example.com/file.pdf',
      '_blank',
      'noopener,noreferrer',
    );
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('downloads protected /api/ files with Authorization + X-Tenant-Slug and Content-Disposition filename', async () => {
    writeStoredSession('demo-spmb', { accessToken: 'tok-abc', refreshToken: 'ref-xyz' });
    expect(window.localStorage.getItem(sessionStorageKey('demo-spmb'))).toContain('tok-abc');

    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({
        'Content-Disposition': 'attachment; filename="dapodik-export.xlsx"',
      }),
      blob: async () => new Blob(['xlsx-bytes'], { type: 'application/octet-stream' }),
    } as unknown as Response);

    const createObjectURL = vi.fn(() => 'blob:mock-url');
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL });

    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

    await downloadProtectedFile('/api/v1/admin/exports/exp-1/download', 'fallback.xlsx', 'demo-spmb');

    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const [reqUrl, reqInit] = fetchSpy.mock.calls[0];
    expect(reqUrl).toBe('/api/v1/admin/exports/exp-1/download');
    expect(reqInit?.headers).toEqual({
      'X-Tenant-Slug': 'demo-spmb',
      Authorization: 'Bearer tok-abc',
    });
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(clickSpy).toHaveBeenCalledTimes(1);
  });

  it('provides accurate terminology presets and applies tenant overrides', () => {
    const { rerender } = render(
      createElement(
        TerminologyProvider,
        { preset: 'pmbm', children: createElement(TerminologyProbe) },
      ),
    );
    expect(screen.getByTestId('acronym')).toHaveTextContent('PMBM');
    expect(screen.getByTestId('institution')).toHaveTextContent('Madrasah');
    expect(screen.getByTestId('period')).toHaveTextContent('Tahun Pelajaran');

    rerender(
      createElement(
        TerminologyProvider,
        {
          preset: 'mandiri',
          overrides: { institutionNoun: 'Dayah Terpadu' },
          children: createElement(TerminologyProbe),
        },
      ),
    );
    expect(screen.getByTestId('acronym')).toHaveTextContent('Penerimaan Baru');
    expect(screen.getByTestId('institution')).toHaveTextContent('Dayah Terpadu');
    expect(screen.getByTestId('applicant')).toHaveTextContent('Calon Murid / Santri');
  });
});
