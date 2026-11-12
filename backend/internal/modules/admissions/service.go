package admissions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pbdb/platform/internal/domain"
	"github.com/pbdb/platform/internal/integrations"
	"github.com/pbdb/platform/internal/storage"
)

// Service orchestrates applicant CRUD, presigned uploads, zoning distance, submission, and verifier workflows.
type Service struct {
	store                 Store
	presigner             storage.ObjectStorage
	idempotency           IdempotencyStore
	paymentGateway        integrations.PaymentGateway
	paymentConfigProvider TenantPaymentConfigProvider
	documentSecret        string
}

// NewService creates an admissions service with persistence, storage, and idempotency protection.
func NewService(store Store, presigner storage.ObjectStorage, idempotency IdempotencyStore) *Service {
	if idempotency == nil {
		idempotency = NewIdempotencyCache(24 * time.Hour)
	}
	return &Service{
		store:          store,
		presigner:      presigner,
		idempotency:    idempotency,
		paymentGateway: &integrations.MockPaymentGateway{},
	}
}

// SetPaymentConfigProvider attaches a lookup provider for per-tenant Midtrans configuration.
func (s *Service) SetPaymentConfigProvider(provider TenantPaymentConfigProvider) {
	s.paymentConfigProvider = provider
}

// SetDocumentSecret configures the HMAC secret used to sign printable QR verification tokens.
func (s *Service) SetDocumentSecret(secret string) {
	s.documentSecret = strings.TrimSpace(secret)
}

func (s *Service) decorateDetail(tenantID uuid.UUID, detail *ApplicationDetail) *ApplicationDetail {
	if detail == nil {
		return nil
	}
	if detail.Schedules == nil {
		detail.Schedules = []AssessmentSchedule{}
	}
	if s.documentSecret != "" && detail.Application.ID != uuid.Nil && tenantID != uuid.Nil {
		detail.VerificationToken = domain.SignDocumentToken(s.documentSecret, tenantID, detail.Application.ID)
	}
	return detail
}

// SetPaymentGateway overrides the checkout gateway used for online payment methods.
func (s *Service) SetPaymentGateway(gw integrations.PaymentGateway) {
	if gw != nil {
		s.paymentGateway = gw
	}
}

func (s *Service) gateway() integrations.PaymentGateway {
	if s.paymentGateway != nil {
		return s.paymentGateway
	}
	return &integrations.MockPaymentGateway{}
}

// ListApplicantApplications returns applications owned by guardianUserID within tenantID.
func (s *Service) ListApplicantApplications(ctx context.Context, tenantID, guardianUserID uuid.UUID) ([]ApplicationSummary, error) {
	return s.store.ListApplicationsByGuardian(ctx, tenantID, guardianUserID)
}

// CreateApplication starts a new admission application in awaiting_payment (paid track) or draft_form (free track).
func (s *Service) CreateApplication(
	ctx context.Context,
	tenantID, guardianUserID uuid.UUID,
	input CreateApplicationInput,
	idempotencyKey string,
	actor ActorContext,
) (*ApplicationDetail, error) {
	studentName := strings.TrimSpace(input.StudentFullName)
	fields := map[string]string{}
	if input.UnitID == uuid.Nil {
		fields["unit_id"] = "Unit sekolah wajib dipilih."
	}
	if input.PeriodID == uuid.Nil {
		fields["period_id"] = "Periode pendaftaran wajib dipilih."
	}
	if input.TrackID == uuid.Nil {
		fields["track_id"] = "Jalur pendaftaran wajib dipilih."
	}
	if studentName == "" {
		fields["student_full_name"] = "Nama lengkap calon murid wajib diisi."
	}
	if len(fields) > 0 {
		return nil, &ValidationError{Message: "Data pendaftaran awal belum lengkap.", Fields: fields}
	}

	var cached ApplicationDetail
	if hit, err := s.idempotency.Lookup(ctx, tenantID, guardianUserID, "create_app", idempotencyKey, &cached); err == nil && hit {
		return s.decorateDetail(tenantID, &cached), nil
	}

	catalog, err := s.store.GetCatalogTrack(ctx, tenantID, input.UnitID, input.PeriodID, input.TrackID)
	if err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, ErrTrackNotFound
	}

	initialStatus := domain.StatusDraftForm
	if catalog.RegistrationFeeIDR > 0 {
		initialStatus = domain.StatusAwaitingPayment
	}

	appID := uuid.Must(uuid.NewV7())
	detail, err := s.store.CreateApplication(
		ctx,
		appID,
		tenantID,
		*catalog,
		guardianUserID,
		initialStatus,
		studentName,
		input.ExtraAnswers,
		actor,
	)
	if err != nil {
		return nil, err
	}

	_ = s.idempotency.Save(ctx, tenantID, guardianUserID, "create_app", idempotencyKey, detail)
	return s.decorateDetail(tenantID, detail), nil
}

// GetApplicantApplication loads full application details owned by guardianUserID.
func (s *Service) GetApplicantApplication(ctx context.Context, tenantID, guardianUserID, applicationID uuid.UUID) (*ApplicationDetail, error) {
	detail, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID)
	return s.decorateDetail(tenantID, detail), err
}

// UpdateProfile saves student biodata, calculates domicile distance when coordinates exist, validates age bounds, and bumps version atomically.
func (s *Service) UpdateProfile(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
	input UpdateProfileInput,
) (*ApplicationDetail, error) {
	detail, err := s.requireOwnedEditableApp(ctx, tenantID, guardianUserID, applicationID)
	if err != nil {
		return nil, err
	}

	cleanProfile, err := normalizeAndValidateProfile(input.ApplicantProfile)
	if err != nil {
		return nil, err
	}
	if cleanProfile.BirthDate != nil {
		if err := s.validateTrackEligibility(ctx, tenantID, detail.Application.TrackID, *cleanProfile.BirthDate, nil); err != nil {
			return nil, err
		}
	}

	var distanceMeters *int
	if cleanProfile.Latitude != nil && cleanProfile.Longitude != nil &&
		detail.UnitLatitude != nil && detail.UnitLongitude != nil {
		d := domain.HaversineMeters(*detail.UnitLatitude, *detail.UnitLongitude, *cleanProfile.Latitude, *cleanProfile.Longitude)
		distanceMeters = &d
	}

	saved, err := s.store.SaveApplicantProfile(ctx, tenantID, applicationID, input.ExpectedVersion, cleanProfile, input.ExtraAnswers, distanceMeters)
	return s.decorateDetail(tenantID, saved), err
}

// ReplaceGuardians validates and saves parent/guardian records for an editable application.
func (s *Service) ReplaceGuardians(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
	guardians []Guardian,
) ([]Guardian, error) {
	if _, err := s.requireOwnedEditableApp(ctx, tenantID, guardianUserID, applicationID); err != nil {
		return nil, err
	}

	normalized, err := normalizeAndValidateGuardians(guardians)
	if err != nil {
		return nil, err
	}

	return s.store.ReplaceGuardians(ctx, tenantID, applicationID, normalized)
}

// ReplaceAchievements validates and replaces claimed achievements on an editable application.
func (s *Service) ReplaceAchievements(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
	achievements []Achievement,
) ([]Achievement, error) {
	if _, err := s.requireOwnedEditableApp(ctx, tenantID, guardianUserID, applicationID); err != nil {
		return nil, err
	}

	fields := map[string]string{}
	out := make([]Achievement, 0, len(achievements))
	for i, item := range achievements {
		item.Category = strings.TrimSpace(item.Category)
		item.Title = strings.TrimSpace(item.Title)
		item.Level = strings.TrimSpace(item.Level)
		if item.Category == "" || item.Title == "" || item.Level == "" {
			fields[fmt.Sprintf("achievements[%d]", i)] = "Kategori, judul prestasi, dan tingkat wajib diisi."
		}
		if item.ID == uuid.Nil {
			item.ID = uuid.Must(uuid.NewV7())
		}
		item.RankAchieved = normalizeOptString(item.RankAchieved)
		item.Organizer = normalizeOptString(item.Organizer)
		out = append(out, item)
	}
	if len(fields) > 0 {
		return nil, &ValidationError{Message: "Data prestasi belum lengkap.", Fields: fields}
	}

	return s.store.ReplaceAchievements(ctx, tenantID, applicationID, out)
}

// PresignUpload validates file constraints against track requirements and returns a short-lived S3 PUT URL.
func (s *Service) PresignUpload(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
	input PresignUploadInput,
) (PresignUploadResult, error) {
	if s.presigner == nil {
		return PresignUploadResult{}, storage.ErrStorageNotConfigured
	}

	detail, err := s.requireOwnedEditableApp(ctx, tenantID, guardianUserID, applicationID)
	if err != nil {
		return PresignUploadResult{}, err
	}

	mimeType := strings.ToLower(strings.TrimSpace(input.MimeType))
	if _, err := validateDocumentConstraints(detail.Requirements, input.RequirementID, input.Filename, mimeType, input.SizeBytes); err != nil {
		return PresignUploadResult{}, err
	}

	key := storage.BuildApplicationDocKey(tenantID, applicationID, input.RequirementID, input.Filename)
	signed, err := s.presigner.PresignPutObject(key, mimeType, 15*time.Minute)
	if err != nil {
		return PresignUploadResult{}, err
	}

	return PresignUploadResult{
		UploadURL:       signed.URL,
		StorageKey:      signed.StorageKey,
		RequiredHeaders: signed.RequiredHeaders,
		ExpiresAt:       signed.ExpiresAt,
	}, nil
}

// ConfirmDocument validates an uploaded object key and stores document metadata with pending review status.
func (s *Service) ConfirmDocument(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
	input ConfirmDocumentInput,
) (*Document, error) {
	detail, err := s.requireOwnedEditableApp(ctx, tenantID, guardianUserID, applicationID)
	if err != nil {
		return nil, err
	}

	mimeType := strings.ToLower(strings.TrimSpace(input.MimeType))
	req, err := validateDocumentConstraints(detail.Requirements, input.RequirementID, input.OriginalFilename, mimeType, input.SizeBytes)
	if err != nil {
		return nil, err
	}

	if !storage.ValidateApplicationDocKey(input.StorageKey, tenantID, applicationID, input.RequirementID) {
		return nil, ErrInvalidStorageKey
	}

	actualSize, err := s.verifyUploadedObject(ctx, input.StorageKey, int64(req.MaxSizeKB)*1024, "storage_key", "Berkas belum ditemukan di penyimpanan. Silakan unggah ulang berkas terlebih dahulu.")
	if err != nil {
		return nil, err
	}

	input.SizeBytes = actualSize
	input.MimeType = mimeType
	input.OriginalFilename = storage.SanitizeFilename(input.OriginalFilename)
	return s.store.UpsertDocument(ctx, tenantID, applicationID, uuid.Must(uuid.NewV7()), input)
}

// PresignApplicantDownload returns a short-lived GET URL for an applicant's own document.
func (s *Service) PresignApplicantDownload(
	ctx context.Context,
	tenantID, guardianUserID, applicationID, documentID uuid.UUID,
) (storage.PresignedResult, error) {
	if _, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID); err != nil {
		return storage.PresignedResult{}, err
	}
	return s.presignDownload(ctx, tenantID, applicationID, documentID)
}

// PresignStaffDownload returns a short-lived GET URL for authorized staff inspecting a document.
func (s *Service) PresignStaffDownload(
	ctx context.Context,
	tenantID, applicationID, documentID uuid.UUID,
) (storage.PresignedResult, error) {
	return s.presignDownload(ctx, tenantID, applicationID, documentID)
}

func (s *Service) presignDownload(ctx context.Context, tenantID, applicationID, documentID uuid.UUID) (storage.PresignedResult, error) {
	if s.presigner == nil {
		return storage.PresignedResult{}, storage.ErrStorageNotConfigured
	}
	doc, err := s.store.GetDocumentByID(ctx, tenantID, applicationID, documentID)
	if err != nil {
		return storage.PresignedResult{}, err
	}
	if doc == nil {
		return storage.PresignedResult{}, ErrDocumentNotFound
	}
	return s.presigner.PresignGetObject(doc.StorageKey, 5*time.Minute)
}

// SubmitApplication verifies pre-submit completeness, enforces optimistic version, transitions to submitted, and queues WA notification.
func (s *Service) SubmitApplication(
	ctx context.Context,
	tenantID, guardianUserID, applicationID uuid.UUID,
	expectedVersion int,
	idempotencyKey string,
	actor ActorContext,
) (*ApplicationDetail, error) {
	var cached ApplicationDetail
	if hit, err := s.idempotency.Lookup(ctx, tenantID, guardianUserID, "submit:"+applicationID.String(), idempotencyKey, &cached); err == nil && hit {
		return s.decorateDetail(tenantID, &cached), nil
	}

	detail, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID)
	if err != nil {
		return nil, err
	}
	if !domain.CanTransition(detail.Application.Status, domain.StatusSubmitted) {
		return nil, ErrApplicationLocked
	}
	if detail.Application.Version != expectedVersion {
		return nil, ErrVersionConflict
	}

	if detail.Profile == nil || strings.TrimSpace(detail.Profile.FullName) == "" {
		return nil, &ValidationError{
			Message: "Biodata calon murid belum lengkap.",
			Fields:  map[string]string{"full_name": "Nama lengkap calon murid wajib diisi sebelum mengirim pendaftaran."},
		}
	}
	birthDate := ""
	if detail.Profile.BirthDate != nil {
		birthDate = *detail.Profile.BirthDate
	}
	if err := s.validateTrackEligibility(ctx, tenantID, detail.Application.TrackID, birthDate, detail.ExtraAnswers); err != nil {
		return nil, err
	}
	if detail.RequiresCoordinates && (detail.Profile.Latitude == nil || detail.Profile.Longitude == nil) {
		return nil, ErrMissingCoordinates
	}
	if detail.RequiresAchievements && len(detail.Achievements) == 0 {
		return nil, ErrMissingAchievements
	}

	if missing := findMissingOrRejectedRequiredDocs(detail.Requirements, detail.Documents); len(missing) > 0 {
		return nil, &MissingDocumentsError{MissingDocCodes: missing}
	}

	fromStatus := string(detail.Application.Status)
	toStatus := string(domain.StatusSubmitted)
	updated, err := s.store.SubmitApplication(
		ctx,
		tenantID,
		applicationID,
		expectedVersion,
		detail.Application.Status,
		AuditWrite{
			ID:          uuid.Must(uuid.NewV7()),
			TenantID:    tenantID,
			Application: &applicationID,
			Actor:       actor,
			ActionType:  "application_submitted",
			EntityType:  "application",
			EntityID:    applicationID,
			FromStatus:  &fromStatus,
			ToStatus:    &toStatus,
		},
		NotificationWrite{
			ID:            uuid.Must(uuid.NewV7()),
			TenantID:      tenantID,
			ApplicationID: applicationID,
			TemplateCode:  integrations.TemplateApplicationSubmitted,
			Variables:     []string{detail.Application.RegistrationNo, detail.Profile.FullName},
		},
	)
	if err != nil {
		return nil, err
	}

	_ = s.idempotency.Save(ctx, tenantID, guardianUserID, "submit:"+applicationID.String(), idempotencyKey, updated)
	return s.decorateDetail(tenantID, updated), nil
}

// ListApplicationsForStaff returns filtered applications visible to staff according to RBAC unit scope.
func (s *Service) ListApplicationsForStaff(ctx context.Context, tenantID uuid.UUID, filter StaffListFilter) ([]ApplicationSummary, error) {
	return s.store.ListApplicationsForStaff(ctx, tenantID, filter)
}

// GetApplicationForStaff loads a full application dossier for administrative inspection.
func (s *Service) GetApplicationForStaff(ctx context.Context, tenantID, applicationID uuid.UUID) (*ApplicationDetail, error) {
	detail, err := s.store.GetApplicationDetail(ctx, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, ErrApplicationNotFound
	}
	return s.decorateDetail(tenantID, detail), nil
}

// ClaimNextForVerification atomically claims the oldest submitted application in unitID for a verifier.
func (s *Service) ClaimNextForVerification(ctx context.Context, tenantID, unitID uuid.UUID, actor ActorContext) (*ApplicationDetail, error) {
	fromStatus := string(domain.StatusSubmitted)
	toStatus := string(domain.StatusUnderVerification)
	claimed, err := s.store.ClaimNextForVerification(ctx, tenantID, unitID, actor.UserID, AuditWrite{
		ID:         uuid.Must(uuid.NewV7()),
		TenantID:   tenantID,
		Actor:      actor,
		ActionType: "verification_claimed",
		EntityType: "application",
		FromStatus: &fromStatus,
		ToStatus:   &toStatus,
	})
	if err != nil {
		return nil, err
	}
	if claimed == nil {
		return nil, ErrVerificationQueueEmpty
	}
	return s.decorateDetail(tenantID, claimed), nil
}

// ReviewDocument approves or rejects a single uploaded document with audit logging.
func (s *Service) ReviewDocument(
	ctx context.Context,
	tenantID, applicationID, documentID uuid.UUID,
	status domain.DocumentReviewStatus,
	reviewNote string,
	actor ActorContext,
) (*Document, error) {
	if status != domain.DocStatusApproved && status != domain.DocStatusRejectedForRevision {
		return nil, &ValidationError{
			Message: "Status verifikasi dokumen tidak valid.",
			Fields:  map[string]string{"status": "Gunakan approved atau rejected_for_revision."},
		}
	}

	trimmedNote := strings.TrimSpace(reviewNote)
	if status == domain.DocStatusRejectedForRevision && trimmedNote == "" {
		return nil, ErrReviewNoteRequired
	}
	var notePtr *string
	if trimmedNote != "" {
		notePtr = &trimmedNote
	}

	toStatus := string(status)
	return s.store.ReviewDocument(ctx, tenantID, applicationID, documentID, actor.UserID, status, notePtr, AuditWrite{
		ID:          uuid.Must(uuid.NewV7()),
		TenantID:    tenantID,
		Application: &applicationID,
		Actor:       actor,
		ActionType:  "document_reviewed",
		EntityType:  "document",
		EntityID:    documentID,
		ToStatus:    &toStatus,
		Reason:      notePtr,
	})
}

// CompleteVerification finalizes dossier verification as verified or revision_required and triggers WA notification.
func (s *Service) CompleteVerification(
	ctx context.Context,
	tenantID, applicationID uuid.UUID,
	expectedVersion int,
	summaryNote string,
	actor ActorContext,
) (*ApplicationDetail, error) {
	detail, err := s.store.GetApplicationDetail(ctx, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, ErrApplicationNotFound
	}
	if detail.Application.Version != expectedVersion {
		return nil, ErrVersionConflict
	}
	if detail.Application.Status != domain.StatusSubmitted && detail.Application.Status != domain.StatusUnderVerification {
		return nil, ErrApplicationLocked
	}

	rejectedCount, pendingCount, missingRequiredCount := summarizeDocuments(detail.Requirements, detail.Documents)

	var toStatus domain.ApplicationStatus
	trimmedSummary := strings.TrimSpace(summaryNote)
	switch {
	case rejectedCount > 0:
		toStatus = domain.StatusRevisionRequired
	case pendingCount > 0 || missingRequiredCount > 0:
		if trimmedSummary == "" {
			return nil, ErrPendingDocsRemain
		}
		toStatus = domain.StatusRevisionRequired
	default:
		toStatus = domain.StatusVerified
	}

	if !domain.CanTransition(domain.StatusUnderVerification, toStatus) {
		return nil, ErrApplicationLocked
	}

	var reasonPtr *string
	if trimmedSummary != "" {
		reasonPtr = &trimmedSummary
	}

	templateCode := integrations.TemplateVerificationApproved
	if toStatus == domain.StatusRevisionRequired {
		templateCode = integrations.TemplateVerificationRevision
	}

	studentName := ""
	if detail.Profile != nil {
		studentName = detail.Profile.FullName
	}

	fromStr := string(detail.Application.Status)
	toStr := string(toStatus)
	completed, err := s.store.CompleteVerification(
		ctx,
		tenantID,
		applicationID,
		expectedVersion,
		detail.Application.Status,
		toStatus,
		time.Now().UTC(),
		AuditWrite{
			ID:          uuid.Must(uuid.NewV7()),
			TenantID:    tenantID,
			Application: &applicationID,
			Actor:       actor,
			ActionType:  "verification_completed",
			EntityType:  "application",
			EntityID:    applicationID,
			FromStatus:  &fromStr,
			ToStatus:    &toStr,
			Reason:      reasonPtr,
		},
		NotificationWrite{
			ID:            uuid.Must(uuid.NewV7()),
			TenantID:      tenantID,
			ApplicationID: applicationID,
			TemplateCode:  templateCode,
			Variables:     []string{detail.Application.RegistrationNo, studentName, trimmedSummary},
		},
	)
	return s.decorateDetail(tenantID, completed), err
}

func (s *Service) verifyUploadedObject(
	ctx context.Context,
	storageKey string,
	maxBytes int64,
	fieldKey string,
	notFoundMsg string,
) (int64, error) {
	if s.presigner == nil {
		return 0, storage.ErrStorageNotConfigured
	}
	meta, err := s.presigner.HeadObject(ctx, storageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return 0, &ValidationError{
				Message: notFoundMsg,
				Fields:  map[string]string{fieldKey: notFoundMsg},
			}
		}
		return 0, fmt.Errorf("verifikasi objek penyimpanan: %w", err)
	}
	if meta.SizeBytes <= 0 || (maxBytes > 0 && meta.SizeBytes > maxBytes) {
		return 0, ErrFileTooLarge
	}
	return meta.SizeBytes, nil
}

func (s *Service) validateTrackEligibility(
	ctx context.Context,
	tenantID, trackID uuid.UUID,
	birthDateStr string,
	extraAnswers map[string]any,
) error {
	track, err := s.store.GetTrackByID(ctx, tenantID, trackID)
	if err != nil || track == nil {
		return err
	}
	fields := map[string]string{}

	if strings.TrimSpace(birthDateStr) != "" && (track.MinAgeMonths != nil || track.MaxAgeMonths != nil) {
		bd, err := time.Parse("2006-01-02", strings.TrimSpace(birthDateStr))
		if err == nil {
			ref := time.Now().UTC()
			if track.AgeReferenceDate != nil && *track.AgeReferenceDate != "" {
				if parsedRef, perr := time.Parse("2006-01-02", *track.AgeReferenceDate); perr == nil {
					ref = parsedRef
				}
			}
			ageMonths := (ref.Year()-bd.Year())*12 + int(ref.Month()-bd.Month())
			if ref.Day() < bd.Day() {
				ageMonths--
			}
			if track.MinAgeMonths != nil && ageMonths < *track.MinAgeMonths {
				fields["birth_date"] = fmt.Sprintf("Usia calon murid (%d bulan) kurang dari batas minimum jalur ini (%d bulan).", ageMonths, *track.MinAgeMonths)
			} else if track.MaxAgeMonths != nil && ageMonths > *track.MaxAgeMonths {
				fields["birth_date"] = fmt.Sprintf("Usia calon murid (%d bulan) melebihi batas maksimum jalur ini (%d bulan).", ageMonths, *track.MaxAgeMonths)
			}
		}
	}

	if extraAnswers != nil && len(track.CustomFormSchema) > 0 {
		for _, cf := range track.CustomFormSchema {
			if !cf.Required {
				continue
			}
			val, exists := extraAnswers[cf.Key]
			if !exists || val == nil || strings.TrimSpace(fmt.Sprintf("%v", val)) == "" {
				fields["extra_answers."+cf.Key] = fmt.Sprintf("Kolom %s wajib diisi pada jalur ini.", cf.Label)
			}
		}
	}

	if len(fields) > 0 {
		return &ValidationError{
			Message: "Persyaratan khusus jalur pendaftaran belum terpenuhi.",
			Fields:  fields,
		}
	}
	return nil
}

func (s *Service) requireOwnedApp(ctx context.Context, tenantID, guardianUserID, applicationID uuid.UUID) (*ApplicationDetail, error) {
	detail, err := s.store.GetApplicationDetail(ctx, tenantID, applicationID)
	if err != nil {
		return nil, err
	}
	if detail == nil || detail.GuardianUserID != guardianUserID {
		return nil, ErrApplicationNotFound
	}
	return detail, nil
}

func (s *Service) requireOwnedEditableApp(ctx context.Context, tenantID, guardianUserID, applicationID uuid.UUID) (*ApplicationDetail, error) {
	detail, err := s.requireOwnedApp(ctx, tenantID, guardianUserID, applicationID)
	if err != nil {
		return nil, err
	}
	if detail.Application.Status != domain.StatusDraftForm && detail.Application.Status != domain.StatusRevisionRequired {
		return nil, ErrApplicationLocked
	}
	return detail, nil
}

func normalizeAndValidateProfile(p ApplicantProfile) (ApplicantProfile, error) {
	fields := map[string]string{}
	p.FullName = strings.TrimSpace(p.FullName)
	if p.FullName == "" {
		fields["full_name"] = "Nama lengkap calon murid wajib diisi."
	}

	p.NIK = normalizeOptDigits(p.NIK, 16, "nik", "NIK harus terdiri dari tepat 16 digit angka.", fields)
	p.KKNumber = normalizeOptDigits(p.KKNumber, 16, "kk_number", "Nomor KK harus terdiri dari tepat 16 digit angka.", fields)
	p.NISN = normalizeOptDigits(p.NISN, 10, "nisn", "NISN harus terdiri dari tepat 10 digit angka.", fields)

	p.KKIssuedDate = validateOptDate(p.KKIssuedDate, "kk_issued_date", fields)
	p.BirthDate = validateOptDate(p.BirthDate, "birth_date", fields)

	p.Gender = normalizeOptString(p.Gender)
	if p.Gender != nil && *p.Gender != "L" && *p.Gender != "P" {
		fields["gender"] = "Jenis kelamin harus L atau P."
	}

	if (p.Latitude != nil) != (p.Longitude != nil) {
		fields["coordinates"] = "Latitude dan longitude domisili harus diisi keduanya."
	} else if p.Latitude != nil && p.Longitude != nil {
		if *p.Latitude < -90 || *p.Latitude > 90 || *p.Longitude < -180 || *p.Longitude > 180 {
			fields["coordinates"] = "Koordinat latitude/longitude di luar rentang valid."
		}
	}

	p.BirthPlace = normalizeOptString(p.BirthPlace)
	p.Religion = normalizeOptString(p.Religion)
	p.SpecialNeedsCode = normalizeOptString(p.SpecialNeedsCode)
	p.AddressStreet = normalizeOptString(p.AddressStreet)
	p.RT = normalizeOptString(p.RT)
	p.RW = normalizeOptString(p.RW)
	p.Hamlet = normalizeOptString(p.Hamlet)
	p.VillageCode = normalizeOptString(p.VillageCode)
	p.VillageName = normalizeOptString(p.VillageName)
	p.DistrictCode = normalizeOptString(p.DistrictCode)
	p.DistrictName = normalizeOptString(p.DistrictName)
	p.RegencyCode = normalizeOptString(p.RegencyCode)
	p.RegencyName = normalizeOptString(p.RegencyName)
	p.ProvinceCode = normalizeOptString(p.ProvinceCode)
	p.ProvinceName = normalizeOptString(p.ProvinceName)
	p.PostalCode = normalizeOptString(p.PostalCode)
	p.PrevSchoolName = normalizeOptString(p.PrevSchoolName)
	p.PrevSchoolNPSN = normalizeOptString(p.PrevSchoolNPSN)
	p.IjazahNumber = normalizeOptString(p.IjazahNumber)

	if strings.TrimSpace(p.Citizenship) == "" {
		p.Citizenship = "WNI"
	}

	if len(fields) > 0 {
		return p, &ValidationError{Message: "Format biodata calon murid tidak valid.", Fields: fields}
	}
	return p, nil
}

func normalizeAndValidateGuardians(guardians []Guardian) ([]Guardian, error) {
	fields := map[string]string{}
	seenRelations := make(map[string]bool)
	out := make([]Guardian, 0, len(guardians))

	for i, g := range guardians {
		g.Relation = strings.ToLower(strings.TrimSpace(g.Relation))
		g.FullName = strings.TrimSpace(g.FullName)
		if g.Relation != "father" && g.Relation != "mother" && g.Relation != "guardian" {
			fields[fmt.Sprintf("guardians[%d].relation", i)] = "Hubungan harus father, mother, atau guardian."
		} else if seenRelations[g.Relation] {
			fields[fmt.Sprintf("guardians[%d].relation", i)] = "Setiap jenis hubungan (father/mother/guardian) hanya boleh muncul satu kali."
		}
		seenRelations[g.Relation] = true

		if g.FullName == "" {
			fields[fmt.Sprintf("guardians[%d].full_name", i)] = "Nama orang tua/wali wajib diisi."
		}

		g.NIK = normalizeOptDigits(g.NIK, 16, fmt.Sprintf("guardians[%d].nik", i), "NIK orang tua/wali harus 16 digit angka.", fields)
		g.EducationLevel = normalizeOptString(g.EducationLevel)
		g.Occupation = normalizeOptString(g.Occupation)
		g.IncomeBracket = normalizeOptString(g.IncomeBracket)
		g.PhoneWhatsApp = normalizeOptString(g.PhoneWhatsApp)
		if g.ID == uuid.Nil {
			g.ID = uuid.Must(uuid.NewV7())
		}
		out = append(out, g)
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Message: "Data orang tua/wali tidak valid.", Fields: fields}
	}
	return out, nil
}

func validateDocumentConstraints(
	reqs []TrackRequirement,
	reqID uuid.UUID,
	filename, mimeType string,
	sizeBytes int64,
) (*TrackRequirement, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, &ValidationError{Message: "Nama berkas wajib diisi.", Fields: map[string]string{"filename": "Nama berkas wajib diisi."}}
	}

	var matched *TrackRequirement
	for i := range reqs {
		if reqs[i].ID == reqID {
			matched = &reqs[i]
			break
		}
	}
	if matched == nil {
		return nil, ErrRequirementNotFound
	}

	if !slices.Contains(matched.AcceptedMimes, mimeType) {
		return nil, ErrUnsupportedMime
	}

	maxBytes := int64(matched.MaxSizeKB) * 1024
	if sizeBytes <= 0 || sizeBytes > maxBytes {
		return nil, ErrFileTooLarge
	}

	return matched, nil
}

func findMissingOrRejectedRequiredDocs(reqs []TrackRequirement, docs []Document) []string {
	docByReq := make(map[uuid.UUID]Document, len(docs))
	for _, d := range docs {
		docByReq[d.RequirementID] = d
	}

	var missing []string
	for _, r := range reqs {
		if !r.IsRequired {
			continue
		}
		doc, ok := docByReq[r.ID]
		if !ok || doc.Status == domain.DocStatusRejectedForRevision {
			missing = append(missing, r.DocCode)
		}
	}
	return missing
}

func summarizeDocuments(reqs []TrackRequirement, docs []Document) (rejected, pending, missingRequired int) {
	docByReq := make(map[uuid.UUID]Document, len(docs))
	for _, d := range docs {
		docByReq[d.RequirementID] = d
		switch d.Status {
		case domain.DocStatusRejectedForRevision:
			rejected++
		case domain.DocStatusPending:
			pending++
		}
	}
	for _, r := range reqs {
		if r.IsRequired {
			if _, ok := docByReq[r.ID]; !ok {
				missingRequired++
			}
		}
	}
	return rejected, pending, missingRequired
}

func normalizeOptString(ptr *string) *string {
	if ptr == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*ptr)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeOptDigits(ptr *string, exactLen int, fieldKey, errMsg string, fields map[string]string) *string {
	cleaned := normalizeOptString(ptr)
	if cleaned == nil {
		return nil
	}
	if len(*cleaned) != exactLen {
		fields[fieldKey] = errMsg
		return cleaned
	}
	for i := 0; i < len(*cleaned); i++ {
		if (*cleaned)[i] < '0' || (*cleaned)[i] > '9' {
			fields[fieldKey] = errMsg
			break
		}
	}
	return cleaned
}

func validateOptDate(ptr *string, fieldKey string, fields map[string]string) *string {
	cleaned := normalizeOptString(ptr)
	if cleaned == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", *cleaned); err != nil {
		fields[fieldKey] = "Format tanggal harus YYYY-MM-DD."
	}
	return cleaned
}
