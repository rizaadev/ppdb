import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { CheckCircle2, QrCode, Search, ShieldAlert, ArrowLeft } from 'lucide-react';
import { ApiError, apiRequest, formatDateTime } from '@/core/api/client';
import type { PublicDocumentVerifyResult, PublicPortalPayload } from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { InputField } from '@/components/ui/InputField';
import { StatusBadge, ToneBadge } from '@/components/ui/StatusBadge';

export function PublicVerifyPage({
  portal,
  tenantSlug,
}: {
  portal: PublicPortalPayload;
  tenantSlug: string;
}) {
  const [searchParams, setSearchParams] = useSearchParams();
  const initialToken = searchParams.get('token') ?? '';
  const [tokenInput, setTokenInput] = useState(initialToken);
  const [result, setResult] = useState<PublicDocumentVerifyResult | null>(null);
  const [errorMsg, setErrorMsg] = useState('');
  const [loading, setLoading] = useState(false);

  const verifyToken = useCallback(
    async (rawToken: string) => {
      const cleaned = rawToken.trim();
      if (!cleaned) {
        setErrorMsg('Masukkan token verifikasi dari QR code kartu atau surat.');
        setResult(null);
        return;
      }
      setLoading(true);
      setErrorMsg('');
      try {
        const res = await apiRequest<PublicDocumentVerifyResult>(
          `/api/v1/public/verify-document?token=${encodeURIComponent(cleaned)}`,
          { tenantSlug, skipAuth: true },
        );
        setResult(res);
      } catch (err) {
        setResult(null);
        setErrorMsg(
          err instanceof ApiError
            ? err.message
            : 'Dokumen tidak ditemukan atau tanda tangan digital tidak valid.',
        );
      } finally {
        setLoading(false);
      }
    },
    [tenantSlug],
  );

  useEffect(() => {
    const queryToken = searchParams.get('token') ?? '';
    setTokenInput(queryToken);
    if (queryToken.trim()) {
      void verifyToken(queryToken);
    }
  }, [searchParams, verifyToken]);

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const cleaned = tokenInput.trim();
    if (!cleaned) {
      setErrorMsg('Masukkan token verifikasi dokumen.');
      setResult(null);
      return;
    }
    setSearchParams({ token: cleaned });
  }

  return (
    <div className="container-public" style={{ paddingTop: 'var(--space-6)', paddingBottom: 'var(--space-8)' }}>
      <div style={{ marginBottom: 'var(--space-4)' }}>
        <Link to="/" style={{ display: 'inline-flex', alignItems: 'center', gap: 6, color: 'var(--brand-primary)', fontWeight: 600, fontSize: 'var(--text-caption)' }}>
          <ArrowLeft size={15} />
          Kembali ke Beranda {portal.tenant.name}
        </Link>
      </div>

      <div style={{ maxWidth: 680, margin: '0 auto', display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Card>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-2)' }}>
            <QrCode size={22} style={{ color: 'var(--brand-primary)' }} />
            <h1 style={{ fontSize: 'var(--text-h2)', fontWeight: 700 }}>
              Verifikasi Keaslian Dokumen
            </h1>
          </div>
          <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', marginBottom: 'var(--space-4)' }}>
            Pindai QR code pada Kartu Tanda Pendaftaran atau Surat Keterangan Hasil Seleksi, atau
            masukkan kode token digital untuk memeriksa keasliannya secara kriptografis pada{' '}
            <strong>{portal.tenant.name}</strong>.
          </p>

          <form onSubmit={handleSubmit} style={{ display: 'flex', gap: 'var(--space-2)', alignItems: 'flex-end', flexWrap: 'wrap' }}>
            <div style={{ flex: '1 1 320px' }}>
              <InputField
                label="Token Verifikasi QR"
                value={tokenInput}
                onChange={(e) => setTokenInput(e.target.value)}
                placeholder="Tempel token dari QR dokumen..."
                mono
              />
            </div>
            <Button type="submit" disabled={loading}>
              <Search size={15} />
              {loading ? 'Memeriksa...' : 'Verifikasi Token'}
            </Button>
          </form>
        </Card>

        {errorMsg && (
          <Card>
            <div role="alert" style={{ display: 'flex', gap: 'var(--space-3)', alignItems: 'flex-start', color: 'var(--status-danger-fg)' }}>
              <ShieldAlert size={20} style={{ flexShrink: 0, marginTop: 2 }} />
              <div>
                <div style={{ fontWeight: 700 }}>Verifikasi Gagal / Token Tidak Sah</div>
                <div style={{ fontSize: 'var(--text-caption)', marginTop: 4 }}>{errorMsg}</div>
              </div>
            </div>
          </Card>
        )}

        {result && (
          <Card>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)', marginBottom: 'var(--space-3)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', color: result.valid ? 'var(--status-success-fg)' : 'var(--status-danger-fg)', fontWeight: 700 }}>
                {result.valid ? <CheckCircle2 size={20} /> : <ShieldAlert size={20} />}
                <span>{result.valid ? 'Dokumen Resmi Terverifikasi' : 'Dokumen Tidak Valid'}</span>
              </div>
              <ToneBadge tone={result.valid ? 'success' : 'danger'} label={result.document_type} />
            </div>

            <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', marginBottom: 'var(--space-3)' }}>
              {result.message}
            </p>

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 'var(--space-3)', fontSize: 'var(--text-label)', padding: 'var(--space-3)', borderRadius: 'var(--radius-md)', backgroundColor: 'var(--bg-subtle)' }}>
              <div>
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>Nomor Pendaftaran</div>
                <div className="mono-code" style={{ fontWeight: 700 }}>{result.registration_no}</div>
              </div>
              <div>
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>Nama Pendaftar</div>
                <div style={{ fontWeight: 600 }}>{result.masked_name}</div>
              </div>
              <div>
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>Instansi &amp; Unit</div>
                <div style={{ fontWeight: 600 }}>{result.tenant_name} — {result.unit_name}</div>
              </div>
              <div>
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>Jalur &amp; Tahun Ajaran</div>
                <div>{result.track_name} ({result.academic_year})</div>
              </div>
              <div>
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>Status Pendaftaran</div>
                <div style={{ marginTop: 2 }}><StatusBadge status={result.status} /></div>
              </div>
              {result.final_score != null && (
                <div>
                  <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>Nilai Akhir</div>
                  <div className="tabular-nums" style={{ fontWeight: 700 }}>{result.final_score.toFixed(2)}</div>
                </div>
              )}
              <div>
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>Waktu Verifikasi Server</div>
                <div>{formatDateTime(result.verified_at)}</div>
              </div>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}

