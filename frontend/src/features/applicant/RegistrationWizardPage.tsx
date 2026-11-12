import { lazy, Suspense, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Award, Plus, Trash2 } from 'lucide-react';
import { ApiError, apiRequest, uploadToPresignedUrl } from '@/core/api/client';
import { useTerminology } from '@/core/terminology/terminology';
import type {
  AchievementItem,
  ApplicationDetail,
  ApplicationDocumentItem,
  PresignUploadResult,
  PresignedDownloadResult,
  PublicPortalPayload,
} from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { DocumentUploader } from '@/components/ui/DocumentUploader';
import { InputField } from '@/components/ui/InputField';
import { Modal } from '@/components/ui/Modal';
import { Stepper } from '@/components/ui/Stepper';
import styles from './RegistrationWizardPage.module.css';

const LazyLocationPickerMap = lazy(() =>
  import('@/components/map/LocationPickerMap').then((mod) => ({
    default: mod.LocationPickerMap,
  })),
);

const WIZARD_STEPS = [
  { id: 'biodata', title: 'Biodata Murid', subtitle: 'Standar Dapodik & EMIS' },
  { id: 'guardians', title: 'Orang Tua / Wali', subtitle: 'Kontak WA aktif' },
  { id: 'achievements', title: 'Prestasi & Piagam', subtitle: 'Opsional / jalur prestasi' },
  { id: 'location', title: 'Alamat & Koordinat', subtitle: 'Peta domisili OSM' },
  { id: 'documents', title: 'Unggah Berkas', subtitle: 'Kompresi otomatis' },
  { id: 'review', title: 'Tinjau & Kirim', subtitle: 'Verifikasi akhir' },
];

export function RegistrationWizardPage({
  tenantSlug,
  portal,
  activeApp,
  onSaveTick,
  onApplicationUpdated,
}: {
  tenantSlug: string;
  portal: PublicPortalPayload;
  activeApp: ApplicationDetail | null;
  onSaveTick: (label: string) => void;
  onApplicationUpdated: (detail: ApplicationDetail) => void;
}) {
  const term = useTerminology();
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [submitBusy, setSubmitBusy] = useState(false);
  const [bannerError, setBannerError] = useState('');
  const [uploadingReqId, setUploadingReqId] = useState<string | null>(null);
  const [docErrors, setDocErrors] = useState<Record<string, string>>({});

  const [fullName, setFullName] = useState('');
  const [nik, setNik] = useState('');
  const [nisn, setNisn] = useState('');
  const [kkNumber, setKkNumber] = useState('');
  const [kkIssuedDate, setKkIssuedDate] = useState('');
  const [birthPlace, setBirthPlace] = useState('');
  const [birthDate, setBirthDate] = useState('');
  const [gender, setGender] = useState('L');
  const [addressStreet, setAddressStreet] = useState('');
  const [extraAnswers, setExtraAnswers] = useState<Record<string, unknown>>({});

  const [fatherName, setFatherName] = useState('');
  const [motherName, setMotherName] = useState('');
  const [guardianPhone, setGuardianPhone] = useState('');

  const [achievements, setAchievements] = useState<AchievementItem[]>([]);

  const [coords, setCoords] = useState({ latitude: -6.2458, longitude: 106.8879 });
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!activeApp) return;
    const p = activeApp.profile;
    setFullName(p?.full_name ?? activeApp.application.student_name ?? '');
    setNik(p?.nik ?? '');
    setNisn(p?.nisn ?? '');
    setKkNumber(p?.kk_number ?? '');
    setKkIssuedDate(p?.kk_issued_date ?? '');
    setBirthPlace(p?.birth_place ?? '');
    setBirthDate(p?.birth_date ?? '');
    setGender(p?.gender ?? 'L');
    setAddressStreet(p?.address_street ?? '');
    setExtraAnswers(activeApp.extra_answers ?? {});

    const father = activeApp.guardians.find((g) => g.relation === 'father');
    const mother = activeApp.guardians.find((g) => g.relation === 'mother');
    const primary = activeApp.guardians.find((g) => g.is_primary_contact) ?? father ?? mother;
    setFatherName(father?.full_name ?? '');
    setMotherName(mother?.full_name ?? '');
    setGuardianPhone(primary?.phone_whatsapp ?? '');

    setAchievements(
      (activeApp.achievements ?? []).map((item) => ({
        ...item,
        rank_achieved: item.rank_achieved ?? '',
        organizer: item.organizer ?? '',
      })),
    );

    const unit = portal.units.find((u) => u.id === activeApp.application.unit_id) ?? portal.units[0];
    setCoords({
      latitude: p?.latitude ?? (unit?.latitude ?? -6.2415) - 0.004,
      longitude: p?.longitude ?? (unit?.longitude ?? 106.8832) + 0.004,
    });
    setErrors({});
    setBannerError('');
  }, [activeApp?.application.id]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!activeApp) {
    return (
      <div className="container-wizard">
        <Card>
          <p style={{ marginBottom: 'var(--space-3)' }}>Pilih atau buat pendaftaran baru terlebih dahulu dari Dashboard.</p>
          <Link to="/app">
            <Button>Ke Dashboard {term.guardianNoun}</Button>
          </Link>
        </Card>
      </div>
    );
  }

  const unit = portal.units.find((u) => u.id === activeApp.application.unit_id) ?? portal.units[0];
  const schoolCoords =
    activeApp.unit_latitude && activeApp.unit_longitude
      ? { latitude: activeApp.unit_latitude, longitude: activeApp.unit_longitude }
      : unit?.latitude && unit?.longitude
        ? { latitude: unit.latitude, longitude: unit.longitude }
        : undefined;

  const isLocked =
    activeApp.application.status !== 'draft_form' && activeApp.application.status !== 'revision_required';

  const activeTrack = portal.periods
    .flatMap((p) => p.tracks)
    .find((t) => t.id === activeApp.application.track_id);
  const customFormSchema = activeApp.custom_form_schema ?? activeTrack?.custom_form_schema ?? [];

  function markSavedTime() {
    const now = new Date();
    const hh = String(now.getHours()).padStart(2, '0');
    const mm = String(now.getMinutes()).padStart(2, '0');
    onSaveTick(`Tersimpan ${hh}:${mm}`);
  }

  async function saveProfileToServer(includeCoords = false): Promise<boolean> {
    if (!activeApp || isLocked) return true;
    const fieldErrs: Record<string, string> = {};
    if (!includeCoords && customFormSchema.length > 0) {
      for (const field of customFormSchema) {
        if (!field.required) continue;
        const val = extraAnswers[field.key];
        const empty =
          field.type === 'checkbox'
            ? val !== true
            : val === undefined || val === null || String(val).trim() === '';
        if (empty) {
          fieldErrs[`extra_${field.key}`] = `${field.label} wajib diisi.`;
        }
      }
    }
    if (Object.keys(fieldErrs).length > 0) {
      setErrors(fieldErrs);
      setBannerError('Lengkapi seluruh pertanyaan tambahan yang wajib diisi.');
      return false;
    }

    setSaving(true);
    setBannerError('');
    setErrors({});
    try {
      const updated = await apiRequest<ApplicationDetail>(
        `/api/v1/applicant/applications/${activeApp.application.id}/profile`,
        {
          method: 'PATCH',
          tenantSlug,
          body: {
            version: activeApp.application.version,
            full_name: fullName,
            nik: nik || null,
            kk_number: kkNumber || null,
            kk_issued_date: kkIssuedDate || null,
            nisn: nisn || null,
            birth_place: birthPlace || null,
            birth_date: birthDate || null,
            gender,
            citizenship: 'WNI',
            address_street: addressStreet || null,
            latitude: includeCoords ? coords.latitude : activeApp.profile?.latitude ?? coords.latitude,
            longitude: includeCoords ? coords.longitude : activeApp.profile?.longitude ?? coords.longitude,
            extra_answers: extraAnswers,
          },
        },
      );
      onApplicationUpdated(updated);
      markSavedTime();
      return true;
    } catch (err) {
      if (err instanceof ApiError) {
        setBannerError(err.message);
        setErrors(err.fields ?? {});
      } else {
        setBannerError('Gagal menyimpan perubahan ke server.');
      }
      return false;
    } finally {
      setSaving(false);
    }
  }

  async function saveGuardiansToServer(): Promise<boolean> {
    if (!activeApp || isLocked) return true;
    const list = [];
    if (fatherName.trim()) {
      list.push({
        relation: 'father',
        full_name: fatherName.trim(),
        phone_whatsapp: guardianPhone.trim() || null,
        is_primary_contact: true,
      });
    }
    if (motherName.trim()) {
      list.push({
        relation: 'mother',
        full_name: motherName.trim(),
        phone_whatsapp: !fatherName.trim() ? guardianPhone.trim() || null : null,
        is_primary_contact: !fatherName.trim(),
      });
    }
    if (list.length === 0) {
      setErrors({ fatherName: 'Minimal isi nama ayah atau ibu/wali.' });
      return false;
    }

    setSaving(true);
    setBannerError('');
    setErrors({});
    try {
      const updated = await apiRequest<ApplicationDetail>(
        `/api/v1/applicant/applications/${activeApp.application.id}/guardians`,
        {
          method: 'PUT',
          tenantSlug,
          body: { guardians: list },
        },
      );
      onApplicationUpdated(updated);
      markSavedTime();
      return true;
    } catch (err) {
      if (err instanceof ApiError) {
        setBannerError(err.message);
        setErrors(err.fields ?? {});
      } else {
        setBannerError('Gagal menyimpan data orang tua/wali.');
      }
      return false;
    } finally {
      setSaving(false);
    }
  }

  function addAchievementRow() {
    setAchievements((prev) => [
      ...prev,
      {
        category: 'akademik',
        title: '',
        level: 'kota',
        rank_achieved: '',
        achievement_year: new Date().getFullYear(),
        organizer: '',
        raw_points: 10,
      },
    ]);
  }

  function updateAchievementRow(index: number, patch: Partial<AchievementItem>) {
    setAchievements((prev) =>
      prev.map((row, idx) => (idx === index ? { ...row, ...patch } : row)),
    );
  }

  function removeAchievementRow(index: number) {
    setAchievements((prev) => prev.filter((_, idx) => idx !== index));
  }

  async function saveAchievementsToServer(): Promise<boolean> {
    if (!activeApp || isLocked) return true;

    for (let i = 0; i < achievements.length; i++) {
      if (!achievements[i].title.trim()) {
        setBannerError(`Judul prestasi baris ke-${i + 1} wajib diisi atau hapus baris yang kosong.`);
        return false;
      }
    }

    if (achievements.length === 0 && activeApp.achievements.length === 0) {
      // No changes needed if applicant leaves empty optional achievements untouched
      return true;
    }

    setSaving(true);
    setBannerError('');
    setErrors({});
    try {
      const payload = achievements.map((item) => ({
        ...(item.id ? { id: item.id } : {}),
        category: item.category || 'akademik',
        title: item.title.trim(),
        level: item.level || 'kota',
        rank_achieved: item.rank_achieved?.trim() || null,
        achievement_year: item.achievement_year ? Number(item.achievement_year) : null,
        organizer: item.organizer?.trim() || null,
        raw_points: Number(item.raw_points) || 0,
      }));
      const saved = await apiRequest<{ achievements: AchievementItem[] }>(
        `/api/v1/applicant/applications/${activeApp.application.id}/achievements`,
        {
          method: 'PUT',
          tenantSlug,
          body: { achievements: payload },
        },
      );
      onApplicationUpdated({
        ...activeApp,
        achievements: saved.achievements ?? [],
      });
      markSavedTime();
      return true;
    } catch (err) {
      if (err instanceof ApiError) {
        setBannerError(err.message);
        setErrors(err.fields ?? {});
      } else {
        setBannerError('Gagal menyimpan data prestasi.');
      }
      return false;
    } finally {
      setSaving(false);
    }
  }

  async function handleDocumentUpload(requirementId: string, file: File) {
    if (!activeApp || isLocked) return;
    setUploadingReqId(requirementId);
    setDocErrors((prev) => ({ ...prev, [requirementId]: '' }));
    try {
      const presigned = await apiRequest<PresignUploadResult>(
        `/api/v1/applicant/applications/${activeApp.application.id}/documents/presign-upload`,
        {
          method: 'POST',
          tenantSlug,
          body: {
            requirement_id: requirementId,
            filename: file.name,
            mime_type: file.type || 'image/jpeg',
            size_bytes: file.size,
          },
        },
      );
      await uploadToPresignedUrl(presigned.upload_url, file, presigned.required_headers);
      await apiRequest<ApplicationDocumentItem>(
        `/api/v1/applicant/applications/${activeApp.application.id}/documents/confirm`,
        {
          method: 'POST',
          tenantSlug,
          body: {
            requirement_id: requirementId,
            storage_key: presigned.storage_key,
            original_filename: file.name,
            mime_type: file.type || 'image/jpeg',
            size_bytes: file.size,
          },
        },
      );
      const fresh = await apiRequest<ApplicationDetail>(
        `/api/v1/applicant/applications/${activeApp.application.id}`,
        { tenantSlug },
      );
      onApplicationUpdated(fresh);
      markSavedTime();
    } catch (err) {
      setDocErrors((prev) => ({
        ...prev,
        [requirementId]: err instanceof Error ? err.message : 'Gagal mengunggah berkas.',
      }));
    } finally {
      setUploadingReqId(null);
    }
  }

  async function handlePreviewDoc(docId: string) {
    if (!activeApp) return;
    try {
      const signed = await apiRequest<PresignedDownloadResult>(
        `/api/v1/applicant/applications/${activeApp.application.id}/documents/${docId}/presign-download`,
        { tenantSlug },
      );
      window.open(signed.url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      setBannerError(err instanceof ApiError ? err.message : 'Gagal membuka berkas.');
    }
  }

  async function handleNext() {
    if (step === 0) {
      if (!(await saveProfileToServer(false))) return;
    } else if (step === 1) {
      if (!(await saveGuardiansToServer())) return;
    } else if (step === 2) {
      if (!(await saveAchievementsToServer())) return;
    } else if (step === 3) {
      if (!(await saveProfileToServer(true))) return;
    }

    if (step < WIZARD_STEPS.length - 1) {
      setStep((s) => s + 1);
    } else {
      setConfirmOpen(true);
    }
  }

  async function handleFinalSubmit() {
    if (!activeApp) return;
    setSubmitBusy(true);
    setBannerError('');
    try {
      const updated = await apiRequest<ApplicationDetail>(
        `/api/v1/applicant/applications/${activeApp.application.id}/submit`,
        {
          method: 'POST',
          tenantSlug,
          idempotencyKey: `submit-${activeApp.application.id}-v${activeApp.application.version}`,
          body: { version: activeApp.application.version },
        },
      );
      setConfirmOpen(false);
      onApplicationUpdated(updated);
      navigate('/app');
    } catch (err) {
      setConfirmOpen(false);
      setBannerError(err instanceof ApiError ? err.message : 'Gagal mengirim pendaftaran.');
    } finally {
      setSubmitBusy(false);
    }
  }

  return (
    <div className={styles.layout}>
      <div className={styles.mobileSteps}>
        <Stepper steps={WIZARD_STEPS} currentIndex={step} compactMobile />
      </div>

      <aside className={styles.desktopSteps}>
        <Card compact>
          <Stepper
            steps={WIZARD_STEPS}
            currentIndex={step}
            onSelectStep={(idx) => setStep(idx)}
          />
        </Card>
      </aside>

      <section className="container-wizard">
        <Card>
          <div style={{ marginBottom: 'var(--space-5)' }}>
            <div style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>
              {activeApp.application.registration_no} · {activeApp.application.unit_name} · {activeApp.application.track_name}
            </div>
            <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
              {WIZARD_STEPS[step].title}
            </h1>
          </div>

          {isLocked && (
            <div style={{ padding: 'var(--space-3)', marginBottom: 'var(--space-4)', borderRadius: 'var(--radius-md)', backgroundColor: 'var(--status-info-bg)', color: 'var(--status-info-fg)', fontWeight: 600 }}>
              Formulir sedang dikunci ({activeApp.application.status}). Anda hanya dapat meninjau data.
            </div>
          )}

          {bannerError && (
            <div style={{ padding: 'var(--space-3)', marginBottom: 'var(--space-4)', borderRadius: 'var(--radius-md)', backgroundColor: 'var(--status-danger-bg)', color: 'var(--status-danger-fg)', fontWeight: 600 }} role="alert">
              {bannerError}
            </div>
          )}

          {step === 0 && (
            <div className={`${styles.fieldGrid} ${styles.fieldGrid2}`}>
              <div style={{ gridColumn: '1 / -1' }}>
                <InputField
                  label={`Nama Lengkap ${term.applicantNoun} (Sesuai Akta)`}
                  value={fullName}
                  onChange={(e) => setFullName(e.target.value)}
                  error={errors.full_name}
                  disabled={isLocked}
                  required
                />
              </div>
              <InputField
                label="NIK (16 Digit)"
                value={nik}
                onChange={(e) => setNik(e.target.value)}
                error={errors.nik}
                disabled={isLocked}
                mono
              />
              <InputField
                label="NISN (10 Digit)"
                value={nisn}
                onChange={(e) => setNisn(e.target.value)}
                error={errors.nisn}
                disabled={isLocked}
                mono
              />
              <InputField
                label="Nomor Kartu Keluarga (16 Digit)"
                value={kkNumber}
                onChange={(e) => setKkNumber(e.target.value)}
                error={errors.kk_number}
                disabled={isLocked}
                mono
              />
              <InputField
                label="Tanggal Terbit KK"
                type="date"
                value={kkIssuedDate}
                onChange={(e) => setKkIssuedDate(e.target.value)}
                error={errors.kk_issued_date}
                helperText="Jalur Domisili SPMB mencatat tanggal terbit KK untuk tie-breaker"
                disabled={isLocked}
              />
              <InputField
                label="Tempat Lahir"
                value={birthPlace}
                onChange={(e) => setBirthPlace(e.target.value)}
                disabled={isLocked}
              />
              <InputField
                label="Tanggal Lahir"
                type="date"
                value={birthDate}
                onChange={(e) => setBirthDate(e.target.value)}
                error={errors.birth_date}
                helperText={
                  activeTrack &&
                  (activeTrack.min_age_months != null || activeTrack.max_age_months != null)
                    ? `Ketentuan usia jalur: ${activeTrack.min_age_months != null ? `min ${activeTrack.min_age_months} bln` : ''}${activeTrack.min_age_months != null && activeTrack.max_age_months != null ? ' · ' : ''}${activeTrack.max_age_months != null ? `maks ${activeTrack.max_age_months} bln` : ''}${activeTrack.age_reference_date ? ` (per ${activeTrack.age_reference_date})` : ''}`
                    : undefined
                }
                disabled={isLocked}
              />
              <div>
                <label htmlFor="gender-select" style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
                  Jenis Kelamin
                </label>
                <select
                  id="gender-select"
                  value={gender}
                  onChange={(e) => setGender(e.target.value)}
                  disabled={isLocked}
                  style={{ width: '100%', padding: '0.6rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-strong)' }}
                >
                  <option value="L">Laki-laki (L)</option>
                  <option value="P">Perempuan (P)</option>
                </select>
              </div>
              {customFormSchema.length > 0 && (
                <div style={{ gridColumn: '1 / -1', marginTop: 'var(--space-2)', paddingTop: 'var(--space-3)', borderTop: '1px solid var(--border-default)' }}>
                  <div style={{ fontWeight: 700, fontSize: 'var(--text-label)', marginBottom: 'var(--space-3)' }}>
                    Pertanyaan Tambahan Khusus {activeApp.application.track_name ?? 'Jalur'}
                  </div>
                  <div className={`${styles.fieldGrid} ${styles.fieldGrid2}`}>
                    {customFormSchema.map((field) => {
                      const val = extraAnswers[field.key];
                      const err = errors[`extra_${field.key}`];
                      if (field.type === 'checkbox') {
                        return (
                          <div key={field.key} style={{ gridColumn: '1 / -1' }}>
                            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 'var(--text-label)' }}>
                              <input
                                type="checkbox"
                                checked={Boolean(val)}
                                disabled={isLocked}
                                onChange={(e) =>
                                  setExtraAnswers((prev) => ({
                                    ...prev,
                                    [field.key]: e.target.checked,
                                  }))
                                }
                              />
                              <span>
                                {field.label} {field.required ? '*' : ''}
                              </span>
                            </label>
                            {err && (
                              <div style={{ color: 'var(--status-danger-fg)', fontSize: 'var(--text-caption)', marginTop: 4 }}>
                                {err}
                              </div>
                            )}
                          </div>
                        );
                      }
                      if (field.type === 'select') {
                        return (
                          <div key={field.key}>
                            <label
                              htmlFor={`extra-${field.key}`}
                              style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}
                            >
                              {field.label} {field.required ? '*' : ''}
                            </label>
                            <select
                              id={`extra-${field.key}`}
                              value={typeof val === 'string' ? val : String(val ?? '')}
                              disabled={isLocked}
                              onChange={(e) =>
                                setExtraAnswers((prev) => ({
                                  ...prev,
                                  [field.key]: e.target.value,
                                }))
                              }
                              style={{ width: '100%', padding: '0.6rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-strong)', backgroundColor: 'var(--bg-surface)' }}
                            >
                              <option value="">-- Pilih --</option>
                              {(field.options ?? []).map((opt) => (
                                <option key={opt} value={opt}>
                                  {opt}
                                </option>
                              ))}
                            </select>
                            {err && (
                              <div style={{ color: 'var(--status-danger-fg)', fontSize: 'var(--text-caption)', marginTop: 4 }}>
                                {err}
                              </div>
                            )}
                          </div>
                        );
                      }
                      if (field.type === 'textarea') {
                        return (
                          <div key={field.key} style={{ gridColumn: '1 / -1' }}>
                            <label
                              htmlFor={`extra-${field.key}`}
                              style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}
                            >
                              {field.label} {field.required ? '*' : ''}
                            </label>
                            <textarea
                              id={`extra-${field.key}`}
                              rows={3}
                              value={typeof val === 'string' ? val : String(val ?? '')}
                              placeholder={field.placeholder ?? ''}
                              disabled={isLocked}
                              onChange={(e) =>
                                setExtraAnswers((prev) => ({
                                  ...prev,
                                  [field.key]: e.target.value,
                                }))
                              }
                              style={{ width: '100%', padding: '0.6rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-strong)' }}
                            />
                            {err && (
                              <div style={{ color: 'var(--status-danger-fg)', fontSize: 'var(--text-caption)', marginTop: 4 }}>
                                {err}
                              </div>
                            )}
                          </div>
                        );
                      }
                      return (
                        <InputField
                          key={field.key}
                          label={`${field.label}${field.required ? ' *' : ''}`}
                          type={field.type === 'number' ? 'number' : field.type === 'date' ? 'date' : 'text'}
                          value={val != null ? String(val) : ''}
                          placeholder={field.placeholder ?? ''}
                          error={err}
                          disabled={isLocked}
                          onChange={(e) =>
                            setExtraAnswers((prev) => ({
                              ...prev,
                              [field.key]:
                                field.type === 'number' && e.target.value !== ''
                                  ? Number(e.target.value)
                                  : e.target.value,
                            }))
                          }
                        />
                      );
                    })}
                  </div>
                </div>
              )}
            </div>
          )}

          {step === 1 && (
            <div className={`${styles.fieldGrid} ${styles.fieldGrid2}`}>
              <InputField
                label="Nama Ayah Kandung"
                value={fatherName}
                onChange={(e) => setFatherName(e.target.value)}
                error={errors.fatherName}
                disabled={isLocked}
              />
              <InputField
                label="Nama Ibu Kandung"
                value={motherName}
                onChange={(e) => setMotherName(e.target.value)}
                disabled={isLocked}
              />
              <div style={{ gridColumn: '1 / -1' }}>
                <InputField
                  label={`Nomor WhatsApp ${term.guardianNoun} (Notifikasi Utama)`}
                  value={guardianPhone}
                  onChange={(e) => setGuardianPhone(e.target.value)}
                  helperText="Pengumuman revisi berkas, jadwal tes, dan kelulusan dikirim ke nomor ini."
                  disabled={isLocked}
                  mono
                />
              </div>
            </div>
          )}

          {step === 2 && (
            <div className={styles.fieldGrid}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
                <div>
                  <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 6 }}>
                    <Award size={18} />
                    Daftar Prestasi & Piagam
                  </h2>
                  <p style={{ fontSize: 'var(--text-caption)', color: 'var(--text-muted)' }}>
                    {activeApp.requires_achievements
                      ? 'Jalur ini mempertimbangkan skor prestasi. Lengkapi minimal satu piagam yang relevan.'
                      : 'Opsional — tambahkan prestasi akademik maupun non-akademik bila dimiliki.'}
                  </p>
                </div>
                {!isLocked && (
                  <Button variant="secondary" size="sm" onClick={addAchievementRow}>
                    <Plus size={14} />
                    Tambah Prestasi
                  </Button>
                )}
              </div>

              {activeApp.requires_achievements && achievements.length === 0 && (
                <div
                  style={{
                    padding: 'var(--space-3)',
                    borderRadius: 'var(--radius-md)',
                    backgroundColor: 'var(--status-warning-bg)',
                    color: 'var(--status-warning-fg)',
                    fontSize: 'var(--text-caption)',
                    fontWeight: 600,
                  }}
                >
                  Jalur pilihan Anda menggunakan bobot prestasi. Tambahkan prestasi di bawah atau unggah piagam pada langkah berikutnya.
                </div>
              )}

              {achievements.length === 0 ? (
                <div
                  style={{
                    padding: 'var(--space-4)',
                    borderRadius: 'var(--radius-md)',
                    border: '1px dashed var(--border-strong)',
                    backgroundColor: 'var(--bg-subtle)',
                    fontSize: 'var(--text-caption)',
                    color: 'var(--text-muted)',
                  }}
                >
                  Belum ada data prestasi yang ditambahkan. Klik <strong>Simpan & Lanjut</strong> jika ingin melewati langkah ini.
                </div>
              ) : (
                achievements.map((item, idx) => (
                  <div
                    key={item.id ?? `ach-${idx}`}
                    style={{
                      padding: 'var(--space-4)',
                      borderRadius: 'var(--radius-md)',
                      border: '1px solid var(--border-default)',
                      backgroundColor: 'var(--bg-subtle)',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 'var(--space-3)',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <strong style={{ fontSize: 'var(--text-body)' }}>Prestasi #{idx + 1}</strong>
                      {!isLocked && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => removeAchievementRow(idx)}
                          aria-label={`Hapus prestasi ${idx + 1}`}
                        >
                          <Trash2 size={14} />
                          Hapus
                        </Button>
                      )}
                    </div>

                    <div className={`${styles.fieldGrid} ${styles.fieldGrid2}`}>
                      <div>
                        <label htmlFor={`ach-cat-${idx}`} style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
                          Kategori
                        </label>
                        <select
                          id={`ach-cat-${idx}`}
                          value={item.category}
                          disabled={isLocked}
                          onChange={(e) => updateAchievementRow(idx, { category: e.target.value })}
                          style={{ width: '100%', padding: '0.6rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-strong)', backgroundColor: 'var(--bg-surface)' }}
                        >
                          <option value="akademik">Akademik / OSN</option>
                          <option value="olahraga">Olahraga / O2SN</option>
                          <option value="seni">Seni / FLS2N</option>
                          <option value="keagamaan">Keagamaan / Tahfidz / MTQ</option>
                          <option value="organisasi">Organisasi / Pramuka</option>
                        </select>
                      </div>
                      <div>
                        <label htmlFor={`ach-lvl-${idx}`} style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
                          Tingkat
                        </label>
                        <select
                          id={`ach-lvl-${idx}`}
                          value={item.level}
                          disabled={isLocked}
                          onChange={(e) => updateAchievementRow(idx, { level: e.target.value })}
                          style={{ width: '100%', padding: '0.6rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-strong)', backgroundColor: 'var(--bg-surface)' }}
                        >
                          <option value="kota">Kabupaten / Kota</option>
                          <option value="provinsi">Provinsi</option>
                          <option value="nasional">Nasional</option>
                          <option value="internasional">Internasional</option>
                        </select>
                      </div>
                      <InputField
                        label="Nama Kejuaraan / Capaian"
                        value={item.title}
                        onChange={(e) => updateAchievementRow(idx, { title: e.target.value })}
                        placeholder="Misal: Juara 1 OSN Matematika"
                        disabled={isLocked}
                      />
                      <InputField
                        label="Peringkat / Predikat"
                        value={item.rank_achieved ?? ''}
                        onChange={(e) => updateAchievementRow(idx, { rank_achieved: e.target.value })}
                        placeholder="Juara 1 / Medali Emas"
                        disabled={isLocked}
                      />
                      <InputField
                        label="Tahun Perolehan"
                        type="number"
                        value={item.achievement_year != null ? String(item.achievement_year) : ''}
                        onChange={(e) =>
                          updateAchievementRow(idx, {
                            achievement_year: e.target.value ? Number(e.target.value) : null,
                          })
                        }
                        placeholder="2025"
                        disabled={isLocked}
                      />
                      <InputField
                        label="Penyelenggara"
                        value={item.organizer ?? ''}
                        onChange={(e) => updateAchievementRow(idx, { organizer: e.target.value })}
                        placeholder="Kemendikbud / Kemenag / KONI"
                        disabled={isLocked}
                      />
                      <InputField
                        label="Poin Klaim (0-100)"
                        type="number"
                        value={String(item.raw_points)}
                        onChange={(e) =>
                          updateAchievementRow(idx, { raw_points: Number(e.target.value) || 0 })
                        }
                        disabled={isLocked}
                        mono
                      />
                    </div>
                  </div>
                ))
              )}
            </div>
          )}

          {step === 3 && (
            <div className={styles.fieldGrid}>
              <InputField
                label="Alamat Jalan / RT / RW"
                value={addressStreet}
                onChange={(e) => setAddressStreet(e.target.value)}
                disabled={isLocked}
              />
              <Suspense fallback={<div className={styles.mapFallback}>Memuat peta OpenStreetMap...</div>}>
                <LazyLocationPickerMap
                  value={coords}
                  schoolCoords={schoolCoords}
                  schoolName={activeApp.application.unit_name}
                  onChange={(next) => {
                    if (!isLocked) setCoords(next);
                  }}
                />
              </Suspense>
              {activeApp.application.distance_meters != null && (
                <div style={{ fontSize: 'var(--text-caption)', color: 'var(--status-success-fg)', fontWeight: 600 }}>
                  Jarak Resmi Tersimpan di Server (Haversine): {activeApp.application.distance_meters} meter
                </div>
              )}
            </div>
          )}

          {step === 4 && (
            <div className={styles.fieldGrid}>
              {activeApp.requirements.map((req) => {
                const uploaded = activeApp.documents.find((d) => d.requirement_id === req.id);
                return (
                  <DocumentUploader
                    key={req.id}
                    label={req.label}
                    required={req.is_required}
                    status={uploaded?.status ?? 'pending'}
                    reviewNote={uploaded?.review_note ?? undefined}
                    initialFileName={uploaded?.original_filename}
                    initialSizeKb={uploaded ? Math.max(1, Math.round(uploaded.size_bytes / 1024)) : undefined}
                    uploading={uploadingReqId === req.id}
                    uploadError={docErrors[req.id]}
                    onPreviewClick={uploaded ? () => void handlePreviewDoc(uploaded.id) : undefined}
                    onFilePrepared={(file) => handleDocumentUpload(req.id, file)}
                  />
                );
              })}
            </div>
          )}

          {step === 5 && (
            <div className={styles.fieldGrid}>
              <p style={{ color: 'var(--text-secondary)' }}>
                Periksa kembali ringkasan data sebelum dikirim ke panitia verifikasi.
              </p>
              <div style={{ padding: 'var(--space-4)', borderRadius: 'var(--radius-md)', backgroundColor: 'var(--bg-subtle)', display: 'flex', flexDirection: 'column', gap: '4px' }}>
                <div><strong>Nama:</strong> {fullName}</div>
                <div className="mono-code"><strong>NIK:</strong> {nik || '-'}</div>
                <div className="mono-code"><strong>NISN:</strong> {nisn || '-'}</div>
                <div className="mono-code"><strong>Koordinat:</strong> {coords.latitude}, {coords.longitude}</div>
                <div>
                  <strong>Prestasi Diklaim:</strong> {activeApp.achievements.length} item
                  {activeApp.achievements.length > 0
                    ? ` (${activeApp.achievements.map((a) => a.title).join(', ')})`
                    : ''}
                </div>
                <div>
                  <strong>Berkas Terunggah:</strong> {activeApp.documents.length} dokumen
                  {activeApp.application.distance_meters != null ? ` · Jarak resmi: ${activeApp.application.distance_meters} m` : ''}
                </div>
                {customFormSchema.length > 0 && (
                  <div style={{ marginTop: 4, paddingTop: 4, borderTop: '1px dashed var(--border-strong)' }}>
                    <strong>Jawaban Pertanyaan Tambahan:</strong>
                    {customFormSchema.map((field) => {
                      const v = extraAnswers[field.key];
                      const displayVal =
                        typeof v === 'boolean' ? (v ? 'Ya' : 'Tidak') : v != null && String(v) !== '' ? String(v) : '-';
                      return (
                        <div key={field.key} style={{ fontSize: 'var(--text-caption)' }}>
                          {field.label}: <strong>{displayVal}</strong>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          )}

          <div className={styles.actionBar}>
            <Button
              variant="secondary"
              disabled={step === 0 || saving}
              onClick={() => setStep((s) => Math.max(0, s - 1))}
            >
              Kembali
            </Button>
            {!isLocked && (
              <Button disabled={saving} onClick={() => void handleNext()}>
                {saving
                  ? 'Menyimpan...'
                  : step === WIZARD_STEPS.length - 1
                    ? 'Kirim Pendaftaran Final'
                    : 'Simpan & Lanjut'}
              </Button>
            )}
          </div>
        </Card>
      </section>

      <Modal
        open={confirmOpen}
        title="Konfirmasi Kirim Pendaftaran"
        onClose={() => setConfirmOpen(false)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirmOpen(false)} disabled={submitBusy}>
              Periksa Lagi
            </Button>
            <Button onClick={() => void handleFinalSubmit()} disabled={submitBusy}>
              {submitBusy ? 'Mengirim...' : 'Ya, Kirim Sekarang'}
            </Button>
          </>
        }
      >
        <p>
          Setelah dikirim, data masuk ke antrean pemeriksaan panitia dan hanya dapat diubah kembali
          jika panitia membuka status <strong>Perlu Perbaikan</strong>. Lanjutkan?
        </p>
      </Modal>
    </div>
  );
}
