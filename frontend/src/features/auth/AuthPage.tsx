import { useState, type FormEvent } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { ArrowLeft, KeyRound, MessageSquare, RotateCcw } from 'lucide-react';
import { ApiError } from '@/core/api/client';
import { useAuth } from '@/core/auth/AuthContext';
import { useTerminology } from '@/core/terminology/terminology';
import type { PublicPortalPayload } from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { InputField } from '@/components/ui/InputField';
import styles from './AuthPage.module.css';

export function AuthPage({ portal }: { portal: PublicPortalPayload }) {
  const auth = useAuth();
  const term = useTerminology();
  const navigate = useNavigate();
  const location = useLocation();
  const nextPath = new URLSearchParams(location.search).get('next');

  const [mode, setMode] = useState<'login' | 'otp' | 'reset'>('login');
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');

  const [otpPurpose, setOtpPurpose] = useState<'register' | 'login'>('register');
  const [waPhone, setWaPhone] = useState('');
  const [otpSent, setOtpSent] = useState(false);
  const [devOtpHint, setDevOtpHint] = useState('');
  const [otpCode, setOtpCode] = useState('');
  const [fullName, setFullName] = useState('');
  const [newPassword, setNewPassword] = useState('');

  const [busy, setBusy] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  function routeAfterAuth(hasStaffRoles: boolean) {
    if (nextPath && nextPath.startsWith('/')) {
      navigate(nextPath, { replace: true });
      return;
    }
    navigate(hasStaffRoles ? '/admin' : '/app', { replace: true });
  }

  async function handlePasswordLogin(e: FormEvent) {
    e.preventDefault();
    setErrorMsg('');
    setFieldErrors({});
    setBusy(true);
    try {
      const logged = await auth.loginWithPassword(identifier, password);
      routeAfterAuth(logged.is_platform_admin || logged.roles.length > 0);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorMsg(err.message);
        setFieldErrors(err.fields ?? {});
      } else {
        setErrorMsg('Gagal menghubungi server autentikasi.');
      }
    } finally {
      setBusy(false);
    }
  }

  async function handleRequestOtp(e: FormEvent) {
    e.preventDefault();
    setErrorMsg('');
    setFieldErrors({});
    setBusy(true);
    try {
      const purpose = mode === 'reset' ? 'reset_pin' : otpPurpose;
      const res = await auth.requestOtp(waPhone, purpose);
      setOtpSent(true);
      setDevOtpHint(res.dev_otp ?? '');
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorMsg(err.message);
        setFieldErrors(err.fields ?? {});
      } else {
        setErrorMsg('Gagal mengirim permintaan OTP WhatsApp.');
      }
    } finally {
      setBusy(false);
    }
  }

  async function handleVerifyOtp(e: FormEvent) {
    e.preventDefault();
    setErrorMsg('');
    setFieldErrors({});
    setBusy(true);
    try {
      if (mode === 'reset') {
        const logged = await auth.resetPasswordWithOtp({
          whatsappPhone: waPhone,
          code: otpCode,
          newPassword,
        });
        routeAfterAuth(logged.is_platform_admin || logged.roles.length > 0);
        return;
      }
      const logged = await auth.verifyOtp({
        whatsappPhone: waPhone,
        purpose: otpPurpose,
        code: otpCode,
        fullName: otpPurpose === 'register' ? fullName : undefined,
        password: otpPurpose === 'register' && newPassword ? newPassword : undefined,
      });
      routeAfterAuth(logged.is_platform_admin || logged.roles.length > 0);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorMsg(err.message);
        setFieldErrors(err.fields ?? {});
      } else {
        setErrorMsg(mode === 'reset' ? 'Reset kata sandi gagal.' : 'Verifikasi OTP gagal.');
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className={styles.wrap}>
      <div className={`container-wizard ${styles.inner}`}>
        <Link to="/" className={styles.backLink}>
          <ArrowLeft size={16} />
          Kembali ke Portal {portal.tenant.name}
        </Link>

        <Card>
          <div style={{ marginBottom: 'var(--space-4)' }}>
            <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
              Masuk Portal {term.programAcronym}
            </h1>
            <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)', marginTop: 4 }}>
              Masuk sebagai {term.guardianNoun}, pendaftar, atau panitia/staf {portal.tenant.name}.
            </p>
          </div>

          <div className={styles.tabRow} role="tablist">
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'login'}
              className={`${styles.tabBtn} ${mode === 'login' ? styles.tabBtnActive : ''}`}
              onClick={() => {
                setMode('login');
                setErrorMsg('');
                setFieldErrors({});
              }}
            >
              <KeyRound size={15} />
              Masuk Kata Sandi
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'otp'}
              className={`${styles.tabBtn} ${mode === 'otp' ? styles.tabBtnActive : ''}`}
              onClick={() => {
                setMode('otp');
                setOtpSent(false);
                setErrorMsg('');
                setFieldErrors({});
              }}
            >
              <MessageSquare size={15} />
              Daftar / OTP WhatsApp
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'reset'}
              className={`${styles.tabBtn} ${mode === 'reset' ? styles.tabBtnActive : ''}`}
              onClick={() => {
                setMode('reset');
                setOtpSent(false);
                setErrorMsg('');
                setFieldErrors({});
              }}
            >
              <RotateCcw size={15} />
              Lupa Sandi
            </button>
          </div>

          {errorMsg && (
            <div className={styles.errorAlert} role="alert">
              {errorMsg}
            </div>
          )}

          {mode === 'login' ? (
            <form onSubmit={handlePasswordLogin} className={styles.form}>
              <InputField
                label="Nomor WhatsApp atau Email"
                value={identifier}
                onChange={(e) => setIdentifier(e.target.value)}
                error={fieldErrors.identifier}
                placeholder="Contoh: 081234567890 atau staff@sekolah.sch.id"
                required
              />
              <InputField
                label="Kata Sandi"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                error={fieldErrors.password}
                required
              />
              <Button type="submit" size="lg" fullWidth disabled={busy}>
                {busy ? 'Memverifikasi...' : 'Masuk Sekarang'}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  setMode('reset');
                  setOtpSent(false);
                  setErrorMsg('');
                  setFieldErrors({});
                }}
              >
                <RotateCcw size={14} />
                Lupa kata sandi? Reset via OTP WhatsApp
              </Button>
            </form>
          ) : (
            <div className={styles.form}>
              {mode === 'otp' && (
                <div className={styles.purposeRow}>
                  <Button
                    type="button"
                    size="sm"
                    variant={otpPurpose === 'register' ? 'primary' : 'secondary'}
                    onClick={() => {
                      setOtpPurpose('register');
                      setOtpSent(false);
                    }}
                  >
                    Buat Akun {term.guardianNoun} Baru
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant={otpPurpose === 'login' ? 'primary' : 'secondary'}
                    onClick={() => {
                      setOtpPurpose('login');
                      setOtpSent(false);
                    }}
                  >
                    Masuk Tanpa Sandi (OTP)
                  </Button>
                </div>
              )}

              {mode === 'reset' && (
                <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-secondary)' }}>
                  Masukkan nomor WhatsApp terdaftar untuk menerima kode OTP reset kata sandi.
                </p>
              )}

              {!otpSent ? (
                <form onSubmit={handleRequestOtp} className={styles.form}>
                  <InputField
                    label="Nomor WhatsApp Aktif"
                    value={waPhone}
                    onChange={(e) => setWaPhone(e.target.value)}
                    error={fieldErrors.whatsapp_phone}
                    placeholder="Contoh: 081355667788"
                    mono
                    required
                  />
                  <Button type="submit" size="lg" fullWidth disabled={busy}>
                    {busy ? 'Mengirim OTP...' : 'Kirim Kode OTP WhatsApp'}
                  </Button>
                </form>
              ) : (
                <form onSubmit={handleVerifyOtp} className={styles.form}>
                  {devOtpHint && (
                    <div className={styles.devOtpBox}>
                      <span>Mode Dev — Kode OTP simulasi WhatsApp:</span>{' '}
                      <strong className="mono-code">{devOtpHint}</strong>
                    </div>
                  )}
                  <InputField
                    label="Kode OTP (6 Digit)"
                    value={otpCode}
                    onChange={(e) => setOtpCode(e.target.value)}
                    error={fieldErrors.code}
                    placeholder="Masukkan 6 digit OTP"
                    mono
                    required
                  />
                  {mode === 'otp' && otpPurpose === 'register' && (
                    <>
                      <InputField
                        label={`Nama Lengkap ${term.guardianNoun}`}
                        value={fullName}
                        onChange={(e) => setFullName(e.target.value)}
                        error={fieldErrors.full_name}
                        required
                      />
                      <InputField
                        label="Buat Kata Sandi (Opsional, min. 6 karakter)"
                        type="password"
                        value={newPassword}
                        onChange={(e) => setNewPassword(e.target.value)}
                        error={fieldErrors.password}
                      />
                    </>
                  )}
                  {mode === 'reset' && (
                    <InputField
                      label="Kata Sandi Baru (Minimal 8 Karakter)"
                      type="password"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      error={fieldErrors.new_password ?? fieldErrors.password}
                      required
                    />
                  )}
                  <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={() => setOtpSent(false)}
                      disabled={busy}
                    >
                      Ganti Nomor
                    </Button>
                    <Button type="submit" fullWidth disabled={busy}>
                      {busy
                        ? 'Memproses...'
                        : mode === 'reset'
                          ? 'Simpan Sandi Baru & Masuk'
                          : 'Verifikasi & Masuk'}
                    </Button>
                  </div>
                </form>
              )}
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}

