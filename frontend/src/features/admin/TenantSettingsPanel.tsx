import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  Building2,
  CalendarPlus,
  CreditCard,
  FileText,
  Globe,
  Pencil,
  Plus,
  Save,
  Trash2,
  Upload,
} from 'lucide-react';
import {
  apiRequest,
  extractErrorMessage,
  formatDateTime,
  formatRupiah,
  uploadToPresignedUrl,
} from '@/core/api/client';
import { useAuth } from '@/core/auth/AuthContext';
import type {
  AdminTenantSettings,
  CustomFormField,
  DomainVerificationResult,
  EducationLevel,
  PeriodSummary,
  PresignUploadResult,
  PublicPortalPayload,
  SelectionMode,
  TerminologyPreset,
  TrackRequirementItem,
  TrackSummary,
  UnitSummary,
} from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { InputField } from '@/components/ui/InputField';
import { Modal } from '@/components/ui/Modal';
import { ToneBadge } from '@/components/ui/StatusBadge';
import adminStyles from './AdminShared.module.css';
import styles from './TenantSettingsPanel.module.css';

const EDUCATION_LEVELS: Array<{ value: EducationLevel; label: string }> = [
  { value: 'ra_tk', label: 'RA / TK' },
  { value: 'sd_mi', label: 'SD / MI' },
  { value: 'smp_mts', label: 'SMP / MTs' },
  { value: 'sma_ma', label: 'SMA / MA' },
  { value: 'smk_mak', label: 'SMK / MAK' },
  { value: 'pesantren', label: 'Pesantren / Ma\'had' },
];

const SELECTION_MODES: Array<{ value: SelectionMode; label: string }> = [
  { value: 'rule_based', label: 'rule_based (Zonasi/Afirmasi/Nilai Otomatis)' },
  { value: 'test_based', label: 'test_based (Tes/Wawancara Panitia)' },
  { value: 'hybrid', label: 'hybrid (Gabungan Aturan & Tes)' },
  { value: 'direct', label: 'direct (Penerimaan Langsung)' },
];

function toLocalDateTimeInput(iso?: string | null): string {
  if (!iso) return '';
  const dt = new Date(iso);
  if (Number.isNaN(dt.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}T${pad(dt.getHours())}:${pad(dt.getMinutes())}`;
}

function toRFC3339(localInput: string): string {
  const dt = new Date(localInput);
  return Number.isNaN(dt.getTime()) ? localInput : dt.toISOString();
}

interface EditableRequirementRow {
  doc_code: string;
  label: string;
  description: string;
  is_required: boolean;
  accepted_mimes_csv: string;
  max_size_kb: number;
}

interface EditableWeightRow {
  code: string;
  pct: string;
}

interface EditableCustomFieldRow {
  key: string;
  label: string;
  type: 'text' | 'textarea' | 'number' | 'select' | 'date' | 'checkbox';
  required: boolean;
  placeholder: string;
  optionsCsv: string;
}

export function TenantSettingsPanel({
  tenantSlug,
  portal,
  activeUnitId,
  onPortalChanged,
}: {
  tenantSlug: string;
  portal: PublicPortalPayload;
  activeUnitId: string;
  onPortalChanged: () => void | Promise<void>;
}) {
  const { hasAnyRole } = useAuth();
  const canReadSettings = hasAnyRole('tenant_admin', 'unit_admin', 'principal');
  const isTenantAdmin = hasAnyRole('tenant_admin');
  const canManageSetup = hasAnyRole('tenant_admin', 'unit_admin', 'principal');
  const canManageUnitsOrReqs = hasAnyRole('tenant_admin', 'unit_admin');

  const [settings, setSettings] = useState<AdminTenantSettings | null>(null);
  const [adminUnits, setAdminUnits] = useState<UnitSummary[]>([]);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  // Tenant white-label form state
  const [tenantName, setTenantName] = useState(portal.tenant.name);
  const [preset, setPreset] = useState<TerminologyPreset>(portal.tenant.terminology_preset);
  const [primaryColor, setPrimaryColor] = useState(portal.tenant.theme_config.primary_color);
  const [logoUrl, setLogoUrl] = useState(portal.tenant.theme_config.logo_url ?? '');
  const [tagline, setTagline] = useState(portal.tenant.theme_config.tagline ?? '');
  const [customDomain, setCustomDomain] = useState('');
  const [midtransMerchantId, setMidtransMerchantId] = useState('');
  const [midtransClientKey, setMidtransClientKey] = useState('');
  const [midtransServerKeyInput, setMidtransServerKeyInput] = useState('');
  const [midtransSandbox, setMidtransSandbox] = useState(true);
  const [uploadingLogo, setUploadingLogo] = useState(false);
  const [verifyingDomain, setVerifyingDomain] = useState(false);
  const [domainCheckResult, setDomainCheckResult] = useState<DomainVerificationResult | null>(null);
  const [savingTenant, setSavingTenant] = useState(false);

  // Unit Modal state
  const [unitModalOpen, setUnitModalOpen] = useState(false);
  const [editingUnit, setEditingUnit] = useState<UnitSummary | null>(null);
  const [unitCode, setUnitCode] = useState('');
  const [unitName, setUnitName] = useState('');
  const [unitLevel, setUnitLevel] = useState<EducationLevel>('smp_mts');
  const [unitAddress, setUnitAddress] = useState('');
  const [unitLat, setUnitLat] = useState('');
  const [unitLng, setUnitLng] = useState('');
  const [unitActive, setUnitActive] = useState(true);
  const [savingUnit, setSavingUnit] = useState(false);

  // Period Modal state
  const [periodModalOpen, setPeriodModalOpen] = useState(false);
  const [editingPeriod, setEditingPeriod] = useState<PeriodSummary | null>(null);
  const [periodUnitId, setPeriodUnitId] = useState(activeUnitId || portal.units[0]?.id || '');
  const [academicYear, setAcademicYear] = useState('2026/2027');
  const [periodName, setPeriodName] = useState('');
  const [opensAt, setOpensAt] = useState('');
  const [closesAt, setClosesAt] = useState('');
  const [periodPublished, setPeriodPublished] = useState(true);
  const [periodActive, setPeriodActive] = useState(true);
  const [savingPeriod, setSavingPeriod] = useState(false);

  // Track Modal state
  const [trackModalOpen, setTrackModalOpen] = useState(false);
  const [editingTrack, setEditingTrack] = useState<TrackSummary | null>(null);
  const [trackPeriodId, setTrackPeriodId] = useState(portal.periods[0]?.id || '');
  const [trackCode, setTrackCode] = useState('');
  const [trackDisplayName, setTrackDisplayName] = useState('');
  const [trackDescription, setTrackDescription] = useState('');
  const [trackSelectionMode, setTrackSelectionMode] = useState<SelectionMode>('rule_based');
  const [trackQuota, setTrackQuota] = useState('64');
  const [trackRegFee, setTrackRegFee] = useState('150000');
  const [trackReEnrollFee, setTrackReEnrollFee] = useState('1500000');
  const [trackRequiresCoords, setTrackRequiresCoords] = useState(false);
  const [trackRequiresAch, setTrackRequiresAch] = useState(false);
  const [trackMinAgeMonths, setTrackMinAgeMonths] = useState('');
  const [trackMaxAgeMonths, setTrackMaxAgeMonths] = useState('');
  const [trackAgeRefDate, setTrackAgeRefDate] = useState('');
  const [trackWeights, setTrackWeights] = useState<EditableWeightRow[]>([]);
  const [trackCustomFields, setTrackCustomFields] = useState<EditableCustomFieldRow[]>([]);
  const [trackActive, setTrackActive] = useState(true);
  const [savingTrack, setSavingTrack] = useState(false);

  // Requirements Modal state
  const [reqTargetTrack, setReqTargetTrack] = useState<TrackSummary | null>(null);
  const [reqRows, setReqRows] = useState<EditableRequirementRow[]>([]);
  const [savingReqs, setSavingReqs] = useState(false);

  const loadAdminMetadata = useCallback(async () => {
    if (!canReadSettings) {
      setSettings(null);
      setAdminUnits([]);
      return;
    }
    try {
      const [s, units] = await Promise.all([
        apiRequest<AdminTenantSettings>('/api/v1/admin/tenant/settings', { tenantSlug }),
        apiRequest<UnitSummary[]>('/api/v1/admin/units', { tenantSlug }),
      ]);
      setSettings(s);
      setAdminUnits(units);
      setTenantName(s.name);
      setPreset(s.terminology_preset);
      setPrimaryColor(s.theme_config.primary_color);
      setLogoUrl(s.theme_config.logo_url ?? '');
      setTagline(s.theme_config.tagline ?? '');
      setCustomDomain(s.custom_domain ?? '');
      setMidtransMerchantId(s.payment_config?.midtrans_merchant_id ?? '');
      setMidtransClientKey(s.payment_config?.midtrans_client_key ?? '');
      setMidtransServerKeyInput('');
      setMidtransSandbox(s.payment_config?.midtrans_sandbox ?? true);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memuat konfigurasi instansi.'));
    }
  }, [canReadSettings, tenantSlug]);

  useEffect(() => {
    void loadAdminMetadata();
  }, [loadAdminMetadata]);

  useEffect(() => {
    if (activeUnitId) setPeriodUnitId(activeUnitId);
  }, [activeUnitId]);

  const info = settings ?? portal.tenant;
  const displayedUnits = adminUnits.length > 0 ? adminUnits : portal.units;

  async function handleSaveTenant(e: FormEvent) {
    e.preventDefault();
    if (!isTenantAdmin) return;
    setSavingTenant(true);
    setError('');
    setNotice('');
    try {
      const paymentConfigBody: Record<string, unknown> = {
        midtrans_merchant_id: midtransMerchantId.trim(),
        midtrans_client_key: midtransClientKey.trim(),
        midtrans_sandbox: midtransSandbox,
      };
      if (midtransServerKeyInput.trim()) {
        paymentConfigBody.midtrans_server_key = midtransServerKeyInput.trim();
      }
      const updated = await apiRequest<AdminTenantSettings>('/api/v1/admin/tenant/settings', {
        method: 'PATCH',
        tenantSlug,
        body: {
          name: tenantName.trim(),
          terminology_preset: preset,
          theme_config: {
            primary_color: primaryColor.trim(),
            logo_url: logoUrl.trim() || null,
            tagline: tagline.trim() || null,
          },
          custom_domain: customDomain.trim() || null,
          payment_config: paymentConfigBody,
        },
      });
      setSettings(updated);
      setMidtransServerKeyInput('');
      setDomainCheckResult(null);
      setNotice('Pengaturan white-label, domain, dan gateway pembayaran berhasil diperbarui.');
      await onPortalChanged();
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memperbarui pengaturan instansi.'));
    } finally {
      setSavingTenant(false);
    }
  }

  async function handleLogoUpload(file: File | undefined) {
    if (!file || !isTenantAdmin) return;
    setUploadingLogo(true);
    setError('');
    setNotice('');
    try {
      const presigned = await apiRequest<PresignUploadResult>('/api/v1/admin/tenant/logo/presign', {
        method: 'POST',
        tenantSlug,
        body: {
          filename: file.name,
          mime_type: file.type || 'image/png',
          size_bytes: file.size,
        },
      });
      await uploadToPresignedUrl(presigned.upload_url, file, presigned.required_headers);
      const updated = await apiRequest<AdminTenantSettings>('/api/v1/admin/tenant/logo/confirm', {
        method: 'POST',
        tenantSlug,
        body: { storage_key: presigned.storage_key },
      });
      setSettings(updated);
      setLogoUrl(updated.theme_config.logo_url ?? '');
      setNotice('Logo instansi berhasil diunggah ke storage dan diaktifkan.');
      await onPortalChanged();
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal mengunggah logo instansi.'));
    } finally {
      setUploadingLogo(false);
    }
  }

  async function handleVerifyDns() {
    if (!isTenantAdmin) return;
    setVerifyingDomain(true);
    setError('');
    setNotice('');
    try {
      const res = await apiRequest<{
        verification: DomainVerificationResult;
        settings?: AdminTenantSettings;
      }>('/api/v1/admin/tenant/domain/verify', {
        method: 'POST',
        tenantSlug,
      });
      setDomainCheckResult(res.verification);
      if (res.settings) {
        setSettings(res.settings);
      }
      if (res.verification.verified) {
        setNotice(`Verifikasi DNS berhasil: ${res.verification.message}`);
        await onPortalChanged();
      } else {
        setError(`Verifikasi DNS belum lolos: ${res.verification.message}`);
      }
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal menjalankan verifikasi DNS domain kustom.'));
    } finally {
      setVerifyingDomain(false);
    }
  }

  function openCreateUnit() {
    setEditingUnit(null);
    setUnitCode('');
    setUnitName('');
    setUnitLevel('smp_mts');
    setUnitAddress('');
    setUnitLat('');
    setUnitLng('');
    setUnitActive(true);
    setUnitModalOpen(true);
  }

  function openEditUnit(u: UnitSummary) {
    setEditingUnit(u);
    setUnitCode(u.code);
    setUnitName(u.name);
    setUnitLevel((u.education_level as EducationLevel) || 'smp_mts');
    setUnitAddress(u.address ?? '');
    setUnitLat(u.latitude != null ? String(u.latitude) : '');
    setUnitLng(u.longitude != null ? String(u.longitude) : '');
    setUnitActive(u.is_active ?? true);
    setUnitModalOpen(true);
  }

  async function handleSubmitUnit(e: FormEvent) {
    e.preventDefault();
    setSavingUnit(true);
    setError('');
    setNotice('');
    try {
      const lat = unitLat.trim() ? Number(unitLat) : null;
      const lng = unitLng.trim() ? Number(unitLng) : null;
      if (editingUnit) {
        await apiRequest<UnitSummary>(`/api/v1/admin/units/${editingUnit.id}`, {
          method: 'PATCH',
          tenantSlug,
          body: {
            name: unitName.trim(),
            education_level: unitLevel,
            address: unitAddress.trim() || null,
            latitude: lat,
            longitude: lng,
            is_active: unitActive,
          },
        });
        setNotice(`Unit "${unitName.trim()}" berhasil diperbarui.`);
      } else {
        await apiRequest<UnitSummary>('/api/v1/admin/units', {
          method: 'POST',
          tenantSlug,
          body: {
            code: unitCode.trim().toUpperCase(),
            name: unitName.trim(),
            education_level: unitLevel,
            address: unitAddress.trim() || null,
            latitude: lat,
            longitude: lng,
          },
        });
        setNotice(`Unit baru "${unitName.trim()}" berhasil ditambahkan.`);
      }
      setUnitModalOpen(false);
      await Promise.all([loadAdminMetadata(), onPortalChanged()]);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal menyimpan data unit pendidikan.'));
    } finally {
      setSavingUnit(false);
    }
  }

  function openCreatePeriod() {
    const now = new Date();
    const nextMonth = new Date(now.getTime() + 60 * 24 * 60 * 60 * 1000);
    setEditingPeriod(null);
    setPeriodUnitId(activeUnitId || displayedUnits[0]?.id || '');
    setAcademicYear('2026/2027');
    setPeriodName('Gelombang Baru');
    setOpensAt(toLocalDateTimeInput(now.toISOString()));
    setClosesAt(toLocalDateTimeInput(nextMonth.toISOString()));
    setPeriodPublished(true);
    setPeriodActive(true);
    setPeriodModalOpen(true);
  }

  function openEditPeriod(p: PeriodSummary) {
    setEditingPeriod(p);
    setPeriodUnitId(p.unit_id);
    setAcademicYear(p.academic_year);
    setPeriodName(p.name);
    setOpensAt(toLocalDateTimeInput(p.opens_at));
    setClosesAt(toLocalDateTimeInput(p.closes_at));
    setPeriodPublished(p.is_published ?? true);
    setPeriodActive(p.is_active ?? true);
    setPeriodModalOpen(true);
  }

  async function handleSubmitPeriod(e: FormEvent) {
    e.preventDefault();
    setSavingPeriod(true);
    setError('');
    setNotice('');
    try {
      if (editingPeriod) {
        await apiRequest(`/api/v1/admin/periods/${editingPeriod.id}`, {
          method: 'PATCH',
          tenantSlug,
          body: {
            academic_year: academicYear.trim(),
            name: periodName.trim(),
            opens_at: toRFC3339(opensAt),
            closes_at: toRFC3339(closesAt),
            is_published: periodPublished,
            is_active: periodActive,
          },
        });
        setNotice(`Periode "${periodName.trim()}" berhasil diperbarui.`);
      } else {
        await apiRequest('/api/v1/admin/periods', {
          method: 'POST',
          tenantSlug,
          body: {
            unit_id: periodUnitId,
            academic_year: academicYear.trim(),
            name: periodName.trim(),
            opens_at: toRFC3339(opensAt),
            closes_at: toRFC3339(closesAt),
            is_published: periodPublished,
          },
        });
        setNotice(`Periode baru "${periodName.trim()}" berhasil dibuat.`);
      }
      setPeriodModalOpen(false);
      await onPortalChanged();
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal menyimpan periode pendaftaran.'));
    } finally {
      setSavingPeriod(false);
    }
  }

  function openCreateTrack(defaultPeriodId?: string) {
    setEditingTrack(null);
    setTrackPeriodId(defaultPeriodId || portal.periods[0]?.id || '');
    setTrackCode('');
    setTrackDisplayName('');
    setTrackDescription('');
    setTrackSelectionMode('rule_based');
    setTrackQuota('40');
    setTrackRegFee('150000');
    setTrackReEnrollFee('1500000');
    setTrackRequiresCoords(false);
    setTrackRequiresAch(false);
    setTrackMinAgeMonths('');
    setTrackMaxAgeMonths('');
    setTrackAgeRefDate('');
    setTrackWeights([]);
    setTrackCustomFields([]);
    setTrackActive(true);
    setTrackModalOpen(true);
  }

  function openEditTrack(t: TrackSummary, fallbackPeriodId: string) {
    setEditingTrack(t);
    setTrackPeriodId(t.period_id || fallbackPeriodId);
    setTrackCode(t.code);
    setTrackDisplayName(t.display_name);
    setTrackDescription(t.description ?? '');
    setTrackSelectionMode(t.selection_mode);
    setTrackQuota(String(t.quota));
    setTrackRegFee(String(t.registration_fee_idr));
    setTrackReEnrollFee(String(t.re_enrollment_fee_idr ?? 0));
    setTrackRequiresCoords(t.requires_coordinates);
    setTrackRequiresAch(t.requires_achievements);
    setTrackMinAgeMonths(t.min_age_months != null ? String(t.min_age_months) : '');
    setTrackMaxAgeMonths(t.max_age_months != null ? String(t.max_age_months) : '');
    setTrackAgeRefDate(t.age_reference_date ?? '');
    setTrackWeights(
      t.scoring_weights
        ? Object.entries(t.scoring_weights).map(([code, pct]) => ({
          code,
          pct: String(pct),
        }))
        : [],
    );
    setTrackCustomFields(
      (t.custom_form_schema ?? []).map((f) => ({
        key: f.key,
        label: f.label,
        type: (f.type as EditableCustomFieldRow['type']) || 'text',
        required: Boolean(f.required),
        placeholder: f.placeholder ?? '',
        optionsCsv: (f.options ?? []).join(', '),
      })),
    );
    setTrackActive(t.is_active ?? true);
    setTrackModalOpen(true);
  }

  async function handleSubmitTrack(e: FormEvent) {
    e.preventDefault();
    setSavingTrack(true);
    setError('');
    setNotice('');
    try {
      const quota = Math.max(0, parseInt(trackQuota, 10) || 0);
      const regFee = Math.max(0, parseInt(trackRegFee, 10) || 0);
      const reEnrollFee = Math.max(0, parseInt(trackReEnrollFee, 10) || 0);
      const minAge = trackMinAgeMonths.trim() ? parseInt(trackMinAgeMonths, 10) : null;
      const maxAge = trackMaxAgeMonths.trim() ? parseInt(trackMaxAgeMonths, 10) : null;
      const ageRefDate = trackAgeRefDate.trim() || null;

      const scoringWeights: Record<string, number> = {};
      for (const row of trackWeights) {
        const key = row.code.trim().toUpperCase();
        const val = Number(row.pct);
        if (key && !Number.isNaN(val) && val > 0) {
          scoringWeights[key] = val;
        }
      }

      const customFormSchema: CustomFormField[] = trackCustomFields
        .filter((f) => f.key.trim() !== '' && f.label.trim() !== '')
        .map((f) => ({
          key: f.key.trim().toLowerCase().replace(/[^a-z0-9_]+/g, '_'),
          label: f.label.trim(),
          type: f.type,
          required: f.required,
          placeholder: f.placeholder.trim() || undefined,
          options:
            f.type === 'select'
              ? f.optionsCsv
                .split(',')
                .map((s) => s.trim())
                .filter(Boolean)
              : undefined,
        }));

      if (editingTrack) {
        await apiRequest(`/api/v1/admin/tracks/${editingTrack.id}`, {
          method: 'PATCH',
          tenantSlug,
          body: {
            display_name: trackDisplayName.trim(),
            description: trackDescription.trim() || null,
            selection_mode: trackSelectionMode,
            quota,
            registration_fee_idr: regFee,
            re_enrollment_fee_idr: reEnrollFee,
            requires_coordinates: trackRequiresCoords,
            requires_achievements: trackRequiresAch,
            min_age_months: minAge,
            max_age_months: maxAge,
            age_reference_date: ageRefDate,
            scoring_weights: scoringWeights,
            custom_form_schema: customFormSchema,
            is_active: trackActive,
          },
        });
        setNotice(`Jalur "${trackDisplayName.trim()}" berhasil diperbarui.`);
      } else {
        await apiRequest('/api/v1/admin/tracks', {
          method: 'POST',
          tenantSlug,
          body: {
            period_id: trackPeriodId,
            code: trackCode.trim().toUpperCase(),
            display_name: trackDisplayName.trim(),
            description: trackDescription.trim() || null,
            selection_mode: trackSelectionMode,
            quota,
            registration_fee_idr: regFee,
            re_enrollment_fee_idr: reEnrollFee,
            requires_coordinates: trackRequiresCoords,
            requires_achievements: trackRequiresAch,
            min_age_months: minAge,
            max_age_months: maxAge,
            age_reference_date: ageRefDate,
            scoring_weights: scoringWeights,
            custom_form_schema: customFormSchema,
          },
        });
        setNotice(`Jalur baru "${trackDisplayName.trim()}" berhasil ditambahkan.`);
      }
      setTrackModalOpen(false);
      await onPortalChanged();
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal menyimpan konfigurasi jalur.'));
    } finally {
      setSavingTrack(false);
    }
  }

  function openRequirementsModal(track: TrackSummary) {
    setReqTargetTrack(track);
    setReqRows(
      (track.requirements ?? []).map((r: TrackRequirementItem) => ({
        doc_code: r.doc_code,
        label: r.label,
        description: r.description ?? '',
        is_required: r.is_required,
        accepted_mimes_csv: (r.accepted_mimes ?? []).join(', '),
        max_size_kb: r.max_size_kb,
      })),
    );
  }

  async function handleSaveRequirements(e: FormEvent) {
    e.preventDefault();
    if (!reqTargetTrack) return;
    setSavingReqs(true);
    setError('');
    setNotice('');
    try {
      const normalized = reqRows.map((row) => ({
        doc_code: row.doc_code.trim().toUpperCase(),
        label: row.label.trim(),
        description: row.description.trim() || null,
        is_required: row.is_required,
        accepted_mimes: row.accepted_mimes_csv
          .split(',')
          .map((m) => m.trim())
          .filter(Boolean),
        max_size_kb: Math.max(100, Number(row.max_size_kb) || 2048),
      }));
      await apiRequest(`/api/v1/admin/tracks/${reqTargetTrack.id}/requirements`, {
        method: 'PUT',
        tenantSlug,
        body: { requirements: normalized },
      });
      setNotice(`Syarat berkas jalur "${reqTargetTrack.display_name}" berhasil diperbarui.`);
      setReqTargetTrack(null);
      await onPortalChanged();
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memperbarui syarat berkas jalur.'));
    } finally {
      setSavingReqs(false);
    }
  }

  return (
    <div className={adminStyles.stack}>
      <div className={adminStyles.headerRow}>
        <div>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
            Pengaturan Instansi &amp; Jalur
          </h1>
          <p className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
            Konfigurasi white-label tenant, unit sekolah/madrasah, kuota jalur, syarat berkas, dan jadwal seleksi aktif.
          </p>
        </div>
      </div>

      {!canReadSettings && (
        <div className={adminStyles.alertInfo}>
          Detail konfigurasi administratif hanya dapat diakses oleh Kepala Sekolah atau Admin
          Instansi/Unit. Ringkasan portal publik tetap ditampilkan di bawah.
        </div>
      )}

      {error && (
        <div role="alert" className={adminStyles.alertError}>
          {error}
        </div>
      )}
      {notice && <div className={adminStyles.alertSuccess}>{notice}</div>}

      <Card>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)', marginBottom: 'var(--space-3)' }}>
          <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>Profil &amp; White-Label Instansi</h2>
          <ToneBadge tone="info" label={`Preset: ${info.terminology_preset.toUpperCase()}`} />
        </div>

        <div className={adminStyles.tableWrap}>
          <table className={adminStyles.table}>
            <tbody>
              <tr>
                <th>Nama Instansi</th>
                <td style={{ fontWeight: 600 }}>{info.name}</td>
              </tr>
              <tr>
                <th>Slug Tenant</th>
                <td className="mono-code">{info.slug}</td>
              </tr>
              <tr>
                <th>Jenis &amp; Terminologi</th>
                <td>
                  {info.institution_type} · <span className="mono-code">{info.terminology_preset}</span>
                </td>
              </tr>
              <tr>
                <th>Warna Merek</th>
                <td style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span
                    style={{
                      display: 'inline-block',
                      width: 16,
                      height: 16,
                      borderRadius: 4,
                      backgroundColor: info.theme_config.primary_color,
                      border: '1px solid var(--border-strong)',
                    }}
                  />
                  <span className="mono-code">{info.theme_config.primary_color}</span>
                </td>
              </tr>
              <tr>
                <th>Tagline</th>
                <td>{info.theme_config.tagline || '-'}</td>
              </tr>
              {settings?.custom_domain && (
                <tr>
                  <th>Custom Domain &amp; DNS</th>
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                      <span className="mono-code">{settings.custom_domain}</span>
                      <ToneBadge
                        tone={settings.domain_verified ? 'success' : 'warning'}
                        label={settings.domain_verified ? 'DNS Terverifikasi' : 'Menunggu Verifikasi DNS'}
                      />
                    </div>
                    {settings.domain_verification_token && (
                      <div className={`mono-code ${adminStyles.metaMuted}`} style={{ fontSize: 12, marginTop: 4 }}>
                        TXT Record: _spmb-verify.{settings.custom_domain} = {settings.domain_verification_token}
                      </div>
                    )}
                  </td>
                </tr>
              )}
              {settings?.payment_config && (
                <tr>
                  <th>Gateway Midtrans Instansi</th>
                  <td>
                    {settings.payment_config.midtrans_server_key_configured ? (
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                        <ToneBadge
                          tone="success"
                          label={`Kredensial Aktif (${settings.payment_config.midtrans_sandbox ? 'Sandbox' : 'Produksi'})`}
                        />
                        {settings.payment_config.midtrans_server_key_masked && (
                          <span className="mono-code">{settings.payment_config.midtrans_server_key_masked}</span>
                        )}
                      </div>
                    ) : (
                      <span className={adminStyles.metaMuted}>
                        Menggunakan fallback konfigurasi platform / transfer manual
                      </span>
                    )}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {isTenantAdmin && (
          <form onSubmit={(e) => void handleSaveTenant(e)} style={{ marginTop: 'var(--space-4)', paddingTop: 'var(--space-4)', borderTop: '1px solid var(--border-default)' }}>
            <h3 style={{ fontSize: 'var(--text-label)', fontWeight: 700, marginBottom: 'var(--space-3)' }}>
              Ubah Konfigurasi White-Label (Khusus Admin Instansi)
            </h3>
            <div className={`${styles.formGrid} ${styles.formGrid2}`}>
              <InputField
                label="Nama Instansi"
                value={tenantName}
                onChange={(e) => setTenantName(e.target.value)}
                required
              />
              <div className={styles.fieldBlock}>
                <label htmlFor="tenant-preset" className={styles.fieldLabel}>
                  Preset Terminologi
                </label>
                <select
                  id="tenant-preset"
                  className={styles.input}
                  value={preset}
                  onChange={(e) => setPreset(e.target.value as TerminologyPreset)}
                >
                  <option value="spmb">SPMB (Sekolah Umum / Kemendikdasmen)</option>
                  <option value="pmbm">PMBM (Madrasah / Kemenag)</option>
                  <option value="mandiri">Mandiri / Yayasan / Pesantren</option>
                </select>
              </div>
              <div className={styles.fieldBlock}>
                <label htmlFor="tenant-color-hex" className={styles.fieldLabel}>
                  Warna Utama Merek (Hex)
                </label>
                <div className={styles.colorRow}>
                  <input
                    type="color"
                    aria-label="Pilih warna merek"
                    className={styles.colorSwatch}
                    value={/^#[0-9a-fA-F]{6}$/.test(primaryColor) ? primaryColor : '#0f766e'}
                    onChange={(e) => setPrimaryColor(e.target.value)}
                  />
                  <input
                    id="tenant-color-hex"
                    className={`${styles.input} mono-code`}
                    value={primaryColor}
                    onChange={(e) => setPrimaryColor(e.target.value)}
                    placeholder="#0F766E"
                  />
                </div>
              </div>
              <InputField
                label="Tagline Portal"
                value={tagline}
                onChange={(e) => setTagline(e.target.value)}
                placeholder="Penerimaan Murid Baru Terpadu"
              />
              <div className={styles.fieldBlock}>
                <span className={styles.fieldLabel}>Logo Instansi (Maks. 2 MB)</span>
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                  {logoUrl && (
                    <img
                      src={logoUrl}
                      alt="Preview Logo"
                      style={{ width: 40, height: 40, objectFit: 'contain', borderRadius: 6, border: '1px solid var(--border-default)' }}
                    />
                  )}
                  <label style={{ cursor: uploadingLogo ? 'wait' : 'pointer' }}>
                    <input
                      type="file"
                      className="sr-only"
                      accept="image/png,image/jpeg,image/webp,image/svg+xml"
                      disabled={uploadingLogo}
                      onChange={(e) => {
                        void handleLogoUpload(e.target.files?.[0]);
                        e.target.value = '';
                      }}
                    />
                    <span
                      style={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: 6,
                        padding: '0.5rem 0.85rem',
                        borderRadius: 'var(--radius-md)',
                        border: '1px solid var(--border-strong)',
                        fontSize: 'var(--text-caption)',
                        fontWeight: 600,
                      }}
                    >
                      <Upload size={14} />
                      {uploadingLogo ? 'Mengunggah Logo...' : 'Unggah Berkas Logo'}
                    </span>
                  </label>
                </div>
                <input
                  aria-label="URL Logo (Opsional)"
                  className={`${styles.input} mono-code`}
                  value={logoUrl}
                  onChange={(e) => setLogoUrl(e.target.value)}
                  placeholder="/api/v1/public/logo atau https://..."
                />
              </div>

              <div className={styles.fieldBlock}>
                <InputField
                  label="Custom Domain (Opsional)"
                  value={customDomain}
                  onChange={(e) => setCustomDomain(e.target.value)}
                  placeholder="ppdb.sekolahanda.sch.id"
                  mono
                />
                {settings?.custom_domain && (
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={verifyingDomain}
                      onClick={() => void handleVerifyDns()}
                    >
                      <Globe size={14} />
                      {verifyingDomain ? 'Memeriksa DNS...' : 'Verifikasi DNS Sekarang'}
                    </Button>
                    {domainCheckResult && (
                      <span className={adminStyles.metaMuted}>{domainCheckResult.message}</span>
                    )}
                  </div>
                )}
              </div>
            </div>

            <div style={{ marginTop: 'var(--space-4)', paddingTop: 'var(--space-3)', borderTop: '1px solid var(--border-default)' }}>
              <h4 style={{ fontSize: 'var(--text-label)', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 6, marginBottom: 'var(--space-2)' }}>
                <CreditCard size={15} />
                Konfigurasi Payment Gateway Midtrans Per-Instansi (Opsional)
              </h4>
              <p className={adminStyles.metaMuted} style={{ marginBottom: 'var(--space-3)' }}>
                Isi jika instansi menggunakan akun Midtrans sendiri. Kosongkan kolom Server Key bila tidak ingin mengubah rahasia yang tersimpan.
              </p>
              <div className={`${styles.formGrid} ${styles.formGrid2}`}>
                <InputField
                  label="Midtrans Merchant ID"
                  value={midtransMerchantId}
                  onChange={(e) => setMidtransMerchantId(e.target.value)}
                  placeholder="G123456789"
                  mono
                />
                <InputField
                  label="Midtrans Client Key"
                  value={midtransClientKey}
                  onChange={(e) => setMidtransClientKey(e.target.value)}
                  placeholder="SB-Mid-client-..."
                  mono
                />
                <InputField
                  label={
                    settings?.payment_config?.midtrans_server_key_configured
                      ? `Midtrans Server Key Baru (${settings.payment_config.midtrans_server_key_masked ?? 'Tersimpan'})`
                      : 'Midtrans Server Key'
                  }
                  type="password"
                  value={midtransServerKeyInput}
                  onChange={(e) => setMidtransServerKeyInput(e.target.value)}
                  placeholder="Kosongkan agar server key lama tetap terjaga"
                  mono
                />
                <div style={{ display: 'flex', alignItems: 'flex-end', paddingBottom: 8 }}>
                  <label className={styles.checkboxLabel}>
                    <input
                      type="checkbox"
                      checked={midtransSandbox}
                      onChange={(e) => setMidtransSandbox(e.target.checked)}
                    />
                    <span>Gunakan Mode Sandbox Midtrans</span>
                  </label>
                </div>
              </div>
            </div>

            <div style={{ marginTop: 'var(--space-3)' }}>
              <Button type="submit" disabled={savingTenant}>
                <Save size={14} />
                {savingTenant ? 'Menyimpan...' : 'Simpan Pengaturan Instansi'}
              </Button>
            </div>
          </form>
        )}
      </Card>

      <Card>
        <div className={adminStyles.headerRow} style={{ marginBottom: 'var(--space-3)' }}>
          <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>
            <Building2 size={18} />
            Daftar Unit Pendidikan
          </h2>
          {isTenantAdmin && (
            <Button size="sm" variant="secondary" onClick={openCreateUnit}>
              <Plus size={14} />
              Tambah Unit
            </Button>
          )}
        </div>

        <div className={adminStyles.tableWrap}>
          <table className={adminStyles.table}>
            <thead>
              <tr>
                <th>Kode &amp; Nama Unit</th>
                <th>Jenjang</th>
                <th>Alamat / Koordinat</th>
                <th>Status</th>
                {canManageUnitsOrReqs && <th>Aksi</th>}
              </tr>
            </thead>
            <tbody>
              {displayedUnits.map((unit) => (
                <tr key={unit.id}>
                  <td>
                    <div style={{ fontWeight: 600 }}>{unit.name}</div>
                    <div className={`mono-code ${adminStyles.metaMuted}`}>{unit.code}</div>
                  </td>
                  <td>
                    <span className="mono-code">{unit.education_level}</span>
                  </td>
                  <td>
                    <div>{unit.address || '-'}</div>
                    {unit.latitude != null && unit.longitude != null && (
                      <div className={`mono-code ${adminStyles.metaMuted}`}>
                        {unit.latitude}, {unit.longitude}
                      </div>
                    )}
                  </td>
                  <td>
                    <ToneBadge
                      tone={unit.is_active === false ? 'danger' : 'success'}
                      label={unit.is_active === false ? 'Nonaktif' : 'Aktif'}
                    />
                  </td>
                  {canManageUnitsOrReqs && (
                    <td>
                      <Button size="sm" variant="ghost" onClick={() => openEditUnit(unit)}>
                        <Pencil size={13} />
                        Ubah
                      </Button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <Card>
        <div className={adminStyles.headerRow} style={{ marginBottom: 'var(--space-3)' }}>
          <h2 style={{ fontSize: 'var(--text-h3)', fontWeight: 700 }}>
            Jadwal Periode &amp; Jalur Pendaftaran
          </h2>
          {canManageSetup && (
            <div className={styles.inlineActions}>
              <Button size="sm" variant="secondary" onClick={openCreatePeriod}>
                <CalendarPlus size={14} />
                Tambah Periode
              </Button>
              <Button
                size="sm"
                variant="secondary"
                disabled={portal.periods.length === 0}
                onClick={() => openCreateTrack()}
              >
                <Plus size={14} />
                Tambah Jalur
              </Button>
            </div>
          )}
        </div>

        <div className={adminStyles.stack}>
          {portal.periods.map((period) => {
            const unit = displayedUnits.find((u) => u.id === period.unit_id);
            return (
              <div key={period.id}>
                <div style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 'var(--space-2)', marginBottom: 'var(--space-2)' }}>
                  <div>
                    <strong>
                      {unit?.name ?? 'Unit'} — {period.name} ({period.academic_year})
                    </strong>
                    <div className={adminStyles.metaMuted}>
                      Pendaftaran: {formatDateTime(period.opens_at)} s.d.{' '}
                      {formatDateTime(period.closes_at)}
                    </div>
                  </div>
                  <div className={styles.inlineActions} style={{ alignItems: 'center' }}>
                    <ToneBadge
                      tone={period.results_published_at ? 'success' : 'neutral'}
                      label={
                        period.results_published_at
                          ? 'Hasil Sudah Diterbitkan'
                          : 'Dalam Proses Penerimaan'
                      }
                    />
                    {canManageSetup && (
                      <>
                        <Button size="sm" variant="ghost" onClick={() => openEditPeriod(period)}>
                          <Pencil size={13} />
                          Ubah Periode
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => openCreateTrack(period.id)}>
                          <Plus size={13} />
                          Jalur
                        </Button>
                      </>
                    )}
                  </div>
                </div>

                <div className={adminStyles.tableWrap}>
                  <table className={adminStyles.table}>
                    <thead>
                      <tr>
                        <th>Jalur</th>
                        <th>Mode Seleksi</th>
                        <th>Kuota</th>
                        <th>Pendaftar Aktif</th>
                        <th>Biaya Daftar</th>
                        <th>Daftar Ulang</th>
                        {canManageSetup && <th>Aksi</th>}
                      </tr>
                    </thead>
                    <tbody>
                      {period.tracks.map((track) => (
                        <tr key={track.id}>
                          <td>
                            <div style={{ fontWeight: 600 }}>{track.display_name}</div>
                            <div className={`mono-code ${adminStyles.metaMuted}`}>{track.code}</div>
                          </td>
                          <td>
                            <span className="mono-code">{track.selection_mode}</span>
                          </td>
                          <td className="tabular-nums">{track.quota}</td>
                          <td className="tabular-nums">{track.active_applicant_count}</td>
                          <td className="tabular-nums">{formatRupiah(track.registration_fee_idr)}</td>
                          <td className="tabular-nums">
                            {formatRupiah(track.re_enrollment_fee_idr ?? 0)}
                          </td>
                          {canManageSetup && (
                            <td>
                              <div className={styles.inlineActions}>
                                <Button
                                  size="sm"
                                  variant="ghost"
                                  onClick={() => openEditTrack(track, period.id)}
                                >
                                  <Pencil size={13} />
                                  Kuota/Biaya
                                </Button>
                                {canManageUnitsOrReqs && (
                                  <Button
                                    size="sm"
                                    variant="ghost"
                                    onClick={() => openRequirementsModal(track)}
                                  >
                                    <FileText size={13} />
                                    Berkas ({track.requirements?.length ?? 0})
                                  </Button>
                                )}
                              </div>
                            </td>
                          )}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            );
          })}
        </div>
      </Card>

      <Modal
        open={unitModalOpen}
        title={editingUnit ? `Ubah Unit: ${editingUnit.code}` : 'Tambah Unit Pendidikan Baru'}
        onClose={() => setUnitModalOpen(false)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setUnitModalOpen(false)} disabled={savingUnit}>
              Batal
            </Button>
            <Button type="submit" form="unit-setup-form" disabled={savingUnit}>
              {savingUnit ? 'Menyimpan...' : 'Simpan Unit'}
            </Button>
          </>
        }
      >
        <form id="unit-setup-form" onSubmit={(e) => void handleSubmitUnit(e)} className={styles.formGrid}>
          {!editingUnit && (
            <InputField
              label="Kode Unit"
              value={unitCode}
              onChange={(e) => setUnitCode(e.target.value)}
              placeholder="SMP-IT-1"
              required
              mono
            />
          )}
          <InputField
            label="Nama Unit Sekolah / Madrasah"
            value={unitName}
            onChange={(e) => setUnitName(e.target.value)}
            placeholder="SMP Islam Terpadu Al-Falah"
            required
          />
          <div className={styles.fieldBlock}>
            <label htmlFor="unit-level-select" className={styles.fieldLabel}>
              Jenjang Pendidikan
            </label>
            <select
              id="unit-level-select"
              className={styles.input}
              value={unitLevel}
              onChange={(e) => setUnitLevel(e.target.value as EducationLevel)}
            >
              {EDUCATION_LEVELS.map((lvl) => (
                <option key={lvl.value} value={lvl.value}>
                  {lvl.label}
                </option>
              ))}
            </select>
          </div>
          <InputField
            label="Alamat Unit"
            value={unitAddress}
            onChange={(e) => setUnitAddress(e.target.value)}
            placeholder="Jl. Pendidikan No. 12"
          />
          <div className={`${styles.formGrid} ${styles.formGrid2}`}>
            <InputField
              label="Latitude Sekolah (Opsional)"
              type="number"
              step="any"
              value={unitLat}
              onChange={(e) => setUnitLat(e.target.value)}
              placeholder="-6.2415"
              mono
            />
            <InputField
              label="Longitude Sekolah (Opsional)"
              type="number"
              step="any"
              value={unitLng}
              onChange={(e) => setUnitLng(e.target.value)}
              placeholder="106.8832"
              mono
            />
          </div>
          {editingUnit && (
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={unitActive}
                onChange={(e) => setUnitActive(e.target.checked)}
              />
              <span>Unit aktif tampil pada portal pendaftaran</span>
            </label>
          )}
        </form>
      </Modal>

      <Modal
        open={periodModalOpen}
        title={editingPeriod ? `Ubah Periode: ${editingPeriod.name}` : 'Tambah Periode Pendaftaran'}
        onClose={() => setPeriodModalOpen(false)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setPeriodModalOpen(false)} disabled={savingPeriod}>
              Batal
            </Button>
            <Button type="submit" form="period-setup-form" disabled={savingPeriod}>
              {savingPeriod ? 'Menyimpan...' : 'Simpan Periode'}
            </Button>
          </>
        }
      >
        <form id="period-setup-form" onSubmit={(e) => void handleSubmitPeriod(e)} className={styles.formGrid}>
          {!editingPeriod && (
            <div className={styles.fieldBlock}>
              <label htmlFor="period-unit-select" className={styles.fieldLabel}>
                Unit Tujuan
              </label>
              <select
                id="period-unit-select"
                className={styles.input}
                value={periodUnitId}
                onChange={(e) => setPeriodUnitId(e.target.value)}
              >
                {displayedUnits.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name} ({u.code})
                  </option>
                ))}
              </select>
            </div>
          )}
          <div className={`${styles.formGrid} ${styles.formGrid2}`}>
            <InputField
              label="Tahun Ajaran"
              value={academicYear}
              onChange={(e) => setAcademicYear(e.target.value)}
              placeholder="2026/2027"
              required
              mono
            />
            <InputField
              label="Nama Periode / Gelombang"
              value={periodName}
              onChange={(e) => setPeriodName(e.target.value)}
              placeholder="Gelombang 1"
              required
            />
            <InputField
              label="Waktu Buka Pendaftaran"
              type="datetime-local"
              value={opensAt}
              onChange={(e) => setOpensAt(e.target.value)}
              required
            />
            <InputField
              label="Waktu Tutup Pendaftaran"
              type="datetime-local"
              value={closesAt}
              onChange={(e) => setClosesAt(e.target.value)}
              required
            />
          </div>
          <div className={styles.checkboxRow}>
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={periodPublished}
                onChange={(e) => setPeriodPublished(e.target.checked)}
              />
              <span>Publikasikan di portal pendaftar</span>
            </label>
            {editingPeriod && (
              <label className={styles.checkboxLabel}>
                <input
                  type="checkbox"
                  checked={periodActive}
                  onChange={(e) => setPeriodActive(e.target.checked)}
                />
                <span>Periode aktif</span>
              </label>
            )}
          </div>
        </form>
      </Modal>

      <Modal
        open={trackModalOpen}
        title={editingTrack ? `Ubah Jalur: ${editingTrack.display_name}` : 'Tambah Jalur Seleksi Baru'}
        onClose={() => setTrackModalOpen(false)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setTrackModalOpen(false)} disabled={savingTrack}>
              Batal
            </Button>
            <Button type="submit" form="track-setup-form" disabled={savingTrack}>
              {savingTrack ? 'Menyimpan...' : 'Simpan Jalur'}
            </Button>
          </>
        }
      >
        <form id="track-setup-form" onSubmit={(e) => void handleSubmitTrack(e)} className={styles.formGrid}>
          {!editingTrack && (
            <div className={`${styles.formGrid} ${styles.formGrid2}`}>
              <div className={styles.fieldBlock}>
                <label htmlFor="track-period-select" className={styles.fieldLabel}>
                  Periode / Gelombang
                </label>
                <select
                  id="track-period-select"
                  className={styles.input}
                  value={trackPeriodId}
                  onChange={(e) => setTrackPeriodId(e.target.value)}
                >
                  {portal.periods.map((p) => {
                    const u = displayedUnits.find((item) => item.id === p.unit_id);
                    return (
                      <option key={p.id} value={p.id}>
                        {u?.code ?? 'UNIT'} — {p.name}
                      </option>
                    );
                  })}
                </select>
              </div>
              <InputField
                label="Kode Jalur"
                value={trackCode}
                onChange={(e) => setTrackCode(e.target.value)}
                placeholder="TAHFIDZ"
                required
                mono
              />
            </div>
          )}

          <div className={`${styles.formGrid} ${styles.formGrid2}`}>
            <InputField
              label="Nama Tampilan Jalur"
              value={trackDisplayName}
              onChange={(e) => setTrackDisplayName(e.target.value)}
              placeholder="Jalur Prestasi & Tahfidz"
              required
            />
            <div className={styles.fieldBlock}>
              <label htmlFor="track-mode-select" className={styles.fieldLabel}>
                Mode Seleksi
              </label>
              <select
                id="track-mode-select"
                className={styles.input}
                value={trackSelectionMode}
                onChange={(e) => setTrackSelectionMode(e.target.value as SelectionMode)}
              >
                {SELECTION_MODES.map((m) => (
                  <option key={m.value} value={m.value}>
                    {m.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className={styles.fieldBlock}>
            <label htmlFor="track-desc" className={styles.fieldLabel}>
              Deskripsi Singkat (Opsional)
            </label>
            <textarea
              id="track-desc"
              className={styles.textarea}
              value={trackDescription}
              onChange={(e) => setTrackDescription(e.target.value)}
              placeholder="Kriteria umum dan ketentuan singkat jalur..."
            />
          </div>

          <div className={`${styles.formGrid} ${styles.formGrid2}`}>
            <InputField
              label="Kuota Kursi"
              type="number"
              min={0}
              value={trackQuota}
              onChange={(e) => setTrackQuota(e.target.value)}
              required
              mono
            />
            <InputField
              label="Biaya Pendaftaran (IDR)"
              type="number"
              min={0}
              value={trackRegFee}
              onChange={(e) => setTrackRegFee(e.target.value)}
              required
              mono
            />
            <InputField
              label="Biaya Daftar Ulang (IDR)"
              type="number"
              min={0}
              value={trackReEnrollFee}
              onChange={(e) => setTrackReEnrollFee(e.target.value)}
              required
              mono
            />
            <InputField
              label="Usia Minimal (Bulan, Opsional)"
              type="number"
              min={0}
              value={trackMinAgeMonths}
              onChange={(e) => setTrackMinAgeMonths(e.target.value)}
              placeholder="Contoh: 72"
              mono
            />
            <InputField
              label="Usia Maksimal (Bulan, Opsional)"
              type="number"
              min={0}
              value={trackMaxAgeMonths}
              onChange={(e) => setTrackMaxAgeMonths(e.target.value)}
              placeholder="Contoh: 252"
              mono
            />
            <InputField
              label="Tanggal Acuan Hitung Usia"
              type="date"
              value={trackAgeRefDate}
              onChange={(e) => setTrackAgeRefDate(e.target.value)}
            />
          </div>

          <div className={styles.checkboxRow}>
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={trackRequiresCoords}
                onChange={(e) => setTrackRequiresCoords(e.target.checked)}
              />
              <span>Wajib koordinat domisili (Zonasi)</span>
            </label>
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={trackRequiresAch}
                onChange={(e) => setTrackRequiresAch(e.target.checked)}
              />
              <span>Menilai poin prestasi</span>
            </label>
            {editingTrack && (
              <label className={styles.checkboxLabel}>
                <input
                  type="checkbox"
                  checked={trackActive}
                  onChange={(e) => setTrackActive(e.target.checked)}
                />
                <span>Jalur aktif dibuka</span>
              </label>
            )}
          </div>

          <div style={{ paddingTop: 'var(--space-2)', borderTop: '1px solid var(--border-default)' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <strong style={{ fontSize: 'var(--text-label)' }}>Bobot Nilai Seleksi Jalur (Opsional)</strong>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => setTrackWeights((prev) => [...prev, { code: '', pct: '25' }])}
              >
                <Plus size={13} />
                Tambah Komponen
              </Button>
            </div>
            {trackWeights.map((w, idx) => (
              <div key={`w-${idx}`} style={{ display: 'grid', gridTemplateColumns: '1fr 110px auto', gap: 8, marginBottom: 6 }}>
                <input
                  aria-label={`Kode komponen bobot ${idx + 1}`}
                  className={`${styles.input} mono-code`}
                  placeholder="TPA / BTQ / WAWANCARA"
                  value={w.code}
                  onChange={(e) =>
                    setTrackWeights((prev) =>
                      prev.map((item, i) => (i === idx ? { ...item, code: e.target.value } : item)),
                    )
                  }
                />
                <input
                  aria-label={`Persen bobot ${idx + 1}`}
                  type="number"
                  min={0}
                  max={100}
                  className={`${styles.input} mono-code`}
                  placeholder="%"
                  value={w.pct}
                  onChange={(e) =>
                    setTrackWeights((prev) =>
                      prev.map((item, i) => (i === idx ? { ...item, pct: e.target.value } : item)),
                    )
                  }
                />
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setTrackWeights((prev) => prev.filter((_, i) => i !== idx))}
                >
                  <Trash2 size={13} />
                </Button>
              </div>
            ))}
          </div>

          <div style={{ paddingTop: 'var(--space-2)', borderTop: '1px solid var(--border-default)' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <strong style={{ fontSize: 'var(--text-label)' }}>Pertanyaan Formulir Kustom Pendaftar</strong>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() =>
                  setTrackCustomFields((prev) => [
                    ...prev,
                    {
                      key: `field_${prev.length + 1}`,
                      label: '',
                      type: 'text',
                      required: false,
                      placeholder: '',
                      optionsCsv: '',
                    },
                  ])
                }
              >
                <Plus size={13} />
                Tambah Pertanyaan
              </Button>
            </div>
            {trackCustomFields.map((cf, idx) => (
              <div key={`cf-${idx}`} className={styles.reqRow} style={{ marginBottom: 8 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <strong>Pertanyaan #{idx + 1}</strong>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => setTrackCustomFields((prev) => prev.filter((_, i) => i !== idx))}
                  >
                    <Trash2 size={13} />
                    Hapus
                  </Button>
                </div>
                <div className={`${styles.formGrid} ${styles.formGrid2}`}>
                  <InputField
                    label="Kunci Field (snake_case)"
                    value={cf.key}
                    onChange={(e) =>
                      setTrackCustomFields((prev) =>
                        prev.map((item, i) => (i === idx ? { ...item, key: e.target.value } : item)),
                      )
                    }
                    mono
                  />
                  <InputField
                    label="Label Pertanyaan"
                    value={cf.label}
                    onChange={(e) =>
                      setTrackCustomFields((prev) =>
                        prev.map((item, i) => (i === idx ? { ...item, label: e.target.value } : item)),
                      )
                    }
                    placeholder="Contoh: Jumlah Hafalan Juz Al-Quran"
                  />
                  <div className={styles.fieldBlock}>
                    <label className={styles.fieldLabel}>Tipe Input</label>
                    <select
                      className={styles.input}
                      value={cf.type}
                      onChange={(e) =>
                        setTrackCustomFields((prev) =>
                          prev.map((item, i) =>
                            i === idx
                              ? { ...item, type: e.target.value as EditableCustomFieldRow['type'] }
                              : item,
                          ),
                        )
                      }
                    >
                      <option value="text">Teks Singkat</option>
                      <option value="textarea">Teks Panjang</option>
                      <option value="number">Angka</option>
                      <option value="select">Pilihan (Dropdown)</option>
                      <option value="date">Tanggal</option>
                      <option value="checkbox">Centang (Ya/Tidak)</option>
                    </select>
                  </div>
                  {cf.type === 'select' ? (
                    <InputField
                      label="Daftar Opsi (pisahkan koma)"
                      value={cf.optionsCsv}
                      onChange={(e) =>
                        setTrackCustomFields((prev) =>
                          prev.map((item, i) => (i === idx ? { ...item, optionsCsv: e.target.value } : item)),
                        )
                      }
                      placeholder="1-5 Juz, 6-15 Juz, >15 Juz"
                    />
                  ) : (
                    <InputField
                      label="Placeholder (Opsional)"
                      value={cf.placeholder}
                      onChange={(e) =>
                        setTrackCustomFields((prev) =>
                          prev.map((item, i) => (i === idx ? { ...item, placeholder: e.target.value } : item)),
                        )
                      }
                    />
                  )}
                </div>
                <label className={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={cf.required}
                    onChange={(e) =>
                      setTrackCustomFields((prev) =>
                        prev.map((item, i) => (i === idx ? { ...item, required: e.target.checked } : item)),
                      )
                    }
                  />
                  <span>Wajib diisi pendaftar pada Langkah 1</span>
                </label>
              </div>
            ))}
          </div>
        </form>
      </Modal>

      <Modal
        open={reqTargetTrack !== null}
        title={reqTargetTrack ? `Syarat Berkas: ${reqTargetTrack.display_name}` : 'Syarat Berkas'}
        onClose={() => setReqTargetTrack(null)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setReqTargetTrack(null)} disabled={savingReqs}>
              Batal
            </Button>
            <Button type="submit" form="track-reqs-form" disabled={savingReqs}>
              {savingReqs ? 'Menyimpan...' : 'Simpan Syarat Berkas'}
            </Button>
          </>
        }
      >
        <form id="track-reqs-form" onSubmit={(e) => void handleSaveRequirements(e)} className={styles.formGrid}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span className={adminStyles.metaMuted}>
              Atur daftar berkas yang harus diunggah pendaftar pada jalur ini.
            </span>
            <Button
              type="button"
              size="sm"
              variant="secondary"
              onClick={() =>
                setReqRows((prev) => [
                  ...prev,
                  {
                    doc_code: `BERKAS_${prev.length + 1}`,
                    label: 'Dokumen Tambahan',
                    description: '',
                    is_required: true,
                    accepted_mimes_csv: 'image/jpeg, image/png, application/pdf',
                    max_size_kb: 2048,
                  },
                ])
              }
            >
              <Plus size={14} />
              Tambah Baris
            </Button>
          </div>

          {reqRows.length === 0 ? (
            <div className={adminStyles.emptyState}>
              Belum ada syarat berkas. Klik Tambah Baris atau Simpan jika jalur bebas unggah berkas.
            </div>
          ) : (
            reqRows.map((row, idx) => (
              <div key={`req-row-${idx}`} className={styles.reqRow}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <strong>Berkas #{idx + 1}</strong>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => setReqRows((prev) => prev.filter((_, i) => i !== idx))}
                  >
                    <Trash2 size={13} />
                    Hapus
                  </Button>
                </div>
                <div className={`${styles.formGrid} ${styles.formGrid2}`}>
                  <InputField
                    label="Kode Berkas"
                    value={row.doc_code}
                    onChange={(e) =>
                      setReqRows((prev) =>
                        prev.map((item, i) => (i === idx ? { ...item, doc_code: e.target.value } : item)),
                      )
                    }
                    required
                    mono
                  />
                  <InputField
                    label="Nama Berkas"
                    value={row.label}
                    onChange={(e) =>
                      setReqRows((prev) =>
                        prev.map((item, i) => (i === idx ? { ...item, label: e.target.value } : item)),
                      )
                    }
                    required
                  />
                  <InputField
                    label="Tipe MIME Diizinkan"
                    value={row.accepted_mimes_csv}
                    onChange={(e) =>
                      setReqRows((prev) =>
                        prev.map((item, i) =>
                          i === idx ? { ...item, accepted_mimes_csv: e.target.value } : item,
                        ),
                      )
                    }
                    mono
                  />
                  <InputField
                    label="Maks Ukuran (KB)"
                    type="number"
                    min={100}
                    value={String(row.max_size_kb)}
                    onChange={(e) =>
                      setReqRows((prev) =>
                        prev.map((item, i) =>
                          i === idx ? { ...item, max_size_kb: Number(e.target.value) || 1024 } : item,
                        ),
                      )
                    }
                    mono
                  />
                </div>
                <label className={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={row.is_required}
                    onChange={(e) =>
                      setReqRows((prev) =>
                        prev.map((item, i) =>
                          i === idx ? { ...item, is_required: e.target.checked } : item,
                        ),
                      )
                    }
                  />
                  <span>Wajib diunggah sebelum kirim final</span>
                </label>
              </div>
            ))
          )}
        </form>
      </Modal>
    </div>
  );
}
