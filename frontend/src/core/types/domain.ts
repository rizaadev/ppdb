export type TerminologyPreset = 'spmb' | 'pmbm' | 'mandiri';

export type InstitutionType = 'school' | 'madrasah' | 'foundation' | 'pesantren';

export type TenantRole =
  | 'tenant_admin'
  | 'unit_admin'
  | 'verifier'
  | 'examiner'
  | 'treasurer'
  | 'principal';

export type ApplicationStatus =
  | 'account_created'
  | 'track_selected'
  | 'awaiting_payment'
  | 'payment_verified'
  | 'draft_form'
  | 'submitted'
  | 'under_verification'
  | 'revision_required'
  | 'verified'
  | 'selection_in_progress'
  | 'ranked'
  | 'accepted'
  | 'waitlisted'
  | 'rejected'
  | 're_enrollment'
  | 'enrolled'
  | 'withdrawn';

export type DocumentReviewStatus = 'pending' | 'approved' | 'rejected_for_revision';

export type SelectionMode = 'rule_based' | 'test_based' | 'hybrid' | 'direct';

export type PaymentPurpose = 'registration_fee' | 're_enrollment_fee';
export type PaymentMethod = 'qris' | 'va' | 'ewallet' | 'manual_transfer';
export type PaymentStatus = 'pending' | 'paid' | 'failed' | 'expired' | 'refunded';

export interface RoleScope {
  tenant_id: string;
  unit_id?: string | null;
  role: TenantRole;
}

export interface CurrentUser {
  id: string;
  full_name: string;
  whatsapp_phone?: string | null;
  email?: string | null;
  is_platform_admin: boolean;
  roles: RoleScope[];
}

export interface AuthTokenBundle {
  access_token: string;
  refresh_token: string;
  expires_in_seconds: number;
  user: CurrentUser;
}

export interface RequestOTPResult {
  whatsapp_phone: string;
  expires_at: string;
  retry_after_seconds: number;
  dev_otp?: string;
}

export interface TenantThemeConfig {
  primary_color: string;
  logo_url?: string | null;
  tagline?: string;
}

export interface TenantInfo {
  id: string;
  slug: string;
  name: string;
  institution_type: InstitutionType;
  terminology_preset: TerminologyPreset;
  terminology_overrides: Record<string, string>;
  theme_config: TenantThemeConfig;
}

export interface UnitSummary {
  id: string;
  code: string;
  name: string;
  education_level: string;
  address?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  is_active?: boolean;
}

export interface TrackRequirementItem {
  id: string;
  track_id?: string;
  doc_code: string;
  label: string;
  description?: string | null;
  is_required: boolean;
  accepted_mimes: string[];
  max_size_kb: number;
  sort_order?: number;
}

export interface CustomFormField {
  key: string;
  label: string;
  type: 'text' | 'textarea' | 'number' | 'select' | 'date' | 'checkbox' | string;
  required: boolean;
  placeholder?: string;
  options?: string[];
}

export interface TrackSummary {
  id: string;
  period_id?: string;
  unit_id?: string;
  code: string;
  display_name: string;
  description?: string | null;
  selection_mode: SelectionMode;
  quota: number;
  active_applicant_count: number;
  registration_fee_idr: number;
  re_enrollment_fee_idr?: number;
  requires_coordinates: boolean;
  requires_achievements: boolean;
  min_age_months?: number | null;
  max_age_months?: number | null;
  age_reference_date?: string | null;
  scoring_weights?: Record<string, number>;
  custom_form_schema?: CustomFormField[];
  spillover_target_track_id?: string | null;
  is_active?: boolean;
  requirements?: TrackRequirementItem[];
}

export interface PeriodSummary {
  id: string;
  unit_id: string;
  academic_year: string;
  name: string;
  opens_at: string;
  closes_at: string;
  announcement_at?: string | null;
  results_published_at?: string | null;
  is_published?: boolean;
  is_active?: boolean;
  tracks: TrackSummary[];
}

export interface PublicPortalPayload {
  tenant: TenantInfo;
  units: UnitSummary[];
  periods: PeriodSummary[];
}

export interface PublicCheckStatusResult {
  registration_no: string;
  masked_name: string;
  unit_name: string;
  track_name: string;
  status: ApplicationStatus;
  rank_position?: number | null;
  decided_at?: string | null;
}

export interface ApplicationSummary {
  id: string;
  registration_no: string;
  status: ApplicationStatus;
  unit_id: string;
  unit_name?: string;
  period_id: string;
  period_name?: string;
  academic_year?: string;
  track_id: string;
  track_name?: string;
  student_name: string | null;
  nisn?: string | null;
  distance_meters?: number | null;
  final_score?: string | null;
  rank_position?: number | null;
  version: number;
  submitted_at?: string | null;
  verified_at?: string | null;
  decided_at?: string | null;
  results_published_at?: string | null;
  created_at: string;
}

export interface ApplicantProfile {
  full_name: string;
  nik?: string | null;
  kk_number?: string | null;
  kk_issued_date?: string | null;
  nisn?: string | null;
  birth_place?: string | null;
  birth_date?: string | null;
  gender?: 'L' | 'P' | string | null;
  religion?: string | null;
  citizenship: string;
  child_order?: number | null;
  siblings_count?: number | null;
  special_needs_code?: string | null;
  address_street?: string | null;
  rt?: string | null;
  rw?: string | null;
  hamlet?: string | null;
  village_name?: string | null;
  district_name?: string | null;
  regency_name?: string | null;
  province_name?: string | null;
  postal_code?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  prev_school_name?: string | null;
  prev_school_npsn?: string | null;
  graduation_year?: number | null;
  ijazah_number?: string | null;
}

export interface GuardianItem {
  id?: string;
  relation: 'father' | 'mother' | 'guardian' | string;
  full_name: string;
  nik?: string | null;
  birth_year?: number | null;
  education_level?: string | null;
  occupation?: string | null;
  income_bracket?: string | null;
  phone_whatsapp?: string | null;
  is_primary_contact: boolean;
}

export interface AchievementItem {
  id?: string;
  category: string;
  title: string;
  level: string;
  rank_achieved?: string | null;
  achievement_year?: number | null;
  organizer?: string | null;
  raw_points: number;
}

export interface ApplicationDocumentItem {
  id: string;
  requirement_id: string;
  doc_code: string;
  requirement_label: string;
  is_required: boolean;
  storage_key: string;
  original_filename: string;
  mime_type: string;
  size_bytes: number;
  status: DocumentReviewStatus;
  reviewer_user_id?: string | null;
  review_note?: string | null;
  reviewed_at?: string | null;
  version: number;
}

export interface PaymentTransaction {
  id: string;
  application_id: string;
  registration_no?: string;
  student_name?: string | null;
  unit_id?: string;
  unit_name?: string;
  track_name?: string;
  purpose: PaymentPurpose;
  method: PaymentMethod;
  provider?: string | null;
  external_ref?: string | null;
  amount_idr: number;
  status: PaymentStatus;
  payment_url?: string | null;
  proof_storage_key?: string | null;
  verified_by_user_id?: string | null;
  verification_note?: string | null;
  paid_at?: string | null;
  expires_at?: string | null;
  created_at: string;
}

export interface AssessmentScore {
  id: string;
  application_id: string;
  component_code: string;
  raw_score: number;
  weight_pct: number;
  weighted_score: number;
  notes?: string | null;
  examiner_user_id?: string | null;
  updated_at: string;
}

export interface AuditEntry {
  id: string;
  actor_user_id?: string | null;
  actor_name?: string | null;
  actor_role: string;
  action_type: string;
  entity_type: string;
  entity_id: string;
  from_status?: string | null;
  to_status?: string | null;
  reason?: string | null;
  created_at: string;
}

export interface AssessmentSchedule {
  id: string;
  application_id: string;
  registration_no?: string;
  student_name?: string | null;
  unit_id?: string;
  unit_name?: string;
  track_name?: string;
  session_name: string;
  room_or_link?: string | null;
  scheduled_at: string;
  examiner_user_id?: string | null;
  examiner_name?: string | null;
  created_at: string;
  updated_at: string;
}

export interface ApplicationDetail {
  application: ApplicationSummary;
  guardian_user_id: string;
  claimed_by_verifier_id?: string | null;
  claimed_at?: string | null;
  unit_latitude?: number | null;
  unit_longitude?: number | null;
  requires_coordinates: boolean;
  requires_achievements: boolean;
  registration_fee_idr: number;
  re_enrollment_fee_idr: number;
  extra_answers: Record<string, unknown>;
  profile: ApplicantProfile | null;
  guardians: GuardianItem[];
  achievements: AchievementItem[];
  requirements: TrackRequirementItem[];
  documents: ApplicationDocumentItem[];
  payments: PaymentTransaction[];
  scores: AssessmentScore[];
  schedules?: AssessmentSchedule[];
  audit_logs: AuditEntry[];
  verification_token?: string;
  track_scoring_weights?: Record<string, number>;
  custom_form_schema?: CustomFormField[];
}

export interface PresignUploadResult {
  upload_url: string;
  storage_key: string;
  required_headers?: Record<string, string>;
  expires_at: string;
}

export interface PresignedDownloadResult {
  url: string;
  storage_key: string;
  expires_at: string;
}

export interface PaymentVerificationResult {
  payment: PaymentTransaction;
  application: ApplicationDetail;
}

export interface UpsertScoresResult {
  application: ApplicationDetail;
  scores: AssessmentScore[];
  final_score: number;
}

export interface RankedCandidate {
  application_id: string;
  registration_no: string;
  student_name?: string | null;
  current_status: ApplicationStatus;
  simulated_decision: ApplicationStatus;
  rank_position: number;
  final_score: number;
  distance_meters?: number | null;
  birth_date?: string | null;
  submitted_at?: string | null;
}

export interface TrackRankingSimulation {
  track_id: string;
  track_name: string;
  unit_id: string;
  quota: number;
  waitlist_slots: number;
  spillover_target_track_id?: string | null;
  accepted_count: number;
  waitlisted_count: number;
  rejected_count: number;
  remaining_quota: number;
  candidates: RankedCandidate[];
}

export interface PublishResultsResult {
  period_id: string;
  published_at: string;
  total_accepted: number;
  total_waitlisted: number;
  total_rejected: number;
  tracks: TrackRankingSimulation[];
}

export interface DocumentReviewItem {
  id: string;
  doc_code: string;
  label: string;
  file_name: string;
  size_kb: number;
  status: DocumentReviewStatus;
  review_note?: string;
  preview_url: string;
}

export interface ChildApplicationPreview {
  id: string;
  registration_no: string;
  student_name: string;
  unit_name: string;
  track_name: string;
  status: ApplicationStatus;
  submitted_at?: string;
  distance_meters?: number;
  documents: DocumentReviewItem[];
}

export type ExportFormat = 'dapodik_xlsx' | 'emis_xlsx' | 'custom_csv';
export type ExportJobStatus = 'pending' | 'processing' | 'completed' | 'failed';

export interface ExportJob {
  id: string;
  unit_id: string;
  unit_name?: string | null;
  period_id: string;
  period_name?: string | null;
  requested_by_user_id: string;
  format: ExportFormat;
  status: ExportJobStatus;
  result_storage_key?: string | null;
  filename?: string | null;
  download_url?: string | null;
  row_count?: number | null;
  error_message?: string | null;
  created_at: string;
  completed_at?: string | null;
}

export interface TenantAuditLogItem {
  id: string;
  application_id?: string | null;
  registration_no?: string | null;
  student_name?: string | null;
  unit_id?: string | null;
  actor_user_id?: string | null;
  actor_name?: string | null;
  actor_role: string;
  action_type: string;
  entity_type: string;
  entity_id: string;
  from_status?: string | null;
  to_status?: string | null;
  reason?: string | null;
  created_at: string;
}

export type EducationLevel = 'ra_tk' | 'sd_mi' | 'smp_mts' | 'sma_ma' | 'smk_mak' | 'pesantren';

export interface TenantPaymentConfigView {
  midtrans_merchant_id?: string;
  midtrans_client_key?: string;
  midtrans_sandbox?: boolean | null;
  midtrans_server_key_configured: boolean;
  midtrans_server_key_masked?: string;
}

export interface TenantPaymentConfigUpdate {
  midtrans_merchant_id?: string | null;
  midtrans_client_key?: string | null;
  midtrans_server_key?: string | null;
  midtrans_sandbox?: boolean | null;
}

export interface DomainVerificationResult {
  domain: string;
  verified: boolean;
  expected_txt_host: string;
  expected_txt_value: string;
  expected_cname_target?: string;
  observed_txt: string[];
  observed_cname?: string;
  message: string;
}

export interface PublicDocumentVerifyResult {
  valid: boolean;
  document_type: string;
  registration_no: string;
  masked_name: string;
  tenant_name: string;
  unit_name: string;
  track_name: string;
  academic_year: string;
  status: ApplicationStatus;
  final_score?: number | null;
  submitted_at?: string | null;
  verified_at: string;
  message: string;
}

export interface StaffRoleView {
  id: string;
  role: TenantRole;
  unit_id?: string | null;
  unit_name?: string | null;
  is_active: boolean;
}

export interface StaffMemberView {
  user_id: string;
  full_name: string;
  email?: string | null;
  whatsapp_phone?: string | null;
  is_platform_admin: boolean;
  roles: StaffRoleView[];
  created_at: string;
}

export interface AdminTenantSettings extends TenantInfo {
  custom_domain?: string | null;
  domain_verified?: boolean;
  domain_verification_token?: string | null;
  payment_config?: TenantPaymentConfigView;
}

export interface PlatformTenantView {
  id: string;
  slug: string;
  name: string;
  institution_type: InstitutionType;
  terminology_preset: TerminologyPreset;
  theme_config: TenantThemeConfig;
  custom_domain?: string | null;
  domain_verified: boolean;
  domain_verification_token?: string | null;
  subscription_plan: string;
  subscription_expires_at?: string | null;
  is_active: boolean;
  unit_count: number;
  created_at: string;
}

export interface PlatformCreateTenantResult {
  tenant: PlatformTenantView;
  initial_admin?: {
    id: string;
    email: string;
    full_name: string;
  } | null;
}

export interface DailyTrendPoint {
  date: string;
  count: number;
}

export interface TrackStatItem {
  track_id: string;
  track_code: string;
  display_name: string;
  unit_id: string;
  unit_name: string;
  quota: number;
  applicant_count: number;
  accepted_count: number;
}

export interface StatusCountItem {
  status: string;
  count: number;
}

export interface DashboardStats {
  total_applicants: number;
  draft_count: number;
  submitted_count: number;
  verification_count: number;
  revision_required_count: number;
  verified_count: number;
  accepted_count: number;
  waitlisted_count: number;
  rejected_count: number;
  re_enrolled_count: number;
  total_revenue_idr: number;
  pending_revenue_idr: number;
  pending_manual_payments: number;
  daily_trends: DailyTrendPoint[];
  track_stats: TrackStatItem[];
  status_breakdown: StatusCountItem[];
  recent_activities: TenantAuditLogItem[];
}

