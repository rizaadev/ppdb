import type { AuthTokenBundle } from '@/core/types/domain';

export class ApiError extends Error {
  status: number;
  code: string;
  fields?: Record<string, string>;

  constructor(status: number, code: string, message: string, fields?: Record<string, string>) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

interface Envelope<T> {
  data: T;
  meta?: Record<string, unknown>;
  error?: {
    code: string;
    message: string;
    fields?: Record<string, string>;
  };
}

export interface StoredSession {
  accessToken: string;
  refreshToken: string;
}

export function sessionStorageKey(tenantSlug: string): string {
  return `pbdb.session.${tenantSlug}`;
}

export function readStoredSession(tenantSlug: string): StoredSession | null {
  try {
    const raw = window.localStorage.getItem(sessionStorageKey(tenantSlug));
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<StoredSession>;
    if (!parsed.accessToken || !parsed.refreshToken) return null;
    return { accessToken: parsed.accessToken, refreshToken: parsed.refreshToken };
  } catch {
    return null;
  }
}

export function writeStoredSession(tenantSlug: string, session: StoredSession | null): void {
  try {
    const key = sessionStorageKey(tenantSlug);
    if (!session) {
      window.localStorage.removeItem(key);
    } else {
      window.localStorage.setItem(key, JSON.stringify(session));
    }
  } catch {
    // Ignore storage quota / privacy mode errors
  }
}

let inFlightRefresh: Promise<StoredSession | null> | null = null;

async function refreshAccessToken(tenantSlug: string, refreshToken: string): Promise<StoredSession | null> {
  if (inFlightRefresh) return inFlightRefresh;

  inFlightRefresh = (async () => {
    try {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' };
      if (tenantSlug) headers['X-Tenant-Slug'] = tenantSlug;

      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers,
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
      if (!res.ok) {
        writeStoredSession(tenantSlug, null);
        return null;
      }
      const env = (await res.json()) as Envelope<AuthTokenBundle>;
      const next: StoredSession = {
        accessToken: env.data.access_token,
        refreshToken: env.data.refresh_token,
      };
      writeStoredSession(tenantSlug, next);
      return next;
    } catch {
      return null;
    } finally {
      inFlightRefresh = null;
    }
  })();

  return inFlightRefresh;
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  tenantSlug?: string;
  idempotencyKey?: string;
  skipAuth?: boolean;
  onSessionExpired?: () => void;
}

export async function apiRequest<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const result = await apiRequestWithMeta<T>(path, opts);
  return result.data;
}

export async function apiRequestWithMeta<T>(
  path: string,
  opts: RequestOptions = {},
): Promise<{ data: T; meta?: Record<string, unknown> }> {
  const tenantSlug = opts.tenantSlug ?? '';
  let session = !opts.skipAuth && tenantSlug ? readStoredSession(tenantSlug) : null;

  const exec = async (token?: string): Promise<Response> => {
    const headers: Record<string, string> = {};
    if (tenantSlug) {
      headers['X-Tenant-Slug'] = tenantSlug;
    }
    if (opts.body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }
    if (opts.idempotencyKey) {
      headers['Idempotency-Key'] = opts.idempotencyKey;
    }
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }

    return fetch(path, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    });
  };

  let res = await exec(session?.accessToken);

  if (res.status === 401 && !opts.skipAuth && session?.refreshToken && tenantSlug) {
    const refreshed = await refreshAccessToken(tenantSlug, session.refreshToken);
    if (refreshed) {
      session = refreshed;
      res = await exec(refreshed.accessToken);
    } else {
      opts.onSessionExpired?.();
    }
  }

  let parsed: Envelope<T> | null = null;
  try {
    parsed = (await res.json()) as Envelope<T>;
  } catch {
    parsed = null;
  }

  if (!res.ok) {
    const errObj = parsed?.error;
    throw new ApiError(
      res.status,
      errObj?.code ?? `HTTP_${res.status}`,
      errObj?.message ?? `Permintaan ke server gagal (HTTP ${res.status}).`,
      errObj?.fields,
    );
  }

  if (!parsed) {
    throw new ApiError(res.status, 'INVALID_RESPONSE', 'Respons server tidak valid.');
  }

  return { data: parsed.data, meta: parsed.meta };
}

export async function uploadToPresignedUrl(
  uploadUrl: string,
  file: File,
  requiredHeaders?: Record<string, string>,
): Promise<void> {
  const headers: Record<string, string> = { ...(requiredHeaders ?? {}) };
  if (!headers['content-type'] && !headers['Content-Type'] && file.type) {
    headers['Content-Type'] = file.type;
  }

  const res = await fetch(uploadUrl, {
    method: 'PUT',
    headers,
    body: file,
  });

  if (!res.ok) {
    throw new Error(`Unggahan berkas gagal (HTTP ${res.status}). Pastikan layanan object storage aktif dan dapat diakses.`);
  }
}

export async function downloadProtectedFile(
  url: string,
  filename: string,
  tenantSlug?: string,
): Promise<void> {
  if (!url.startsWith('/api/')) {
    window.open(url, '_blank', 'noopener,noreferrer');
    return;
  }

  const slug = tenantSlug ?? '';
  let session = slug ? readStoredSession(slug) : null;

  const exec = async (token?: string): Promise<Response> => {
    const headers: Record<string, string> = {};
    if (slug) {
      headers['X-Tenant-Slug'] = slug;
    }
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    return fetch(url, { method: 'GET', headers });
  };

  let res = await exec(session?.accessToken);
  if (res.status === 401 && session?.refreshToken && slug) {
    const refreshed = await refreshAccessToken(slug, session.refreshToken);
    if (refreshed) {
      session = refreshed;
      res = await exec(refreshed.accessToken);
    }
  }

  if (!res.ok) {
    let errMsg = `Gagal mengunduh file (HTTP ${res.status}).`;
    try {
      const parsed = (await res.json()) as { error?: { code?: string; message?: string } };
      if (parsed?.error?.message) {
        errMsg = parsed.error.message;
      }
      throw new ApiError(res.status, parsed?.error?.code ?? `HTTP_${res.status}`, errMsg);
    } catch (e) {
      if (e instanceof ApiError) throw e;
      throw new ApiError(res.status, `HTTP_${res.status}`, errMsg);
    }
  }

  const blob = await res.blob();
  const disposition = res.headers.get('Content-Disposition') ?? '';
  const match = disposition.match(/filename\*?=(?:UTF-8''|")?([^";\n]+)"?/i);
  const resolvedName = match?.[1] ? decodeURIComponent(match[1].replace(/"/g, '')) : filename;

  const objectUrl = URL.createObjectURL(blob);
  try {
    const link = document.createElement('a');
    link.href = objectUrl;
    link.download = resolvedName;
    document.body.appendChild(link);
    link.click();
    link.remove();
  } finally {
    window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
  }
}

export function formatRupiah(amount: number): string {
  if (amount <= 0) return 'Gratis';
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(amount);
}

export function formatDateTime(iso?: string | null): string {
  if (!iso) return '-';
  const dt = new Date(iso);
  if (Number.isNaN(dt.getTime())) return iso;
  return new Intl.DateTimeFormat('id-ID', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(dt);
}

export function extractErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}
