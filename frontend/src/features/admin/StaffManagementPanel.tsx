import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Pencil, RefreshCw, Shield, UserPlus } from 'lucide-react';
import { apiRequest, extractErrorMessage, formatDateTime } from '@/core/api/client';
import { useAuth } from '@/core/auth/AuthContext';
import type { StaffMemberView, TenantRole, UnitSummary } from '@/core/types/domain';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { InputField } from '@/components/ui/InputField';
import { Modal } from '@/components/ui/Modal';
import { ToneBadge } from '@/components/ui/StatusBadge';
import adminStyles from './AdminShared.module.css';

const ROLE_OPTIONS: Array<{ value: TenantRole; label: string }> = [
  { value: 'tenant_admin', label: 'Admin Instansi (Semua Unit)' },
  { value: 'unit_admin', label: 'Admin Unit' },
  { value: 'verifier', label: 'Verifikator Berkas' },
  { value: 'examiner', label: 'Penguji Seleksi' },
  { value: 'treasurer', label: 'Bendahara' },
  { value: 'principal', label: 'Kepala Sekolah / Pimpinan' },
];

export function StaffManagementPanel({
  tenantSlug,
  units,
}: {
  tenantSlug: string;
  units: UnitSummary[];
}) {
  const auth = useAuth();
  const isTenantAdmin = auth.hasAnyRole('tenant_admin');

  const [staff, setStaff] = useState<StaffMemberView[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<StaffMemberView | null>(null);
  const [saving, setSaving] = useState(false);

  const [fullName, setFullName] = useState('');
  const [email, setEmail] = useState('');
  const [whatsappPhone, setWhatsappPhone] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<TenantRole>('verifier');
  const [unitId, setUnitId] = useState('');
  const [isActive, setIsActive] = useState(true);

  const loadStaff = useCallback(async () => {
    if (!isTenantAdmin) return;
    setLoading(true);
    setError('');
    try {
      const list = await apiRequest<StaffMemberView[]>('/api/v1/admin/staff', { tenantSlug });
      setStaff(list ?? []);
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal memuat daftar staf panitia.'));
    } finally {
      setLoading(false);
    }
  }, [isTenantAdmin, tenantSlug]);

  useEffect(() => {
    void loadStaff();
  }, [loadStaff]);

  function openCreate() {
    setEditing(null);
    setFullName('');
    setEmail('');
    setWhatsappPhone('');
    setPassword('');
    setRole('verifier');
    setUnitId(units[0]?.id ?? '');
    setIsActive(true);
    setModalOpen(true);
  }

  function openEdit(member: StaffMemberView) {
    const primaryRole = member.roles[0];
    setEditing(member);
    setFullName(member.full_name);
    setEmail(member.email ?? '');
    setWhatsappPhone(member.whatsapp_phone ?? '');
    setPassword('');
    setRole(primaryRole?.role ?? 'verifier');
    setUnitId(primaryRole?.unit_id ?? '');
    setIsActive(primaryRole ? primaryRole.is_active : true);
    setModalOpen(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError('');
    setNotice('');
    try {
      if (editing) {
        const body: Record<string, unknown> = {
          full_name: fullName.trim(),
          whatsapp_phone: whatsappPhone.trim() || null,
          role,
          is_active: isActive,
        };
        if (role === 'tenant_admin' || !unitId) {
          body.clear_unit = true;
        } else {
          body.unit_id = unitId;
        }
        if (password.trim()) {
          body.password = password.trim();
        }
        await apiRequest<StaffMemberView>(`/api/v1/admin/staff/${editing.user_id}`, {
          method: 'PATCH',
          tenantSlug,
          body,
        });
        setNotice(`Data staf "${fullName.trim()}" berhasil diperbarui.`);
      } else {
        await apiRequest<StaffMemberView>('/api/v1/admin/staff', {
          method: 'POST',
          tenantSlug,
          body: {
            full_name: fullName.trim(),
            email: email.trim(),
            whatsapp_phone: whatsappPhone.trim() || null,
            password: password.trim(),
            role,
            unit_id: role === 'tenant_admin' || !unitId ? null : unitId,
          },
        });
        setNotice(`Staf baru "${fullName.trim()}" berhasil ditambahkan.`);
      }
      setModalOpen(false);
      await loadStaff();
    } catch (err) {
      setError(extractErrorMessage(err, 'Gagal menyimpan akun staf.'));
    } finally {
      setSaving(false);
    }
  }

  if (!isTenantAdmin) {
    return (
      <Card>
        <div className={adminStyles.alertInfo}>
          Manajemen staf &amp; hak akses RBAC hanya dapat dibuka oleh <strong>Admin Instansi</strong>.
        </div>
      </Card>
    );
  }

  return (
    <div className={adminStyles.stack}>
      <div className={adminStyles.headerRow}>
        <div>
          <h1 style={{ fontSize: 'var(--text-h1)', fontWeight: 700 }}>
            Manajemen Staf &amp; Hak Akses (RBAC)
          </h1>
          <p className={adminStyles.metaMuted} style={{ marginTop: 4 }}>
            Kelola akun panitia, bendahara, penguji, kepala sekolah, serta pembatasan akses per unit.
          </p>
        </div>

        <div className={adminStyles.filterBar}>
          <Button size="sm" variant="secondary" onClick={() => void loadStaff()} disabled={loading}>
            <RefreshCw size={14} />
            Muat Ulang
          </Button>
          <Button size="sm" onClick={openCreate}>
            <UserPlus size={14} />
            Tambah Staf Baru
          </Button>
        </div>
      </div>

      {error && (
        <div role="alert" className={adminStyles.alertError}>
          {error}
        </div>
      )}
      {notice && (
        <div role="status" className={adminStyles.alertSuccess}>
          {notice}
        </div>
      )}

      <Card>
        {loading ? (
          <div className={adminStyles.emptyState}>Memuat daftar staf...</div>
        ) : staff.length === 0 ? (
          <div className={adminStyles.emptyState}>Belum ada akun staf terdaftar pada instansi ini.</div>
        ) : (
          <div className={adminStyles.tableWrap}>
            <table className={adminStyles.table}>
              <thead>
                <tr>
                  <th>Nama Staf</th>
                  <th>Kontak Login</th>
                  <th>Peran &amp; Cakupan Unit</th>
                  <th>Status</th>
                  <th>Terdaftar</th>
                  <th>Aksi</th>
                </tr>
              </thead>
              <tbody>
                {staff.map((member) => {
                  const activeAny = member.roles.some((r) => r.is_active);
                  return (
                    <tr key={member.user_id}>
                      <td style={{ fontWeight: 600 }}>{member.full_name}</td>
                      <td>
                        <div className="mono-code">{member.email || '-'}</div>
                        {member.whatsapp_phone && (
                          <div className={`mono-code ${adminStyles.metaMuted}`}>
                            {member.whatsapp_phone}
                          </div>
                        )}
                      </td>
                      <td>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                          {member.roles.map((r) => (
                            <div key={r.id} style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                              <ToneBadge tone="info" label={r.role} />
                              <span className={adminStyles.metaMuted}>
                                {r.unit_name ? r.unit_name : 'Semua Unit Instansi'}
                              </span>
                            </div>
                          ))}
                        </div>
                      </td>
                      <td>
                        <ToneBadge
                          tone={activeAny ? 'success' : 'danger'}
                          label={activeAny ? 'Aktif' : 'Nonaktif'}
                        />
                      </td>
                      <td className={adminStyles.metaMuted}>{formatDateTime(member.created_at)}</td>
                      <td>
                        <Button size="sm" variant="ghost" onClick={() => openEdit(member)}>
                          <Pencil size={13} />
                          Ubah Peran
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal
        open={modalOpen}
        title={editing ? `Ubah Akun Staf: ${editing.full_name}` : 'Tambah Akun Staf Panitia'}
        onClose={() => setModalOpen(false)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setModalOpen(false)} disabled={saving}>
              Batal
            </Button>
            <Button type="submit" form="staff-member-form" disabled={saving}>
              <Shield size={14} />
              {saving ? 'Menyimpan...' : 'Simpan Staf'}
            </Button>
          </>
        }
      >
        <form id="staff-member-form" onSubmit={(e) => void handleSubmit(e)} style={{ display: 'grid', gap: 'var(--space-3)' }}>
          <InputField
            label="Nama Lengkap"
            value={fullName}
            onChange={(e) => setFullName(e.target.value)}
            placeholder="Ust. Ahmad Fauzi"
            required
          />
          <InputField
            label="Email Kerja"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="verifikator@sekolah.sch.id"
            disabled={Boolean(editing)}
            required={!editing}
            mono
          />
          <InputField
            label="Nomor WhatsApp (Opsional)"
            value={whatsappPhone}
            onChange={(e) => setWhatsappPhone(e.target.value)}
            placeholder="081234567890"
            mono
          />
          <InputField
            label={editing ? 'Kata Sandi Baru (Kosongkan jika tetap)' : 'Kata Sandi Awal'}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={editing ? 'Biarkan kosong jika tidak diganti' : 'Minimal 8 karakter'}
            required={!editing}
          />
          <div>
            <label htmlFor="staff-role-select" style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
              Peran RBAC
            </label>
            <select
              id="staff-role-select"
              className={adminStyles.select}
              style={{ width: '100%' }}
              value={role}
              onChange={(e) => setRole(e.target.value as TenantRole)}
            >
              {ROLE_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
          {role !== 'tenant_admin' && (
            <div>
              <label htmlFor="staff-unit-select" style={{ display: 'block', fontSize: 'var(--text-label)', fontWeight: 600, marginBottom: 4 }}>
                Cakupan Unit Sekolah / Madrasah
              </label>
              <select
                id="staff-unit-select"
                className={adminStyles.select}
                style={{ width: '100%' }}
                value={unitId}
                onChange={(e) => setUnitId(e.target.value)}
              >
                <option value="">Semua Unit (Lintas Jenjang)</option>
                {units.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name} ({u.code})
                  </option>
                ))}
              </select>
            </div>
          )}
          {editing && (
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 'var(--text-label)' }}>
              <input
                type="checkbox"
                checked={isActive}
                onChange={(e) => setIsActive(e.target.checked)}
              />
              <span>Akun staf aktif &amp; dapat masuk panel panitia</span>
            </label>
          )}
        </form>
      </Modal>
    </div>
  );
}
